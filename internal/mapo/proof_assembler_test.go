package mapo

import (
	"context"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/mapprotocol/compass/internal/constant"
	"github.com/mapprotocol/compass/pkg/ethclient"
)

func TestProofAssemblerReturnsOPReceiptFetchError(t *testing.T) {
	wantErr := errors.New("rpc failed")
	receipts := []*types.Receipt{{TxHash: common.HexToHash("0x1")}}
	reader := opReceiptReaderFunc(func(context.Context, common.Hash) (*ethclient.OpReceipt, error) {
		return nil, wantErr
	})
	assembler := &ProofAssembler{opReceiptBuilder: NewOPReceiptBuilder(reader)}

	_, _, err := assembler.buildEthProof(context.Background(), constant.BaseChainId, 0, receipts)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ProofAssembler.buildEthProof error = %v, want wrapped %v", err, wantErr)
	}
}

func TestProofAssemblerReturnsErrorWhenLogIsMissing(t *testing.T) {
	receipt := &types.Receipt{Logs: []*types.Log{{Index: 1}}}
	target := &types.Log{Index: 2, TxHash: common.HexToHash("0x2")}

	_, _, err := (&ProofAssembler{}).receiptAndLogIndex([]*types.Receipt{receipt}, target)
	if err == nil {
		t.Fatal("ProofAssembler.receiptAndLogIndex returned nil error for a missing log")
	}
}

func TestProofAssemblerReturnsPositionWithinReceipt(t *testing.T) {
	receipt := &types.Receipt{Logs: []*types.Log{
		{Index: 3},
		{Index: 7},
	}}
	target := &types.Log{Index: 7}

	gotReceipt, gotIndex, err := (&ProofAssembler{}).receiptAndLogIndex([]*types.Receipt{receipt}, target)
	if err != nil {
		t.Fatalf("ProofAssembler.receiptAndLogIndex returned error: %v", err)
	}
	if gotReceipt != receipt || gotIndex != 1 {
		t.Fatalf("receiptAndLogIndex = (%p, %d), want (%p, 1)", gotReceipt, gotIndex, receipt)
	}
}

func TestProofAssemblerObjectAPIRejectsMissingLog(t *testing.T) {
	assembler := NewProofAssembler(nil)
	log := &types.Log{
		Index:  2,
		Topics: []common.Hash{{}, common.HexToHash("0x1")},
	}
	receipts := []*types.Receipt{{Logs: []*types.Log{{Index: 1}}}}

	_, err := assembler.AssembleEth(log, receipts, nil, "", 0, 0, nil)
	if err == nil {
		t.Fatal("ProofAssembler.AssembleEth returned nil error for a missing log")
	}
}

func TestProofAssemblerObjectAPIRejectsInvalidMapLog(t *testing.T) {
	assembler := NewProofAssembler(nil)
	log := &types.Log{Topics: []common.Hash{{}, common.HexToHash("0x1")}}

	_, _, err := assembler.AssembleMap(log, nil, nil, 0, "", "", 0, nil)
	if err == nil {
		t.Fatal("ProofAssembler.AssembleMap returned nil error for an invalid log")
	}
}
