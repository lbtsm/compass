# Chain Lifecycle Base Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Consolidate duplicated chain lifecycle methods and replace the MAP chain concrete assertion with a checked optional capability.

**Architecture:** An embedded `internal/chain.Base` owns common lifecycle dependencies and behavior. A narrow `core.EthClientProvider` interface represents the optional MAP EVM client capability, while all other chain-specific behavior remains on concrete chain types.

**Tech Stack:** Go 1.26.3, standard `testing`, existing Compass core interfaces.

---

### Task 1: Specify Common Lifecycle Behavior

**Files:**
- Create: `internal/chain/base_test.go`
- Create: `internal/chain/base.go`

- [ ] **Step 1: Write failing lifecycle tests**

Create fakes for `core.Connection`, `core.Writer`, `core.Listener`, and
`core.Router`. Exercise `NewBase` through `SetRouter`, `Start`, `Id`, `Name`,
`Conn`, and `Stop`. Assert that a listener error is returned unchanged and that
stopping closes both the stop channel and connection.

- [ ] **Step 2: Verify the tests fail because `Base` is missing**

Run: `go test ./internal/chain -run TestBase -count=1`

Expected: build failure containing `undefined: NewBase`.

- [ ] **Step 3: Implement the minimal lifecycle base**

```go
type Base struct {
	cfg    *core.ChainConfig
	conn   core.Connection
	writer core.Writer
	listen core.Listener
	stop   chan<- int
}

func NewBase(cfg *core.ChainConfig, conn core.Connection, writer core.Writer,
	listen core.Listener, stop chan<- int) Base {
	return Base{cfg: cfg, conn: conn, writer: writer, listen: listen, stop: stop}
}
```

Implement the six existing lifecycle methods without changing their control
flow or log message.

- [ ] **Step 4: Verify the lifecycle tests pass**

Run: `go test ./internal/chain -run TestBase -count=1`

Expected: `ok github.com/mapprotocol/compass/internal/chain`.

### Task 2: Replace the Unchecked MAP Chain Assertion

**Files:**
- Modify: `core/chain.go`
- Modify: `cmd/compass/main.go`
- Create: `cmd/compass/main_chain_test.go`

- [ ] **Step 1: Write failing capability tests**

Define a fake `core.Chain` and a fake that additionally provides
`EthClient() *ethclient.Client`. Assert that `ethClientFromChain` returns the
client for the provider and returns an error containing `does not provide an
eth client` for the plain chain.

- [ ] **Step 2: Verify the tests fail because the helper is missing**

Run: `go test ./cmd/compass -run TestEthClientFromChain -count=1`

Expected: build failure containing `undefined: ethClientFromChain`.

- [ ] **Step 3: Add the optional capability and checked helper**

```go
type EthClientProvider interface {
	EthClient() *ethclient.Client
}

func ethClientFromChain(c core.Chain) (*ethclient.Client, error) {
	provider, ok := c.(core.EthClientProvider)
	if !ok {
		return nil, fmt.Errorf("chain %q does not provide an eth client", c.Name())
	}
	return provider.EthClient(), nil
}
```

Use the helper when initializing `mapprotocol.GlobalMapConn`, and use
`newChain.Conn()` for validator construction.

- [ ] **Step 4: Verify capability tests pass**

Run: `go test ./cmd/compass -run TestEthClientFromChain -count=1`

Expected: `ok github.com/mapprotocol/compass/cmd/compass`.

### Task 3: Embed Base in All Four Chain Implementations

**Files:**
- Modify: `internal/chain/chain.go`
- Modify: `chains/eth2/chain.go`
- Modify: `chains/sol/chain.go`
- Modify: `chains/tron/chain.go`

- [ ] **Step 1: Replace duplicated fields with embedded `Base`**

Use `type Chain struct { chain.Base }` outside `internal/chain` and
`type Chain struct { Base }` inside it. Construct each value with
`chain.NewBase(chainCfg, conn, writer, listen, stop)` (or `NewBase` in-package).

- [ ] **Step 2: Delete promoted lifecycle duplicates**

Remove each concrete chain's `SetRouter`, `Start`, `Id`, `Name`, `Stop`, and
`Conn` methods. Keep `EthClient` only on the generic EVM and Eth2 chain types,
implemented through `c.Conn().Client()`.

- [ ] **Step 3: Format and run focused tests**

Run: `gofmt -w internal/chain/base.go internal/chain/base_test.go internal/chain/chain.go core/chain.go cmd/compass/main.go cmd/compass/main_chain_test.go chains/eth2/chain.go chains/sol/chain.go chains/tron/chain.go`

Run: `go test ./internal/chain ./cmd/compass -count=1`

Expected: both packages pass.

### Task 4: Verify and Deliver

**Files:**
- Verify all modified files.

- [ ] **Step 1: Run the build**

Run: `make build`

Expected: exit code 0 and `build/compass` produced.

- [ ] **Step 2: Run repository tests**

Run: `go test ./...`

Expected: all changed-package tests pass. Record separately any known baseline
failures in `chains/sol`, `core`, `config`, or `tests`.

- [ ] **Step 3: Inspect the final diff**

Run: `git diff --check && git status --short && git diff --stat`

Expected: no whitespace errors and only WORK-18 files changed.

- [ ] **Step 4: Commit, push, and open the linked PR**

```bash
git add docs/superpowers/specs/2026-08-18-chain-lifecycle-base-design.md \
  docs/superpowers/plans/2026-08-18-chain-lifecycle-base.md \
  internal/chain/base.go internal/chain/base_test.go internal/chain/chain.go \
  core/chain.go cmd/compass/main.go cmd/compass/main_chain_test.go \
  chains/eth2/chain.go chains/sol/chain.go chains/tron/chain.go
git commit -m "refactor: consolidate chain lifecycle"
git push -u origin agent/codex-reviewrer/work-18-chain-base
gh pr create --title "WORK-18: consolidate chain lifecycle" --body-file ./pr-body.md
```

The PR body must summarize behavior, tests, and contain `Closes WORK-18` only if
merging this first slice is intended to close the whole issue. Otherwise use the
title/branch link only.
