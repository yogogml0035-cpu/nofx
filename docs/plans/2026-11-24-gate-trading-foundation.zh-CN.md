# Gate 交易基座：统一账户权益、仓位与下单、行情全切 Gate

> 状态：实施中（2026-11-24）。子任务 1–5 已合入父分支；U1 待确认，只阻塞子任务 6。进度见第 10.5 节。
> 父分支：`feat/2026_1123_gate-trading-foundation`
> Trellis 记录：`.trellis/tasks/11-24-gate-trading-foundation/`（2026-11-24 补登；需求与决策以本文件为准，任务记录只做索引与子任务映射）

## 1. 背景与本质问题

NOFX 的账户读写、下单保护与行情获取建立在三套与现状不符的假设上：

- **账户假设**：合约账户是"经典合约账户"，权益 = `total` + 未实现盈亏。实际交易账户是 Gate 统一账户子账户，`total` 恒为 0，权益被算成 0，单仓上限随之变成 0，模型只能 `wait`。
- **下单保护假设**：条件单触发规则按"1 = 小于等于、2 = 大于等于"填写，止损止盈靠开仓后的第二次调用补挂。Gate 文档与官方 SDK 的定义正好相反，四个方向全部填反；补挂失败只记日志。
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
| 全仓 / 逐仓 | 全仓：整个账户余额为仓位兜底。逐仓：亏损以该仓位保证金为限 | 由杠杆接口的参数决定。NOFX 用新接口 `set_leverage` 的 `margin_mode`（`cross` / `isolated`，子任务 5）；旧接口 `positions/{contract}/leverage` 里 `leverage` 为 0 并给 `cross_leverage_limit` 是全仓，`leverage` 非 0 是逐仓 |

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
| 2026-11-24 复核 | 账户无持仓、无挂单；可用 107.53 USDT；`margin_mode` 仍为 1（跨币种），`position_mode` 为 `single`；BTC_USDT 的持仓设置为全仓 10 倍。用户此前的手动 BTC 多单已于 2026-11-23 19:37 UTC 平掉 | 只读实测 |

现货与合约共用同一份余额。文档（统一账户分组页）原文："账户内的各币种资产将会根据其流动性，定义相应的调整系数，再统一折算为USD，来统一计算账户的资产及持仓价值"。

### 3.2 合约张数粒度

下单数量是合约张数，名义价值 = 张数 × `quanto_multiplier` × 价格。下表为 2026-11-24 只读实测（带 `X-Gate-Size-Decimal: 1` 请求头）。

| 合约 | `quanto_multiplier` | `order_size_min`（最小步长） | `enable_decimal` | 1 张名义价值（USDT） | 默认市价滑点上限 | 最大杠杆 |
|---|---:|---:|---|---:|---:|---:|
| BTC | 0.0001 | 1 | false | 6.02 | 1% | 200 |
| ETH | 0.01 | 0.1 | true | 16.93 | 1% | 200 |
| SOL | 1 | 0.1 | true | 71.16 | 2% | 100 |
| ZEC | 0.01 | 1 | false | 7.56 | 2% | 75 |
| HYPE | 0.1 | 1 | false | 6.31 | 2% | 75 |
| DOGE | 10 | 1 | false | 0.56 | 5% | 75 |
| XAU | 0.0001 | 1 | false | 0.36 | 1.5% | 150.01 |

- 不带 `X-Gate-Size-Decimal: 1` 时，`order_size_min` 的小数部分被丢弃（ETH、SOL 返回 0）；带上后返回真实的 `"0.1"`，响应里的数量字段相应变为字符串。
- 小数张合约下单：带同一请求头，把小数 `size` 以十进制字符串提交。SDK v7.2.184 默认带该请求头。
- NOFX 的 Gate 适配层目前把张数截断成整数、不足 1 张时强制改成 1 张，不使用小数张；截断用的是浮点除法，`int64(0.0003 / 0.0001)` 得 2 而不是 3。

### 3.3 K 线周期

`GET /futures/{settle}/candlesticks` 单次最多 2000 点，`limit` 与 `from/to` 互斥，返回 `t`（秒）、`o/h/l/c`、`v`（合约张数）、`sum`（成交额）。

| 周期 | 文档枚举表 | 实测 |
|---|---|---|
| `10s 1m 5m 15m 30m 1h 4h 8h 1d 7d` | 列出 | 可用；`7d` 与 Unix 初始时间对齐（周四起算） |
| `1w`、`30d` | 仅在参数说明中出现 | 可用；`1w` 为自然周（周一起算） |
| `3m 2h 6h 12h 3d` | 未出现 | 可用 |

### 3.4 下单与保护

「NOFX 现状」列记录的是 2026-11-24 开工时的状态；各行的处理进度见第 10.5 节。

