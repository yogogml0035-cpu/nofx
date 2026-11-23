# Gate 交易基座：统一账户权益、仓位与下单、行情全切 Gate

> 状态：需求收敛中（问答第 3 轮，2026-11-24）。实施计划见第 10 节，待第 7 节问题确认后启动。
> 父分支：`feat/2026_1123_gate-trading-foundation`

## 1. 背景与本质问题

NOFX 的账户读写、下单保护与行情获取建立在三套与现状不符的假设上：

- **账户假设**：合约账户是"经典合约账户"，权益 = `total` + 未实现盈亏。实际交易账户是 Gate 统一账户子账户，`total` 恒为 0，权益被算成 0，单仓上限随之变成 0，模型只能 `wait`。
- **下单保护假设**：条件单触发规则按"1 = 小于等于、2 = 大于等于"填写。Gate 文档与官方 SDK 的定义正好相反，多头止损、空头止损、多头止盈、空头止盈四个方向全部与文档相反；挂单失败只记日志。
- **行情假设**：K 线与价格取自 CoinAnk（Binance 通道）和 Binance 接口。实际运行环境只能直连 Gate，交易场所也是 Gate。

已发现的各项缺陷都是这三处错配的表现。

## 2. 术语

| 概念 | 含义 | 与其他概念的关系 |
|---|---|---|
| 主账号 / 子账号 | 归属与管理关系；子账号是独立账户，有自己的余额和 API key | 与账户模式无关。子账号可以是统一账户（`GET /unified/accounts` 支持 `sub_uid` 参数） |
| 经典合约账户 | 合约钱包独立记账；`/futures/usdt/accounts` 的 `total` 只适用这一类型 | 即统一账户模式里的 `classic` |
| 统一账户 | 现货、合约、期权等业务的资产按币种折算成 USD 统一计算，共用账户资产作保证金 | 四种模式：`classic`、`single_currency`、`multi_currency`、`portfolio` |
| 单币种保证金模式 | 各币种保证金独立；全仓保证金指标只对 USDT 有效 | 交易所有 USDT 永续不需要跨币种模式 |
| 跨币种保证金模式 | 多币种资产折算后共享保证金，可借币，各业务盈亏互抵；USDT 合约与期权开关只能打开 | 强平时可能损失账户内全部保证金资产 |

## 3. 账户与接口事实

### 3.1 账户

| 项目 | 事实 | 来源 |
|---|---|---|
| 账户类型 | Gate 统一账户子账号 | `GET /futures/usdt/accounts` 的 `enable_credit` |
| `total` | 统一账户下为 0，单币种与跨币种两种模式都如此 | 同上；文档"只适用于经典合约账户" |
| 资金位置 | 自有 USDT、无借款；全部在现货侧作统一账户保证金，合约侧余额为 0 | `GET /wallet/total_balance`、`GET /spot/accounts` |
| 盈亏结算 | 合约已实现盈亏、手续费、资金费结算到现货（`history.dnw` 为 0 而盈亏类流水非 0） | `history.cross_settle`、`GET /futures/usdt/account_book` |
| 本金不含浮盈亏 | 带持仓读数中浮盈亏变动，`total.amount` 不变；可用额度 = `amount + unrealised_pnl − position_initial_margin` | `GET /wallet/total_balance`、`GET /futures/usdt/accounts` |
| 统一账户接口 | `GET /unified/*` 返回 403：当前 API key 未勾选 unified 权限 | 实测 |
| 钱包总额接口 | `GET /wallet/total_balance?currency=USDT` 返回 `total{amount, borrowed, unrealised_pnl}` 与按业务线的 `details{spot, futures, margin, finance, options, …}` | 文档 `AccountBalance` 模型；实测可访问 |

现货与合约共用同一份余额。文档（统一账户分组页）原文："账户内的各币种资产将会根据其流动性，定义相应的调整系数，再统一折算为USD，来统一计算账户的资产及持仓价值"。

### 3.2 合约张数粒度

