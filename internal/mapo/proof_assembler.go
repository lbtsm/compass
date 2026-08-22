package mapo

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
	maptypes "github.com/mapprotocol/atlas/core/types"
	"github.com/mapprotocol/compass/internal/arb"
	"github.com/mapprotocol/compass/internal/constant"
	"github.com/mapprotocol/compass/internal/mapprotocol"
	"github.com/mapprotocol/compass/internal/proof"
	"github.com/mapprotocol/compass/pkg/ethclient"
	"github.com/mapprotocol/compass/pkg/msg"
	"github.com/mapprotocol/compass/pkg/util"
	"github.com/pkg/errors"
)

const defaultOPReceiptTimeout = 30 * time.Second

type ProofAssembler struct {
	client           *ethclient.Client
	opReceiptBuilder *OPReceiptBuilder
	opReceiptTimeout time.Duration
}

func NewProofAssembler(client *ethclient.Client) *ProofAssembler {
	return &ProofAssembler{
		client:           client,
		opReceiptBuilder: NewOPReceiptBuilder(client),
		opReceiptTimeout: defaultOPReceiptTimeout,
	}
}

func (a *ProofAssembler) AssembleEth(log *types.Log, receipts []*types.Receipt, header *maptypes.Header,
	method string, fromChainID msg.ChainId, proofType int64, sign [][]byte) ([]byte, error) {
	if log == nil {
		return nil, errors.New("nil log")
	}
	if len(log.Topics) < 2 {
		return nil, fmt.Errorf("log has %d topics, want at least 2", len(log.Topics))
	}
	sourceReceipt, logIndex, err := a.receiptAndLogIndex(receipts, log)
	if err != nil {
		return nil, err
	}

	receipt, err := mapprotocol.GetTxReceipt(sourceReceipt)
	if err != nil {
		return nil, err
	}
	proofNodes, receiptHash, err := a.ethProof(fromChainID, log.TxIndex, receipts)
	if err != nil {
		return nil, err
	}

	var key []byte
	key = rlp.AppendUint64(key[:0], uint64(log.TxIndex))
	orderID := log.Topics[1]
	var pack []byte
	switch proofType {
	case constant.ProofTypeOfOrigin:
	case constant.ProofTypeOfZk:
	case constant.ProofTypeOfOracle:
		pack, err = proof.Oracle(log.BlockNumber, receipt, key, proofNodes, fromChainID, method, logIndex,
			mapprotocol.ProofAbi, orderID, false)
	case constant.ProofTypeOfNewOracle:
		if receiptHash.Hex() != header.ReceiptHash.Hex() {
			return nil, fmt.Errorf("receipt hash %s does not match header receipt hash %s", receiptHash, header.ReceiptHash)
		}
		fallthrough
	case constant.ProofTypeOfLogOracle:
		pack, err = proof.SignOracle(&maptypes.Header{
			ReceiptHash: receiptHash,
			Number:      big.NewInt(int64(log.BlockNumber)),
		}, receipt, key, proofNodes, fromChainID, logIndex, method, sign, log, proofType)
	}
	if err != nil {
		return nil, err
	}
	return pack, nil
}

