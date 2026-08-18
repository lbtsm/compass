package chain

import (
	"github.com/ethereum/go-ethereum/log"
	"github.com/mapprotocol/compass/core"
	"github.com/mapprotocol/compass/pkg/msg"
)

// Base owns the lifecycle dependencies shared by all chain implementations.
type Base struct {
	cfg    *core.ChainConfig
	conn   core.Connection
	writer core.Writer
	listen core.Listener
	stop   chan<- int
}

func NewBase(cfg *core.ChainConfig, conn core.Connection, writer core.Writer, listen core.Listener, stop chan<- int) Base {
	return Base{
		cfg:    cfg,
		conn:   conn,
		writer: writer,
		listen: listen,
		stop:   stop,
	}
}

func (b *Base) SetRouter(r core.Router) {
	r.Listen(b.cfg.Id, b.writer)
	b.listen.SetRouter(r)
}

func (b *Base) Start() error {
	if err := b.listen.Sync(); err != nil {
		return err
	}

	log.Debug("Successfully started Chain")
	return nil
}

func (b *Base) Id() msg.ChainId {
	return b.cfg.Id
}

func (b *Base) Name() string {
	return b.cfg.Name
}

// Stop signals to any running routines to exit.
func (b *Base) Stop() {
	close(b.stop)
	if b.conn != nil {
		b.conn.Close()
	}
}

// Conn returns the connection used by the chain.
func (b *Base) Conn() core.Connection {
	return b.conn
}
