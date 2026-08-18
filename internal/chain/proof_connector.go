package chain

import (
	"math/big"
	"strconv"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	connection "github.com/mapprotocol/compass/connections/ethereum"
	"github.com/mapprotocol/compass/core"
	"github.com/mapprotocol/compass/internal/constant"
	"github.com/mapprotocol/compass/internal/mapprotocol"
	"github.com/mapprotocol/compass/pkg/abi"
	"github.com/mapprotocol/compass/pkg/contract"
	"github.com/mapprotocol/compass/pkg/ethclient"
	"github.com/mapprotocol/compass/pkg/msg"
	"github.com/pkg/errors"
)

// Dialer opens a read-only connection to a chain endpoint. Each chain package
// supplies its own, which is the only part of proof connection setup that differs
// between them.
type Dialer func(endpoint string) core.Connection

// HeightRegistrar records how the MAP chain and a peer chain observe each other's
// height. It runs once per chain, right after the connection is established.
type HeightRegistrar func(registry *mapprotocol.Registry, id msg.ChainId, conn core.Connection, lightNode string) error

// DefaultProofDialer dials an EVM endpoint the way the proof API needs it:
// over HTTP, without a keypair, with the package gas defaults.
func DefaultProofDialer(endpoint string) core.Connection {
	return connection.NewConnection(endpoint, true, nil, nil, big.NewInt(DefaultGasLimit),
		big.NewInt(DefaultGasPrice), DefaultGasMultiplier)
}

// ProofConnector serves the Proffer.Connect half of the expose proof API: it opens
// the connection a proof request needs and registers that chain's contract callers.
//
// It replaces five near-identical Connect bodies, each of which wrapped its
// registration in a sync.OnceFunc that was allocated inside the call and therefore
// never actually deduplicated anything. Every proof request re-ran the
// registration (writing shared state from an HTTP handler goroutine) and leaked a
// fresh connection. Here the connection is cached per chain and endpoint, so
// registration happens once and later requests reuse the client.
type ProofConnector struct {
	mu    sync.Mutex
	conns map[string]core.Connection

	registry *mapprotocol.Registry
	dial     Dialer
	register HeightRegistrar
}

// NewProofConnector returns a connector that dials with the given dialer and
// publishes what it opens into registry. When register is nil the connector
// installs the standard "ask the peer's light node how far it followed MAP"
// provider.
func NewProofConnector(registry *mapprotocol.Registry, dial Dialer, register HeightRegistrar) *ProofConnector {
	return &ProofConnector{
		conns:    make(map[string]core.Connection),
		registry: registry,
		dial:     dial,
		register: register,
	}
}

// Connect returns a client for the chain, opening and registering it on first use.
func (p *ProofConnector) Connect(id, endpoint, mcs, lightNode, oracleNode string) (*ethclient.Client, error) {
	chainID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "proof connect: invalid chain id %q", id)
	}

	key := id + "|" + endpoint
	p.mu.Lock()
	defer p.mu.Unlock()
	if conn, ok := p.conns[key]; ok {
		return conn.Client(), nil
	}

	conn := p.dial(endpoint)
	if err := conn.Connect(); err != nil {
		return nil, err
	}

	if err := p.registerLocked(msg.ChainId(chainID), conn, mcs, lightNode, oracleNode); err != nil {
		conn.Close()
		return nil, err
	}

	p.conns[key] = conn
	return conn.Client(), nil
}

// registerLocked publishes the chain's contract callers and height provider.
func (p *ProofConnector) registerLocked(id msg.ChainId, conn core.Connection, mcs, lightNode, oracleNode string) error {
	oracleAbi, err := abi.New(mapprotocol.OracleAbiJson)
	if err != nil {
		return errors.Wrap(err, "proof connect: parse mos abi")
	}
	signerAbi, err := abi.New(mapprotocol.SignerJson)
	if err != nil {
		return errors.Wrap(err, "proof connect: parse signer abi")
	}

	registry := p.registry
	registry.SetMosCall(id, contract.New(conn, []common.Address{common.HexToAddress(mcs)}, oracleAbi))
	registry.SetSignerCall(id, contract.New(conn, []common.Address{common.HexToAddress(oracleNode)}, signerAbi))

	if p.register != nil {
		return p.register(registry, id, conn, lightNode)
	}
	registry.SetMap2OtherHeight(id, mapprotocol.Map2EthHeight(constant.ZeroAddress.Hex(),
		common.HexToAddress(lightNode), conn.Client()))
	return nil
}
