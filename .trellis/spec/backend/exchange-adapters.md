# Exchange Adapter Contracts

> How the trading path talks to exchange adapters when an order's risk depends on an exchange-side setting.
> Layers: `trader/` (trading path) ↔ `trader/<exchange>/` (adapters). Shared types live in `trader/types`.

---

## Scenario: Applying margin mode and leverage before an order (Gate)

### 1. Scope / Trigger

An order's risk depends on an exchange-side setting (margin mode, leverage). The setting has to be in effect
before the order is placed. Without an explicit setting, Gate applies its own default to a contract opened for
the first time: on the trading sub-account this was cross margin at the contract's maximum leverage
(DOGE_USDT 75x, ETH_USDT 200x; observed 2026-11-24).

### 2. Signatures

```go
// trader/types/interface.go
type MarginSettingsApplier interface {
    ApplyMarginSettings(symbol string, isCrossMargin bool, leverage int) error
}

type ProtectedOpenRequest struct {
    // ...
    Leverage      int
    IsCrossMargin bool // cross = true, isolated = false; applied together with Leverage
}

// trader/gate/margin_settings.go
func (t *GateTrader) ApplyMarginSettings(symbol string, isCrossMargin bool, leverage int) error
func (t *GateTrader) PositionMarginSettings(symbol string) (MarginSettings, error)
var ErrNoPositionSettings = errors.New("no position settings for contract")
```

Exchange call: `POST /futures/{settle}/positions/{contract}/set_leverage?leverage={n}&margin_mode={cross|isolated}`
without `dual_side` (the account trades in one-way position mode). It returns a `Position`.

### 3. Contracts

- `IsCrossMargin` comes from the trader's UI setting (`AutoTraderConfig.IsCrossMargin`); `Leverage` comes from
  the decision or the grid config.
- **Success means a read-back equals the target.** The `set_leverage` response is checked first. If the call
  errors, or the response cannot be read or differs from the target, the position entry
  (`GET /futures/{settle}/positions/{contract}`) is checked instead. If the entry equals the target, the call
  succeeds. Otherwise it fails, and the returned error wraps the exchange error, so callers can inspect the
  label with `errors.As` → `GateAPIError`. That holds when the exchange answered with a labelled JSON error;
  a transport error is wrapped as whatever the SDK returned.
- `MarginSettings{Cross, Leverage}` is parsed from a position entry:
  - mode from `pos_margin_mode` (`cross` / `isolated`);
  - leverage has two sources: `lever` and the legacy field of the same mode (`cross_leverage_limit` for cross,
    `leverage` for isolated), each read as an exact decimal that must be a positive integer. When only one is
    readable, it is used. When both are readable they must be equal; a difference is a contradictory read-back
    and is an error. Do not pick one of them.
  - Only the same-mode legacy field takes part: `cross_leverage_limit` keeps the last cross value while the
    contract is isolated, and `leverage` is `0` while it is cross.
- `Position.Mode`, when present, must be `single`.
- An adapter that implements `ProtectedOpener` applies the request's margin settings before submitting the order.
- The trading path does not call `SetMarginMode` on its own: an adapter that supports the one-shot apply gets a
  single call with both values.

### 4. Validation & Error Matrix

| Condition | Result |
|---|---|
| `leverage < 1` | error, no request sent |
| Exchange rejects the call (e.g. `LEVERAGE_EXCEEDED`) and the entry does not already match | error; the caller must not place the order |
| Response is unreadable or differs from the target; the entry equals the target | success |
| Response and entry both differ from the target or cannot be read (unknown mode, leverage not a positive integer, contradictory leverage) | error |
| The call errors; the entry equals the target (e.g. the request timed out after it was applied) | success |
| The call errors; the entry differs or cannot be read | error, with the exchange error in the chain |
| `GET positions/{contract}` answers `POSITION_NOT_FOUND` (HTTP 400 on Gate) | `ErrNoPositionSettings` |
| `GET positions/{contract}` fails with any other label | that error is returned as is; it is never treated as "no entry" |
| Entry `mode` is present and is not `single` | error |

Not verified against the real API: `set_leverage` on a contract that has no position entry yet (the first
open of a new contract), changing the setting while a position or pending orders exist, and accounts in
single-currency margin mode. The adapter has no branch for these situations. Only the missing-entry case is
covered by a unit test with a mock exchange.

`SetLeverage(symbol, leverage)` on Gate keeps the contract's current mode and changes only the leverage;
a contract with no position entry is set to isolated. `SetMarginMode(symbol, isCross)` keeps the current
leverage and returns an error wrapping `ErrNoPositionSettings` when there is no entry to read it from.

### 5. Good/Base/Bad Cases

- Good: cross 10x requested; the response shows `pos_margin_mode=cross`, `cross_leverage_limit=10`, `lever=10`;
  the order is placed.
- Base: the same setting is requested again; the exchange answers 200; success.
- Bad: 100x on a contract whose maximum is 75; the exchange answers `LEVERAGE_EXCEEDED` (`limit [1, 75]`);
  the call returns an error, no order is placed, the entry is unchanged.

### 6. Tests Required

- `trader/gate/margin_settings_test.go`: request path and query per mode, every branch of the
  fallback-to-entry table (response error / unreadable / mismatch, entry equal or not), the exchange error
  being in the error chain, `POSITION_NOT_FOUND` being the only label that means "no entry", the parse matrix
  including contradictory leverage, and the keep-semantics of the two legacy methods.
  Secondary error paths are pinned too: the exchange error stays in the chain both when the read-back fails
  and when reading the entry fails, and a parse error stays in the chain.
- `trader/gate/protected_open_test.go`: the settings request precedes the order request; a failed setting
  leaves no order body.
- `trader/auto_trader_orders_test.go`, `trader/auto_trader_grid_margin_test.go`: the trader's setting reaches
  the request; a failed setting leaves the grid uninitialized.
- Checks against the real API live in the owning task's `probe/` directory, never in the unit test suite.

### 7. Wrong vs Correct

#### Wrong

```go
// The order goes out with whatever setting the exchange already has.
if err := t.SetLeverage(symbol, leverage); err != nil {
    logger.Warnf("leverage not applied: %v", err)
}
result, _, err := t.client.FuturesApi.CreateFuturesOrder(ctx, "usdt", order, nil)
```

#### Correct

```go
if err := t.ApplyMarginSettings(symbol, req.IsCrossMargin, req.Leverage); err != nil {
    return types.ProtectedOpenResult{}, fmt.Errorf("refusing to open %s: %w", symbol, err)
}
result, _, err := t.client.FuturesApi.CreateFuturesOrder(ctx, "usdt", order, nil)
```
