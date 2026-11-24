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

---

## Scenario: Reading the account balance (principal, unrealized PnL, available, equity)

### 1. Scope / Trigger

Any code that needs the account's principal, unrealized PnL, available margin or equity: the trading context built every
cycle, the account info API, the position-value cap before an order, the grid drawdown check and grid context, trader
creation and the initial-balance rebuild, balance sync, and the exchange account state shown on the settings page.

On Gate the trading account is a unified account. `GET /futures/{settle}/accounts` reports `total` as 0 there (the docs
say `total` only applies to classic futures accounts), so reading `total` as the wallet balance made equity 0, the
position-value cap 0, and stored an initial balance of 0 for every new trader.

### 2. Signatures

```go
// trader/types/balance.go
const (
    BalanceKeyPrincipal     = "totalWalletBalance"
    BalanceKeyUnrealizedPnL = "totalUnrealizedProfit"
    BalanceKeyAvailable     = "availableBalance"
)

type AccountBalance struct {
    Principal     float64 // excludes unrealized PnL, includes realized PnL
    UnrealizedPnL float64
    Available     float64 // margin available for futures
    Equity        float64
}
func (b AccountBalance) InitialBalance() float64 // the principal
func ParseAccountBalance(m map[string]interface{}) (AccountBalance, error)
func ReadAccountBalance(t Trader) (AccountBalance, error)
func ReadInitialBalance(t Trader) (float64, error)
```

Exchange calls on Gate:

- `GET /wallet/total_balance?currency=USDT` — principal = `total.amount − total.borrowed`, unrealized PnL = `total.unrealised_pnl`
- `GET /futures/{settle}/accounts` — `available`

### 3. Contracts

- Adapters keep returning a map from `GetBalance()` with the three keys above. What the keys mean is defined once, in
  `trader/types/balance.go`.
- Consumers do not read those keys themselves. They call `types.ReadAccountBalance`, or `ParseAccountBalance` when they
  must tell a failed request from an unreadable reading (the exchange account state classifies the two differently).
- Equity is the first positive value among `totalEquity` and `total_equity`; when the adapter reports neither, it is
  `Principal + UnrealizedPnL`. Gate reports neither key, so its equity is the sum.
- The initial balance is the principal (`InitialBalance()`), never the equity.
- Gate's `GetBalance` returns exactly the three keys, does not report equity, and keeps its 15-second cache.
- `total.borrowed`, `total.unrealised_pnl` and `available` may be omitted by Gate when they are 0; an omitted value reads
  as 0. A value that is present but malformed is an error.
- `total.amount` is required. An omitted or malformed value is an error, never 0: on a unified account it is the only
  source of the principal.

### 4. Validation & Error Matrix

| Condition | Result |
|---|---|
| Any of the three keys is missing or is not a finite `float64` | `ParseAccountBalance` errors and names the key; nothing is treated as 0 |
| The exchange request fails | `ReadAccountBalance` errors; the exchange error stays in the error chain |
| Gate: `total.amount` missing, empty, malformed, or in scientific notation | error; no result is returned and nothing is cached |
| Gate: `total.currency` is present and is not `USDT` | error |
| Trading context, account info, or the pre-order check cannot read the balance | the cycle, the request, or the order fails. There is no fallback to another balance field |
| Grid drawdown check or grid context cannot read the balance | unchanged fail-soft behavior: the check is skipped with a warning; the context keeps zero account fields |
| Stored initial balance ≤ 0 at load | rebuilt with `ReadInitialBalance`, persisted, logged as a warning |
| Stored initial balance ≤ 0 at load and the principal cannot be read or is not positive | the trader does not load |
| `PUT /traders/:id` with a negative `initial_balance` | 400. `0` or an absent field keeps the stored value |

Not verified against the real API: accounts with borrowing (`total.borrowed` ≠ 0) and accounts in single-currency margin
mode. The trading sub-account holds USDT only, with no borrowing.

### 5. Good/Base/Bad Cases

- Good: Gate unified account, flat — `total.amount = 107.49769421`, `borrowed = 0`, `unrealised_pnl = 0`, futures
  `available = 107.49769` → principal 107.49769421, equity 107.49769421; a trader created now gets initial balance 107.49769421.
- Base: the same account with an open position — the principal does not move with the position; unrealized PnL and
  available follow it; equity = principal + unrealized PnL.
- Bad: the wallet request fails while a trader with a stored initial balance of 0 is loaded — the trader does not load,
  and the error names the invalid value and carries the exchange error.

### 6. Tests Required

- `trader/types/balance_test.go`: the parse matrix (both equity keys, derived equity, each key missing or non-numeric)
  and `InitialBalance()` returning the principal.
- `trader/gate/trader_account_test.go`: `httptest` replies in Gate's real response format — borrowing subtracted as exact
  decimals, each unreadable field, a non-USDT currency, request failures, and the cache.
- `trader/auto_trader_account_test.go`: the initial balance kept, rebuilt, persisted, and failing closed; the trading
  context, account info, and position-value cap reading the parsed balance.
- `api/handler_trader_initial_balance_test.go`, `agent/trader_initial_balance_test.go`: creation, update, sync, the
  exchange account state, and the assistant's fetcher, with a stubbed probe adapter.
- Readings against the real API live in the owning task's `probe/` directory, never in the unit test suite.

### 7. Wrong vs Correct

#### Wrong

```go
// A second copy of the key list and the equity formula; a missing key silently becomes 0.
balance, _ := at.trader.GetBalance()
equity := 0.0
if eq, ok := balance["totalEquity"].(float64); ok && eq > 0 {
    equity = eq
} else if eq, ok := balance["totalWalletBalance"].(float64); ok && eq > 0 {
    equity = eq
}
```

#### Correct

```go
account, err := types.ReadAccountBalance(at.trader)
if err != nil {
    return nil, err
}
equity := account.Equity
```
