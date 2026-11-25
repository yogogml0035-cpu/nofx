# Quality Guidelines

> Code quality standards for backend development.

---

## Overview

This file records the standards the team has actually agreed on. Sections that are still
"(To be filled by the team)" have no project-specific rule yet; follow the surrounding code.

---

## Forbidden Patterns

### Deriving order sizes with float division

Do not convert between base-asset quantity, price, and contract count with `float64` arithmetic
and then truncate (`int64(quantity / multiplier)`). The result depends on binary rounding:
`0.0003 / 0.0001` truncates to 2, and `8.6 / 0.1` is `85.99999999999999`.

Use the exact-decimal helpers in `trader/gate/decimal.go` (`math/big.Rat`), or integers scaled
to the field's precision, for every conversion that decides what is sent to an exchange.

### Falling back to a guessed value when exchange data cannot be parsed

A missing, malformed, or self-contradictory field in an exchange response is an error. Do not
substitute a default (for example forcing an order to one contract). Return an error that names
the field and the raw value.

### Logging and continuing when an exchange-side setting fails

When the caller asked for a setting that decides the order's risk (leverage, margin mode) and the
exchange call fails or cannot be read back, do not log a warning and place the order anyway. The
order would go out with whatever the exchange already has. Return the error and place nothing.
Contract and reference implementation: [Exchange Adapter Contracts](./exchange-adapters.md).

Known exceptions, kept as they are: the shared `GridTraderAdapter.PlaceLimitOrder` (in `trader/interface.go`
and `trader/types/interface.go`) and the two-call path for adapters without `MarginSettingsApplier` in
`trader/auto_trader_grid.go`. Both still log and continue, because other exchanges' adapters are not being
changed; new code must not add to them.

### Reading account balance fields outside `trader/types`

Do not read `totalWalletBalance`, `totalUnrealizedProfit`, `availableBalance`, `totalEquity` or `total_equity` from
the map returned by `GetBalance()` outside `trader/types/balance.go`, and do not restate "equity = wallet balance +
unrealized PnL" or what the initial balance is anywhere else. A key list or formula copied per call site drifts: the
Gate unified account ended up with 0 as its wallet balance and 0 as every new trader's initial balance that way.
Call `types.ReadAccountBalance` or `types.ReadInitialBalance`. Contract and reference implementation:
[Exchange Adapter Contracts](./exchange-adapters.md).

---

## Required Patterns

### Exchange quantity and price fields are decimal strings

Parse them as exact decimals. Gate contract counts are decimal strings (`"3"`, `"0.6"`) and are
the canonical unit for an order's `size`; base-asset quantity is derived for display only.

Gate metadata is read with `X-Gate-Size-Decimal: 1` (the SDK default). Without it,
`order_size_min` is `0` for contracts that support fractional sizes.

Reference implementation: `trader/gate/contract.go` (`ContractSpec`, `SizeForNotional`,
`SizeForCloseRatio`, `ContractsFromQuantity`).

---

## Testing Requirements

### Demonstrating floating-point error

Compute the value at runtime (from variables or function results). A constant expression is
folded exactly by the compiler, so `3 * 0.0001` written as a constant carries no error and the
test proves nothing.

### Exchange adapters

Use `httptest` servers that reply in the exchange's real response format, and say so in the test
comment. Read-only checks against the real API belong in a separate script under the task's
`probe/` directory, never in the unit test suite.

Error responses in these servers must set `Content-Type: application/json` **before** calling
`WriteHeader`. The Gate SDK only decodes the error `label` into `GateAPIError` when the response
content type is JSON; otherwise the code under test receives `GenericOpenAPIError` and a label
check silently takes the wrong branch. Go's test server sniffs a body written without a content
type as `text/plain`. Gate's real error responses are JSON, for example an unknown position is
HTTP 400 with `{"label":"POSITION_NOT_FOUND"}`.

---

## Code Review Checklist

(To be filled by the team)