| 项目 | 文档（合约分组页、官方 SDK 模型） | NOFX 现状（`trader/gate/trader_orders.go`） |
|---|---|---|
| 条件单 `rule` | 1 = 价格 ≥ 触发价（触发价须高于最新价）；2 = 价格 ≤ 触发价（触发价须低于最新价） | 多头止损、空头止盈用 1；空头止损、多头止盈用 2——四个方向均与文档相反 |
| 全平仓 | `close: true` 时 `size` 应为 0 | `close: true` 同时带非零 `size` |
| 仓位止盈止损 | `order_type` 取 `close-long-position` / `close-short-position`，随仓位总量生效 | 未设置 `order_type` |
| 随单附带止盈止损 | 下单接口可选 `tpsl_tp_trigger_price`、`tpsl_sl_trigger_price` | 开仓后另发两次调用补挂，失败只记日志 |
| 小数张 | 下单的 `size` 以十进制字符串提交并带 `X-Gate-Size-Decimal: 1`；条件单的初始委托另有等价的 `amount` 字段，两者同时给出时以 `amount` 为准 | 未使用 |
| 市价单滑点 | `market_order_slip_ratio`；不传时用合约的默认值（第 3.2 节） | 未设置 |
| 委托查询 | 已完结的委托在完结 10 分钟后不能按 ID 查询 | 开仓路径依赖按 ID 查询 |
| 条件单改单 | `UpdatePriceTriggeredOrder`（`PUT /futures/{settle}/price_orders/amend/{order_id}`） | 无；撤单函数撤掉该品种全部条件单 |
| 全仓 / 逐仓 | `UpdateContractPositionLeverage`（杠杆与 `marginMode`：`cross` / `isolated`）；`leverage` 为 0 并给 `cross_leverage_limit` 是全仓；`leverage` 非 0 是逐仓 | `SetLeverage` 传非零杠杆（逐仓）；`SetMarginMode` 是空操作。界面设置的全仓不生效（子任务 5 已重写） |
| 杠杆读回 | `GetLeverage` 在单向持仓模式下不能用：必须同时带 `pos_margin_mode` 与 `dual_side` 才返回 200，返回值不反映该合约的设置（三个合约都返回 10）；持仓条目带 `leverage`、`cross_leverage_limit`、`lever`、`pos_margin_mode`，从未设置过的合约没有条目（`GET positions/{contract}` 返回 HTTP 400 + `POSITION_NOT_FOUND`）；逐仓时 `cross_leverage_limit` 残留上一次全仓的值 | 未读取（子任务 5 起由 `PositionMarginSettings` 读回；回撤监控取实际杠杆在子任务 6） |
| 市价单最大张数 | `market_order_size_max`：市价下单支持的最大张数，默认 0；为 0 时取 `order_size_max`（文档 changelog v4.106.7、v4.106.8） | 未校验；子任务 2 起在张数换算中按两者较小值校验 |
| 最小步长的依据 | 官方资料没有写明张数必须是 `order_size_min` 的整数倍。《永续合约市场张数精度调整公告》（2026-03-20）把张数精度调整表述为"最小下单张数"由 1 调为 0.1 | 步长取 `order_size_min` 是 D30 的推论，由 W3 真实下单验证 |
| 请求头的后续 | 公告 48788：默认将改为返回字符串，`X-Gate-Size-Decimal` 随后废弃，生效日期另行公告 | 已按字符串解析，两种情况都兼容 |
| 条件单的小数张 | `initial.size` 是整数，`initial.amount` 是字符串；两者同时给出时以 `amount` 为准 | 子任务 3 的保护单适配须用 `amount` 提交小数张 |

### 3.5 官方 SDK

| 项目 | 事实 |
|---|---|
| 项目使用的版本 | `github.com/gate/gateapi-go/v7 v7.2.184`（2026-11-19 的最新标签；模块路径在 v7.2 系列迁到了 `gate/` 组织下）；`go 1.12`；依赖仅 `github.com/antihax/optional v1.0.0` |
| 下单模型 | `tpsl_tp_trigger_price`、`tpsl_sl_trigger_price`、`market_order_slip_ratio`、`pos_margin_mode`、`action_mode` |
| 条件单 | 初始委托带 `amount`（小数张）；`UpdatePriceTriggeredOrder` 改单；响应带 `order_type`、`pos_margin_mode`、`id_string` |
| 杠杆与持仓模式 | `UpdateContractPositionLeverage`（杠杆 + `marginMode`）、`SetPositionMode`（`single` / `dual` / `dual_plus`）、`UpdatePositionLeverage`、`UpdatePositionCrossMode` |
| 数量字段 | SDK 默认带 `X-Gate-Size-Decimal: 1`，张数类字段（委托、持仓、成交的 `size`、`left`、`close_size`，合约的 `order_size_min`、`order_size_max`）都是字符串 |
| v6.104.3 与 v7.1.8 | 两版合约接口函数完全相同（各 50 个），都没有上面的下单字段与改单接口 |

### 3.7 文档原文留档

合约张数相关的原文摘录留档在子任务 2 的研究文件
`.trellis/tasks/archive/2026-11/11-24-contract-math/research/gate-contract-size-docs.md`，按主题分节并标注来源：
API v4 文档站的根页与 Futures 分页、Agent 文档页、官方 Go SDK 文档（v7.2.184）、两篇 Gate 公告。
文档站在本机只能经网页阅读器抓取（直连被重置），SDK 文档可从官方仓库直接取。

条件单（price_orders）与随单止盈止损的原文摘录与实测结论留档在子任务 3 的研究文件
`.trellis/tasks/archive/2026-11/11-24-protective-order-adapter/research/gate-price-order-docs.md`
（A–G 节：触发规则、order_type、size/amount、查询撤销改单、随单字段、未找到项；H 节：2026-11-24 实测——
仓位级条件单无持仓被拒、API 触价单当前不可改单）。子任务 4、10、11 在接入前先读它。

### 3.6 回撤监控

`trader/auto_trader_risk.go` 每 1 分钟对交易所账户里的每个持仓执行一次：