func (a *ProofAssembler) AssembleMap(log *types.Log, receipts []*types.Receipt, header *maptypes.Header,
	fromChainID msg.ChainId, method, zkURL string, proofType int64, sign [][]byte) (uint64, []byte, error) {
	if log == nil {
		return 0, nil, errors.New("nil log")
	}
	if len(log.Topics) < 3 {
		return 0, nil, fmt.Errorf("log has %d topics, want at least 3", len(log.Topics))
	}
	sourceReceipt, logIndex, err := a.receiptAndLogIndex(receipts, log)
	if err != nil {
		return 0, nil, err
	}

	toChainID := big.NewInt(0).SetBytes(log.Topics[2].Bytes()[8:16]).Uint64()
	orderID := log.Topics[1]
	aggPK, ist, _, err := mapprotocol.GetAggPK(a.client, new(big.Int).Sub(header.Number, big.NewInt(1)), header.Extra)
	if err != nil {
		return 0, nil, err
	}
	receipt, err := mapprotocol.GetTxReceipt(sourceReceipt)
	if err != nil {
		return 0, nil, err
	}
	proofNodes, err := proof.Get(types.Receipts(receipts), log.TxIndex)
	if err != nil {
		return 0, nil, err
	}

	var key []byte
	key = rlp.AppendUint64(key[:0], uint64(log.TxIndex))
	keyIndex := util.Key2Hex(key, len(proofNodes))
	istanbulExtra := mapprotocol.ConvertIstanbulExtra(ist)
	mapReceipt := mapprotocol.MapTxReceipt{
		PostStateOrStatus: receipt.PostStateOrStatus,
		CumulativeGasUsed: receipt.CumulativeGasUsed,
		Bloom:             receipt.Bloom,
		Logs:              receipt.Logs,
	}
	receiptRLP, err := rlp.EncodeToBytes(mapReceipt)
	if err != nil {
		return 0, nil, err
	}
	receiptProof := mapprotocol.NewMapReceiptProof{
		Header:   mapprotocol.ConvertHeader(header),
		AggPk:    aggPK,
		KeyIndex: keyIndex,
		Proof:    proofNodes,
		Ist:      *istanbulExtra,
		TxReceiptRlp: mapprotocol.TxReceiptRlp{
			ReceiptType: receipt.ReceiptType,
			ReceiptRlp:  receiptRLP,
		},
	}

	var payload []byte
	switch proofType {
	case constant.ProofTypeOfZk:
		zkProof, err := mapprotocol.GetZkProof(zkURL, fromChainID, header.Number.Uint64())
		if err != nil {
			return 0, nil, errors.Wrap(err, "GetZkProof failed")
		}
		payload, err = proof.Pack(fromChainID, method, mapprotocol.Mcs, receiptProof, zkProof)
	case constant.ProofTypeOfOracle:
		payload, err = proof.Oracle(header.Number.Uint64(), receipt, key, proofNodes, fromChainID,
			method, logIndex, mapprotocol.ProofAbi, orderID, false)
	case constant.ProofTypeOfNewOracle:
		fallthrough
	case constant.ProofTypeOfLogOracle:
		payload, err = proof.SignOracle(header, receipt, key, proofNodes, fromChainID, logIndex, method, sign, log, proofType)
	default:
		payload, err = proof.V3Pack(fromChainID, method, mapprotocol.Map2Other, logIndex, orderID, true, receiptProof)
	}
	if err != nil {
		return 0, nil, err
	}
	return toChainID, payload, nil
}

func (a *ProofAssembler) receiptAndLogIndex(receipts []*types.Receipt, target *types.Log) (*types.Receipt, int, error) {
	if target.TxIndex >= uint(len(receipts)) {
		return nil, 0, fmt.Errorf("receipt index %d out of range for %d receipts", target.TxIndex, len(receipts))
	}
	receipt := receipts[target.TxIndex]
	if receipt == nil {
		return nil, 0, errors.New("nil receipt")
	}
	for idx, entry := range receipt.Logs {
		if entry != nil && entry.Index == target.Index {
			return receipt, idx, nil
		}
	}
	return nil, 0, fmt.Errorf("log index %d not found in receipt %s", target.Index, target.TxHash)
}

func (a *ProofAssembler) ethProof(fromChainID msg.ChainId, txIndex uint,
	receipts []*types.Receipt) ([][]byte, common.Hash, error) {
	ctx, cancel := context.WithTimeout(context.Background(), a.opReceiptTimeout)
	defer cancel()
	return a.buildEthProof(ctx, fromChainID, txIndex, receipts)
}

func (a *ProofAssembler) buildEthProof(ctx context.Context, fromChainID msg.ChainId, txIndex uint,
	receipts []*types.Receipt) ([][]byte, common.Hash, error) {
	var derivable proof.DerivableList
	switch fromChainID {
	case constant.ArbChainId, constant.Robinhood:
		arbReceipts := arb.Receipts{}
		for _, receipt := range receipts {
			arbReceipts = append(arbReceipts, &arb.Receipt{Receipt: receipt})
		}
		derivable = arbReceipts
	case constant.BaseChainId, constant.OptimismChainId:
		opReceipts, err := a.opReceiptBuilder.Build(ctx, receipts)
		if err != nil {
			return nil, constant.ZeroAddress.Hash(), err
		}
		derivable = opReceipts
	default:
		derivable = types.Receipts(receipts)
	}

	proofNodes, err := proof.Get(derivable, txIndex)
	if err != nil {
		return nil, constant.ZeroAddress.Hash(), err
	}
	receiptTrie, _ := trie.New(common.Hash{}, trie.NewDatabase(memorydb.New()))
	receiptTrie = proof.DeriveTire(derivable, receiptTrie)
	return proofNodes, receiptTrie.Hash(), nil
}
