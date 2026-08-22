package chain

import (
	"math/big"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethkeystore "github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/mapprotocol/compass/core"
	"github.com/mapprotocol/compass/internal/mapprotocol"
	"github.com/mapprotocol/compass/pkg/ethclient"
	"github.com/mapprotocol/compass/pkg/msg"
	"github.com/pkg/errors"
)

// fakeConn is a core.Connection that records whether it was opened and closed. Only
// Connect/Client/Close matter to the proof connector.
type fakeConn struct {
	connected atomic.Int32
	closed    atomic.Int32
}

func (f *fakeConn) Connect() error                         { f.connected.Add(1); return nil }
func (f *fakeConn) Keypair() *ethkeystore.Key              { return nil }
func (f *fakeConn) Opts() *bind.TransactOpts               { return nil }
func (f *fakeConn) CallOpts() *bind.CallOpts               { return nil }
func (f *fakeConn) LockAndUpdateOpts(bool) error           { return nil }
func (f *fakeConn) UnlockOpts()                            {}
func (f *fakeConn) Client() *ethclient.Client              { return nil }
func (f *fakeConn) EnsureHasBytecode(common.Address) error { return nil }
func (f *fakeConn) LatestBlock() (*big.Int, error)         { return big.NewInt(0), nil }
func (f *fakeConn) WaitForBlock(*big.Int, *big.Int) error  { return nil }
func (f *fakeConn) Close()                                 { f.closed.Add(1) }

func TestProofConnectorReusesConnection(t *testing.T) {
	var dialed atomic.Int32
	conn := &fakeConn{}
	registry := mapprotocol.NewRegistry()

	pc := NewProofConnector(registry, func(string) core.Connection {
		dialed.Add(1)
		return conn
	}, nil)

	for i := 0; i < 3; i++ {
		if _, err := pc.Connect("97", "http://node", "0x1", "0x2", "0x3"); err != nil {
			t.Fatalf("Connect #%d: %v", i, err)
		}
	}

	// Before this change every proof request dialed again and re-ran registration.
	if got := dialed.Load(); got != 1 {
		t.Fatalf("dialed %d times, want 1", got)
	}
	if got := conn.connected.Load(); got != 1 {
		t.Fatalf("Connect called %d times on the connection, want 1", got)
	}
	if _, ok := registry.MosCall(97); !ok {
		t.Fatal("mos caller was not registered")
	}
	if _, ok := registry.SignerCall(97); !ok {
		t.Fatal("signer caller was not registered")
	}
	if _, ok := registry.Map2OtherHeight(97); !ok {
		t.Fatal("map2other height provider was not registered")
	}
}

func TestProofConnectorSeparatesEndpoints(t *testing.T) {
	var dialed atomic.Int32
	registry := mapprotocol.NewRegistry()
	pc := NewProofConnector(registry, func(string) core.Connection {
		dialed.Add(1)
		return &fakeConn{}
	}, nil)

	if _, err := pc.Connect("97", "http://a", "0x1", "0x2", "0x3"); err != nil {
		t.Fatalf("Connect a: %v", err)
	}
	if _, err := pc.Connect("97", "http://b", "0x1", "0x2", "0x3"); err != nil {
		t.Fatalf("Connect b: %v", err)
	}
	if got := dialed.Load(); got != 2 {
		t.Fatalf("dialed %d times, want 2: a changed endpoint is a different connection", got)
	}
}

func TestProofConnectorRejectsBadChainID(t *testing.T) {
	var dialed atomic.Int32
	pc := NewProofConnector(mapprotocol.NewRegistry(), func(string) core.Connection {
		dialed.Add(1)
		return &fakeConn{}
	}, nil)

	if _, err := pc.Connect("not-a-number", "http://node", "0x1", "0x2", "0x3"); err == nil {
		t.Fatal("Connect with a non-numeric chain id returned no error")
	}
	if got := dialed.Load(); got != 0 {
		t.Fatalf("dialed %d times for an invalid chain id, want 0", got)
	}
}

func TestProofConnectorClosesConnectionWhenRegistrationFails(t *testing.T) {
	conn := &fakeConn{}
	boom := errors.New("boom")
	pc := NewProofConnector(mapprotocol.NewRegistry(), func(string) core.Connection { return conn },
		func(*mapprotocol.Registry, msg.ChainId, core.Connection, string) error { return boom })

	_, err := pc.Connect("97", "http://node", "0x1", "0x2", "0x3")
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if got := conn.closed.Load(); got != 1 {
		t.Fatalf("connection closed %d times after a failed registration, want 1", got)
	}

	// A failed attempt must not be cached, so the next request retries.
	if _, err := pc.Connect("97", "http://node", "0x1", "0x2", "0x3"); !errors.Is(err, boom) {
		t.Fatalf("second attempt err = %v, want boom", err)
	}
	if got := conn.connected.Load(); got != 2 {
		t.Fatalf("Connect called %d times, want 2: the failed attempt must not be cached", got)
	}
}

// TestProofConnectorConcurrentConnect mirrors what the expose proof API does: many
// HTTP handler goroutines asking for the same chain at once. Run with -race.
func TestProofConnectorConcurrentConnect(t *testing.T) {
	var dialed atomic.Int32
	conn := &fakeConn{}
	pc := NewProofConnector(mapprotocol.NewRegistry(), func(string) core.Connection {
		dialed.Add(1)
		return conn
	}, nil)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := pc.Connect("97", "http://node", "0x1", "0x2", "0x3"); err != nil {
				t.Errorf("Connect: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := dialed.Load(); got != 1 {
		t.Fatalf("dialed %d times under concurrency, want 1", got)
	}
}