1. 收益率 = 价格变动百分比 × 杠杆 × 100；杠杆取自适配层返回的值，取不到时按 10 倍计。Gate 适配层返回的是整数类型，监控按浮点数读取会失败，因此恒按 10 倍计。
2. 记录该持仓收益率的峰值。
3. 当前收益率 > 5% 且（峰值 − 当前）÷ 峰值 ≥ 40% 时，市价全平该持仓。

例：峰值收益率 10%（10 倍杠杆下价格朝有利方向走了 1%），回落到 6%（回撤 0.4%）时全平。由于恒按 10 倍计，对实际杠杆低于 10 倍的仓位，价格朝有利方向走出 0.83% 就满足峰值条件。该监控作用于账户内全部持仓，不区分是否由 NOFX 开出。

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
| D23 | 百分比由用户在策略里选择，上下限固定为 1%–20%，步长 0.1%，默认 10% |
| D24 | 下单金额以 USDT 表达，合约张数由系统换算 |
| D25 | 周期选项只用文档列出的周期加 `1w`、`30d` |
| D26 | 其他交易所 trader：分析行情用 Gate，下单取价用该交易所自己的价格，Gate 没有的品种不交易 |
| D27 | 真实下单验证由用户在容器内启动智能体实测完成 |
| D28 | 本轮扩展分批平仓与修改止损止盈 |
| D29 | 交付采用父子任务：父 feature 下拆子任务，串行推进 |
| D30 | 合约张数按该合约的最小步长向下取整（整数张合约为 1 张，小数张合约为 0.1 张）；取整后低于最小下单金额或不足一个最小步长则不下单 |
| D31 | 分批平仓是否执行、平多少由模型按行情决定（D36），模型给出比例（0–100%），系统换算为张数；剩余部分不足一个最小下单单位时拒绝分批 |
| D32 | 止损允许向两个方向移动，止盈允许向两个方向移动（亏损上限见 D34） |
| D33 | 授权在用户现有手动持仓上，用修复后的代码挂一组远离现价的止损止盈，验证后立即撤销；该 key 有交易权限 |
| D34 | 亏损上限：价格到止损价时的亏损不超过本金的 2%，超过即拒绝；开仓与每次修改止损都校验 |
| D35 | 授权第 10.3 节的全部真实写操作（W1–W4），约束见该节 |
| D36 | 分批平仓是否执行、平多少，由模型按行情自行决定，与开仓、平仓的决策方式一致 |
| D37 | 回撤监控保留 |
| D38 | 全仓或逐仓按 trader 界面的设置生效，由用户自行配置 |
| D39 | 第 10 节的子任务拆分与串行顺序确认 |

## 5. 口径与不变量（设计基线）

| 项目 | 定义 | 来源 |
|---|---|---|
| 本金 | `total.amount − total.borrowed`，不含未实现盈亏；已实现盈亏计入 | `GET /wallet/total_balance?currency=USDT` |
| 权益 | 本金 + `total.unrealised_pnl` | 同上 |
| 可用于合约的额度 | `available` | `GET /futures/usdt/accounts` |
| 初始资金 | 创建 trader 时取当时的本金；界面可编辑；存量无效值（不大于 0）在下次加载时以当前本金重建并记录日志 | NOFX |
| 最小下单金额 | 策略配置项，默认 10 USDT | NOFX |
| 开仓名义价值 | 本金 × 策略百分比（1%–20%） | NOFX |
| 行情 | K 线、ticker、持仓量、资金费率全部来自 Gate | Gate |
| 保护不变量 | NOFX 开出的仓位从存在那一刻起就带有止损与止盈：保护单随开仓单一并提交；成交后核对保护单存在，不存在则立即平仓并报错 | NOFX |
| 亏损上限 | 价格到止损价时的亏损 = 名义价值 × 入场价到止损价的距离，不超过本金的 2% | NOFX |

## 6. 对抗式审查发现

