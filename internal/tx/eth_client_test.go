package tx

import (
	"errors"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
)

func TestPublicReceiptFetchersReturnForEmptyInput(t *testing.T) {
	tests := []struct {
		name  string
		fetch func() ([]*types.Receipt, error)
	}{
		{name: "standard", fetch: func() ([]*types.Receipt, error) {
			return GetReceiptsByTxsHash(nil, nil)
		}},
		{name: "matic", fetch: func() ([]*types.Receipt, error) {
			return GetMaticReceiptsByTxsHash(nil, nil)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done := make(chan error, 1)
			go func() {
				receipts, err := tt.fetch()
				if err == nil && len(receipts) != 0 {
					err = errors.New("empty input returned non-empty receipts")
				}
				done <- err
			}()

			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("empty input returned error: %v", err)
				}
			case <-time.After(25 * time.Millisecond):
				t.Fatal("empty input did not return promptly")
			}
		})
	}
}
