# Gate 交易基座：统一账户权益、仓位与下单、行情全切 Gate

> 状态：需求收敛中（问答第 2 轮，2026-11-24）。实施计划待问答收敛后写入第 10 节。
> 分支：`feat/2026_1123_gate-trading-foundation`

## 1. 背景与本质问题

NOFX 的账户读写与行情获取建立在两套与现状不符的假设上：

- **账户假设**：合约账户是"经典合约账户"，权益 = `total` + 未实现盈亏。实际交易账户是 Gate 统一账户子账户，`total` 恒为 0，权益被算成 0，单仓上限随之变成 0，模型只能 `wait`。
- **行情假设**：K 线与价格取自 CoinAnk（Binance 通道）和 Binance 接口。实际运行环境只能直连 Gate，交易场所也是 Gate。

已发现的各项缺陷都是这两处错配的表现。

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
| 账户类型 | Gate 统一账户子账号，当前为跨币种保证金模式（`margin_mode: 1`、`margin_mode_name: multi_currency`、`enable_credit: true`） | `GET /futures/usdt/accounts` |
| `total` | 统一账户下为 0，单币种与跨币种两种模式都如此 | 同上；文档"只适用于经典合约账户" |
| 资金位置 | 自有 USDT、无借款；全部在现货侧作统一账户保证金，合约侧余额为 0，合约 `available` 等于现货余额 | `GET /wallet/total_balance`、`GET /spot/accounts` |
| 盈亏结算 | 合约已实现盈亏、手续费、资金费结算到现货（`history.dnw` 为 0 而盈亏类流水非 0） | `history.cross_settle`、`GET /futures/usdt/account_book` |
| 统一账户接口 | `GET /unified/*` 返回 403：当前 API key 未勾选 unified 权限 | 实测 |
| 钱包总额接口 | `GET /wallet/total_balance?currency=USDT` 返回 `total{amount, borrowed, unrealised_pnl}` 与按业务线的 `details{spot, futures, margin, finance, options, …}` | 文档 `AccountBalance` 模型；实测可访问 |

现货与合约共用同一份余额。`total.amount` 与 `total.unrealised_pnl` 是两个独立字段；带持仓时 `amount` 的取值需要一次真实读数确认（见第 7 节 Q10）。

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

## 5. 口径定义（设计基线）

| 口径 | 定义 | 来源 |
|---|---|---|
| 本金 | `total.amount − total.borrowed`，不含未实现盈亏；已实现盈亏计入 | `GET /wallet/total_balance?currency=USDT` |
| 权益 | 本金 + `total.unrealised_pnl` | 同上 |
| 可用于合约的额度 | `available` | `GET /futures/usdt/accounts` |
| 初始资金 | 创建 trader 时取当时的本金；界面可编辑；存量无效值（不大于 0）在下次加载时以当前本金重建并记录日志 | NOFX |
| 最小下单金额 | 策略配置项，默认 10 USDT | NOFX |
| 开仓金额 | 本金 × 配置百分比（百分比口径见第 7 节 Q2、Q3） | NOFX |
| 行情 | K 线、ticker、持仓量、资金费率全部来自 Gate（其他交易所 trader 的边界见第 7 节 Q6） | Gate |

## 6. 对抗式审查发现

| 编号 | 发现 | 证据 | 影响 |
|---|---|---|---|
| F1 | 切换保证金模式不能解决权益为 0 | 单币种与跨币种两种模式下 `total` 都是 0 | 权益取数必须换来源 |
| F2 | 现货与合约共用一份余额 | 资金全在现货侧，合约 `available` 等于现货余额，盈亏结算到现货；文档写明统一账户统一计算资产及持仓价值 | 本金必须按账户整体取；账户若同时放其他资产，会被计入本金并承担强平风险 |
| F3 | "最小下单 10U"与现有下单实现冲突 | 适配层截断张数并强制至少 1 张；SOL 1 张约 75U、ETH 1 张约 17.7U | 不足 1 张的订单会被放大，最大约 7.5 倍 |
| F4 | 仓位百分比口径有歧义 | "按本金百分比设置开仓金额"可指名义价值，也可指占用保证金 | 两种理解的仓位规模相差一个杠杆倍数 |
| F5 | `position_size_usd` 的决定方未定 | 现状由模型在"权益 × 倍数"的上限内自行决定，提示词另有一套按置信度的指引 | 与"按本金百分比设置"的确定性要求不一致 |
| F6 | K 线周期的文档与实测不一致 | 实测接受 `3m/2h/6h/12h/3d`，文档未列；`1w` 是周一起算的自然周 | 周线应用 `1w`；未列周期的去留需要决定 |
| F7 | 引擎没有持仓管理动作 | 决策动作只有 `open_long/open_short/close_long/close_short/hold/wait`；适配层对同标的同方向已有仓拒绝再开 | K哥策略提示词里的加仓、分批止盈、推保护无法执行 |
| F8 | 其他交易所 trader 的数据边界未定 | 行情全部来自 Gate，但下单数量按价格折算 | 用 Gate 的价格在别的交易所折算数量会出错；Gate 没有的品种没有行情 |
| F9 | 下单相关改动无法由本轮自行做真实验证 | D20 规定不启动 trader | 小数张、滑点上限、取整规则需要有人用真实订单验证 |

## 7. 待确认问题