| 编号 | 发现 | 证据 | 影响 |
|---|---|---|---|
| F1 | 切换保证金模式不能解决权益为 0 | 单币种与跨币种两种模式下 `total` 都是 0 | 权益取数必须换来源 |
| F2 | 现货与合约共用一份余额 | 资金全在现货侧，合约 `available` 等于现货余额，盈亏结算到现货 | 本金按账户整体取；账户若同时放其他资产，会被计入本金并承担强平风险 |
| F3 | 张数截断与强制 1 张会放大订单 | SOL 1 张约 75U、ETH 1 张约 17.7U | 不足 1 张的订单最大放大约 7.5 倍 |
| F4 | 引擎没有持仓管理动作 | 决策动作只有六个；同标的同方向已有仓拒绝再开 | 已由 D28、D31 确定扩展范围 |
| F5 | 保留其他交易所适配器与"行情全部来自 Gate"之间需要边界 | 下单数量按价格折算 | 已由 D26 确定 |
| F6 | 保护单触发方向四个都与文档相反 | 第 3.4 节 | 开仓后止损、止盈会被交易所拒绝 |
| F7 | 保护单挂失败只记日志 | `executeOpenLongWithRecord` 对 `SetStopLoss`、`SetTakeProfit` 的错误只打日志 | 仓位留在交易所且没有止损 |
| F8 | 保护单 `close: true` 同时带非零 `size`，且未设 `order_type` | 第 3.4 节 | 与文档不符；仓位级保护单才能在分批平仓后继续覆盖剩余仓位 |
| F9 | 止损止盈价不落库，提示词里没有当前止损止盈 | `store/position.go` 无相关字段；持仓信息不含止损止盈 | 模型无法决定是否移动止损 |
| F10 | 撤条件单不区分止损与止盈 | `cancelTriggerOrders` 撤掉该品种全部条件单 | 修改止损时会把止盈一起撤掉 |
| F11 | 百分比范围、最小下单金额与本金互相制约 | 第 6.1 节 | 本金较小时多数百分比组合不会下单 |
| F12 | 回撤监控的杠杆恒按 10 倍计，且对账户内全部持仓生效 | 第 3.6 节：Gate 适配层把杠杆以整数类型返回，监控按浮点数读取失败后回落到 10 倍 | 实际杠杆低于 10 倍时，价格朝有利方向走出 0.83% 就满足触发条件，远早于阈值字面上的收益率；高于 10 倍时触发偏晚；手动持仓也会被处理 |
| F13 | 保证金模式尚未切回 | 接口仍返回 `multi_currency` | 不影响设计；带仓时可能无法切换 |
| F14 | "开仓后补挂"这个结构本身不可靠 | 第 3.4 节：Gate 下单接口支持随单附带止盈止损 | 已由保护不变量取代：保护单随开仓单一并提交 |
| F15 | v6.104.3 与 v7.1.8 都没有实现保护不变量所需的下单字段与改单接口 | 第 3.5 节 | 已在子任务 1 解决：升级到 v7.2.184 |
| F16 | 界面设置的全仓不生效 | `SetMarginMode` 是空操作；`SetLeverage` 传非零杠杆即逐仓 | 实际持仓模式与用户设置不一致 |
| F17 | 止损向亏损方向移动时没有上限 | D32 | 止损可以一直外移，直到当前价格这一侧的边界；已由 D34 确定亏损上限 |
| F18 | 小数张的最小步长取决于请求头 | 带 `X-Gate-Size-Decimal: 1` 时 ETH、SOL 的 `order_size_min` 为 `0.1` 张；不带时返回 0 | 最小步长取自合约信息；W3 验证 0.1 张可成交 |
| F19 | 手动持仓与机器人共用账户 | 回撤监控与模型看到的都是账户内全部持仓 | 机器人运行时，手动持仓会被当作自己的仓位处理 |
| F20 | 回撤监控是按收益率计算的回吐全平，不含趋势判断 | 第 3.6 节：固定阈值、触发即全平 | 触发后仓位已全部平掉，模型的分批平仓与修改止损只在它触发之前有操作空间 |
| F22 | 手动持仓的归属未定 | 适配层的持仓查询返回账户内全部持仓 | 见 U1 |
| F23 | 张数截断用浮点除法 | `int64(quantity / quantoMultiplier)`：0.0003 ÷ 0.0001 得 2 | 子任务 2 改为精确十进制换算 |
| F24 | `GetLeverage` 在单向持仓模式下不可用 | SDK 固定发送空的 `dual_side`，接口返回 `INVALID_PARAM_VALUE`；带上 `dual_side=dual_long` 虽返回 200，但三个合约都返回 10，与持仓条目的 `lever` 不一致（归档子任务 5 的 research G.2） | 杠杆与保证金模式从持仓条目或设置接口的返回值读取 |
| F25 | 已完结的委托在完结 10 分钟后不能按 ID 查询 | 下单接口文档说明 | 开仓后的成交核对以下单返回值为准（`action_mode` 默认 `FULL`），不依赖 `GetOrderStatus` |
| F26 | 卖单部分成交时已成交数量多算一个 `left` | 原式对 `size − left` 取绝对值，而卖单的 `size` 是负数 | 已在子任务 1 修正：先对 `size` 取绝对值再减 `left` |
| F27 | W1 的对象已不存在 | 用户的手动持仓已于 2026-11-23 平掉 | 保护单的读回、改单、撤销改在 W2 的测试仓位上验证 |
| F28 | 市价单另有最大张数 | `market_order_size_max` 为 0 时取 `order_size_max`；实测 SOL 为 200000（单笔上限 300000）、ETH 为 8000000（10000000） | 只按 `order_size_max` 校验会放过交易所拒绝的市价单；子任务 2 起按两者较小值校验 |
| F29 | "步长等于 `order_size_min`"没有官方明文 | 公告只把下单张数精度表述为最小下单张数；没有"整数倍"的说法，也没有不在步长上时交易所如何处理的说明 | 代码从每个合约读步长，不写死 0.1；由 W3 实测确认 |
| F30 | 交易所层面没有最小下单金额的规定 | 文档站、SDK 文档与公告均未见 | 最小下单金额（默认 10 USDT）只是 NOFX 的策略配置 |
| F31 | 分批比例向下取整时，"剩余不足一个最小步长"只出现在比例 100% | 平仓量不超过持仓 × 比例；比例小于 100 时剩余至少一个最小步长 | 子任务 10 的"拒绝分批"针对的是比例为 100% 的分批动作 |
| F32 | 条件单的小数张数量走 `amount` | `initial.size` 是整数，`initial.amount` 是字符串，同时给出时以 `amount` 为准 | 子任务 3 的保护单适配须用 `amount` 提交小数张 |
| F33 | 以 `float64` 传入的名义价值恰为整数个步长、且带着向下的浮点噪声时会少一格 | `10000 × 1.13 / 100` 得 `112.99999999999999`；换算按入参的最短十进制表示取值 | 方向保守，不会放大订单。子任务 9 计算名义价值时用不出噪声的算法（本金 × 百分比的十分位整数 ÷ 1000），或改传十进制字符串 |
| F34 | 同一合约接口有两套缓存与两种 `quanto_multiplier` 解析并存 | 子任务 2 新增 `GetContractSpec`（缓存 10 分钟、精确解析）；旧的 `getContract` 永久缓存、按浮点解析 | 子任务 4 接入时统一到 `GetContractSpec`，删除旧的 `getContract` 与各处按浮点读取 `quanto_multiplier` 的代码 |
| F35 | 平仓换算与币数量换算不校验市价单最大张数；合约元数据不含合约状态 | `SizeForCloseRatio`、`ContractsFromQuantity` 只按步长换算；`ContractSpec` 不读 `in_delisting`、`status` | 子任务 4（开仓前）与子任务 10（全平仓）接入时按需补上 |
| F36 | 不显式设置时，首次建仓的合约在本账户下落到全仓 + 该合约的最大杠杆 | 子任务 4 的 W2/W3 传杠杆 0、没有调用设置接口，之后 DOGE_USDT、ETH_USDT 的条目是全仓 75x、200x（不是 `cross_leverage_default` 的 10） | 已由子任务 5 消除：开仓前按 trader 的设置生效并读回，失败不开仓。助手的普通市价原语在 `leverage=0` 时仍会落到交易所默认设置（生产调用方都要求杠杆大于 0） |
| F37 | 超过杠杆上限的 label 是 `LEVERAGE_EXCEEDED`，不在文档的 label 表里；从没有设置过的合约上 `set_leverage` 的行为没有实测 | W4 实测（归档子任务 5 的 research G.3、G.4） | 适配器不按 label 分支，错误原样透出；首次开新合约的路径只有单测模拟，交易所若拒绝则开不了仓（闭合） |

