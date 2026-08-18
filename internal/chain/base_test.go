package chain

import (
	"errors"
	"testing"

	"github.com/mapprotocol/compass/core"
	"github.com/mapprotocol/compass/pkg/msg"
)

type baseTestConnection struct {
	core.Connection
	closed bool
}

func (c *baseTestConnection) Close() {
	c.closed = true
}

type baseTestWriter struct{}

func (*baseTestWriter) ResolveMessage(msg.Message) bool {
	return true
}

type baseTestListener struct {
	router  core.Router
	syncErr error
	synced  bool
}

func (l *baseTestListener) Sync() error {
	l.synced = true
	return l.syncErr
}

func (l *baseTestListener) SetRouter(r core.Router) {
	l.router = r
}

type baseTestRouter struct {
	id     msg.ChainId
	writer core.Writer
}

func (*baseTestRouter) Send(msg.Message) error {
	return nil
}

func (r *baseTestRouter) Listen(id msg.ChainId, writer core.Writer) {
	r.id = id
	r.writer = writer
}

func TestBaseWiresChainLifecycle(t *testing.T) {
	cfg := &core.ChainConfig{Name: "test-chain", Id: msg.ChainId(17)}
	conn := &baseTestConnection{}
	writer := &baseTestWriter{}
	listener := &baseTestListener{}
	stop := make(chan int)
	base := NewBase(cfg, conn, writer, listener, stop)

	if base.Id() != cfg.Id {
		t.Fatalf("Id() = %d, want %d", base.Id(), cfg.Id)
	}
	if base.Name() != cfg.Name {
		t.Fatalf("Name() = %q, want %q", base.Name(), cfg.Name)
	}
	if base.Conn() != conn {
		t.Fatal("Conn() did not return the configured connection")
	}

	router := &baseTestRouter{}
	base.SetRouter(router)
	if router.id != cfg.Id || router.writer != writer {
		t.Fatalf("router registration = (%d, %T), want (%d, %T)", router.id, router.writer, cfg.Id, writer)
	}
	if listener.router != router {
		t.Fatal("SetRouter did not wire the listener")
	}

	if err := base.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	if !listener.synced {
		t.Fatal("Start() did not sync the listener")
	}

	base.Stop()
	if !conn.closed {
		t.Fatal("Stop() did not close the connection")
	}
	select {
	case _, ok := <-stop:
		if ok {
			t.Fatal("Stop() sent a value instead of closing the stop channel")
		}
	default:
		t.Fatal("Stop() did not close the stop channel")
	}
}

func TestBaseReturnsListenerError(t *testing.T) {
	wantErr := errors.New("sync failed")
	base := NewBase(
		&core.ChainConfig{},
		&baseTestConnection{},
		&baseTestWriter{},
		&baseTestListener{syncErr: wantErr},
		make(chan int),
	)

	if err := base.Start(); !errors.Is(err, wantErr) {
		t.Fatalf("Start() error = %v, want %v", err, wantErr)
	}
}
