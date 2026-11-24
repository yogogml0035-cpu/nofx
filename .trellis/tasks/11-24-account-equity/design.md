# 技术设计：账户权益与本金（account-equity）

## 边界

接入交易路径、API、助手与界面，删除旧口径。合入父分支后：

- Gate trader 的权益、可用于合约的额度是真实值；本金取自钱包总额，不再用恒为 0 的合约账户 `total`
- 本金、未实现盈亏、可用于合约的额度、权益、初始资金在 `trader/types` 里各有一处定义，其余代码只用解析结果
- 初始资金：创建时取当时的本金；不大于 0 的存量值在加载时以当前本金重建并写回；编辑表单可以修改

改动文件：

- `trader/types/balance.go`：新增。账户读数的规范口径与解析函数
- `trader/gate/trader_account.go`：`GetBalance` 改取钱包总额与合约账户 `available`
- `trader/auto_trader.go`：初始资金的校验与重建拆成独立函数，取本金
- `trader/auto_trader_loop.go`、`auto_trader_decision.go`、`auto_trader_orders.go`、`auto_trader_grid.go`：
  删除各自按键名取数与权益公式，改用解析函数
- `api/handler_trader.go`：创建时取本金；更新时拒绝负的初始资金
- `api/handler_trader_status.go`：同步余额取本金；旧值为 0 时的除零防护
- `api/exchange_account_state.go`：账户状态用解析结果；删除 `extractExchangeTotalEquity`、`extractFirstNumeric`
- `agent/tools.go`：创建 trader 时取本金；删除 `extractTraderInitialBalance`
- `web/`：编辑表单的初始资金输入、创建表单的提示、类型与三种语言文案
- `docs/pnl.md`、`.trellis/spec/backend/exchange-adapters.md`

不动：其他交易所的适配器（D3）、提示词文本与金额计算（子任务 9）、回撤监控（子任务 6）、`sync-balance` 路由本身。

## 文档依据

原文摘录见 `research/gate-account-balance-docs.md`（章节号指该文件；文档站钱包分页与 Agent 文档页各抓取一次成功）。
与本设计直接相关的结论：

| 设计项 | 依据 | 性质 |
|---|---|---|
| `GET /wallet/total_balance`「查询个人账户总额」：参数只有 `currency`（BTC / CNY / USD / USDT，默认 USDT）；返回 `TotalBalance`，只有 `total` 与 `details` 两个属性；`total.amount`「账户总额数字」、`total.currency`「币种」 | A.1 | 文档站原文 |
| `total.unrealised_pnl`「未实现盈亏总和,这个字段只会在futures,options,delivery,total 账户中出现」；`total.borrowed`「杠杆借贷总和,这个字段只会在margin,cross_margin账户中出现」 | A.1 | 文档站原文 |
| 「折算价以及相关各账户的余额信息最长会有1分钟的缓存，不推荐在即时计算时使用该接口的数据。即时计算可根据账户类型查询对应的余额接口，如：…… `GET /futures/{settle}/accounts` 查询合约账户」 | A.1 | 文档站原文 |
| 文档**没有写** `total.amount` 是否包含未实现盈亏、是否扣除借款，也没有写 `details` 与 `total` 的加总关系 | B.10、F 节第 10–12 项 | 文档站原文（缺失） |
| SDK v7.2.184：`WalletApi.GetTotalBalance(ctx, *GetTotalBalanceOpts{Currency})` 返回 `TotalBalance{Total AccountBalance; Details map[string]AccountBalance}`；`AccountBalance{Amount, Currency, UnrealisedPnl, Borrowed string}`，四个字段都是 optional | D.2、D.3 | SDK 原文 |
| 文档站与 SDK 对该接口的定义一致，只有 `borrowed` 的中英文措辞等三处表述差异 | D.5 | 归纳（逐项对照） |
| `GET /futures/{settle}/accounts`：`total`「只适用于经典合约账户……不包含仓位的未实现盈亏」；`available`「逐仓可用的转出或交易的额度，统一账户下包含授信额度的逐仓可用额度」；`unrealised_pnl`「未实现盈亏」；`position_initial_margin`「头寸占用的起始保证金，适用于统一账户模式」 | 归档子任务 5 的 `research/gate-leverage-margin-docs.md` A.13（FuturesAccount 模型全文） | 文档站原文 |
| Agent 文档页：`cex_wallet_get_total_balance`（账户总资产）、`cex_fx_get_fx_accounts`（查询合约账户）、`cex_unified_get_unified_accounts`（统一账户信息）；页面只有工具名与一句说明，没有参数与返回字段 | E.2、E.6 | 文档站原文 |
| 实际返回：`total.amount = "107.49769421"`、`total.borrowed = "0"`、`total.unrealised_pnl = "0"`、`total.currency = "USDT"`；数值为 0 时这三个字段仍然返回；`details` 实际 13 个键，与文档的 12 键清单不完全相同（SDK 按 map 解析） | G.3 | 实测 |
| 合约账户 `total = "0.000000005"`（≈ 0）、`available = "107.49769"`（5 位小数）、`unrealised_pnl = "0"`；`enable_credit = true`、`margin_mode = 1`、`position_mode = "single"` | G.2 | 实测 |
| 带持仓时 `total.amount` 不变、浮盈亏变动；可用额度 = `amount + unrealised_pnl − position_initial_margin` | 计划文档 3.1（本任务带仓读数复核） | 实测 |