### 6.1 百分比、最小下单金额与本金的制约

按 D18、D19、D22、D23、D24、D30，名义价值 = 本金 × 百分比；张数按该合约的最小步长向下取整，取整后低于最小下单金额（10 USDT）则不下单。下表按 2026-11-24 的价格计算（第 3.2 节）。

| 本金 | 百分比 | 名义价值 | BTC | ETH | SOL | ZEC | DOGE | HYPE | XAU |
|---|---:|---:|---|---|---|---|---|---|---|
| 100U | 5% | 5.0U | 不下单 | 不下单 | 不下单 | 不下单 | 不下单 | 不下单 | 不下单 |
| 100U | 10% | 10.0U | 不下单（1 张 6.0U） | 不下单（0.5 张 8.5U） | 不下单（0.1 张 7.1U） | 不下单（1 张 7.6U） | 不下单（17 张 9.5U） | 不下单（1 张 6.3U） | 不下单（27 张 9.8U） |
| 100U | 20% | 20.0U | 3 张 18.1U | 1.1 张 18.6U | 0.2 张 14.2U | 2 张 15.1U | 35 张 19.6U | 3 张 18.9U | 54 张 19.7U |
| 107.5U（当前本金） | 10% | 10.75U | 不下单（1 张 6.0U） | 0.6 张 10.2U | 不下单（0.1 张 7.1U） | 不下单（1 张 7.6U） | 19 张 10.6U | 不下单（1 张 6.3U） | 29 张 10.6U |
| 107.5U | 20% | 21.5U | 3 张 18.1U | 1.2 张 20.3U | 0.3 张 21.3U | 2 张 15.1U | 38 张 21.2U | 3 张 18.9U | 59 张 21.5U |
| 500U | 10% | 50.0U | 8 张 48.2U | 2.9 张 49.1U | 0.7 张 49.8U | 6 张 45.3U | 89 张 49.7U | 7 张 44.2U | 137 张 49.9U |

当前本金下，各合约能下单的最小百分比：

| 合约 | 满足 10 USDT 的最小下单 | 所需百分比 |
|---|---|---:|
| DOGE | 18 张 10.1U | 9.4% |
| ETH | 0.6 张 10.2U | 9.5% |
| XAU | 28 张 10.2U | 9.5% |
| BTC | 2 张 12.0U | 11.3% |
| HYPE | 2 张 12.6U | 11.8% |
| SOL | 0.2 张 14.2U | 13.3% |
| ZEC | 2 张 15.1U | 14.1% |

## 7. 待确认问题

| 编号 | 问题 | 推荐 |
|---|---|---|
| U1 | 账户里不是本 trader 开出的持仓（手动仓）是否归机器人管 | 不归：对模型不可见，回撤监控也不处理；trader 启动时账户里存在这类持仓则拒绝启动并说明 |

## 8. 范围与删除清单

### 8.1 SDK 与下单保护

