// Copyright 2021 Compass Systems
// SPDX-License-Identifier: LGPL-3.0-only

package mapprotocol

import (
	"math/big"
	"sync"

	"github.com/mapprotocol/compass/pkg/contract"
	"github.com/mapprotocol/compass/pkg/ethclient"
	"github.com/mapprotocol/compass/pkg/msg"
	"github.com/pkg/errors"
)

// ErrNotRegistered is returned when a chain was never registered for the role the
// caller needs. It replaces the old "missing map key" behaviour, which either
// panicked or silently produced a nil height.
var ErrNotRegistered = errors.New("chain is not registered")

// Registry owns the cross-chain runtime state that a relay process discovers while
// it assembles chains: the MAP connection, the per-chain contract callers, the
// height providers and the light-client manager.
//
// It replaces the package level maps this state used to live in. Every accessor
// takes the lock, because the state is written during chain construction and by
// the expose proof API (one goroutine per HTTP request) while the
// maintainer/messenger/oracle loops read it.
type Registry struct {
	mu              sync.RWMutex
	mapID           string
	mapConn         *ethclient.Client
	chainNames      map[msg.ChainId]string
	map2OtherInit   map[msg.ChainId]*big.Int
	map2OtherHeight map[msg.ChainId]GetHeight
	mosCall         map[msg.ChainId]*contract.Call
	lightNodeCall   map[msg.ChainId]*contract.Call
	signerCall      map[msg.ChainId]*contract.Call
	mosAddress      map[msg.ChainId]string
	lightManager    *LightManager
}

// NewRegistry returns an empty registry. Tests build their own instead of reaching
// for Default, which is what keeps them isolated from each other.
func NewRegistry() *Registry {
	return &Registry{
		chainNames:      make(map[msg.ChainId]string),
		map2OtherInit:   make(map[msg.ChainId]*big.Int),
		map2OtherHeight: make(map[msg.ChainId]GetHeight),
		mosCall:         make(map[msg.ChainId]*contract.Call),
		lightNodeCall:   make(map[msg.ChainId]*contract.Call),
		signerCall:      make(map[msg.ChainId]*contract.Call),
		mosAddress:      make(map[msg.ChainId]string),
	}
}

// defaultRegistry is the process wide registry the composition root fills in.
// It stays a singleton for now so this change keeps the current call graph; the
// point of routing everything through a *Registry is that the next step can pass
// one in explicitly.
var defaultRegistry = NewRegistry()

// Default returns the process wide registry.
func Default() *Registry {
	return defaultRegistry
}

// SetMapID records the configured MAP relay chain id.
func (r *Registry) SetMapID(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mapID = id
}

// MapID returns the configured MAP relay chain id.
func (r *Registry) MapID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mapID
}

// SetMapConn records the client used for every MAP chain contract read.
func (r *Registry) SetMapConn(client *ethclient.Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mapConn = client
}

// MapConn returns the MAP chain client, or nil when no MAP chain was built yet.
func (r *Registry) MapConn() *ethclient.Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mapConn
}

// SetChainName records the human readable name of an online chain.
func (r *Registry) SetChainName(id msg.ChainId, name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chainNames[id] = name
}

// ChainName returns the name of an online chain.
func (r *Registry) ChainName(id msg.ChainId) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	name, ok := r.chainNames[id]
	return name, ok
}

// ChainNameOf returns the name of an online chain, or an empty string when the
// chain is unknown. Alarm and log messages use it, where a missing name is not an
// error worth handling.
func (r *Registry) ChainNameOf(id msg.ChainId) string {
	name, _ := r.ChainName(id)
	return name
}

// SetMosCall registers the MOS contract caller of a chain (messenger role).
func (r *Registry) SetMosCall(id msg.ChainId, call *contract.Call) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mosCall[id] = call
}

// MosCall returns the MOS contract caller of a chain.
func (r *Registry) MosCall(id msg.ChainId) (*contract.Call, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	call, ok := r.mosCall[id]
	return call, ok
}

// SetLightNodeCall registers the light node contract caller of a chain (oracle role).
func (r *Registry) SetLightNodeCall(id msg.ChainId, call *contract.Call) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lightNodeCall[id] = call
}