下单数量是合约张数，名义价值 = 张数 × `quanto_multiplier` × 价格。下表为 2026-11-24 现价下 1 张的名义价值。

| 合约 | `quanto_multiplier` | 1 张名义价值（USDT） | 小数张 |
|---|---|---:|---|
| BTC | 0.0001 | 6.3 | 否 |
| ETH | 0.01 | 17.7 | 是 |
| SOL | 1 | 75 | 是 |
| ZEC | 0.01 | 8.5 | 否 |
| DOGE | 10 | 0.58 | 否 |
| HYPE | 0.1 | 6.6 | 否 |
| XAU | 0.0001 | 0.37 | 否 |

NOFX 的 Gate 适配层目前把张数截断成整数、不足 1 张时强制改成 1 张，不使用小数张。

### 3.3 K 线周期

`GET /futures/{settle}/candlesticks` 单次最多 2000 点，`limit` 与 `from/to` 互斥，返回 `t`（秒）、`o/h/l/c`、`v`（合约张数）、`sum`（成交额）。

| 周期 | 文档枚举表 | 实测 |
|---|---|---|
| `10s 1m 5m 15m 30m 1h 4h 8h 1d 7d` | 列出 | 可用；`7d` 与 Unix 初始时间对齐（周四起算） |
| `1w`、`30d` | 仅在参数说明中出现 | 可用；`1w` 为自然周（周一起算） |
| `3m 2h 6h 12h 3d` | 未出现 | 可用 |

### 3.4 条件单

| 项目 | 文档（合约分组页、SDK `FuturesPriceTrigger`） | NOFX 现状（`trader/gate/trader_orders.go`） |
|---|---|---|
| `rule` | 1 = 价格 ≥ 触发价（触发价须高于最新价）；2 = 价格 ≤ 触发价（触发价须低于最新价） | 多头止损、空头止盈用 1；空头止损、多头止盈用 2——四个方向均与文档相反 |
| 全平仓 | `close: true` 时 `size` 应为 0 | `close: true` 同时带非零 `size` |
| 仓位止盈止损 | `order_type` 取 `close-long-position` / `close-short-position`，随仓位总量生效 | 未设置 `order_type` |
| 小数张 | 订单 `amount`（字符串）字段 | 未使用 |
| 市价单滑点 | `market_order_slip_ratio` | 未设置 |

## 4. 已确认的决策

| 编号 | 决策 |
|---|---|
| D1 | 目标：行情与数据全部来自 Gate，做彻底 |
| D2 | 仓库不再与上游 NOFX 合并，只参考其代码 |
| D3 | 保留其他交易所的交易适配器 |
| D4 | 初始资金：创建 trader 时记录真实权益，界面可修改，修正存量 trader 的值 |
| D5 | 核对统一账户下杠杆、逐仓/全仓相关接口 |
| D6 | 引擎决策 K 线改为 Gate 合约 K 线直连；成交量口径用成交额（USDT） |
| D7 | 两个 K哥策略的提示词随数据语义变化改写（未收线 K 线条款等） |
| D8 | 下单取价改用 Gate 最新价，市价单加滑点上限 1% |
| D9 | 前端图表 K 线改为 Gate 直连；界面没有的周期不保留 |
| D10 | 删除 CoinAnk 的 K 线代码与旧的内嵌行情路径；CoinAnk 的其他数据能力保留 |
| D11 | 删除 `/api/agent/klines` 与 `/api/agent/ticker`；24h 行情改 Gate |
| D12 | 删除无效的盈亏比校验，盈亏比只靠提示词 |
| D13 | `.dockerignore` 补 `.nofx-local`、`web/node_modules`、`scratch` |
| D14 | 前端健康检查统一为镜像内一份 |
| D15 | 删除 32 个未被容器使用的镜像，只保留两个栈正在使用的 4 个 |
| D16 | 主栈重建固化为 Makefile 命令，使用干净检出作构建上下文 |
| D17 | 扫描按 K 线收线对齐 |
| D18 | 开仓金额按本金的百分比设置；本金不含浮动盈亏 |
| D19 | 最小下单金额可自定义，默认 10 USDT 起步 |
| D20 | 主栈重建后不激活任何策略、不启动任何 trader 或智能体 |
| D21 | 保证金模式切回单币种；NOFX 的修改对单币种与跨币种都适用 |
| D22 | 开仓名义价值 = 本金 × 百分比；杠杆只决定占用多少保证金 |
| D23 | 百分比由用户在策略里选择，可选范围 1%–10% |
| D24 | 下单金额以 USDT 表达，合约张数由系统换算 |
| D25 | 周期选项只用文档列出的周期加 `1w`、`30d` |
| D26 | 其他交易所 trader：分析行情用 Gate，下单取价用该交易所自己的价格，Gate 没有的品种不交易 |
| D27 | 真实下单验证由用户在容器内启动智能体实测完成 |
| D28 | 本轮扩展分批平仓与修改止损止盈 |
| D29 | 交付采用父子任务：父 feature 下拆子任务，串行推进 |