| 编号 | 问题 | 推荐 |
|---|---|---|
| Q1 | 保证金模式留在跨币种，还是切回单币种 | 切回单币种：交易 USDT 永续不需要跨币种，后者会把其他资产算作保证金并开放借款。NOFX 的修改对两种模式都适用 |
| Q2 | 百分比作用于名义价值，还是作用于占用保证金 | 名义价值 = 本金 × 百分比；杠杆只决定占用多少保证金 |
| Q3 | 百分比由谁决定 | 按置信度分档并在策略里配置；金额由代码计算，模型不再输出 `position_size_usd` 与 `risk_usd` |
| Q4 | 合约张数取整规则 | 支持小数张的合约用小数张；只支持整数张的向下取整；取整后低于最小下单金额或不足 1 张则不下单，不上调 |
| Q5 | 文档未列的周期（`3m/2h/6h/12h/3d`）去留 | 从可选项移除，只用文档列出的周期加 `1w`、`30d` |
| Q6 | 其他交易所 trader 的边界 | 分析行情用 Gate；下单取价用该交易所自己的价格；Gate 没有的品种不交易 |
| Q7 | 真实下单验证由谁执行 | 由用户在实测环节执行 |
| Q8 | 是否扩展持仓管理动作（分批平仓、修改止损止盈、加仓） | 本轮不扩展，把两个 K哥策略的提示词改成与现有动作一致；扩展另立任务 |
| Q9 | 交付拆分 | 两个 feature 依次交付：①账户权益、仓位与下单安全、基础设施；②行情全切 Gate。各自经 dev-offline 实测后合入 main |
| Q10 | 带持仓时的本金口径验证 | 用户实测时开一笔小仓位，允许只读读取一次接口 |

## 8. 范围与删除清单（初稿）

### 8.1 Feature ①：账户权益、仓位与下单安全、基础设施

| 位置 | 现有逻辑 | 处理 |
|---|---|---|
| `trader/gate/trader_account.go` `GetBalance` | 以 `total` 作钱包余额 | 改为钱包总额（本金、借款、未实现盈亏）与合约 `available` |
| `trader/auto_trader_loop.go` | 权益 = `totalWalletBalance + totalUnrealizedProfit`；单仓上限 = 权益 × 倍数 | 按第 5 节口径重写 |
| `trader/auto_trader.go` | `InitialBalance` 取数与校验 | 按第 5 节口径重写 |
| `kernel/engine_position.go` | 硬编码最小下单（BTC/ETH 60、其他 12）；`SL + (TP-SL)*0.2` 的盈亏比校验；权益 × 倍数的上限 | 删除盈亏比校验；最小下单读策略配置；上限按百分比 |
| `kernel/engine_prompt.go` | "仓位大小指引"段；`position_size_usd`、`risk_usd` 输出契约 | 依 Q2、Q3 的结论重写 |
| `store/strategy.go` | `btc_eth_max_position_value_ratio`、`altcoin_max_position_value_ratio` 及其 clamp 与调整提示；`min_position_size` 默认值 | 依 Q2、Q3 的结论重写；`min_position_size` 默认 10 |
| 前端策略编辑器、`agent/strategy_field_catalog.go` 等 | 引用上述字段 | 同步 |
| `trader/gate/trader_orders.go` | 张数截断与强制 1 张；市价单无滑点上限 | 按 Q4 重写；加滑点上限 |
| `trader/auto_trader_orders.go` | 四处 `market.GetWithExchange` 取价 | 改用 `GetMarketPrice` |
| 扫描调度 | 从启动时刻起算 | 按 D17 重写 |
| `docker/Dockerfile.frontend`、`docker-compose.yml` | 两份健康检查 | 按 D14 统一 |
| `.dockerignore` | 未排除本地文件 | 按 D13 |
| `Makefile` | 主栈重建靠手工 | 按 D16 |

### 8.2 Feature ②：行情全切 Gate

| 位置 | 现有逻辑 | 处理 |
|---|---|---|
| `market/data_klines.go`、`market/data.go` | CoinAnk K 线取数；旧 3m/4h 内嵌路径（`Get`、`GetWithExchange`） | 改 Gate 合约 K 线；删除旧路径及只为它服务的结构（`IntradaySeries`、`LongerTermContext`、`BuildDataFromKlines`、`Format`，以切换后无调用方为准） |
| `provider/coinank/coinank_api/` | `kline.go`、`kline_ws.go`；无调用方的 `depth_ws.go`、`WsConn` | 删除 |
| `api/handler_klines.go` | 加密货币 K 线走 CoinAnk，按交易所映射，支持秒级周期 | 改 Gate 直连；删除映射与秒级周期。股票（Alpaca）、外汇与金属（TwelveData）路由保留 |
| `agent/web.go`、`api/agent_routes.go` | `HandleKlines`、`HandleTicker`、`proxyBinance` | 删除前两者与 `proxyBinance`；`HandleTickers` 改 Gate |
| `agent/tools.go`、`agent/brain.go`、`agent/sentinel.go`、`agent/agent.go` | Binance 取数与 `market.Get` | 改 Gate |
| `kernel/engine.go` | `FetchMarketData`（无调用方） | 删除 |
| 前端 `AdvancedChart` | `exchange` 默认 `binance`，K 线请求带 `exchange` | 删除 |

## 9. 实施与交付约束

1. 不启动 trader、不激活策略。主栈重建前核对 `traders.is_running` 全为 0，重建后核对日志没有自动拉起。
2. 交付顺序：feature → dev-offline 实测 → main → 主栈重建。
3. Gate 接口开发与调用前读文档站对应的 Tag 分页。
4. 不做兼容分支，旧语义一次性删除。
5. 验证方式：单元测试与只读的真实接口验证；真实下单验证的执行方见 Q7。

## 10. 实施计划

待问答收敛后补充。
