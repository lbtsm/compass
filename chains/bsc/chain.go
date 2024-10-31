package bsc

import (
	"bytes"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"math/big"
	"strconv"

	"github.com/ChainSafe/log15"
	"github.com/ethereum/go-ethereum/core/types"
	connection "github.com/mapprotocol/compass/connections/ethereum"
	"github.com/mapprotocol/compass/core"
	"github.com/mapprotocol/compass/internal/bsc"
	"github.com/mapprotocol/compass/internal/chain"
	"github.com/mapprotocol/compass/internal/proof"
	"github.com/mapprotocol/compass/internal/tx"
	"github.com/mapprotocol/compass/mapprotocol"
	"github.com/mapprotocol/compass/msg"
	"github.com/mapprotocol/compass/pkg/ethclient"
)

func New(chainCfg *core.ChainConfig, logger log15.Logger, sysErr chan<- error,
	role mapprotocol.Role) (core.Chain, error) {
	return chain.New(chainCfg, logger, sysErr, role, connection.NewConnection,
		chain.OptOfSync2Map(syncHeaderToMap),
		chain.OptOfInitHeight(mapprotocol.HeaderCountOfBsc),
		chain.OptOfAssembleProof(assembleProof),
		chain.OptOfOracleHandler(chain.DefaultOracleHandler))
}

const (
	BLSPublicKeyLength = 48
	BLSSignatureLength = 96
)

type VoteAttestation struct {
	VoteAddressSet ValidatorsBitSet // The bitset marks the voted validators.
	AggSignature   BLSSignature     // The aggregated BLS signature of the voted validators' signatures.
	Data           *VoteData        // The vote data for fast finality.
	Extra          []byte           // Reserved for future usage.
}

type BLSPublicKey [BLSPublicKeyLength]byte
type BLSSignature [BLSSignatureLength]byte
type ValidatorsBitSet uint64

type VoteData struct {
	SourceNumber uint64      // The source block number should be the latest justified block number.
	SourceHash   common.Hash // The block hash of the source block.
	TargetNumber uint64      // The target block number which validator wants to vote for.
	TargetHash   common.Hash // The block hash of the target block.
}

func syncHeaderToMap(m *chain.Maintainer, latestBlock *big.Int) error {
	remainder := big.NewInt(0).Mod(latestBlock, big.NewInt(mapprotocol.EpochOfBsc))
	if remainder.Cmp(mapprotocol.Big0) != 0 {
		fmt.Println("------------------------")
		return nil
	}
	//// synced height check
	//syncedHeight, err := mapprotocol.Get2MapHeight(m.Cfg.Id)
	//if err != nil {
	//	m.Log.Error("Get current synced Height failed", "err", err)
	//	return err
	//}
	//if latestBlock.Cmp(syncedHeight) <= 0 {
	//	m.Log.Info("CurrentBlock less than synchronized headerHeight", "synced height", syncedHeight,
	//		"current height", latestBlock)
	//	return nil
	//}
	// step1: 获取当前block，解析extraData
	if latestBlock.Int64()%mapprotocol.EpochOfBsc == 0 {

	}
	headerHeight := new(big.Int).Add(latestBlock, new(big.Int).SetInt64(int64(1)))
	header, err := m.Conn.Client().BscHeaderByNumber(m.Cfg.Endpoint, headerHeight)
	if err != nil {
		return err
	}

	//ret := struct {
	//	ValidatorsBitSet interface{}
	//	BLSSignature interface{}
	//}{}
	var extra VoteAttestation
	m.Log.Info("Find sync block", "current height", headerHeight, "extra", extra, "hex", common.Bytes2Hex(header.Extra), "len", len(header.Extra))
	err = rlp.Decode(bytes.NewBuffer(header.Extra[33:len(header.Extra)-65]), &extra)
	if err != nil {
		return err
	}

	m.Log.Info("Find sync block", "current height", headerHeight, "extra", extra, "hex", common.Bytes2Hex(header.Extra), "len", len(header.Extra))
	//headers := make([]*ethclient.BscHeader, 0)

	//for i := 0; i < mapprotocol.HeaderCountOfBsc; i++ {
	//	headerHeight := new(big.Int).Sub(latestBlock, new(big.Int).SetInt64(int64(i)))
	//	header, err := m.Conn.Client().BscHeaderByNumber(m.Cfg.Endpoint, headerHeight)
	//	if err != nil {
	//		return err
	//	}
	//	headers[mapprotocol.HeaderCountOfBsc-i-1] = header
	//}
	//
	//params := make([]bsc.Header, 0, len(headers))
	//for _, h := range headers {
	//	params = append(params, bsc.ConvertHeader(h))
	//}
	//input, err := mapprotocol.Bsc.Methods[mapprotocol.MethodOfGetHeadersBytes].Inputs.Pack(params)
	//if err != nil {
	//	m.Log.Error("Failed to abi pack", "err", err)
	//	return err
	//}

	//id := big.NewInt(0).SetUint64(uint64(m.Cfg.Id))
	//msgpayload := []interface{}{id, input}
	//message := msg.NewSyncToMap(m.Cfg.Id, m.Cfg.MapChainID, msgpayload, m.MsgCh)

	//err = m.Router.Send(message)
	//if err != nil {
	//	m.Log.Error("Subscription error: failed to route message", "err", err)
	//	return err
	//}

	err = m.WaitUntilMsgHandled(1)
	if err != nil {
		return err
	}
	return nil
}

