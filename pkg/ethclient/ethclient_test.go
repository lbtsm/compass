package ethclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestOpReceiptHonorsContext(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseRequest) })
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		select {
		case <-r.Context().Done():
			return
		case <-releaseRequest:
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
		}
	}))
	defer server.Close()
	defer release()

	client := NewClient(nil, server.URL, server.Client())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.OpReceipt(ctx, common.Hash{})
		done <- err
	}()

	<-requestStarted
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("OpReceipt error = %v, want context.Canceled", err)
		}
	case <-time.After(25 * time.Millisecond):
		release()
		<-done
		t.Fatal("OpReceipt did not stop after context cancellation")
	}
}

func TestOpReceiptReturnsJSONRPCError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"receipt unavailable"}}`))
	}))
	defer server.Close()

	client := NewClient(nil, server.URL, server.Client())
	_, err := client.OpReceipt(context.Background(), common.Hash{})
	if err == nil || !strings.Contains(err.Error(), "receipt unavailable") {
		t.Fatalf("OpReceipt error = %v, want JSON-RPC error message", err)
	}
}
