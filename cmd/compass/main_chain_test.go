package main

import (
	"strings"
	"testing"

	"github.com/mapprotocol/compass/core"
	"github.com/mapprotocol/compass/pkg/ethclient"
)

type mainTestChain struct {
	core.Chain
	name string
}

func (c *mainTestChain) Name() string {
	return c.name
}

type mainTestEthClientChain struct {
	*mainTestChain
	client *ethclient.Client
}

func (c *mainTestEthClientChain) EthClient() *ethclient.Client {
	return c.client
}

func TestEthClientFromChainReturnsProviderClient(t *testing.T) {
	want := new(ethclient.Client)
	chain := &mainTestEthClientChain{
		mainTestChain: &mainTestChain{name: "map"},
		client:        want,
	}

	got, err := ethClientFromChain(chain)
	if err != nil {
		t.Fatalf("ethClientFromChain returned error: %v", err)
	}
	if got != want {
		t.Fatal("ethClientFromChain did not return the provider client")
	}
}

func TestEthClientFromChainRejectsUnsupportedChain(t *testing.T) {
	_, err := ethClientFromChain(&mainTestChain{name: "solana"})
	if err == nil {
		t.Fatal("ethClientFromChain returned nil error for an unsupported chain")
	}
	if !strings.Contains(err.Error(), "solana") || !strings.Contains(err.Error(), "does not provide an eth client") {
		t.Fatalf("ethClientFromChain error lacks context: %v", err)
	}
}

func TestEthClientFromChainRejectsNilProviderClient(t *testing.T) {
	chain := &mainTestEthClientChain{
		mainTestChain: &mainTestChain{name: "map"},
	}

	_, err := ethClientFromChain(chain)
	if err == nil {
		t.Fatal("ethClientFromChain returned nil error for a nil provider client")
	}
	if !strings.Contains(err.Error(), "map") || !strings.Contains(err.Error(), "returned a nil eth client") {
		t.Fatalf("ethClientFromChain error lacks context: %v", err)
	}
}