## 5. 口径定义（设计基线）

| 口径 | 定义 | 来源 |
|---|---|---|
| 本金 | `total.amount − total.borrowed`，不含未实现盈亏；已实现盈亏计入 | `GET /wallet/total_balance?currency=USDT` |
| 权益 | 本金 + `total.unrealised_pnl` | 同上 |
| 可用于合约的额度 | `available` | `GET /futures/usdt/accounts` |
| 初始资金 | 创建 trader 时取当时的本金；界面可编辑；存量无效值（不大于 0）在下次加载时以当前本金重建并记录日志 | NOFX |
| 最小下单金额 | 策略配置项，默认 10 USDT | NOFX |
| 开仓名义价值 | 本金 × 策略百分比（1%–10%） | NOFX |
| 行情 | K 线、ticker、持仓量、资金费率全部来自 Gate | Gate |

## 6. 对抗式审查发现

| 编号 | 发现 | 证据 | 影响 |
|---|---|---|---|
| F1 | 切换保证金模式不能解决权益为 0 | 单币种与跨币种两种模式下 `total` 都是 0 | 权益取数必须换来源 |
| F2 | 现货与合约共用一份余额 | 资金全在现货侧，合约 `available` 等于现货余额，盈亏结算到现货 | 本金按账户整体取；账户若同时放其他资产，会被计入本金并承担强平风险 |
| F3 | 张数截断与强制 1 张会放大订单 | SOL 1 张约 75U、ETH 1 张约 17.7U | 不足 1 张的订单最大放大约 7.5 倍 |
| F4 | 引擎没有持仓管理动作 | 决策动作只有六个；同标的同方向已有仓拒绝再开 | K哥策略提示词里的加仓、分批止盈、推保护无法执行（D28 已决定扩展分批平仓与修改止损止盈） |
| F5 | 保留其他交易所适配器与"行情全部来自 Gate"之间需要边界 | 下单数量按价格折算 | 已由 D26 确定 |
| F6 | 保护单触发方向四个都与文档相反 | 第 3.4 节 | 开仓后止损、止盈会被交易所拒绝 |
| F7 | 保护单挂失败只记日志 | `executeOpenLongWithRecord` 对 `SetStopLoss`、`SetTakeProfit` 的错误只打日志 | 仓位留在交易所且没有止损 |
| F8 | 保护单 `close: true` 同时带非零 `size`，且未设 `order_type` | 第 3.4 节 | 与文档不符；仓位级保护单才能在分批平仓后继续覆盖剩余仓位 |
| F9 | 止损止盈价不落库，提示词里没有当前止损止盈 | `store/position.go` 无相关字段；持仓信息不含止损止盈 | 模型无法决定是否移动止损 |
| F10 | 撤条件单不区分止损与止盈 | `cancelTriggerOrders` 撤掉该品种全部条件单 | 修改止损时会把止盈一起撤掉，必须成对重建 |
| F11 | 百分比范围、最小下单金额与本金互相制约 | 第 6.1 节 | 本金较小时多数百分比组合不会下单 |
| F12 | 代码里有一条自动全平规则 | `checkPositionDrawdown`：浮盈 > 5% 且自峰值回撤 ≥ 40% 时全平 | 与模型管理持仓并存 |
| F13 | 保证金模式尚未切回 | 接口仍返回 `multi_currency` | 不影响设计；带仓时可能无法切换 |