「本金 = `total.amount − total.borrowed`、不含未实现盈亏」在官方文档里没有明文，依据是实测（计划文档 3.1 与本任务的带仓读数）。
本账户没有借款（`borrowed = "0"`），相减与否当前结果相同；`total.borrowed` 缺失时按 0 计。

## 语义

### 账户读数的规范口径（`trader/types/balance.go`）

`types.Trader.GetBalance()` 返回的映射保持原样（十个适配器都在用），键名与含义在这里定义一次：

| 键 | `AccountBalance` 字段 | 含义 |
|---|---|---|
| `totalWalletBalance` | `Principal` | 本金：不含未实现盈亏，已实现盈亏计入 |
| `totalUnrealizedProfit` | `UnrealizedPnL` | 未实现盈亏 |
| `availableBalance` | `Available` | 可用于合约的额度 |
| （解析得出） | `Equity` | 权益 |
| （`InitialBalance()`） | — | 初始资金：创建或重建时记录的读数，即本金 |

`ParseAccountBalance(map[string]interface{}) (AccountBalance, error)` 是唯一读这些键的地方：

- 三个规范键缺失或不是数值都报错，错误里带键名与实际类型。它们决定能否开仓与开多大，不按 0 处理
- 权益：适配器直接报告了权益（先看 `totalEquity`，再看 `total_equity`，取第一个大于 0 的）就用它；
  否则权益 = 本金 + 未实现盈亏。Gate、Binance、OKX、Aster 不报告权益，走后一条
- `InitialBalance()` 返回本金。创建、重建、同步初始资金都只经它取值

对其他交易所的取值不变：交易路径原本就是「`totalEquity` 大于 0 时用它，否则钱包余额 + 未实现盈亏」；
网格原本只认 `total_equity`，现在与交易路径同一条规则（bitget、kucoin、lighter 的 `total_equity` 等于各自的钱包余额 + 未实现盈亏）。

### Gate 取数（`GetBalance`）

```
GET /wallet/total_balance?currency=USDT   → total.amount、total.borrowed、total.unrealised_pnl、total.currency
GET /futures/usdt/accounts                → available
本金 = total.amount − total.borrowed      （精确十进制相减，再转 float64）
```

- `total.amount` 必然存在：缺失或解析失败即报错。统一账户下它才是本金所在，合约账户的 `total` 只适用于经典合约账户
- `total.borrowed`、`total.unrealised_pnl` 在数值为 0 时交易所可能不返回，缺省按 0 计（Gate 的返回约定，不是猜测值）；
  有值但解析失败即报错
- `total.currency` 带值时必须是 `USDT`：请求的就是 USDT 口径，回了别的币种说明读数单位不对
- 适配器不单独报告权益：Gate 的权益就是「本金 + 未实现盈亏」，由解析函数得出，不再写第二遍公式
- 15 秒缓存不变。缓存期间本金、未实现盈亏、可用于合约的额度来自同一次读数

钱包总额接口的官方说明写明它返回的是折算后的估算值、数据可能缓存最多 1 分钟、不建议用于实时计算
（SDK `GetTotalBalance` 的函数说明）。本金随已实现盈亏、手续费、资金费变动，是慢变量，可以接受；
权益里的未实现盈亏是否明显滞后，在带仓读数里与合约账户的 `unrealised_pnl`、持仓条目的 `unrealised_pnl` 对比后确定，
结论写进 `probe/README.md` 与计划文档。

