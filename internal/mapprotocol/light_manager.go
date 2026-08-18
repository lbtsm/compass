// Copyright 2021 Compass Systems
// SPDX-License-Identifier: LGPL-3.0-only

package mapprotocol

import (
	"context"
	"math/big"

	goeth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/mapprotocol/compass/internal/constant"
	"github.com/mapprotocol/compass/pkg/msg"
	"github.com/pkg/errors"
)

// ContractCaller is the only thing the light manager needs from a chain client.
// Keeping it this small is what lets the reads below be unit tested without an RPC
// endpoint.
type ContractCaller interface {
	CallContract(ctx context.Context, call goeth.CallMsg, blockNumber *big.Int) ([]byte, error)
}

// LightManager reads the MAP light-client manager contract.
//
// It replaces four package level function variables that were each installed by an
// Init* call at startup: the connection and the contract address are now fields of
// one object instead of values captured in closures that other goroutines could
// swap out mid-flight.
type LightManager struct {
	caller ContractCaller
	addr   common.Address
}

// NewLightManager binds a light manager reader to a client and contract address.
func NewLightManager(caller ContractCaller, addr common.Address) *LightManager {
	return &LightManager{caller: caller, addr: addr}
}

// Data performs a raw read and returns the manager's bytes output.
func (l *LightManager) Data(method string, params ...interface{}) ([]byte, error) {
	output, err := l.call(method, params...)
	if err != nil {
		return nil, err
	}

	outputs := LightManger.Methods[method].Outputs
	unpack, err := outputs.Unpack(output)
	if err != nil {
		return nil, err
	}
	ret := make([]byte, 0)
	if err = outputs.Copy(&ret, unpack); err != nil {
		return nil, err
	}
	return ret, nil
}

// NodeType returns the light node type the manager holds for a chain.
func (l *LightManager) NodeType(method string, params ...interface{}) (*big.Int, error) {
	output, err := l.call(method, params...)
	if err != nil {
		return nil, err
	}

	outputs := LightManger.Methods[method].Outputs
	unpack, err := outputs.Unpack(output)
	if err != nil {
		return nil, err
	}
	ret := new(big.Int)
	if err = outputs.Copy(&ret, unpack); err != nil {
		return nil, err
	}
	return ret, nil
}

// HeaderHeight reports how far the MAP chain has followed the given chain.
func (l *LightManager) HeaderHeight(chainID msg.ChainId) (*big.Int, error) {
	output, err := l.call(MethodOfHeaderHeight, big.NewInt(int64(chainID)))
	if err != nil {
		return nil, errors.Wrap(err, "get other2map headerHeight by lightManager failed")
	}
	return UnpackHeaderHeightOutput(output)
}

// Eth2MapNumber reports the block range the MAP chain expects next from an eth2
// style chain.
func (l *LightManager) Eth2MapNumber(chainID msg.ChainId) (*big.Int, *big.Int, error) {
	output, err := l.call(MethodClientState, big.NewInt(int64(chainID)))
	if err != nil {
		return nil, nil, err
	}

	outputs := LightManger.Methods[MethodClientState].Outputs
	unpack, err := outputs.Unpack(output)
	if err != nil {
		return nil, nil, err
	}

	back := make([]byte, 0)
	if err = outputs.Copy(&back, unpack); err != nil {
		return nil, nil, err
	}

	ret := struct {
		StartNumber *big.Int
		EndNumber   *big.Int
	}{}
	analysis, err := Eth2.Methods[MethodClientStateAnalysis].Outputs.Unpack(back)
	if err != nil {
		return nil, nil, errors.Wrap(err, "analysis")
	}
	if err = Eth2.Methods[MethodClientStateAnalysis].Outputs.Copy(&ret, analysis); err != nil {
		return nil, nil, errors.Wrap(err, "analysis copy")
	}

	return ret.StartNumber, ret.EndNumber, nil
}

// call packs the arguments for a light manager method and performs the read.
func (l *LightManager) call(method string, params ...interface{}) ([]byte, error) {
	if l.caller == nil {
		return nil, errors.New("map light manager has no client")
	}
	input, err := PackInput(LightManger, method, params...)
	if err != nil {
		return nil, errors.Wrapf(err, "pack lightManager %s input failed", method)
	}
	return l.caller.CallContract(
		context.Background(),
		goeth.CallMsg{From: constant.ZeroAddress, To: &l.addr, Data: input},
		nil,
	)
}