### 6.1 百分比、最小下单金额与本金的制约

按 D18、D19、D22、D23、D24，取"不足最小下单金额或不足 1 张则不下单"的规则，名义价值 = 本金 × 百分比。

| 本金 | 百分比 | 名义价值 | BTC | ETH | SOL | ZEC | DOGE | HYPE |
|---|---|---:|---|---|---|---|---|---|
| 100U | 5% | 5.0U | 不下单 | 不下单 | 不下单 | 不下单 | 不下单 | 不下单 |
| 100U | 10% | 10.0U | 不下单（1 张 6.3U） | 10.0U | 10.0U | 不下单（1 张 8.5U） | 不下单（17 张 9.9U） | 不下单（1 张 6.6U） |
| 500U | 2% | 10.0U | 不下单（1 张 6.3U） | 10.0U | 10.0U | 不下单（1 张 8.5U） | 不下单（17 张 9.9U） | 不下单（1 张 6.6U） |
| 500U | 10% | 50.0U | 7 张 43.8U | 50.0U | 50.0U | 5 张 42.4U | 85 张 49.7U | 7 张 46.4U |

名义价值低于最小下单金额的行一律不下单；只支持整数张的合约向下取整后若低于最小下单金额，同样不下单。

## 7. 待确认问题

| 编号 | 问题 | 推荐 |
|---|---|---|
| R1 | 合约张数取整规则：向下取整且低于最小下单金额不下单；四舍五入到最近张数；不足时补足到最小可交易张数 | 第一种 |
| R2 | 百分比的默认取值；1%–10% 是否为固定上下限 | 默认 10%，上下限固定 |
| R3 | 分批平仓如何表达 | 模型给出平仓比例（0–100%），系统换算为张数；仓位只有 1 张时拒绝分批 |
| R4 | 修改止损的约束 | 止损只允许向盈利方向移动（多头只上调、空头只下调），由代码强制；止盈可上可下 |
| R5 | 开仓后止损挂不上时怎么办 | 立即平掉该仓位并报错 |
| R6 | 第 6 节 F12 的自动全平规则如何处理 | 本轮保留，作为代码级安全网 |
| R7 | 保护单修复的真实验证方式 | 用户现有手动持仓上挂一组远离现价的止损止盈，验证后立即撤销 |
| R8 | 第 10 节的父子任务拆分与串行顺序 | 按第 10 节执行 |

## 8. 范围与删除清单

### 8.1 下单保护

| 位置 | 现有逻辑 | 处理 |
|---|---|---|
| `trader/gate/trader_orders.go` `SetStopLoss`、`SetTakeProfit` | 触发规则与文档相反；`close: true` 带非零 `size`；未设 `order_type` | 改为仓位级保护单，触发规则按文档 |
| `trader/gate/trader_orders.go` `OpenLong`、`OpenShort`、`CloseLong`、`CloseShort` | 张数截断与强制 1 张 | 按 D24 与 R1 重写 |
| `trader/gate/trader_orders.go` 市价开仓 | 无滑点上限 | 按 D8 |
| `trader/auto_trader_orders.go` | 保护单失败只记日志；四处 `market.GetWithExchange` 取价 | 按 R5；改用 `GetMarketPrice` |

### 8.2 账户权益

| 位置 | 现有逻辑 | 处理 |
|---|---|---|
| `trader/gate/trader_account.go` `GetBalance` | 以 `total` 作钱包余额 | 改为钱包总额与合约 `available` |
| `trader/auto_trader_loop.go` | 权益 = `totalWalletBalance + totalUnrealizedProfit` | 按第 5 节口径 |
| `trader/auto_trader.go` | `InitialBalance` 取数与校验 | 按第 5 节口径 |

