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

Verified against the real API on 2026-11-26 (W5, `LIVECHECK_PASS`): `set_leverage` on a contract that has
no position entry yet (the first open of a new contract) returns 200, the adapter reads the target back from
the response, and the position entry appears with the same field shape as an existing entry (an isolated
entry keeps the previous cross leverage in `cross_leverage_limit`; `lever` is the current leverage). Evidence:
`.trellis/tasks/11-24-gate-trading-foundation/probe/README.md`.

Not verified against the real API: changing the setting while a position or pending orders exist, and
accounts in single-currency margin mode. The adapter has no branch for these situations.

`SetLeverage(symbol, leverage)` on Gate keeps the contract's current mode and changes only the leverage;
a contract with no position entry is set to isolated. `SetMarginMode(symbol, isCross)` keeps the current
leverage and returns an error wrapping `ErrNoPositionSettings` when there is no entry to read it from.

### 5. Good/Base/Bad Cases

- Good: cross 10x requested; the response shows `pos_margin_mode=cross`, `cross_leverage_limit=10`, `lever=10`;
  the order is placed.
- Base: the same setting is requested again; the exchange answers 200; success.
- Bad: 100x on a contract whose maximum is 75; the exchange answers `LEVERAGE_EXCEEDED` (`limit [1, 75]`);
  the call returns an error, no order is placed, the entry is unchanged.

### 6. Reconciling stale position records

An OPEN record can outlive its exchange position when the closing fills fall outside the sync window
(the trader was stopped for more than 24 hours, or the account produced more than 100 fills in 24 hours).
Such a record must never adopt a later position on the same contract and side, because the trader would
then manage or even market-close a position somebody else opened.

- After every successful position read, `reconcileOpenPositionRecords` runs (never on a failed read):
  - no position on the exchange -> the record is closed;
  - position exists but its entry price differs from the record by more than 0.5% -> the record is closed
    and the position is treated as not the trader's (never manage what cannot be attributed);
  - entry price matches and only the quantity differs (a partial close was missed) -> the record quantity
    is aligned to the exchange (`store.SyncPositionSnapshot`).
- Grid traders do not start the order sync: they do not consume `trader_positions` and their ownership is
  decided by the grid symbol, so syncing would only file the AI trader's fills under the grid trader.

### 7. Tests Required

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

On Gate the trading account is a unified account. `GET /futures/{settle}/accounts` reports a `total` that has nothing to
do with the principal there (it reads about 0; the docs say `total` only applies to classic futures accounts), so reading
`total` as the wallet balance made equity about 0, the position-value cap 0, and stored an initial balance of 0 for every
new trader.

### 2. Signatures

```go
// trader/types/balance.go
const (
    BalanceKeyPrincipal     = "totalWalletBalance"
    BalanceKeyUnrealizedPnL = "totalUnrealizedProfit"
    BalanceKeyAvailable     = "availableBalance"
)

var (
    ErrBalanceUnreadable    error // the exchange request failed, or the reading does not match the contract
    ErrPrincipalNotPositive error // the principal is not above 0, so it cannot be an initial balance
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
func ReadInitialBalance(t Trader) (float64, error) // errors with ErrPrincipalNotPositive when the principal is not above 0
```

Exchange calls on Gate:

- `GET /wallet/total_balance?currency=USDT` — principal = `total.amount − total.borrowed`, unrealized PnL = `total.unrealised_pnl`
- `GET /futures/{settle}/accounts` — `available`

### 3. Contracts

- Adapters keep returning a map from `GetBalance()` with the three keys above. What the keys mean is defined once, in
  `trader/types/balance.go`.
- Consumers do not read those keys themselves. They call `types.ReadAccountBalance`, or `ParseAccountBalance` when they
  must tell a failed request from an unreadable reading (the exchange account state classifies the two differently).
- Equity is the first finite positive value among `totalEquity` and `total_equity`; when the adapter reports neither, it is
  `Principal + UnrealizedPnL`. Gate reports neither key, so its equity is the sum. OKX puts its equity (`totalEq`) in
  `totalWalletBalance` and therefore also reports `totalEquity`; without that key its equity would count unrealized PnL twice.
- The initial balance is the principal (`InitialBalance()`), never the equity. `ReadInitialBalance` is the only way to read
  it for creation, rebuild and sync, so a principal that is not above 0 is rejected everywhere at once.
