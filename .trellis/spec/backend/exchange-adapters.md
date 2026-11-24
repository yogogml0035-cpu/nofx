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
  errors or the response cannot be read, the position entry (`GET /futures/{settle}/positions/{contract}`) is
  checked instead. If neither equals the target, the call fails.
- `MarginSettings{Cross, Leverage}` is parsed from a position entry:
  - mode from `pos_margin_mode` (`cross` / `isolated`);
  - leverage from `lever` (must be a positive integer). Only when `lever` is unusable, fall back to the mode's
    legacy field: `cross_leverage_limit` for cross, `leverage` for isolated.
  - `cross_leverage_limit` keeps the last cross value while the contract is isolated, so it must not be read as
    the leverage of an isolated position.
- `Position.Mode`, when present, must be `single`.
- An adapter that implements `ProtectedOpener` applies the request's margin settings before submitting the order.
- The trading path does not call `SetMarginMode` on its own: an adapter that supports the one-shot apply gets a
  single call with both values.

### 4. Validation & Error Matrix

| Condition | Result |
|---|---|
| `leverage < 1` | error, no request sent |
| Exchange rejects the call (e.g. `LEVERAGE_EXCEEDED`) and the entry does not already match | error; the caller must not place the order |
| Response or entry is unreadable (unknown mode, leverage not a positive integer) | error |
| Response and entry differ from the target (e.g. the exchange clamped the value) | error |
| The call errors but the entry already equals the target (e.g. the request timed out after it was applied) | success |
| `GET positions/{contract}` answers `POSITION_NOT_FOUND` (HTTP 400 on Gate) | `ErrNoPositionSettings` |
| Entry `mode` is present and is not `single` | error |

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

- `trader/gate/margin_settings_test.go`: request path and query per mode, read-back mismatch, exchange
  rejection, repeated setting, the parse matrix, and the keep-semantics of the two legacy methods.
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
