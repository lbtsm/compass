package chain

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ChainSafe/log15"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/mapprotocol/compass/internal/constant"
	"github.com/mapprotocol/compass/internal/stream"
	"github.com/mapprotocol/compass/pkg/blockstore"
	"github.com/mapprotocol/compass/pkg/ethclient"
	"github.com/mapprotocol/compass/pkg/msg"
)

func receiptLogFixture() (*types.Log, *types.Log) {
	target := &types.Log{
		Address:     common.HexToAddress("0x1000000000000000000000000000000000000001"),
		Topics:      []common.Hash{common.HexToHash("0x1")},
		Data:        []byte{1, 2, 3},
		BlockNumber: 100,
		TxHash:      common.HexToHash("0x2"),
		TxIndex:     4,
		BlockHash:   common.HexToHash("0x3"),
		Index:       7,
	}
	onChain := *target
	return target, &onChain
}

func TestRequireMatchingReceiptLogReturnsOnChainLog(t *testing.T) {
	target, onChain := receiptLogFixture()
	got, err := requireMatchingReceiptLog([]*types.Log{onChain}, target)
	if err != nil {
		t.Fatalf("requireMatchingReceiptLog returned error: %v", err)
	}
	if got != onChain {
		t.Fatal("requireMatchingReceiptLog did not return the receipt log")
	}
}

func TestRequireMatchingReceiptLogRejectsMismatch(t *testing.T) {
	target, onChain := receiptLogFixture()
	onChain.Data = []byte{9}
	_, err := requireMatchingReceiptLog([]*types.Log{onChain}, target)
	if err == nil {
		t.Fatal("requireMatchingReceiptLog returned nil error for mismatched data")
	}
	if !strings.Contains(err.Error(), target.TxHash.Hex()) || !strings.Contains(err.Error(), "logIndex(7)") {
		t.Fatalf("error lacks receipt context: %v", err)
	}
}

type latestBlockConnection struct {
	latest *big.Int
}

func (c *latestBlockConnection) Connect() error                         { return nil }
func (c *latestBlockConnection) Keypair() *keystore.Key                 { return nil }
func (c *latestBlockConnection) Opts() *bind.TransactOpts               { return nil }
func (c *latestBlockConnection) CallOpts() *bind.CallOpts               { return nil }
func (c *latestBlockConnection) LockAndUpdateOpts(bool) error           { return nil }
func (c *latestBlockConnection) UnlockOpts()                            {}
func (c *latestBlockConnection) Client() *ethclient.Client              { return nil }
func (c *latestBlockConnection) EnsureHasBytecode(common.Address) error { return nil }
func (c *latestBlockConnection) LatestBlock() (*big.Int, error) {
	return new(big.Int).Set(c.latest), nil
}
func (c *latestBlockConnection) WaitForBlock(*big.Int, *big.Int) error { return nil }
func (c *latestBlockConnection) Close()                                {}

type projectFilterClient struct {
	responses map[int64]*stream.MosListResp
}

func (c *projectFilterClient) LatestBlock(int64) (*big.Int, error) { return big.NewInt(0), nil }
func (c *projectFilterClient) MaxID(int64) (*big.Int, error)       { return big.NewInt(0), nil }
func (c *projectFilterClient) ListMosLogs(req FilterListRequest) (*stream.MosListResp, error) {
	if response := c.responses[req.ProjectID]; response != nil {
		return response, nil
	}
	return &stream.MosListResp{}, nil
}

func filterLog(id int64, block uint64, address string) *stream.GetMosResp {
	return &stream.GetMosResp{
		Id:              id,
		BlockNumber:     block,
		ContractAddress: address,
		Topic: strings.Join([]string{
			common.HexToHash("0x1").Hex(),
			common.HexToHash("0x2").Hex(),
			common.Hash{}.Hex(),
		}, ","),
	}
}

func TestFilterOracleDoesNotAdvancePastUnfinalizedLog(t *testing.T) {
	mcs := common.HexToAddress("0x1000000000000000000000000000000000000001")
	cfg := Config{
		Id:                 msg.ChainId(constant.MapChainId),
		MapChainID:         msg.ChainId(constant.MapChainId),
		Filter:             true,
		StartBlock:         big.NewInt(100),
		BlockConfirmations: big.NewInt(10),
		McsContract:        []common.Address{mcs},
	}
	filterClient := &projectFilterClient{responses: map[int64]*stream.MosListResp{
		constant.ProjectOfOracle: {List: []*stream.GetMosResp{filterLog(110, 90, mcs.Hex())}},
		constant.ProjectOfMsger:  {List: []*stream.GetMosResp{filterLog(111, 91, mcs.Hex())}},
	}}
	commonSync := NewCommonSync(
		&latestBlockConnection{latest: big.NewInt(100)},
		&cfg,
		log15.New(),
		make(chan int),
		make(chan error, 1),
		&blockstore.EmptyStore{},
		OptOfFilterClient(filterClient),
	)
	oracle := NewOracle(commonSync)

	if err := oracle.filterOracle(); err != nil {
		t.Fatalf("filterOracle returned error: %v", err)
	}
	if commonSync.Cfg.StartBlock.Int64() != 100 {
		t.Fatalf("StartBlock advanced to %d, want 100", commonSync.Cfg.StartBlock.Int64())
	}
}

func TestFilterBatchesFinalizedBoundaries(t *testing.T) {
	mcs := common.HexToAddress("0x1000000000000000000000000000000000000001")
	oracle := NewOracle(&CommonSync{
		Cfg:                Config{McsContract: []common.Address{mcs}},
		BlockConfirmations: big.NewInt(10),
	})

	for _, tt := range []struct {
		name      string
		block     uint64
		address   string
		finalized bool
	}{
		{name: "exact confirmation depth", block: 90, address: mcs.Hex(), finalized: true},
		{name: "one confirmation short", block: 91, address: mcs.Hex(), finalized: false},
		{name: "future block", block: 101, address: mcs.Hex(), finalized: false},
		{name: "unrelated address", block: 101, address: common.HexToAddress("0x2").Hex(), finalized: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			batches := []*stream.MosListResp{{List: []*stream.GetMosResp{filterLog(1, tt.block, tt.address)}}}
			got, _ := oracle.filterBatchesFinalized(batches, big.NewInt(100))
			if got != tt.finalized {
				t.Fatalf("filterBatchesFinalized returned %t, want %t", got, tt.finalized)
			}
		})
	}
}