func assembleProof(m *chain.Messenger, log *types.Log, proofType int64, toChainID uint64, sign [][]byte) (*msg.Message, error) {
	var (
		message   msg.Message
		orderId   = log.Topics[1]
		method    = m.GetMethod(log.Topics[0])
		bigNumber = big.NewInt(int64(log.BlockNumber))
	)
	txsHash, err := mapprotocol.GetTxsByBn(m.Conn.Client(), bigNumber)
	if err != nil {
		return nil, fmt.Errorf("unable to get tx hashes Logs: %w", err)
	}
	var receipts []*types.Receipt
	key := strconv.FormatUint(uint64(m.Cfg.Id), 10) + "_" + bigNumber.String()
	if v, ok := proof.CacheReceipt[key]; ok {
		receipts = v
		m.Log.Info("use cache receipt", "latestBlock ", bigNumber, "txHash", log.TxHash)
	} else {
		receipts, err = tx.GetReceiptsByTxsHash(m.Conn.Client(), txsHash)
		if err != nil {
			return nil, fmt.Errorf("unable to get receipts hashes Logs: %w", err)
		}
		proof.CacheReceipt[key] = receipts
	}

	var orderId32 [32]byte
	for idx, v := range orderId {
		orderId32[idx] = v
	}
	headers := make([]*ethclient.BscHeader, mapprotocol.HeaderCountOfBsc)
	for i := 0; i < mapprotocol.HeaderCountOfBsc; i++ {
		headerHeight := new(big.Int).Add(bigNumber, new(big.Int).SetInt64(int64(i)))
		header, err := m.Conn.Client().BscHeaderByNumber(m.Cfg.Endpoint, headerHeight)
		if err != nil {
			return nil, err
		}
		headers[i] = header
	}

	params := make([]bsc.Header, 0, len(headers))
	for _, h := range headers {
		params = append(params, bsc.ConvertHeader(h))
	}

	payload, err := bsc.AssembleProof(params, log, receipts, method, m.Cfg.Id, proofType, sign, orderId32)
	if err != nil {
		return nil, fmt.Errorf("unable to Parse Log: %w", err)
	}

	msgPayload := []interface{}{payload, orderId32, log.BlockNumber, log.TxHash}
	message = msg.NewSwapWithProof(m.Cfg.Id, m.Cfg.MapChainID, msgPayload, m.MsgCh)
	return &message, nil
}