- `ErrBalanceUnreadable` and `ErrPrincipalNotPositive` are matched with `errors.Is`. Callers that explain a failure to the
  user classify by error type, never by matching the message: the API layer sanitizes error text, and a message that
  contains both `:` and `.` is wiped entirely.
- Gate's `GetBalance` returns exactly the three keys, does not report equity, and keeps its 15-second cache.
- `total.borrowed`, `total.unrealised_pnl` and `available` default to 0 when absent (the SDK marks them optional; in the
  readings taken so far Gate returned `"0"` for zero values, so the default is defensive). A value that is present but
  malformed is an error.
- `total.amount` is required. An omitted or malformed value is an error, never 0: on a unified account it is the only
  source of the principal.
- An initial balance that a trader already has belongs to the exchange account it was recorded for. When the trader is
  rebound to another exchange, it is reset to 0 and rebuilt from the new account's principal on load, unless the request
  gives a new value. The edit form sends `initial_balance` only when the user changed it, for exactly this reason.

### 4. Validation & Error Matrix

| Condition | Result |
|---|---|
| Any of the three keys is missing or is not a finite `float64` | `ParseAccountBalance` errors and names the key; nothing is treated as 0 |
| A reported equity is `0`, NaN or infinite | ignored; equity falls back to `Principal + UnrealizedPnL` |
| The exchange request fails | `ReadAccountBalance` errors with `ErrBalanceUnreadable`; the exchange error stays in the error chain |
| The reading does not match the contract | `ReadAccountBalance` errors with `ErrBalanceUnreadable` |
| The principal is not above 0 | `ReadInitialBalance` errors with `ErrPrincipalNotPositive` |
| Gate: `total.amount` missing, empty, malformed, or in scientific notation | error; no result is returned and nothing is cached |
| Gate: `total.currency` is present and is not `USDT` | error |
| Trading context, account info, or the pre-order check cannot read the balance | the cycle, the request, or the order fails. There is no fallback to another balance field |
| Grid drawdown check or grid context cannot read the balance | unchanged fail-soft behavior: the check is skipped with a warning; the context keeps zero account fields |
| Trader creation: the principal cannot be read or is not positive | creation still succeeds; the initial balance is stored as 0 and rebuilt on load |
| Stored initial balance ≤ 0 or NaN at load | rebuilt with `ReadInitialBalance`, persisted, logged as a warning |
| Stored initial balance ≤ 0 at load and the principal cannot be read or is not positive | the trader does not load; the error carries `ErrBalanceUnreadable` or `ErrPrincipalNotPositive` |
| `PUT /traders/:id` with a negative `initial_balance` | 400 |
| `PUT /traders/:id` with `0`, no field, or the value already stored | the stored value is kept (the same exchange), or reset to 0 (the exchange changed) |
| `PUT /traders/:id` with a new positive value | the new value is stored, whether or not the exchange changed |
| Balance sync: the principal cannot be read or is not positive | 500; the stored value is not touched |

Not verified against the real API: accounts with borrowing (`total.borrowed` ≠ 0) and accounts in single-currency margin
mode. The trading sub-account holds USDT only, with no borrowing.

### 5. Good/Base/Bad Cases

- Good: Gate unified account, flat — `total.amount = 107.49769421`, `borrowed = 0`, `unrealised_pnl = 0`, futures
  `available = 107.49769` → principal 107.49769421, equity 107.49769421; a trader created now gets initial balance 107.49769421.
- Base: the same account with an open position — the principal does not move with the position; unrealized PnL and
  available follow it; equity = principal + unrealized PnL.
- Bad: the wallet request fails while a trader with a stored initial balance of 0 is loaded — the trader does not load,
  the error names the invalid value and carries the exchange error, and the API still tells the user that the account
  balance could not be read.
- Bad: the trader is rebound to another exchange and the form sends the stored initial balance back unchanged — it is
  treated as not given and reset.

### 6. Tests Required

- `trader/types/balance_test.go`: the parse matrix (both equity keys, derived equity, each key missing or non-numeric,
  non-finite reported equity), `InitialBalance()` returning the principal, the typed errors of both read functions, and
  `ReadInitialBalance` rejecting a principal that is not above 0.
- `trader/gate/trader_account_test.go`: `httptest` replies in Gate's real response format — borrowing subtracted as exact
  decimals, each unreadable field, a non-USDT currency, request failures (`errors.As` → `GateAPIError`), the source of the
  unrealized PnL, and the cache.
