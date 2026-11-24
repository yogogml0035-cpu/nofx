# PNL 计算与初始余额

账户读数有四个口径：**本金**、**未实现盈亏**、**可用于合约的额度**、**权益**。PNL 由它们与**初始余额**算出。
这些口径在代码里各只有一处定义（见文末「口径的定义位置」），本文与它保持一致。

## 1. 初始余额（Initial Balance）

**定义：** 创建 trader 时交易所账户的**本金**，作为所有 PNL 计算的基准。

```
Initial Balance = 创建时的本金
                （不含未实现盈亏，已实现盈亏计入）
```

**存储位置：** `traders.initial_balance`

**行为：**

| 场景 | 行为 |
|---|---|
| 创建 trader（页面、REST `POST /api/traders`、NOFXi 助手） | 自动取交易所账户的当前本金，不需要用户输入；创建请求里没有这个字段。页面上读不到账户读数或本金不为正时不阻断创建，初始余额留 0，下次加载时重建；助手直接返回原因 |
| 加载 trader | 初始余额大于 0：照用。不大于 0（存量 trader 的无效值）：以当前本金重建、写回数据库并记一条告警日志；读不到本金或本金不为正时加载失败并说明是哪一种原因 |
| 编辑 trader | 页面「编辑」里的「初始余额」可改，必须大于 0；只有改过才随保存发送。`PUT /api/traders/:id` 的 `initial_balance` 是新的正值时更新，缺省、0 或与原值相同时保持不变，负值返回 400 |
| 换绑交易所 | 旧初始余额属于原来那个账户：没有同时给出新的初始余额时重置为 0，下次加载时以新账户的本金重建；换绑时明确给出了新的正值则采用它。页面的编辑、REST 与 NOFXi 助手三条路径规则相同；助手换绑成功后会在回复里说明初始余额已重置 |
| 同步余额（`POST /api/traders/:id/sync-balance`） | 把初始余额设为交易所账户的当前本金；读不到或本金不为正时报错，已保存的值不变 |

**运行期间系统不会自行修改初始余额。** 充值、提现或想重新开始统计时，由用户在编辑里修改。

**为什么取本金而不是权益：** 本金不含浮动盈亏。以权益作基准会把创建当时的浮动盈亏算进基准，之后盈亏统计与开仓金额
（本金 × 百分比）都会跟着漂移。

## 2. 权益（Equity）

**定义：** 账户的实时总价值。

```
Equity = Principal + Unrealized PnL
```

**数据来源：** 实时从交易所 API 读取。

- `Principal`（本金）：账户里的实际余额，包括已实现盈亏，**不含**仓位的未实现盈亏
- `Unrealized PnL`（未实现盈亏）：所有持仓的未实现盈亏总和

权益随市场价格和持仓变化实时变化。

## 3. PNL（盈亏）

### 3.1 总盈亏

```
Total PNL     = Current Equity − Initial Balance
Total PNL %   = (Total PNL / Initial Balance) × 100%
```

```
Initial Balance: 10,000 USDT  （创建时）
Current Equity:  11,500 USDT  （实时）
-----------------------------------
Total PNL:       +1,500 USDT
Total PNL %:     +15%
```

### 3.2 未实现盈亏

当前所有持仓的未实现盈亏总和，直接取交易所返回的值。

### 3.3 单个持仓的盈亏比例

```
Position PNL % = (Unrealized PnL / Margin Used) × 100%
Margin Used    = Position Value / Leverage
```

## 4. 可用于合约的额度（Available）

开仓时能动用的额度，取自交易所的合约账户。开仓前的保证金余量检查用它：
`可开名义价值 ≤ 可用于合约的额度 ÷（保证金开销系数 / 杠杆 + taker 费率）`。

## 5. 各交易所的取数

适配器（`trader/<交易所>/`）的 `GetBalance` 返回三个键，含义固定：

| 键 | 口径 |
|---|---|
| `totalWalletBalance` | 本金 |
| `totalUnrealizedProfit` | 未实现盈亏 |
| `availableBalance` | 可用于合约的额度 |

- 权益：适配器直接报告了权益（`totalEquity` 或 `total_equity`）时用它，否则等于本金 + 未实现盈亏。
  OKX 把权益放在 `totalWalletBalance` 里，所以另外报告 `totalEquity`，权益才不会多算一次未实现盈亏
- 初始余额：一律取本金。直接报告权益的交易所（bybit、bitget、kucoin、lighter）此前取的是权益，
  两者只在账户当时有浮动盈亏时不同
- 读不到账户读数、或本金不为正时不会记下一个无效的初始余额：创建时留 0 等加载重建，同步与助手的创建直接报错

**Gate（统一账户）**：本金与未实现盈亏取自钱包总额 `GET /wallet/total_balance?currency=USDT`
（本金 = `total.amount − total.borrowed`），可用于合约的额度取自 `GET /futures/usdt/accounts` 的 `available`。
合约账户的 `total` 只适用于经典合约账户，统一账户下是一个约等于 0 的数（与本金无关；读数看起来是合约侧
自上次结算以来未结算的手续费与已实现盈亏，这是按实测读数做的推断），不能作本金。
钱包总额接口的官方说明写明数据最长缓存 1 分钟；实测里手续费与已实现盈亏是即时结算的，未实现盈亏也没有看到滞后。

## 6. 用户操作场景

| 场景 | 用户操作 | 系统行为 |
|---|---|---|
| 创建 trader | 填写名称、模型、交易所、策略，不填余额 | 初始余额 = 当时账户的本金 |
| 正常运行 | 无 | 实时计算 PNL，不修改初始余额 |
| 充值 / 提现后重新校准 | 编辑 trader，把初始余额改成新的基准值 | PNL 从新基准起算 |
| 重新开始统计 | 编辑 trader，把初始余额改成当前本金 | PNL 归零 |

## 7. 口径的定义位置

| 内容 | 位置 |
|---|---|
| 口径与解析（本金、未实现盈亏、可用于合约的额度、权益、初始余额） | `trader/types/balance.go` |
| Gate 取数 | `trader/gate/trader_account.go` 的 `GetBalance` |
| 创建时取本金 | `api/handler_trader.go` 的 `handleCreateTrader`；助手 `agent/tools.go` 的 `defaultTraderInitialBalanceFetcher` |
| 加载时校验与重建 | `trader/auto_trader.go` 的 `resolveInitialBalance` |
| 编辑 | `api/handler_trader.go` 的 `handleUpdateTrader`；前端 `web/src/components/trader/TraderConfigModal.tsx` |
| 同步余额 | `api/handler_trader_status.go` 的 `handleSyncBalance` |
| 交易所账户状态（设置页显示的余额） | `api/exchange_account_state.go` 的 `probeExchangeAccountState` |
