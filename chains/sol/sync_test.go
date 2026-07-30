package sol

import (
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ChainSafe/log15"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mapprotocol/compass/internal/chain"
	"github.com/mapprotocol/compass/internal/stream"
)

const (
	failedSolanaBudgetTxHash  = "43bb94tYCsdiQdsMLm96GeuyJMbefHtSiRp2Uorrm1PegaJ6fBrFiCQJ5Vpg4U2zVBzdAefM3ZapvKNiYichKdmA"
	failedSolanaProgramTxHash = "4LmJMRMrPrmHmDJdM3NyAt6TcXTiircWrbUc2YAewoxvp8sRU2s1KJ3idwyi8uCVoALe9ZRGVELwDmF4sed7sM2F"
)

func newFailedTransactionRPCServer(t *testing.T, txErr interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode JSON-RPC request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": map[string]interface{}{
				"slot":        1,
				"meta":        map[string]interface{}{"err": txErr},
				"transaction": []string{"AA==", "base64"},
				"version":     0,
			},
		}); err != nil {
			t.Errorf("encode JSON-RPC response: %v", err)
		}
	}))
}

func TestCheckLogRejectsFailedTransaction(t *testing.T) {
	for _, tt := range []struct {
		name        string
		txHash      string
		instruction int
		reason      string
	}{
		{name: "computational budget exceeded", txHash: failedSolanaBudgetTxHash, instruction: 10, reason: "ComputationalBudgetExceeded"},
		{name: "program failed to complete", txHash: failedSolanaProgramTxHash, instruction: 8, reason: "ProgramFailedToComplete"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := newFailedTransactionRPCServer(t, map[string]interface{}{
				"InstructionError": []interface{}{tt.instruction, tt.reason},
			})
			t.Cleanup(server.Close)

			err := (&sync{solClient: rpc.New(server.URL)}).checkLog(&Log{TxHash: tt.txHash})
			if !errors.Is(err, errSolanaTransactionFailed) {
				t.Fatalf("expected failed transaction error, got %v", err)
			}
			if !strings.Contains(err.Error(), tt.txHash) || !strings.Contains(err.Error(), tt.reason) {
				t.Fatalf("error lacks transaction context: %v", err)
			}
		})
	}
}

type staticSolFilterClient struct {
	response *stream.MosListResp
}

func (c *staticSolFilterClient) LatestBlock(int64) (*big.Int, error) { return big.NewInt(0), nil }
func (c *staticSolFilterClient) MaxID(int64) (*big.Int, error)       { return big.NewInt(0), nil }
func (c *staticSolFilterClient) ListMosLogs(chain.FilterListRequest) (*stream.MosListResp, error) {
	return c.response, nil
}

type recordingBlockStore struct {
	block *big.Int
}

func (s *recordingBlockStore) StoreBlock(block *big.Int) error {
	s.block = new(big.Int).Set(block)
	return nil
}

func TestSyncOnceSkipsFailedTransactionWithoutWaitingForMessage(t *testing.T) {
	server := newFailedTransactionRPCServer(t, map[string]interface{}{
		"InstructionError": []interface{}{10, "ComputationalBudgetExceeded"},
	})
	t.Cleanup(server.Close)

	const logID int64 = 424
	const program = "mos-program"
	cfg := &Config{
		Config: chain.Config{
			Filter:             true,
			StartBlock:         big.NewInt(0),
			BlockConfirmations: big.NewInt(0),
		},
		McsContract: []string{program},
	}
	store := &recordingBlockStore{}
	commonSync := chain.NewCommonSync(
		nil,
		&cfg.Config,
		log15.New(),
		make(chan int),
		make(chan error, 1),
		store,
		chain.OptOfFilterClient(&staticSolFilterClient{response: &stream.MosListResp{
			List: []*stream.GetMosResp{{Id: logID, TxHash: failedSolanaBudgetTxHash, ContractAddress: program}},
		}}),
	)
	m := newSync(commonSync, oracleHandler, nil, cfg, rpc.New(server.URL))

	type outcome struct {
		result handlerResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := m.syncOnce()
		done <- outcome{result: result, err: err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("syncOnce returned error: %v", got.err)
		}
		if got.result.cursor != logID {
			t.Fatalf("syncOnce returned cursor %d, want %d", got.result.cursor, logID)
		}
		if got.result.messageCount != 0 {
			t.Fatalf("syncOnce returned message count %d, want 0", got.result.messageCount)
		}
	case <-time.After(time.Second):
		t.Fatal("syncOnce blocked waiting for a message that was never routed")
	}

	if m.Cfg.StartBlock.Int64() != logID {
		t.Fatalf("sync cursor is %d, want %d", m.Cfg.StartBlock.Int64(), logID)
	}
	if store.block == nil || store.block.Int64() != logID {
		t.Fatalf("stored cursor is %v, want %d", store.block, logID)
	}
}

func TestSyncOnceWaitsForRoutedMessageBeforeStoringCursor(t *testing.T) {
	const logID int64 = 425
	cfg := &Config{Config: chain.Config{StartBlock: big.NewInt(0)}}
	store := &recordingBlockStore{}
	handlerCalled := make(chan struct{})
	commonSync := chain.NewCommonSync(
		nil,
		&cfg.Config,
		log15.New(),
		make(chan int),
		make(chan error, 1),
		store,
	)
	m := newSync(commonSync, func(*sync) (handlerResult, error) {
		close(handlerCalled)
		return handlerResult{cursor: logID, messageCount: 1}, nil
	}, nil, cfg, nil)

	done := make(chan error, 1)
	go func() {
		_, err := m.syncOnce()
		done <- err
	}()
	<-handlerCalled

	select {
	case err := <-done:
		t.Fatalf("syncOnce returned before routed message was handled: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if store.block != nil {
		t.Fatalf("stored cursor %s before routed message was handled", store.block)
	}

	m.MsgCh <- struct{}{}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("syncOnce returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("syncOnce remained blocked after routed message was handled")
	}
	if store.block == nil || store.block.Int64() != logID {
		t.Fatalf("stored cursor is %v, want %d", store.block, logID)
	}
}