- `trader/okx/trader_account_test.go`: equity reported separately, not counted twice.
- `trader/auto_trader_account_test.go`: the initial balance kept, rebuilt (0, negative, NaN), persisted, and failing closed
  with a typed error that survives wrapping; the trading context, account info, position-value cap, grid drawdown check and
  grid context reading the parsed balance.
- `api/handler_trader_initial_balance_test.go`, `agent/trader_initial_balance_test.go`: creation, update (including the
  exchange-rebinding reset), sync, the exchange account state, the load-failure classification through wrapping and
  sanitizing, and the assistant's fetcher and rebinding, with a stubbed probe adapter.
- `web/src/components/trader/TraderConfigModal.test.tsx`, `traderUpdateRequest.test.ts`: the field is sent only when
  changed, non-positive values block saving, the accessible error state, and the three languages.
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

#### Wrong

```go
// The reason is matched on text that was sanitized first. A message with both ":" and "." is
// wiped to "", so this branch is never reached and the user only gets the generic hint.
switch {
case strings.Contains(SanitizeError(err, ""), "account principal is not positive"):
    return "trader.reason.exchange_principal_not_positive"
}
```

#### Correct

```go
switch {
case errors.Is(err, types.ErrPrincipalNotPositive):
    return "trader.reason.exchange_principal_not_positive"
case errors.Is(err, types.ErrBalanceUnreadable):
    return "trader.reason.exchange_balance_unavailable"
}
```

---

## Scenario: Partial close by ratio (Gate)

### 1. Scope / Trigger

The model may reduce an existing position by a percentage instead of closing it fully. The ratio is a
decision-contract value; the exchange adapter converts it to the venue's quantity unit and places a
reduce-only order. The conversion must never round up, and an adapter that cannot do it precisely must
refuse the action before sending anything.

### 2. Signatures

```go
// trader/types/interface.go
type PartialCloseStatus struct {
    Supported bool
    Reason    string
    Contracts string
    SizeStep  string
}

type PartialCloseRequest struct {
    Symbol  string
    Side    string  // "long" / "short"
    Percent float64 // (0, 100)
}

type PartialCloseResult struct {
    OrderID   string
    Contracts string
    Remaining string
    FillPrice float64
    Quantity  float64
    Notional  float64
}

type PartialCloser interface {
    PartialCloseStatus(symbol, side string) (PartialCloseStatus, error)
    ClosePartial(req PartialCloseRequest) (PartialCloseResult, error)
}

// trader/gate/partial_close.go
func (t *GateTrader) PartialCloseStatus(symbol, side string) (types.PartialCloseStatus, error)
func (t *GateTrader) ClosePartial(req types.PartialCloseRequest) (types.PartialCloseResult, error)

var _ types.PartialCloser = (*GateTrader)(nil)
```

### 3. Contracts

- `PartialCloseStatus.Supported` is true only when the position's contracts are an exact positive
  multiple of the size step and strictly greater than one step. One-step positions cannot be split:
  any ratio below 100% floors to zero steps.
- `Percent` is in `(0, 100)`. `100` is a full close and must use `CloseLong`/`CloseShort`; the adapter
  rejects it even if called directly.
- Conversion uses the same exact-decimal helpers as opens: `ContractSpec.SizeForCloseRatio` floors
  `floor(positionSteps × percent ÷ 100)`. A result below one step is `ErrCloseBelowStep`.
- The close quantity must not exceed the contract's market order maximum
  (`min(order_size_max, market_order_size_max)`); `ContractSpec.validateMarketOrderSize` enforces it.
- The order is a market `reduce_only` order: long close has negative `size`, short close positive.
  It is not a full close: no `close=true`, no `size=0`.
- The position is read with an uncached exchange query (`findPositionSize`), not the cached positions
  list, so the quantity being reduced is current.
- The trading path asserts `types.PartialCloser` before doing anything. An adapter without the
  capability refuses the decision; it never falls back to a full close or an unprotected partial close.
- Gate's attached take-profit/stop-loss is order-level protection and it is reduce-only. Partial close
  does not cancel or amend it; the attached order keeps its original contract size but `is_reduce_only`
  caps the exit at the remaining position (probe 2026-11-25: 3 → 2 contracts, trigger closed the
  remainder without flipping). Reading and updating protection is the next scenario.

### 4. Validation & Error Matrix