### 交易路径

`buildTradingContext`、`GetAccountInfo`、开仓前的仓位上限计算（`auto_trader_orders.go`）、网格的回撤检查与上下文，
都改成「取余额 → `ParseAccountBalance` → 用字段」。解析失败按各处原有的失败方式处理：
决策上下文、账户信息、开仓返回错误（本轮不交易）；网格回撤检查返回未触发；网格上下文保持原有的失败不阻断。

删除的东西：

- 五处 `balance["totalWalletBalance"].(float64)` 式的取数与三处「权益 = 钱包余额 + 未实现盈亏」公式
- 开仓时「权益读不到就退回可用余额」的回退——本金有了真实来源，不再需要用可用余额冒充权益
- `GetAccountInfo` 返回的 `wallet_balance` 键保留，值为本金

### 初始资金

| 场景 | 行为 |
|---|---|
| 创建 trader（REST 与助手） | 读交易所账户的本金作为初始资金。读取或解析失败时不阻断创建，初始资金留 0，交给加载时的重建 |
| 加载 trader（`NewAutoTrader`） | 配置里的初始资金大于 0：照用。不大于 0：读当前本金，用它重建、写回数据库、记一条告警日志；读不到或本金不为正则加载失败并说明原因 |
| 编辑 trader（`PUT /traders/:id`） | `initial_balance` 大于 0：更新；缺省或 0：保持原值；负值：400 |
| 同步余额（`POST /traders/:id/sync-balance`） | 初始资金取当前本金；旧值不大于 0 时变化百分比记 0（原实现对 0 取百分比会得到无穷大） |

创建请求里的 `initial_balance` 字段删除：创建时一律取本金，该字段此前只在交易所读数失败时作兜底，
而界面从不发送它（助手的创建路径已经是不理会入参、一律自动读取）。

**对其他交易所的影响**：初始资金取本金对全部交易所一致。Binance、Aster、Gate、OKX 记录的值不变
（它们没有权益键，取的本来就是 `totalWalletBalance`）；bybit、bitget、kucoin、lighter 原先取权益，
现在取本金，两者只在创建或同步当时账户有浮动盈亏时不同。这与 D18「本金不含浮动盈亏」一致，
盈亏统计的基准不再随当时的浮动盈亏变动。

### 界面（`TraderConfigModal`）

- **编辑模式**：交易参数一节新增「初始余额」输入框，初值为该 trader 当前的初始资金；下方提示「用于手动更新初始余额基准
  （例如充值/提现后）」。值为空、非数字或不大于 0 时保存按钮不可用，并显示「初始余额必须大于 0」。
  保存时随 `PUT /traders/:id` 发送 `initial_balance`
- **创建模式**：没有输入框。原提示「系统将自动获取您的账户净值作为初始余额」改为
  「创建时自动把交易所账户的当前本金（不含浮动盈亏）记为初始余额，之后可在编辑中修改」
- 文案：标签与提示复用文案表里已有的 `initialBalanceLabel`、`balanceUpdateHint`（三种语言齐全、当前无人使用）；
  `autoFetchBalanceInfo` 改写；新增 `initialBalanceInvalid`
- 类型：`CreateTraderRequest` 增加 `initial_balance?: number`（只在更新时发送）；表单状态里初始余额以字符串保存，
  避免数字输入框在输入过程中被强制改写

## 数据结构与函数

```go
// trader/types/balance.go

// AccountBalance 账户读数的规范口径
type AccountBalance struct {
	Principal     float64 // 本金：不含未实现盈亏，已实现盈亏计入
	UnrealizedPnL float64 // 未实现盈亏
	Available     float64 // 可用于合约的额度
	Equity        float64 // 权益：适配器报告的值，否则 Principal + UnrealizedPnL
}

// InitialBalance 创建或重建 trader 初始资金时记录的读数：当时的本金
func (b AccountBalance) InitialBalance() float64

// ParseAccountBalance 解析适配器 GetBalance 的返回值；规范键缺失或不是数值即报错
func ParseAccountBalance(m map[string]interface{}) (AccountBalance, error)

// trader/gate/trader_account.go
func (t *GateTrader) GetBalance() (map[string]interface{}, error) // 钱包总额 + 合约账户 available

// trader/auto_trader.go
// resolveInitialBalance 配置里的初始资金不大于 0 时，以交易所账户的当前本金重建并写回数据库
func resolveInitialBalance(trader Trader, st *store.Store, userID string, config *AutoTraderConfig) error
```

