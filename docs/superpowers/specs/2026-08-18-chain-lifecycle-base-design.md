# Chain Lifecycle Base Design

## Scope

Implement WORK-18 P0-2 as one behavior-preserving refactor. Consolidate the
duplicated `SetRouter`, `Start`, `Id`, `Name`, `Stop`, and `Conn` methods used by
the generic, Eth2, Solana, and Tron chains. Replace the unchecked concrete type
assertion used for the MAP chain with an explicit optional capability.

This change does not alter transaction construction, proof encoding, chain
configuration, RPC behavior, or the global protocol registry. The other WORK-18
recommendations remain separate changes.

## Design

Add `internal/chain.Base`, constructed by `NewBase`. It owns the five values
shared by every chain lifecycle: chain config, connection, writer, listener,
and stop channel. Its methods preserve the current behavior and are promoted to
each chain through struct embedding.

`Base` deliberately does not expose `EthClient`. Add `core.EthClientProvider`
as an optional capability implemented only by chain types that can supply the
MAP EVM client. The command layer resolves this capability through a small
helper and returns a contextual error when it is missing. The validator setup
continues to use the existing `core.Chain.Conn()` method, so no concrete chain
type crosses the `core` boundary.

## Error Handling

The command must fail with a normal error if the configured first/MAP chain
does not implement `core.EthClientProvider`; it must not panic. Listener sync
errors are returned unchanged, matching current chain behavior. `Stop` closes
the existing stop channel and closes a non-nil connection, also matching the
current behavior.

## Testing

Add focused tests for `Base` that verify router/listener wiring, listener error
propagation, identity accessors, stop channel closure, and connection closure.
Add command tests for both supported and unsupported `EthClientProvider`
chains. Run targeted package tests, `make build`, and the repository test suite;
known unrelated failures must be reported separately.