| Condition | Result |
|---|---|
| Adapter does not implement `PartialCloser` | decision refused before any exchange call |
| `percent <= 0`, `percent >= 100`, NaN/Inf | error, no request |
| `percent == 100` | error naming the full-close action |
| No position in that direction | error, no request |
| Position size is not a step multiple | error, no request |
| Position is one step and ratio < 100% | conversion floors to zero → `ErrCloseBelowStep`, no request |
| Close quantity above market order maximum | error, no request |
| Exchange rejects the close | error propagated; no retry and no full-close fallback |

### 5. Good/Base/Bad Cases

- Good: 17 contracts, step 1, 50% → 8 contracts closed, 9 remain.
- Base: 0.2 ETH contracts, step 0.1, 50% → 0.1 closed, 0.1 remains.
- Bad: 17 contracts, 6% → 1 closed, 16 remain; 1% → zero steps, refused.
- Bad: 100% through the partial action → refused; caller must use the full-close action.

### 6. Tests Required

- `trader/gate/partial_close_test.go`: httptest Gate responses for the status matrix (17/1 DOGE,
  0.2/0.1 ETH, non-multiple, missing direction), conversion matrix, payload (`reduce_only`, signed
  size, market price, no `close`), market-order maximum, and every rejection path with zero order
  bodies recorded.
- `kernel/partial_close_test.go`: action parsing, percent boundaries, 100% full-close hint, prompt
  action list/example/marker in both languages.
- `trader/auto_trader_partial_close_test.go`: adapter capability refusal, request routing, recorded
  order fields, position marker for supported/unsupported/read-error, sort priority.
- Real checks live in the owning task's `probe/` directory: 2026-11-25 W2 opened 17 DOGE with
  attached TP/SL, closed 8 partially (9 left), verified protection read-back, rejected 100%/1% with
  no side effects, then fully closed with zero residue (`LIVECHECK_PASS`).

---

## Scenario: Reading and updating protective orders (Gate)

### 1. Scope / Trigger

The model may move a position's stop loss or take profit (`update_stop_loss` / `update_take_profit`).
Every trading cycle also has to show the model the current protection. The exchange's trigger-order
amend endpoint is unusable (new path returns 404, old path rejects API-created trigger orders with
code 1077), so an update is a cancel-and-replace.

### 2. Signatures

```go
// trader/types/interface.go
type ProtectionKind string // "stop_loss" | "take_profit"
type PositionProtection struct {
    StopLoss, TakeProfit float64 // effective trigger prices, 0 = unknown
    StopLossOrders, TakeProfitOrders int
    Known bool
    Reason string
}
type ProtectionUpdateRequest struct {
    Symbol, Side string
    Kind ProtectionKind
    Price, Principal, MaxLossPercent float64
}
type ProtectionUpdateResult struct {
    OrderID string; Kind ProtectionKind
    OldPrice, NewPrice float64
    ReplacedOrderIDs []string
}
type ProtectiveOrderManager interface {
    PositionProtection(symbol, side string) (PositionProtection, error)
    UpdateProtection(req ProtectionUpdateRequest) (ProtectionUpdateResult, error)
}
var ErrPositionUnprotected = errors.New("position is not protected")

// trader/gate
func (t *GateTrader) PositionProtection(symbol, side string) (types.PositionProtection, error)
func (t *GateTrader) UpdateProtection(req types.ProtectionUpdateRequest) (types.ProtectionUpdateResult, error)
func (s ContractSpec) AlignTriggerPrice(price float64, direction PriceAlignDirection) (string, *big.Rat, error)
```

### 3. Contracts

- The current protection is read from `GET /futures/{settle}/price_orders?status=open` and classified by
  `order_type` (`close-long/short-order`, `close-long/short-position`, `plan-close-*`) plus
  `trigger.rule` (1 = price ≥ trigger, 2 = price ≤ trigger). Both SL and TP present → `Known`;
  multiple same-kind orders → the one closest to the market is the effective price, the count is reported.
- All trigger prices are aligned to the contract's `order_price_round` (`ContractSpec.AlignTriggerPrice`):
  long protection rounds up, short protection rounds down. For a stop this only tightens the stop
  (never increases the loss); for a take profit it never moves the target closer. If the aligned price
  crosses the current price the update is refused.
- Stop-loss updates are checked against the execution-time position read:
  `quantity × |entry − stop| ≤ principal × maxLossPercent / 100` (`quantity = |contracts| × quanto_multiplier`).
  A stop on the profit side counts as zero loss.