| 位置 | 现有逻辑 | 处理 |
|---|---|---|
| `go.mod` 及 7 个引用文件 | `gateapi-go/v6 v6.104.3` | 升级到 `github.com/gate/gateapi-go/v7 v7.2.184` 并迁移（已在子任务 1 完成） |
| `trader/gate/trader_orders.go` `SetStopLoss`、`SetTakeProfit` | 触发规则与文档相反；`close: true` 带非零 `size`；未设 `order_type` | 删除；保护单随开仓单一并提交，修改时用条件单改单 |
| `trader/gate/trader_orders.go` `SetLeverage`、`SetMarginMode` | 恒为逐仓；保证金模式为空操作 | 按 D38 重写（子任务 5 已完成：`trader/gate/margin_settings.go`） |
| `trader/gate/trader_orders.go` `OpenLong`、`OpenShort`、`CloseLong`、`CloseShort` | 张数截断与强制 1 张 | 按 D24、D30 重写 |
| `trader/gate/trader_orders.go` 市价开仓 | 无滑点上限；无随单止盈止损 | 按 D8 与保护不变量 |
| `trader/auto_trader_orders.go` | 开仓后补挂保护单且失败只记日志；四处 `market.GetWithExchange` 取价 | 删除补挂；改用 `GetMarketPrice`；成交后核对保护单 |
| `trader/gate/trader_account.go` `GetPositions`、`trader/auto_trader_risk.go` 回撤监控 | 杠杆以整数类型返回；监控取不到杠杆时恒按 10 倍计；处理账户内全部持仓 | 按 D37：杠杆取实际值，只处理本 trader 开出的仓位（按 U1） |

### 8.2 账户权益

| 位置 | 现有逻辑 | 处理 |
|---|---|---|
| `trader/gate/trader_account.go` `GetBalance` | 以 `total` 作钱包余额 | 改为钱包总额与合约 `available` |
| `trader/auto_trader_loop.go` | 权益 = `totalWalletBalance + totalUnrealizedProfit` | 按第 5 节口径 |
| `trader/auto_trader.go` | `InitialBalance` 取数与校验 | 按第 5 节口径 |

### 8.3 仓位

| 位置 | 现有逻辑 | 处理 |
|---|---|---|
| `store/strategy.go` | 两个仓位价值倍数字段及其 clamp 与调整提示；`min_position_size` 默认 12；`normalizeTimeframe` 接受未列周期 | 删除倍数字段；新增百分比（1–20，默认 10）；`min_position_size` 默认 10；按 D25 收敛周期 |
| `kernel/engine_position.go` | 硬编码最小下单（BTC/ETH 60、其他 12）；`SL + (TP-SL)*0.2` 的盈亏比校验；权益 × 倍数的上限 | 删除盈亏比校验与硬编码最小下单；金额由代码计算 |
| `kernel/engine_prompt.go` | "仓位大小指引"段；`position_size_usd`、`risk_usd` 输出契约 | 按第 5 节重写 |
| 前端策略编辑器、`agent/strategy_field_catalog.go` 等 | 引用上述字段与周期 | 同步 |

### 8.4 持仓管理

| 位置 | 现有逻辑 | 处理 |
|---|---|---|
| `kernel/engine_prompt.go`、`kernel/engine_position.go` | 六个动作 | 新增分批平仓与修改止损止盈 |
| `trader/auto_trader_decision.go`、`trader/auto_trader_orders.go` | 无对应执行路径 | 新增 |
| `trader/gate/trader_orders.go` | 无条件单改单；撤条件单不分止损止盈 | 新增改单 |
| 持仓信息 | 不含当前止损止盈 | 每轮从未触发条件单读入 |

### 8.5 行情

| 位置 | 现有逻辑 | 处理 |
|---|---|---|
| `market/data_klines.go`、`market/data.go` | CoinAnk K 线取数；旧 3m/4h 内嵌路径（`Get`、`GetWithExchange`） | 改 Gate 合约 K 线；删除旧路径及只为它服务的结构（`IntradaySeries`、`LongerTermContext`、`BuildDataFromKlines`、`Format`，以切换后无调用方为准） |
| `provider/coinank/coinank_api/` | `kline.go`、`kline_ws.go`；无调用方的 `depth_ws.go`、`WsConn` | 删除 |
| `api/handler_klines.go` | 加密货币 K 线走 CoinAnk，按交易所映射，支持秒级周期 | 改 Gate 直连；删除映射与秒级周期。股票（Alpaca）、外汇与金属（TwelveData）路由保留 |
| 前端周期选项 | 接受 `3m/2h/6h/12h/3d` | 按 D25 移除 |
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
5. 验证方式：单元测试、只读的真实接口验证，以及 D35 授权范围内的真实写操作验证。

## 10. 实施计划

### 10.1 拆分原则

- **先适配、后接入**：先交付纯新增的外部接口适配与纯函数，行为不变，各自用真实接口验证；再接入交易路径并删除旧代码。
- **可独立完成**：每个子任务只依赖已合入父分支的成果，输入在启动时全部具备。
- **可闭环验证**：每个子任务有客观的通过标准（单元测试、真实接口读数、D35 范围内的真实写操作），不依赖主观判断。
- **失败易重试**：改动面局限在单一关注点，失败时返工不外溢。
- **安全性不变量**：任何子任务合入后，父分支都不得出现"能开仓但没有有效止损"的状态。恢复开仓能力的子任务排在开仓路径加固之后。

### 10.2 父子任务

父分支：`feat/2026_1123_gate-trading-foundation`。
子分支：`feat/2026_1123_gate-trading-foundation--<slug>`，worktree 位于 `nofx-wt/feat-2026-1123-gate-trading-foundation--<slug>`。串行推进，前三个子任务合入后运行中的 trader 行为不变。