### 8.3 仓位

| 位置 | 现有逻辑 | 处理 |
|---|---|---|
| `store/strategy.go` | 两个仓位价值倍数字段及其 clamp 与调整提示；`min_position_size` 默认 12 | 删除倍数字段；新增百分比（1–10）；`min_position_size` 默认 10 |
| `kernel/engine_position.go` | 硬编码最小下单（BTC/ETH 60、其他 12）；`SL + (TP-SL)*0.2` 的盈亏比校验；权益 × 倍数的上限 | 删除盈亏比校验与硬编码最小下单；金额由代码计算 |
| `kernel/engine_prompt.go` | "仓位大小指引"段；`position_size_usd`、`risk_usd` 输出契约 | 按第 5 节重写 |
| 前端策略编辑器、`agent/strategy_field_catalog.go` 等 | 引用上述字段 | 同步 |

### 8.4 持仓管理

| 位置 | 现有逻辑 | 处理 |
|---|---|---|
| `kernel/engine_prompt.go`、`kernel/engine_position.go` | 六个动作 | 新增分批平仓与修改止损止盈 |
| `trader/auto_trader_decision.go`、`trader/auto_trader_orders.go` | 无对应执行路径 | 新增 |
| `trader/gate/trader_orders.go` | 无保护单替换；撤条件单不分止损止盈 | 新增 |
| 持仓信息 | 不含当前止损止盈 | 每轮从未触发条件单读入 |

### 8.5 行情

| 位置 | 现有逻辑 | 处理 |
|---|---|---|
| `market/data_klines.go`、`market/data.go` | CoinAnk K 线取数；旧 3m/4h 内嵌路径（`Get`、`GetWithExchange`） | 改 Gate 合约 K 线；删除旧路径及只为它服务的结构（`IntradaySeries`、`LongerTermContext`、`BuildDataFromKlines`、`Format`，以切换后无调用方为准） |
| `provider/coinank/coinank_api/` | `kline.go`、`kline_ws.go`；无调用方的 `depth_ws.go`、`WsConn` | 删除 |
| `api/handler_klines.go` | 加密货币 K 线走 CoinAnk，按交易所映射，支持秒级周期 | 改 Gate 直连；删除映射与秒级周期。股票（Alpaca）、外汇与金属（TwelveData）路由保留 |
| `store/strategy.go` `normalizeTimeframe`、前端周期选项 | 接受 `3m/2h/6h/12h/3d` | 按 D25 移除 |
| `agent/web.go`、`api/agent_routes.go` | `HandleKlines`、`HandleTicker`、`proxyBinance` | 删除；`HandleTickers` 改 Gate |
| `agent/tools.go`、`agent/brain.go`、`agent/sentinel.go`、`agent/agent.go` | Binance 取数与 `market.Get` | 改 Gate |
| `kernel/engine.go` | `FetchMarketData`（无调用方） | 删除 |
| 前端 `AdvancedChart` | `exchange` 默认 `binance`，K 线请求带 `exchange` | 删除 |

### 8.6 调度与基础设施

| 位置 | 现有逻辑 | 处理 |
|---|---|---|
| 扫描调度 | 从启动时刻起算 | 按 D17 |
| `docker/Dockerfile.frontend`、`docker-compose.yml` | 两份健康检查 | 按 D14 统一 |
| `.dockerignore` | 未排除本地文件 | 按 D13 |
| `Makefile` | 主栈重建靠手工 | 按 D16 |

## 9. 实施与交付约束

1. 不启动 trader、不激活策略。主栈重建前核对 `traders.is_running` 全为 0，重建后核对日志没有自动拉起。
2. 交付顺序：子任务合入父分支 → 父分支经 dev-offline 实测 → main → 主栈重建。
3. Gate 接口开发与调用前读文档站对应的 Tag 分页。
4. 不做兼容分支，旧语义一次性删除。
5. 验证方式：单元测试与只读的真实接口验证；真实下单验证由用户在实测环节执行（R7 例外，需用户授权）。