- Update order: place the new position-level order → verify it in the open list → cancel every old order
  of that kind → verify only the new one remains. On failure cancel the new order and confirm old
  protection remains; if no same-kind protection can be confirmed, restore the old price or return an
  error wrapping `ErrPositionUnprotected` (the trading path closes the position immediately).
- Protection is unreadable when either SL or TP is missing: modifications are refused. The trading path
  marks such positions `Protection: unknown (reason)` in the prompt and the model must not modify them.

### 4. Validation & Error Matrix

| Condition | Result |
|---|---|
| Adapter does not implement `ProtectiveOrderManager` | decision refused before any exchange call |
| No position in that direction | error, no request |
| Current SL or TP missing | error `not readable`, no request |
| Invalid kind / side / non-positive price | error, no request |
| Aligned price on the wrong side of the market | error, no request |
| Loss at the new stop above the cap | error, no request |
| Place-new rejected | error; old protection untouched |
| New order not visible after placement | cancel new, old protection remains, error |
| Cancel-old rejected | cancel new, old protection remains, error |
| Old order still listed after a successful cancel | cancel new, error |
| No same-kind protection can be confirmed and restore fails | error wrapping `ErrPositionUnprotected`; caller closes |

### 5. Good/Base/Bad Cases

- Good: DOGE long 17 contracts, stop 0.48 → 0.55 (entry 0.56, loss 1.7 USDT, principal 100): new stop
  placed, old stop cancelled, take profit untouched.
- Base: take profit 0.65 → 0.62 (toward the market, allowed by D32).
- Bad: stop 0.58 while the market is 0.56 → refused (wrong side); stop 0.50 with principal 10 → refused
  (loss above 2%); update while only a stop-loss order exists → refused (protection unreadable).

### 6. Tests Required

- `trader/gate/contract_test.go`: alignment matrix (up/down, already aligned, tick precision, invalid
  direction, zero/negative, below one tick).
- `trader/gate/protection_management_test.go`: protection read matrix (pair, duplicates, missing kinds,
  short side), update payload (`close-long-position`, market price, `close=true`, rule), and every
  failure path (place rejected, new not visible, cancel rejected, cancel ineffective) with the
  expected restore behaviour and no unintended requests.
- `trader/gate/protected_open_test.go`: open aligns the attached prices; verification requires the
  trigger-order list (order `tpsl_*` fields alone do not pass); a closed position needs no protection.
- `kernel/update_protection_test.go`: validation matrix (position missing, unreadable protection, side,
  2% boundary, field mismatch, symbol variants), prompt action list/example/marker in both languages.
- `trader/auto_trader_protection_test.go`: routing, capability refusal, no-position refusal,
  `ErrPositionUnprotected` → emergency close, context markers for known/unknown/unsupported, sort priority.
- Real checks live in the owning task's `probe/`: 2026-11-25 W2 opened 17 DOGE, partially closed, moved
  the stop and the take profit (each: new order + old cancelled), then fully closed with zero residue;
  ETH 0.1 verified the attached pair is listed and a stop update works.

### 7. Wrong vs Correct

#### Wrong

```go
// Place the new stop first and cancel the old one without checking.
order, _ := BuildPositionProtectiveOrder(symbol, side, kind, price)
_, _ = t.PlaceProtectiveOrder(order)
_, _ = t.CancelProtectiveOrdersByKind(symbol, kind)
```

#### Correct

```go
// Align, place, verify, then cancel the old orders, then verify only the new one remains.
text, value, err := spec.AlignTriggerPrice(price, align)
newID, err := t.PlaceProtectiveOrder(order)
err = t.verifyProtectiveOrder(symbol, side, kind, newID, ratFloat(value))
for _, old := range oldOrders { err = t.CancelProtectiveOrder(old.OrderID()) }
// on any failure: restoreReplacement(...) keeps the position protected
```

## Scenario: Scheduling decisions on candle close (all exchanges)

### 1. Scope / Trigger

The automatic trading loop picks the next decision time. It applies to every trader, grid or not.

### 2. Signatures

- `trader/scan_schedule.go`: `scanTimeframe(config *store.StrategyConfig) string`,
  `nextScanTime(now time.Time, timeframe string, delay time.Duration) (time.Time, error)`,
  `scanCloseDelay` (5 seconds), `(*AutoTrader).scanFallbackInterval()`.

### 3. Contracts

- The cadence timeframe is resolved exactly like the engine's primary timeframe:
  `PrimaryTimeframe` -> `SelectedTimeframes[0]` -> `"5m"`; a value outside the D25 set is skipped.
  An empty result means the trader falls back to `ScanInterval` (1 minute when it is not positive).