| 顺序 | 子任务 | 内容 | 闭环验证 | 依赖 |
|---|---|---|---|---|
| 1 | `gate-sdk-v7` SDK 升级 | 升级到 `github.com/gate/gateapi-go/v7 v7.2.184`，迁移 7 个引用文件，张数字段按字符串解析 | 全量 `go test`；只读真实读数（账户、持仓、委托、条件单、行情、合约信息） | 无 |
| 2 | `contract-math` 张数换算 | 纯函数：USDT 名义价值换算为下单数量（小数张、整数张向下取整、最小可交易额）；分批比例换算为张数；读取合约元数据 | 单测矩阵；7 个合约元数据真实读数 | 1 |
| 3 | `protective-order-adapter` 保护单适配 | 纯新增：仓位级止损止盈构造、随单附带止盈止损、查询未触发条件单、条件单改单、撤销 | 单测构造矩阵（方向 × 止损止盈 → `rule`、`order_type`、`size`）；真实挂、查、改、撤 | 1 |
| 4 | `open-position-safety` 开仓路径加固 | 接入 2、3：取价改 `GetMarketPrice`、张数换算、随单保护、滑点上限、成交后核对保护单、缺失即平仓；删除开仓后补挂与 CoinAnk 取价 | 单测；真实最小开仓全流程 | 2、3 |
| 5 | `unified-trading-params` 杠杆与保证金模式 | 按 trader 设置生效全仓或逐仓，使用文档的新接口 | 单测；闲置合约上真实设置并读回 | 1、4 |
| 6 | `drawdown-monitor` 回撤监控 | 杠杆取持仓的实际值；只处理本 trader 开出的仓位，启动时账户里有其他持仓则拒绝启动（按 U1）；全平走张数换算 | 单测（杠杆取值、阈值判定、仓位归属）；带仓的只读真实读数 | 2、5 |
| 7 | `account-equity` 账户权益与本金 | 钱包总额与合约 `available` 取数；权益、本金口径；初始资金及界面编辑 | 单测；带仓与不带仓的只读真实读数；提示词预览 | 1 |
| 8 | `strategy-config` 策略配置 | 百分比（1–20，默认 10）、最小下单默认 10、删除倍数字段、周期收敛；前端编辑器与助手字段目录 | 单测；前端测试与构建 | 无 |
| 9 | `sizing-engine` 金额计算 | 金额由代码计算；校验，含开仓时的亏损上限（D34）；提示词契约去掉金额字段；删除盈亏比校验与硬编码最小下单 | kernel 单测；提示词预览；`/strategies/test-run` | 6、7 |
| 10 | `partial-close` 分批平仓 | 决策契约新增分批平仓，比例由模型给出（D36）；张数换算；持仓信息标明该仓位能否分批；剩余不足一个最小步长时拒绝 | 契约解析与换算单测；真实分批平仓 | 4、8 |
| 11 | `protective-order-management` 止损止盈管理 | 每轮读入当前止损止盈；决策契约新增修改动作；合法性校验；改单执行；修改止损时的亏损上限（D34） | 单测；真实改单 | 3、4、8 |
| 12 | `market-source` 行情数据源 | 纯新增：Gate 合约 K 线（周期映射含 `1w`，成交量取 `sum`）与 ticker 取数 | 单测；7 个标的 × 全部保留周期的真实取数 | 1 |
| 13 | `engine-klines-gate` 引擎 K 线切换 | 决策 K 线与网格长周期箱体改 Gate；删除 CoinAnk K 线与旧内嵌路径 | 单测；策略试运行显示 Gate 数据 | 11 |
| 14 | `chart-klines-gate` 图表 K 线切换 | `/api/klines` 加密货币改 Gate；删除交易所映射与秒级周期；前端去掉 `exchange` | 单测；前端测试与构建 | 11 |
| 15 | `assistant-market` 助手行情切换 | 24h 行情、K 线工具、简报、价格告警改 Gate；删除两个接口与 Binance 取数函数 | 单测；工具真实调用 | 11 |
| 16 | `scan-alignment` 扫描对齐 | 调度改为墙钟网格，收线后固定延迟触发 | 单测 | 无 |
| 17 | `infra-hygiene` 基础设施 | 健康检查统一；`.dockerignore`；Makefile 主栈重建命令（含运行中 trader 的拦截） | 配置解析检查；构建上下文不含本地文件 | 无 |

### 10.3 真实写操作验证清单

全部在交易子账户上执行，单笔测试名义价值不超过 10 USDT，每一步完成后立即还原，不在用户有手动持仓的合约上开新仓。W1–W4 均已授权（D33、D35）。账户在 2026-11-24 复核时无持仓（F27），原定在手动持仓上做的保护单验证并入 W2。

| 编号 | 操作 | 对应子任务 |
|---|---|---|
| W1 | 已并入 W2 | — |
| W2 | DOGE_USDT 开 17 张（约 9.5U，带随单止盈止损）→ 核对保护单 → 分批平仓 → 改止损止盈 → 全平 → 确认无残留委托。这里列的是验证步骤；实盘里是否分批、平多少由模型按行情决定（D36） | 4、10、11 |
| W3 | ETH_USDT 以 0.1 张（约 1.7U）开仓并平仓，验证最小步长与随单止盈止损 | 4 |
| W4 | 在一个未持仓的合约上设置全仓、逐仓与杠杆，从设置接口的返回值与持仓条目读回，结束后还原原设置 | 5 |

### 10.4 父任务收口

