// Copyright 2021 Compass Systems
// SPDX-License-Identifier: LGPL-3.0-only

package core

import (
	"fmt"
	"github.com/mapprotocol/compass/internal/mapprotocol"
	"github.com/mapprotocol/compass/pkg/msg"
	"os"
	"os/signal"
	"syscall"

	utilcore "github.com/ChainSafe/chainbridge-utils/core"
	"github.com/ChainSafe/log15"
)

type Core struct {
	Registry []Chain
	route    *Route
	log      log15.Logger
	sysErr   <-chan error
	role     mapprotocol.Role
}

func NewCore(sysErr <-chan error, mapcid msg.ChainId, role mapprotocol.Role) *Core {
	return &Core{
		Registry: make([]Chain, 0),
		route:    NewRouter(log15.New("system", "router"), mapcid),
		log:      log15.New("system", "core"),
		sysErr:   sysErr,
		role:     role,
	}
}

// AddChain registers the chain in the Registry and calls Chain.SetRouter()
func (c *Core) AddChain(chain Chain) {
	c.Registry = append(c.Registry, chain)
	chain.SetRouter(c.route)
}

// Start will call all registered chains' Start methods and block forever (or until signal is received)
func (c *Core) Start() {
	for _, chain := range c.Registry {
		err := chain.Start()
		if err != nil {
			c.log.Error("failed to start chain", "chain", chain.Id(), "err", err)
			return
		}
		c.log.Info(fmt.Sprintf("Started %s chain", chain.Name()))
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigc)

	// Block here and wait for a signal
	select {
	case err := <-c.sysErr:
		c.log.Error("FATAL ERROR. Shutting down.", "err", err)
	case <-sigc:
		c.log.Warn("Interrupt received, shutting down now.")
	}

	// Signal chains to shutdown
	for _, chain := range c.Registry {
		chain.Stop()
	}
}

func (c *Core) Errors() <-chan error {
	return c.sysErr
}

func (c *Core) ToUCoreRegistry() []utilcore.Chain {
	ucRegistry := make([]utilcore.Chain, len(c.Registry))

	for idx, reg := range c.Registry {
		ucRegistry[idx] = reg.(utilcore.Chain)
	}
	return ucRegistry
}