- Boundaries use the UTC wall clock: `1w` starts on Monday 00:00, `30d` on the first day of the month,
  every other retained interval (including `7d`) uses the Unix-epoch grid. The trigger is the boundary
  plus 5 seconds.
- `nextScanTime` returns the trigger at or after `now`; when `now` equals the trigger, the next boundary
  is returned (nothing runs twice).
- The loop runs one round immediately on start, then waits for the next trigger; a round that overruns a
  boundary skips it instead of catching up. `stopMonitorCh` stops the timer and returns at once.
- `scan_interval_minutes` is a fallback only. UI copy, assistant skills and the API schema must not
  describe it as the decision cadence.

### 4. Validation & Error Matrix

| Situation | Result |
|---|---|
| Negative `delay` | error |
| Timeframe outside the D25 set | error from `nextScanTime`; the caller logs and falls back |
| `ScanInterval <= 0` on the fallback path | 1 minute |
| Primary and selected timeframes both empty | `"5m"` (same as the engine) |

### 5. Good/Base/Bad Cases

- Base: strategy primary `1h` -> a round at 12:00:05, 13:00:05, ... UTC.
- Good: primary empty, selected `["15m","1h"]` -> the 15-minute grid.
- Bad: primary `"3m"` (legacy) -> skipped; the next valid candidate is used, or the fallback interval.

### 6. Tests Required

`trader/scan_schedule_test.go` covers the epoch grid for every retained interval, the exact-trigger
boundary, the `1w`/`30d` calendar boundaries, invalid input, and the `scanTimeframe` fallback chain.

### 7. Wrong vs Correct

- Wrong: a fixed `time.NewTicker(ScanInterval)` — the model sees half-formed candles and the phase
  depends on when the trader started.
- Correct: compute the next candle close of the engine's own primary timeframe and wait for it.

## Scenario: Attributing exchange fills to the trader (Gate)

### 1. Scope / Trigger

The order sync (`SyncOrdersFromGate`) turns exchange fills into local order, fill and position records.
These records decide which positions an AI trader manages (plan D40): a position the trader did not open
must stay invisible to the model and must never be closed by it.

### 2. Signatures

- `trader/gate/order_texts.go`: `aiTraderOpenText` (`t-nofx-ai`), `assistantOpenText`, `closeText`,
  `partialCloseText`.
- `GateTrade.OrderText` carries the order `text` of each fill (the `my_trades` endpoint returns it).

### 3. Contracts

- Only **opening** fills whose text is `t-nofx-ai` create order, fill and OPEN position records.
  Opens placed by the assistant, the grid, the user or other tools are skipped.
- **Closing** fills are applied only when the trader already has an OPEN record for that symbol and side.
- When the position write of an opening fill fails, the just-created order and fill rows are deleted so the
  next sync replays the trade; otherwise the existing order row would make every later sync skip it.
- Every new entry point that is allowed to open positions for the trader must use `aiTraderOpenText`.
  Assistant and grid opens keep `t-nofx` on purpose: those positions are not the trader's.
- The trader loop adds its own in-memory registration for a position it just opened (before the sync
  has written the record); it is cleared once the position disappears from the exchange.

### 4. Validation & Error Matrix

| Situation | Result |
|---|---|
| Opening fill, text `t-nofx-ai` | order + fill + position records under the trader |
| Opening fill, any other text | skipped (not the trader's position) |
| Closing fill, trader has an OPEN record for the symbol/side | applied, record reduced/closed |
| Closing fill, no OPEN record | skipped (nothing of this trader to close) |
| Opening fill whose position write fails | rows rolled back, replayed on the next sync |
| Two AI traders on one exchange account | not distinguishable (same text) — one account, one AI trader |

### 5. Good/Base/Bad Cases

- Base: trader opens DOGE_USDT with attached TP/SL → the next sync creates its OPEN record.
- Good: the user opens SOL_USDT from the Gate app → no record; the model never sees it.
- Bad: a new trader entry point opens with text `t-nofx` → the position is never attributed (unmanaged).

### 6. Tests Required

`trader/gate/order_sync_ownership_test.go` covers the four rules above; `trader/auto_trader_ownership_test.go`
covers the trader-side visibility and the ownership assertions before close, partial close and protection updates;
`trader/auto_trader_reconcile_test.go` covers the reconciliation matrix.
