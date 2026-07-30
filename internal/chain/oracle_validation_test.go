package chain

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
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
