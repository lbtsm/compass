package mapo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/mapprotocol/compass/pkg/ethclient"
)

type opReceiptReaderFunc func(context.Context, common.Hash) (*ethclient.OpReceipt, error)

func (f opReceiptReaderFunc) OpReceipt(ctx context.Context, hash common.Hash) (*ethclient.OpReceipt, error) {
	return f(ctx, hash)
}

func TestOPReceiptBuilderObjectAPI(t *testing.T) {
	receipts := []*types.Receipt{{TxHash: common.HexToHash("0x1")}}
	reader := opReceiptReaderFunc(func(context.Context, common.Hash) (*ethclient.OpReceipt, error) {
		return &ethclient.OpReceipt{DepositNonce: "0x2"}, nil
	})
	builder := NewOPReceiptBuilder(reader)

	got, err := builder.Build(context.Background(), receipts)
	if err != nil {
		t.Fatalf("OPReceiptBuilder.Build returned error: %v", err)
	}
	if len(got) != 1 || got[0].DepositNonce == nil || *got[0].DepositNonce != 2 {
		t.Fatalf("OPReceiptBuilder.Build returned %+v, want nonce 2", got)
	}
}

func TestOPReceiptBuilderReturnsFetchError(t *testing.T) {
	wantErr := errors.New("rpc failed")
	receipts := []*types.Receipt{
		{TxHash: common.HexToHash("0x1")},
		{TxHash: common.HexToHash("0x2")},
	}
	reader := opReceiptReaderFunc(func(_ context.Context, hash common.Hash) (*ethclient.OpReceipt, error) {
		if hash == receipts[1].TxHash {
			return nil, wantErr
		}
		return &ethclient.OpReceipt{}, nil
	})

	_, err := NewOPReceiptBuilder(reader).Build(context.Background(), receipts)
	if !errors.Is(err, wantErr) {
		t.Fatalf("OPReceiptBuilder.Build error = %v, want wrapped %v", err, wantErr)
	}
}

func TestOPReceiptBuilderParsesOptionalFieldsAndPreservesOrder(t *testing.T) {
	receipts := []*types.Receipt{
		{TxHash: common.HexToHash("0x1")},
		{TxHash: common.HexToHash("0x2")},
	}
	reader := opReceiptReaderFunc(func(_ context.Context, hash common.Hash) (*ethclient.OpReceipt, error) {
		if hash == receipts[0].TxHash {
			return &ethclient.OpReceipt{
				DepositNonce:          "0x2",
				DepositReceiptVersion: "0x1",
			}, nil
		}
		return &ethclient.OpReceipt{}, nil
	})

	got, err := NewOPReceiptBuilder(reader).Build(context.Background(), receipts)
	if err != nil {
		t.Fatalf("OPReceiptBuilder.Build returned error: %v", err)
	}
	if len(got) != len(receipts) || got[0].Receipt != receipts[0] || got[1].Receipt != receipts[1] {
		t.Fatalf("OPReceiptBuilder.Build did not preserve receipt order: %+v", got)
	}
	if got[0].DepositNonce == nil || *got[0].DepositNonce != 2 {
		t.Fatalf("deposit nonce = %v, want 2", got[0].DepositNonce)
	}
	if got[0].DepositReceiptVersion == nil || *got[0].DepositReceiptVersion != 1 {
		t.Fatalf("deposit receipt version = %v, want 1", got[0].DepositReceiptVersion)
	}
	if got[1].DepositNonce != nil || got[1].DepositReceiptVersion != nil {
		t.Fatalf("missing optional fields should remain nil: %+v", got[1])
	}
}

func TestOPReceiptBuilderFetchesConcurrently(t *testing.T) {
	receipts := []*types.Receipt{
		{TxHash: common.HexToHash("0x1")},
		{TxHash: common.HexToHash("0x2")},
	}
	started := make(chan struct{}, len(receipts))
	release := make(chan struct{})
	reader := opReceiptReaderFunc(func(context.Context, common.Hash) (*ethclient.OpReceipt, error) {
		started <- struct{}{}
		<-release
		return &ethclient.OpReceipt{}, nil
	})

	done := make(chan error, 1)
	go func() {
		_, err := NewOPReceiptBuilder(reader).Build(context.Background(), receipts)
		done <- err
	}()
	<-started
	select {
	case <-started:
		close(release)
	case <-time.After(25 * time.Millisecond):
		close(release)
		<-done
		t.Fatal("OP receipt fetches ran serially")
	}
	if err := <-done; err != nil {
		t.Fatalf("OPReceiptBuilder.Build returned error: %v", err)
	}
}
