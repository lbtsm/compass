package tx

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type receiptReaderFunc func(context.Context, common.Hash) (*types.Receipt, error)

func (f receiptReaderFunc) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	return f(ctx, hash)
}

func TestReceiptFetcherObjectAPI(t *testing.T) {
	hashes := []common.Hash{common.HexToHash("0x1"), common.HexToHash("0x2")}
	reader := receiptReaderFunc(func(_ context.Context, hash common.Hash) (*types.Receipt, error) {
		return &types.Receipt{TxHash: hash}, nil
	})

	receipts, err := NewReceiptFetcher(reader, RetryNotFound).Fetch(context.Background(), hashes)
	if err != nil {
		t.Fatalf("ReceiptFetcher.Fetch returned error: %v", err)
	}
	for idx, receipt := range receipts {
		if receipt == nil || receipt.TxHash != hashes[idx] {
			t.Fatalf("receipt %d = %+v, want hash %s", idx, receipt, hashes[idx])
		}
	}
}

func TestReceiptFetcherEmpty(t *testing.T) {
	called := false
	reader := receiptReaderFunc(func(context.Context, common.Hash) (*types.Receipt, error) {
		called = true
		return nil, nil
	})

	receipts, err := NewReceiptFetcher(reader, RetryNotFound).Fetch(context.Background(), nil)
	if err != nil {
		t.Fatalf("ReceiptFetcher.Fetch returned error: %v", err)
	}
	if len(receipts) != 0 {
		t.Fatalf("ReceiptFetcher.Fetch returned %d receipts, want 0", len(receipts))
	}
	if called {
		t.Fatal("ReceiptFetcher.Fetch called reader for empty input")
	}
}

func TestReceiptFetcherPreservesOrder(t *testing.T) {
	hashes := []common.Hash{
		common.HexToHash("0x1"),
		common.HexToHash("0x2"),
		common.HexToHash("0x3"),
	}
	reader := receiptReaderFunc(func(_ context.Context, hash common.Hash) (*types.Receipt, error) {
		return &types.Receipt{TxHash: hash}, nil
	})

	receipts, err := NewReceiptFetcher(reader, PreserveNotFound).Fetch(context.Background(), hashes)
	if err != nil {
		t.Fatalf("ReceiptFetcher.Fetch returned error: %v", err)
	}
	if len(receipts) != len(hashes) {
		t.Fatalf("ReceiptFetcher.Fetch returned %d receipts, want %d", len(receipts), len(hashes))
	}
	for idx, receipt := range receipts {
		if receipt == nil || receipt.TxHash != hashes[idx] {
			t.Fatalf("receipt %d = %+v, want hash %s", idx, receipt, hashes[idx])
		}
	}
}

func TestReceiptFetcherRetriesNotFound(t *testing.T) {
	hash := common.HexToHash("0x1")
	calls := 0
	reader := receiptReaderFunc(func(_ context.Context, got common.Hash) (*types.Receipt, error) {
		calls++
		if calls == 1 {
			return nil, ethereum.NotFound
		}
		return &types.Receipt{TxHash: got}, nil
	})

	receipts, err := NewReceiptFetcher(reader, RetryNotFound).Fetch(context.Background(), []common.Hash{hash})
	if err != nil {
		t.Fatalf("ReceiptFetcher.Fetch returned error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("reader called %d times, want 2", calls)
	}
	if len(receipts) != 1 || receipts[0] == nil || receipts[0].TxHash != hash {
		t.Fatalf("ReceiptFetcher.Fetch returned %+v, want receipt %s", receipts, hash)
	}
}

func TestReceiptFetcherPreservesMaticNotFoundPosition(t *testing.T) {
	hashes := []common.Hash{common.HexToHash("0x1"), common.HexToHash("0x2")}
	reader := receiptReaderFunc(func(_ context.Context, hash common.Hash) (*types.Receipt, error) {
		if hash == hashes[0] {
			return nil, ethereum.NotFound
		}
		return &types.Receipt{TxHash: hash}, nil
	})

	receipts, err := NewReceiptFetcher(reader, PreserveNotFound).Fetch(context.Background(), hashes)
	if err != nil {
		t.Fatalf("ReceiptFetcher.Fetch returned error: %v", err)
	}
	if len(receipts) != len(hashes) || receipts[0] != nil {
		t.Fatalf("Matic missing receipt position was not preserved: %+v", receipts)
	}
	if receipts[1] == nil || receipts[1].TxHash != hashes[1] {
		t.Fatalf("receipt 1 = %+v, want hash %s", receipts[1], hashes[1])
	}
}

func TestReceiptFetcherStopsRetryWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	called := make(chan struct{}, 1)
	reader := receiptReaderFunc(func(ctx context.Context, _ common.Hash) (*types.Receipt, error) {
		called <- struct{}{}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, ethereum.NotFound
	})

	done := make(chan error, 1)
	go func() {
		_, err := NewReceiptFetcher(reader, RetryNotFound).Fetch(ctx, []common.Hash{{1}})
		done <- err
	}()
	<-called
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ReceiptFetcher.Fetch error = %v, want context.Canceled", err)
		}
	case <-time.After(25 * time.Millisecond):
		err := <-done
		t.Fatalf("ReceiptFetcher.Fetch did not stop promptly after cancellation; eventual error: %v", err)
	}
}