## 测试

- `trader/types/balance_test.go`：解析矩阵——三个键齐全、权益的两个报告键、报告值为 0 时用公式、
  每个规范键缺失、每个规范键不是数值；`InitialBalance()` 等于本金
- `trader/gate/trader_account_test.go`：按真实响应格式回复的 httptest 服务——本金含借款时的相减、
  `total.amount` 缺失与格式错误、币种不是 USDT、合约账户请求失败、缓存命中时不发请求；
  既有测试套件的 mock 补上钱包总额端点
- `trader/auto_trader_initial_balance_test.go`：适配器桩 + 临时 sqlite 库——有效值不读交易所；
  无效值重建并写回；读不到或本金不为正时报错且不写库
- 交易路径：决策上下文与账户信息的权益、可用余额来自解析结果；读数缺键时返回错误；开仓上限不再回退到可用余额
- `api`：创建取本金、更新拒绝负值、同步余额取本金与旧值为 0 的防护（适配器桩）
- `agent`：创建 trader 的初始资金取本金
- 前端：`TraderConfigModal.test.tsx`——编辑模式显示当前值并随保存发出、非正值禁用保存、创建模式不发送该字段

## 真实读数（`probe/`，只读；带仓读数前沿用 W3 开一个测试仓位）

脚本调用交付的适配器与 `resolveInitialBalance`（与线上同一路径），每步打印读数，证据为 JSONL：

| 步 | 操作 | 核对 |
|---|---|---|
| 预检 | 账户无持仓、无未完结委托、无未触发条件单；记录 ETH_USDT 的仓位设置与三个合约的设置条目 | 否则中止 |
| 1 | 不带仓读数：`GetBalance` 与两个原始接口 | 本金 = `total.amount − total.borrowed`；可用 = 合约账户 `available`；权益 = 本金 + 未实现盈亏；解析结果与原始值一致 |
| 2 | 不带仓的初始资金重建：`resolveInitialBalance`（不接数据库） | 得到本金 |
| 3 | W3：ETH_USDT 以 0.1 张随单保护开多，请求带该合约现有的保证金模式与杠杆 | 下单张数 0.1；设置读回与基线一致 |
| 4 | 带仓读数：间隔采样若干次 | 本金与第 1 步一致（已实现盈亏为 0）；未实现盈亏与合约账户、持仓条目的读数对比（缓存是否滞后）；可用 = 本金 + 未实现盈亏 − `position_initial_margin`；权益 = 本金 + 未实现盈亏 |
| 5 | 全平 | 无持仓、无未完结委托、无未触发条件单 |
| 6 | 终态 | 三个合约的设置条目与基线一致；本金 = 基线 − 手续费 ± 这一段的已实现盈亏 |
| 7 | 提示词预览 | 用第 1 步的读数渲染系统提示词与用户提示词，账户段为非零的真实值 |

容器出口走 VPN、不在 Gate key 白名单：脚本在 Docker 内交叉编译 `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64`，宿主机运行；
凭据经环境变量 `GATE_KEY` / `GATE_SECRET` 传入，不落盘、不打印。

## 已知边界

- 其他交易所的适配器没有改。OKX 把 `totalEq`（权益）放在 `totalWalletBalance` 里，共享路径再加上未实现盈亏，
  权益会多算一次未实现盈亏——这是既有行为，本子任务保持不变
- 助手的 `manage_trader` 更新动作不能修改初始资金（沿用原值）；界面可以
- 账户仍是跨币种保证金模式；单币种模式下的读数待账户切换后复验（本金取自钱包总额，与保证金模式无关）
- 钱包总额里的 `total.unrealised_pnl` 覆盖全部业务线（合约、期权、杠杆等），合约账户的 `unrealised_pnl` 只覆盖 USDT 永续；
  只做 USDT 永续时两者相等，账户若持有其他业务线的仓位，权益会包含那些仓位的浮动盈亏
- 加载时的重建要读交易所；容器出口不在白名单的环境里读不到，trader 加载失败并给出原因（与原先「初始资金必须大于 0」的失败一致）
- 界面实测在隔离的容器栈里做：容器读不到 Gate，创建流程里「取本金」这一步在容器里必然失败，
  由真实读数（第 2 步，同一函数）与适配器桩的单测覆盖；容器里验证的是编辑流程与创建表单的提示