// LightNodeCall returns the light node contract caller of a chain.
func (r *Registry) LightNodeCall(id msg.ChainId) (*contract.Call, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	call, ok := r.lightNodeCall[id]
	return call, ok
}

// SetSignerCall registers the oracle signer contract caller of a chain.
func (r *Registry) SetSignerCall(id msg.ChainId, call *contract.Call) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.signerCall[id] = call
}

// SignerCall returns the oracle signer contract caller of a chain.
func (r *Registry) SignerCall(id msg.ChainId) (*contract.Call, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	call, ok := r.signerCall[id]
	return call, ok
}

// SetMosAddress records the MOS contract address of a chain.
func (r *Registry) SetMosAddress(id msg.ChainId, addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mosAddress[id] = addr
}

// MosAddress returns the MOS contract address of a chain.
func (r *Registry) MosAddress(id msg.ChainId) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	addr, ok := r.mosAddress[id]
	return addr, ok
}

// SetMap2OtherInitHeight records the height a chain had synced from MAP when the
// maintainer for that chain started.
func (r *Registry) SetMap2OtherInitHeight(id msg.ChainId, height *big.Int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.map2OtherInit[id] = height
}

// Map2OtherInitHeights returns a snapshot of the recorded start heights. Callers
// iterate the copy, so a chain registering itself concurrently cannot corrupt it.
func (r *Registry) Map2OtherInitHeights() map[msg.ChainId]*big.Int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[msg.ChainId]*big.Int, len(r.map2OtherInit))
	for id, height := range r.map2OtherInit {
		out[id] = height
	}
	return out
}

// Map2OtherInitHeight returns the recorded start height of a chain.
func (r *Registry) Map2OtherInitHeight(id msg.ChainId) (*big.Int, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	height, ok := r.map2OtherInit[id]
	return height, ok
}

// SetMap2OtherHeight registers the provider that reports how far a chain's light
// node has followed the MAP chain.
func (r *Registry) SetMap2OtherHeight(id msg.ChainId, fn GetHeight) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.map2OtherHeight[id] = fn
}

// Map2OtherHeight returns the height provider registered for a chain.
func (r *Registry) Map2OtherHeight(id msg.ChainId) (GetHeight, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.map2OtherHeight[id]
	return fn, ok
}

// Map2OtherSyncedHeight asks a chain's light node how far it followed MAP. It
// reports ErrNotRegistered instead of panicking when the chain was never
// registered, which is what indexing the old map did.
func (r *Registry) Map2OtherSyncedHeight(id msg.ChainId) (*big.Int, error) {
	fn, ok := r.Map2OtherHeight(id)
	if !ok {
		return nil, errors.Wrapf(ErrNotRegistered, "map2other height provider for chain %d", id)
	}
	return fn()
}

// SetLightManager records the MAP light-client manager reader.
func (r *Registry) SetLightManager(lm *LightManager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lightManager = lm
}

// LightManager returns the MAP light-client manager reader.
func (r *Registry) LightManager() (*LightManager, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.lightManager == nil {
		return nil, errors.New("map light manager is not initialized")
	}
	return r.lightManager, nil
}

// Chain2MapHeight reports how far the MAP chain has followed the given chain.
func (r *Registry) Chain2MapHeight(id msg.ChainId) (*big.Int, error) {
	lm, err := r.LightManager()
	if err != nil {
		return nil, err
	}
	return lm.HeaderHeight(id)
}

// Eth2MapNumber reports the block range the MAP chain expects next from an eth2
// style chain.
func (r *Registry) Eth2MapNumber(id msg.ChainId) (*big.Int, *big.Int, error) {
	lm, err := r.LightManager()
	if err != nil {
		return nil, nil, err
	}
	return lm.Eth2MapNumber(id)
}

// LightNodeType reports the light node type the manager holds for a chain.
func (r *Registry) LightNodeType(method string, params ...interface{}) (*big.Int, error) {
	lm, err := r.LightManager()
	if err != nil {
		return nil, err
	}
	return lm.NodeType(method, params...)
}

// LightManagerData performs a raw read against the light-client manager.
func (r *Registry) LightManagerData(method string, params ...interface{}) ([]byte, error) {
	lm, err := r.LightManager()
	if err != nil {
		return nil, err
	}
	return lm.Data(method, params...)
}