1. 逐项对照本文件做对抗式审查并修正。
2. 串联全部子任务做端到端验收与完整项目门禁。
3. 合入 dev-offline 并重建送测栈；用户在容器内启动智能体做真实下单实测。
4. 合入 main，重建主栈并核对没有 trader 被自动拉起。
5. 改写主栈两个 K哥策略的提示词（数据语义、仓位、持仓管理动作、盈亏比表述）。
6. 删除未使用镜像，归档证据。

### 10.5 实施进度

- [x] 1 `gate-sdk-v7`：`32f63003`，全量 `go test` 通过，只读真实读数 19 项全部成功
- [x] 2 `contract-math`：`590ba577`，单测矩阵通过，7 个合约元数据真实读数全部一致（`failed=0 mismatched=0`），`make test` 与 `make build` 通过；独立检查通过
- [x] 3 `protective-order-adapter`：`12c2a249`（381d7f02 + 5f5160a4），单测矩阵通过，真实挂、查、撤与随单止盈止损全部成功（`LIVECHECK_PASS steps=10`，终态零残留）；改单被交易所拒绝（新路径 404 未部署、旧路径 code 1077 拒绝 API 触价单，research H.2），撤旧建新替代路径验证通过；仓位级条件单无持仓被拒（H.1），其真实验证并入 W2；独立检查 PASS（1 P2 + 6 P3 已修复），`make test` 与 `make build` 通过
- [x] 4 `open-position-safety`：`60db88da`（0df83b9a + 9156ec34 + 7850ab5d + 7e45d47f），门禁 `git diff --check`、`make test`、`make build` 全过；W2（DOGE_USDT 17 张随单保护开仓→核对→全平→零残留）与 W3（ETH_USDT 0.1 张最小步长开平）实测 `LIVECHECK_PASS`；独立检查 1 P1 + 1 P2 + 4 P3 全部修复并复核 PASS。实测发现：随单 tpsl 是委托级保护，小数张合约（ETH）以 `close-long-order` 出现在未触发条件单列表（可按 ID 撤销、分类正确），整数张合约（DOGE）不出现在列表——`VerifyOrderProtection` 以委托回读 + 列表双路径覆盖两种形态；子任务 10、11 的管理路径设计须先复核此行为（证据见归档任务的 `probe/README.md`）
- [x] 5 `unified-trading-params`（W4）：`ccd2f51c`（b8252754 + d92328b9 + 4eedd743 + d758d3ec + eb84f2e4 + e1b6fdc3 + aa06dc75），最终候选 `e1b6fdc3` 上 `git diff --check`、`make test`、`make build` 全过；W4（DOGE_USDT 全仓 / 逐仓 × 10x / 5x / 3x 设置并读回、重复设置、超上限被拒、两个旧方法）在 `d758d3ec` 上实测 `LIVECHECK_PASS`（23 步 + 1 条观察），DOGE_USDT 还原为全仓 75x，BTC_USDT、ETH_USDT 条目不变，可用余额前后一致。交付：`trader/gate/margin_settings.go`（`ApplyMarginSettings` 走新接口 `positions/{contract}/set_leverage`，以返回值或持仓条目读回一致为成功；`PositionMarginSettings`；重写的 `SetLeverage`、`SetMarginMode`）、能力接口 `types.MarginSettingsApplier`、`ProtectedOpenRequest.IsCrossMargin`；随单保护开仓在下单前生效设置、失败不开仓；网格初始化在设置生效后才标记已初始化。独立检查首轮 NEEDS FIX（无 P0/P1），修复后复核 PASS；变异实验的 53 个可编译变体最终全部被测试杀死（其中 3 个由主会话修复后复验）。实测发现：(a) `set_leverage` 线上已部署，单向持仓模式下不带 `dual_side`；(b) 逐仓条目的 `cross_leverage_limit` 残留旧值，读回用 `lever` 与同模式旧字段互相印证；(c) 重复设置返回 200；(d) 超上限的 label 为 `LEVERAGE_EXCEEDED`，不在文档 label 表（F37）；(e) `get_leverage` 在单向持仓模式下不能用；(f) 不显式设置时首次建仓的合约在本账户落到全仓 + 最大杠杆（F36）；(g) 账户仍是跨币种模式（`margin_mode=1`），逐仓在该模式下可用，单币种下的读回待账户切换后复验。未实测（归档任务 design.md 已知边界）：从没有设置过的合约上的 `set_leverage`（要留一条无法还原的条目，需用户授权）、带仓或有挂单时能否修改设置。对后续子任务的提示：⑥ 回撤监控取实际杠杆读条目的 `lever`（`GetPositions` 现在仍返回整数 `leverage`，全仓为 0），`PositionMarginSettings` 可直接读回合约的设置；设置是合约级的，同一合约上已有反向持仓时，开仓前生效设置会同时改动那个仓位
- [ ] 6 `drawdown-monitor`（等 U1）
- [ ] 7 `account-equity`
- [ ] 8 `strategy-config`
- [ ] 9 `sizing-engine`
- [ ] 10 `partial-close`（W2 分批平仓）
- [ ] 11 `protective-order-management`（W2 改止损止盈）
- [ ] 12 `market-source`
- [ ] 13 `engine-klines-gate`
- [ ] 14 `chart-klines-gate`
- [ ] 15 `assistant-market`
- [ ] 16 `scan-alignment`
- [ ] 17 `infra-hygiene`