## 10. 实施计划

### 10.1 拆分原则

- 每个子任务是一个可以单独合入父分支、单独回退的纵向切片。
- 每个子任务有自己的闭环验证：单元测试，加不上真实交易也能完成的只读或函数级真实验证。
- 子任务之间串行；后一个从前一个合入后的父分支最新提交切出。
- 任何子任务合入后，父分支都不得出现"能开仓但没有有效止损"的状态。

### 10.2 父子任务

父分支：`feat/2026_1123_gate-trading-foundation`。
子分支：`feat/2026_1123_gate-trading-foundation--<slug>`，worktree 位于 `nofx-wt/feat-2026-1123-gate-trading-foundation--<slug>`。

| 顺序 | 子任务 | 内容 | 闭环验证 | 依赖 |
|---|---|---|---|---|
| 1 | `order-safety` 下单与保护单安全 | 保护单改为仓位级并按文档修正触发规则；名义价值换算张数（R1）；滑点上限；取价改 `GetMarketPrice`；止损挂不上时的处理（R5） | 名义价值→张数矩阵单测；保护单构造矩阵单测（方向 × 止损止盈 → `rule`、`order_type`、`size`）；保护单真实挂撤（R7） | 无 |
| 2 | `account-equity` 账户权益与本金 | `GetBalance` 改读钱包总额与合约 `available`；权益、本金口径；初始资金及界面编辑；核对统一账户下杠杆、逐仓/全仓、持仓模式接口 | 单测；带仓与不带仓的只读真实读数；提示词预览 | 无 |
| 3 | `position-sizing` 仓位与最小下单 | 策略配置改为百分比（R2）；删除仓位价值倍数字段；金额由代码计算；删除盈亏比校验与硬编码最小下单；提示词契约；前端编辑器 | 百分比边界与最小下单单测；契约解析单测；提示词预览；`/strategies/test-run` | 1、2 |
| 4 | `position-management` 分批平仓与修改止损止盈 | 决策契约新增两个动作；执行路径；保护单替换；每轮读入当前止损止盈；止损方向约束（R4）；分批比例（R3） | 契约解析、比例换算、止损方向校验单测；真实下单由用户实测 | 1、3 |
| 5 | `gate-klines` K 线全切 Gate | 新增 Gate 合约 K 线取数；引擎、网格长周期箱体、前端图表改用它；周期选项收敛；删除 CoinAnk K 线与旧内嵌路径 | 单测；7 个标的 × 全部保留周期的真实取数；`/api/klines` 返回 Gate 数据 | 无 |
| 6 | `assistant-market` 助手行情切 Gate | 24h 行情、K 线工具、简报、价格告警改 Gate；删除两个接口与 Binance 取数函数 | 单测；工具真实调用返回 Gate 数据 | 5 |
| 7 | `scan-alignment` 扫描按收线对齐 | 调度改为墙钟网格，收线后固定延迟触发 | 单测：间隔与主周期的组合下触发时刻正确 | 无 |
| 8 | `infra-hygiene` 基础设施 | 健康检查统一；`.dockerignore`；Makefile 主栈重建命令（含运行中 trader 的拦截） | 主栈 compose 配置解析；构建上下文不含本地文件 | 无 |

### 10.3 父任务收口

1. 逐项对照本文件做对抗式审查并修正。
2. 串联全部子任务做端到端验收与完整项目门禁。
3. 合入 dev-offline 并重建送测栈；用户在容器内启动智能体做真实下单实测。
4. 合入 main，重建主栈并核对没有 trader 被自动拉起。
5. 改写主栈两个 K哥策略的提示词（数据语义、仓位、持仓管理动作、盈亏比表述）。
6. 删除未使用镜像，归档证据。
