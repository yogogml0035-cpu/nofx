# Gate 交易基座：统一账户权益、仓位与下单、行情全切 Gate

> 状态：实施中（2026-11-26）。子任务 1–17 已全部合入父分支；U1 已确认（D40）；W5 已实测 `LIVECHECK_PASS`（D41）。进度见第 10.5 节；下一步父任务收口（多智能体对抗审查、`make ci-local`、串联 E2E 与无 AI 冒烟、dev-offline 集成）。
> 依赖表更正（2026-11-25）：⑬⑭⑮ 的依赖列原写 ⑪（protective-order-management），实际消费 ⑫ `market-source` 的 Gate 取数；实施顺序 12→13→14→15 已保证，依赖列按 ⑫ 更正。
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
| 本金不含浮盈亏 | 带持仓读数中浮盈亏变动，`total.amount` 不变。「可用额度 = `amount + unrealised_pnl − position_initial_margin`」只是量级旁证：残差 −0.0003…+0.0001 且不恒定（F40），可用额度直接取合约账户的 `available` | `GET /wallet/total_balance`、`GET /futures/usdt/accounts`；子任务 7 带仓实测 |
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
| `1w`、`30d` | 仅在参数说明中出现 | 可用；`1w` 为自然周（周一起算）；`30d` 是自然月（相邻开盘 28–31 天，⑫ 实测 2026-11-25）；limit 覆盖全部历史时首根为每月 1 日，小 limit 返回非月初窗口 |
| `3m 2h 6h 12h 3d` | 未出现 | 可用 |

### 3.4 下单与保护

「NOFX 现状」列记录的是 2026-11-24 开工时的状态；各行的处理进度见第 10.5 节。

| 项目 | 文档（合约分组页、官方 SDK 模型） | NOFX 现状（`trader/gate/trader_orders.go`） |
|---|---|---|
| 条件单 `rule` | 1 = 价格 ≥ 触发价（触发价须高于最新价）；2 = 价格 ≤ 触发价（触发价须低于最新价） | 多头止损、空头止盈用 1；空头止损、多头止盈用 2——四个方向均与文档相反 |
| 全平仓 | `close: true` 时 `size` 应为 0 | `close: true` 同时带非零 `size` |
| 仓位止盈止损 | `order_type` 取 `close-long-position` / `close-short-position`，随仓位总量生效 | 未设置 `order_type` |
| 随单附带止盈止损 | 下单接口可选 `tpsl_tp_trigger_price`、`tpsl_sl_trigger_price` | 开仓后另发两次调用补挂，失败只记日志 |
| 触发价对齐 | 条件单触发价必须是 `order_price_round` 的整数倍。随单 tpsl 的触发价不对齐时，交易所接受委托上的 `tpsl_*` 字段，但**不创建条件单**（子任务 11 真实探针，F44）；仓位级条件单直接返回 `AUTO_INVALID_PARAM_TRIGGER_PRICE` | 开仓随单保护未做对齐；子任务 11 起所有触发价按合约步长对齐，保护核对以未触发条件单列表为准 |
| 小数张 | 下单的 `size` 以十进制字符串提交并带 `X-Gate-Size-Decimal: 1`；条件单的初始委托另有等价的 `amount` 字段，两者同时给出时以 `amount` 为准 | 未使用 |
| 市价单滑点 | `market_order_slip_ratio`；不传时用合约的默认值（第 3.2 节） | 未设置 |
| 委托查询 | 已完结的委托在完结 10 分钟后不能按 ID 查询 | 开仓路径依赖按 ID 查询 |
| 条件单改单 | `UpdatePriceTriggeredOrder`（`PUT /futures/{settle}/price_orders/amend/{order_id}`） | 无；撤单函数撤掉该品种全部条件单 |
| 全仓 / 逐仓 | `UpdateContractPositionLeverage`（杠杆与 `marginMode`：`cross` / `isolated`）；`leverage` 为 0 并给 `cross_leverage_limit` 是全仓；`leverage` 非 0 是逐仓 | `SetLeverage` 传非零杠杆（逐仓）；`SetMarginMode` 是空操作。界面设置的全仓不生效（子任务 5 已重写） |
| 杠杆读回 | `GetLeverage` 在单向持仓模式下不能用：必须同时带 `pos_margin_mode` 与 `dual_side` 才返回 200，返回值不反映该合约的设置（三个合约都返回 10）；持仓条目带 `leverage`、`cross_leverage_limit`、`lever`、`pos_margin_mode`，从未设置过的合约没有条目（`GET positions/{contract}` 返回 HTTP 400 + `POSITION_NOT_FOUND`）；逐仓时 `cross_leverage_limit` 残留上一次全仓的值 | 子任务 5 起由 `PositionMarginSettings` 读回；子任务 6 起持仓列表的杠杆读 `lever`（同模式旧字段兜底，读不出按 0，监控回落 10 倍） |
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

`trader/auto_trader_risk.go` 每 1 分钟对本 trader 开出的每个持仓执行一次（子任务 6 已按此交付）：

1. 收益率 = 价格变动百分比 × 杠杆 × 100；杠杆取自适配层返回的实际值——Gate 持仓列表读条目的 `lever`（同模式旧字段兜底），读不出按 0（不可用）计，监控侧此时按 10 倍计（F12 已修复：不再恒按 10 倍）。
2. 记录该持仓收益率的峰值。
3. 当前收益率 > 5% 且（峰值 − 当前）÷ 峰值 ≥ 40% 时，市价全平该持仓（走适配器 `close=true + size=0` 的全平原语）。

例：峰值收益率 10%（10 倍杠杆下价格朝有利方向走了 1%），回落到 6%（回撤 0.4%）时全平。不是本 trader 开出的持仓直接无视：不进入计算、不记峰值、不平仓，也不因此拒绝 trader 启动（D40）。

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
| D26（2026-11-26 补充：随单保护开仓目前只有 Gate 实现，其他交易所的 AI trader 开仓会被拒绝，见 F59） | 其他交易所 trader：分析行情用 Gate，下单取价用该交易所自己的价格，Gate 没有的品种不交易 |
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
| D40 | U1 已确认（2026-11-25）：不是本 trader 开出的持仓不归机器人管，出现时直接无视——沿用原推荐的口径（对模型不可见，回撤监控不处理），不管理、不平仓；与原推荐不同的是不因此拒绝 trader 启动（"启动时存在这类持仓则拒绝启动并说明"不采用）。用户的交易子账户后续不再手动开仓，这类持仓正常情况下不会出现 |
| D41 | 授权 W5（2026-11-25）：在一个从没有设置过仓位设置的合约上调用 `set_leverage` 并读回。这会在该合约上留下一条仓位设置条目，无法还原为"没有条目"；其余约束同第 10.3 节 |

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
| F12 | 回撤监控的杠杆恒按 10 倍计，且对账户内全部持仓生效 | 第 3.6 节：Gate 适配层把杠杆以整数类型返回，监控按浮点数读取失败后回落到 10 倍 | 已由子任务 6 解决：杠杆读 `lever` 实际值（读不出才回落 10 倍），只处理本 trader 开出的仓位（D40） |
| F13 | 保证金模式尚未切回 | 接口仍返回 `multi_currency` | 不影响设计；带仓时可能无法切换 |
| F14 | "开仓后补挂"这个结构本身不可靠 | 第 3.4 节：Gate 下单接口支持随单附带止盈止损 | 已由保护不变量取代：保护单随开仓单一并提交 |
| F15 | v6.104.3 与 v7.1.8 都没有实现保护不变量所需的下单字段与改单接口 | 第 3.5 节 | 已在子任务 1 解决：升级到 v7.2.184 |
| F16 | 界面设置的全仓不生效 | `SetMarginMode` 是空操作；`SetLeverage` 传非零杠杆即逐仓 | 实际持仓模式与用户设置不一致 |
| F17 | 止损向亏损方向移动时没有上限 | D32 | 止损可以一直外移，直到当前价格这一侧的边界；已由 D34 确定亏损上限 |
| F18 | 小数张的最小步长取决于请求头 | 带 `X-Gate-Size-Decimal: 1` 时 ETH、SOL 的 `order_size_min` 为 `0.1` 张；不带时返回 0 | 最小步长取自合约信息；W3 验证 0.1 张可成交 |
| F19 | 手动持仓与机器人共用账户 | 回撤监控与模型看到的都是账户内全部持仓 | 机器人运行时，手动持仓会被当作自己的仓位处理 |
| F20 | 回撤监控是按收益率计算的回吐全平，不含趋势判断 | 第 3.6 节：固定阈值、触发即全平 | 触发后仓位已全部平掉，模型的分批平仓与修改止损只在它触发之前有操作空间 |
| F22 | 手动持仓的归属未定 | 适配层的持仓查询返回账户内全部持仓 | 已定（D40）：不归机器人管，出现时直接无视 |
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
| F36 | 不显式设置时，首次建仓的合约在本账户下落到全仓 + 该合约的最大杠杆 | 子任务 4 的 W2/W3 传杠杆 0、没有调用设置接口，之后 DOGE_USDT、ETH_USDT 的条目是全仓 75x、200x（不是 `cross_leverage_default` 的 10） | 已由子任务 5 消除：开仓前按 trader 的设置生效并读回，失败不开仓。助手的普通市价原语在 `leverage=0` 时仍会落到交易所默认设置（生产调用方都要求杠杆大于 0）。助手在对话里直接下单不跟随 trader 的全仓/逐仓开关，沿用合约当前设置：2026-11-25 向用户说明后保持现状，用户以后实际用助手下单时再议 |
| F37 | 超过杠杆上限的 label 是 `LEVERAGE_EXCEEDED`，不在文档的 label 表里；从没有设置过的合约上 `set_leverage` 的行为没有实测 | W4 实测（归档子任务 5 的 research G.3、G.4） | 适配器不按 label 分支，错误原样透出；首次开新合约的路径已由 W5 实测闭合（2026-11-26，SOL_USDT 无条目上全仓/逐仓/杠杆设置与读回全部一致，`LIVECHECK_PASS`，证据见父任务 `probe/README.md`） |
| F38 | 统一账户下合约账户的 `total` 不是本金：读数 ≈ 0，与「合约侧自上次结算以来的未结算净额（手续费 + 已实现盈亏）」对得上；清零的触发条件没有验证（文档也没写它在统一账户下的含义） | 子任务 7 的三轮带仓读数（归档任务 research H.4、probe/README.md 实测结论 4） | 旧口径把它当钱包余额 → 权益 ≈ 0、开不了仓。已由子任务 7 根除：本金改取钱包总额，合约账户只读 `available` |
| F39 | 本金（钱包总额 `total.amount − total.borrowed`）不随未实现盈亏变动，但手续费与已实现盈亏在开平仓那一刻即时结算；钱包总额的 `unrealised_pnl` 13 秒采样看不到「1 分钟缓存」滞后 | 子任务 7 第 4 轮带仓读数：7 次采样本金恒定、开仓降幅 = 开仓手续费（差 1e-8）、平仓 5 秒与 70 秒读数相同 | 「权益 = 本金 + 未实现盈亏」成立；初始资金创建时取本金，读不到或本金不为正时按类型失败闭合 |
| F40 | 「可用额度 = `amount` + `unrealised_pnl` − `position_initial_margin`」不严格成立：残差 −0.0003…+0.0001（约 1.7U 仓位上到 −0.0016）且不恒定，交易所内部的计算方式没有文档 | 子任务 7 第 4 轮带仓读数的量级校验（阈值 0.01 内通过，残差不恒定） | 可用额度直接取合约账户的 `available`，公式只作量级旁证；§3.1 的表述已同步更正 |
| F41 | ⑧ 交付时新增的许可周期 `10s`/`7d`/`30d` 在当前引擎取数链路（CoinAnk/市场旧路径）没有实现；选了这些周期的策略运行时按既有「逐币 log 跳过」路径失败，不崩溃 | ⑧ 独立检查 P3 留档（D25 定义的是配置选项集合，取数在 ⑫ `market-source`/⑬ `engine-klines-gate` 落地） | 父 feature 是整体送测/交付（⑫⑬ 均排在合入 main 之前），不构成 ⑧ 缺陷；⑬ 验收须覆盖新周期的真实取数 |
| F42 | `position_percent` 同名异义：新策略字段 `risk_control.position_percent`（开仓金额百分比，D23）与网格风险展示 `web/src/types/trading.ts` 的 `RiskInfo.position_percent`（持仓占用率）无关 | ⑧ 检查期间的跨层检索确认 | 不改名、不在 ⑧ 范围；后续检索/沟通时必须带前缀区分 |
| F43 | 分批平仓后，DOGE_USDT 的未触发条件单列表出现 2 条；同一合约在开仓后（分批前）为 0 条。**⑪ 已辨明**：这 2 条就是随单止盈止损生成的委托级条件单（`close-long-order`，各 1 条）。④ W2 时列表为空是因为触发价未对齐价格步长（F44），交易所在委托上保留 `tpsl_*` 字段但没有创建条件单 | ⑩ W2 两次实测；⑪ 探针 `probe-2026-11-25`（对齐后列表立即出现 2 条、可按 ID 撤销） | 保护核对必须以条件单列表为准；⑪ 已在开仓路径对齐触发价并收紧核对 |
| F44 | 随单 tpsl 的触发价未对齐 `order_price_round` 时，交易所**静默**接受委托上的 `tpsl_*` 字段但不创建条件单：仓位从开出起就没有任何保护，而只读委托字段的 `VerifyOrderProtection` 会判为受保护 | ⑪ 真实探针（2026-11-25，DOGE_USDT tick `0.00001`：SL `0.048915` → 列表空；SL `0.04884`/TP `0.0597` → 列表 2 条）；④ W2 的 SL `0.049104` 同样未对齐 | ⑪ 已根除：开仓、改单的触发价按合约步长对齐（止损收紧、止盈远离）；`VerifyOrderProtection` 改为以未触发条件单列表为准，委托 `tpsl_*` 只作旁证；持仓已不存在视为无需保护 |
| F45 | 随单保护单（`close-*-order`）是 reduce-only（`is_reduce_only=true`），可按 ID 撤销；分批平仓后它的张数仍是开仓时张数、触发价不变，触发时按剩余仓位收口，不会反向开仓 | ⑪ 触发探针（2026-11-25：3 张开仓、分批剩 2 张、随单保护 `size=-3` 且 `is_reduce_only=true`，SL 触发后仓位归零、无反向持仓） | ⑩ 的“分批平仓不撤不改保护单”可保留；⑪ 的改单按类型撤销旧单（含委托级）再挂仓位级新单即可 |
| F46 | 主周期与 1h/4h 目标窗口不匹配时，`PriceChange1h/4h` 退回可用窗口：10s 主周期最多约 33 分钟（200 根），7d/30d 主周期为 1 根；提示词字段名仍为 1h/4h，读数是近似而非真实 1h/4h | ⑬ 实现与 design 留档 | 数据可用但不精确，属已知边界；需要精确 1h/4h 变化时调用方应额外取对应周期 K 线（后续可评估） |
| F47 | Gate 的 `30d` K 线在小 limit 下不可靠：返回非月初窗口并夹杂无成交占位根（价格 0.1268、v=0、sum=0）；limit=200/2000 时为月初对齐的真实月线（1w/7d 小 limit 正常） | ⑮ 真实探针（2026-11-25）与 Gate 原始接口对照 | `GetGateKlines` 对 30d 小 limit 先按 2000 拉全历史再截取最后 limit 根；其他周期不变 |
| F48 | 助手直接下单与网格开仓不带随单保护：`agent/tools.go` 的 `execute_trade`（`AllowTradeExecution` 默认 true）、`agent/trade.go` 的文本下单命令走适配器的普通市价原语；网格策略按网格挂单，自带 `stop_loss_pct` 等风控字段而不是随单条件单 | 父任务对抗审查（视角 B，2026-11-26）；F36 已记录助手下单为「向用户说明后保持现状」的例外 | 这些入口开出的仓位没有止损止盈，与第 5 节保护不变量的字面范围不符；trader 路径（`ProtectedOpen`）不受影响。本次收口不改行为，需用户知情；要让所有入口都带保护须单独立项（助手下单需要新增止损止盈入参并在技能里收集）；F48 补充：助手开仓也不受 2% 亏损上限与开仓金额百分比约束（名义上限为权益 100% 与 100k USDT 取小，达到权益 25% 或 5000 USDT 需大额确认），这些仓位在系统内没有补挂止损的路径；Gate 上的网格实际无法开仓（适配器的 `SetStopLoss` 恒返回错误）；其他交易所的网格软件止损现在每根主周期 K 线检查一次（原为每 ScanInterval 一次）。能开出不带保护仓位的入口：助手 `execute_trade` 与文本下单（所有交易所，需用户确认，Gate 上唯一现实的入口）、网格（非 Gate 交易所）。2026-11-26 起助手开仓确认文案固定注明「不带止损止盈、不由 trader 管理」
| F49 | Gate 公共行情限频（2026-11-26 查证文档「限频规则」）：公共接口「单个接口 200r/10s、按 IP」；合约 K 线与 tickers 小节没有单独标注；被限频返回 429 与 JSON 的 label/message，响应头带 `X-Gate-RateLimit-Requests-Remain` / `X-Gate-RateLimit-Limit` / `X-Gate-RateLimit-Reset-Timestamp`，文档未承诺 `Retry-After` | 父任务对抗审查（视角 C）：K 线与 ticker 无缓存、无并发合并，图表（每 5 秒 1500 根）、落地页（每次 6 个请求）、助手与全部 trader 共用同一 IP 配额 | `market/gate_cache.go`：短 TTL 缓存 + 并发合并（K 线按周期缩放到 2–10 秒，缓存 key 带当前 K 线的时间桶，收线后的请求不会拿到收线前的快照；1w/30d 只用 2 秒合并并发；ticker/指标/contract_stats 10 秒），错误与 429 不缓存；引擎另断言末根 K 线新鲜（F57）；429 → `ErrGateRateLimited`（带恢复时间），`CONTRACT_NOT_FOUND` → `ErrGateContractNotFound`；`/api/klines` 映射为 503 + `Retry-After: 1` 与 404；引擎遇 429 沿用逐币跳过并记日志 |
| F50 | 父任务对抗审查（2026-11-26，四视角）的发现与处置。视角 B/C 已回传并修复（`6f25505a`）：助手策略技能的周期枚举/文案仍是旧集合且不校验（`3m` 被静默归一成空串）→ 统一为 D25、`applyStrategyConfigPatch` 校验回绝、`StrategyClampWarnings` 增加周期告警；`scan_interval_minutes` 仍被界面/助手/文档描述为决策节奏 → 统一为「回退间隔」并显示实际节奏；`scanTimeframe` 与引擎主周期解析不一致（主周期为空的存量策略回到固定间隔）→ 改为 Primary → Selected[0] → 5m；「币种误判股票」在 `get_market_price`/`execute_trade` 仍存在（HYPE、ZEC 等 827 个 Gate 合约会被判成股票）→ 改为 Gate 优先；提示词数据字典成交量单位写 base asset、权益/可用公式与第 5 节不符、`OIChange` 数据并不提供 → 已订正；一条非法决策会让同批的平仓/改保护一起作废 → 改为逐条校验、只丢弃非法的那条并记录原因；`/api/klines` 对未知合约返回 500 → 404；若干过期注释、孤儿文案、输入下限不一致 → 已清理。视角 D（测试交付）：冒烟脚本按实际响应形状必然失败（策略 config 的嵌套、旧周期被清空）→ 已修；缓存测试有 7 处可被弱实现绕过 → 按审查给出的写法补强（并发合并、默认开启、条数上限、恢复头、三个入口）；助手路径吞掉配置校验错误 → 被拒的值带回下一条回复；子任务门禁依赖宿主机 Go → `make test/build` 改经 `scripts/go` 在 Docker 里执行；`ci-local` 改用独立镜像 tag、纳入 `infra-check`；FAQ 与应用内文案的扫描口径、归档 implement 未回填、计划表格损坏、`.gitignore` 未排除 `.nofx-local` → 已修。视角 A 的发现见 F52–F58 | 父任务收口的四视角审查 | 修复后定向测试（market/api/kernel/agent/store/trader）、`go build ./...`、前端 164 用例与 `tsc --noEmit` 全过 |
| F51 | 主周期下限与模型调用频率：D25 保留 `10s`，决策按主周期收线触发，`10s` 主周期约每 15 秒调用一次模型、`1m` 约每 65 秒 | 视角 C 指出节奏下限从「扫描间隔至少 3 分钟」变成 10 秒且界面没有护栏 | 本次不新增下限（属产品决策，需用户另行裁定）；界面在交易员配置弹窗显示实际节奏，`10s`/`1m` 时提示调用频率，助手与指南写明「周期越短调用越频繁」 |
| F52 | 同一交易所账户挂多个 trader 时仓位归属不可靠：`trader/gate/order_sync.go` 把账户内全部成交记到当前 `traderID` 名下、`store/order.go` 只按交易所账户与成交号去重（先到先得），`ApplyMarginSettings` 是合约级设置会互相改动对方仓位的杠杆与模式 | 视角 A 审查（2026-11-26）按代码推断，未实测 | 约束：**一个交易所账户只挂一个 trader（任意类型）**（多个 AI trader 共用账户时它们的成交备注相同，仍无法区分归属；网格 trader 与 AI trader 共用账户时网格不启用订单同步，见 F64）。助手、网格、用户手动或其他工具开出的仓位在同步层就被排除（F60），在「单账户单 AI trader」的口径下 D40 成立 |
| F53 | D40「非本 trader 仓位对模型不可见、不管理、不平仓」此前只实现在回撤监控：模型能看到账户内全部持仓，平仓、分批平仓、改保护都会直接执行，最大持仓数也按全部持仓计；归档的 drawdown-monitor 任务把「对模型不可见」交接给子任务 9，但 9 没有实现 | 视角 A 审查（2026-11-26），P1 | 已修：`buildTradingContext` 只把本 trader 的仓位交给模型；平仓/分批/改保护前做方向与归属断言；开仓只数本 trader 的仓位，同一合约上有不可归属持仓时拒绝开仓，自己的反向仓位也要求先平（单向持仓模式下反向开仓是净额减仓）；本进程刚开出、OrderSync 还没落库的仓位用自开登记补上归属空窗。复核发现同步层仍把账户内全部成交记到 trader 名下（见 F60），已随 F60 闭合 |
| F54 | 持仓在存续期间失去止损（用户在交易所撤单、止损触发后市价单没成交完、同账户其他操作撤掉条件单）时没有代码级兜底：只在开仓后与改单后各核对一次，之后靠模型自愿平仓；补挂路径两头堵死（改单被拒、执行层也拒绝） | 视角 A 审查（2026-11-26），P2 | 已修：每轮对「条件单列表读取成功且止损不存在」的本 trader 仓位立即市价全平并记入决策记录；保护读不到（接口失败）时不据此平仓。**这是新增的确定性平仓行为，需用户知情** |
| F55 | 全平、分批平仓与普通市价开仓不校验成交：IOC 零成交或部分成交都无条件报 `FILLED`，紧急平仓会静默失败（仓位无保护地留在交易所，日志却说已平）；开仓部分成交按计划张数记账 | 视角 A 审查用 overlay 测试实证（2026-11-26） | 已修：`finish_as=filled` 才算全部成交；全平以持仓是否消失为准，没平掉再提交一次，仍在则报错；分批平仓要求一次成交完；开仓按实际成交张数（下单张数 − left）记账；开仓请求报错后若交易所上出现了仓位立即平掉。全平不设滑点上限（平仓以成交为优先） |
| F56 | 2% 亏损上限只在决策校验时按取数时刻的 K 线收盘价计算，提交前不复核：模型调用期间的价格漂移加 1% 滑点，最坏可到本金 2.1%–3.06%（仓位比例 ×（漂移 + 滑点）） | 视角 A 审查（2026-11-26），P2 | 已修：提交前按最坏成交价（最新价 × (1 ± 滑点比率)）复核，超过上限时按 上限 ÷ 亏损 等比收缩名义价值并重算张数；收缩后不足最小下单金额则拒绝开仓 |
| F57 | 引擎取数的形状是 limit=200，命中 3–10 秒档的缓存；主周期 ≥5m 时，收线前 5 秒内的同 key 请求会让引擎读到收线前的快照（缺刚收的那根），模型可能把刚收的 K 线当成未收线；引擎也不校验末根 K 线的新鲜度 | 视角 A/D 审查（2026-11-26） | 已修：缓存 key 带当前 K 线的时间桶（收线后是新桶）；取数后断言末根开盘时间距今不超过两个周期，否则跳过该币；行情全部不可用且没有仓位时不再调用模型 |
| F58 | 逐条决策校验后被丢弃的决策不落库、不计失败：模型每轮输出一条违规开仓加一条 `wait` 时，记录显示成功、trader 看起来正常，只是永远不开仓 | 视角 A/D 审查（2026-11-26） | 已修：被拒决策逐条写入决策记录的 `ExecutionLog`（助手诊断的证据优先级：决策记录 > 日志），连续 3 轮出现被拒决策升格为 Error 日志 |
| F59 | 随单保护开仓（`ProtectedOpener`）只有 Gate 适配器实现：Binance、Bybit、OKX、Bitget、Kucoin、Lighter、Aster、Indodax 的 AI trader 每轮开仓都会报「does not support protected open」并被拒绝 | 视角 A 审查（2026-11-26），P2；`trader/auto_trader_orders.go` 的能力检查 | 方向是 fail-closed（不会开出无保护仓位），但与 D26 的表述不一致。**需用户知情**：这些交易所目前只能平仓/查询，不能由 AI trader 开仓；要恢复须为对应适配器实现随单保护 |
| F60 | 归属在同步层失真：`SyncOrdersFromGate` 把账户内最近 24 小时的全部成交（最多 100 笔）记到当前 `traderID` 名下并建 OPEN 持仓记录，不区分下单入口；助手与 trader 的开仓备注又都是 `t-nofx`，无从区分。2026-11-26 的只读探针显示该账户的成交历史里确有其他入口的成交（备注 `app`、`ao-…`、`app-auto-trail-…`） | 视角 A 复核（2026-11-26），P1；只读探针 `probe/mytrades-text-2026-11-26.jsonl`（`my_trades` 的 `text` 原样带回下单备注） | 已修：AI trader 的随单保护开仓改用专属备注 `t-nofx-ai`（助手/网格仍为 `t-nofx`）；同步只为带该备注的**开仓成交**建订单、成交与持仓记录；平仓成交只作用于本 trader 已有 OPEN 记录的仓位，别的入口的平仓不进它的成交历史与盈亏统计；本 trader 的开仓成交没写进持仓记录时回滚订单行，下一次同步重放。部署前已由旧逻辑记下的记录不变；此前没被同步到的仓位会被视为不可归属（不管理，方向安全） |
| F61 | 每轮保护兜底的边角：止损刚触发的瞬间（持仓缓存里还在、条件单列表里止损已不在）会被判成「确认没有止损」，平仓报「持仓不存在」却记成失败并中止本轮；「确认没有止损」只有一次读数就市价平仓；一个仓位平不掉会让每一轮都在这里中止 | 视角 A 复核（2026-11-26），P3 | 已修：平仓前再读一次保护，第二次读到止损或读失败就不平；平仓报 `ErrPositionNotFound` 按已解决处理；平仓失败不再中止本轮（其余仓位照常交给模型），但本轮禁止开仓（`openBlockReason`）并写入决策记录。**用户须在 Gate 上用「仓位止盈止损」手动挂止损**：系统只认 `close-*` / `plan-close-*` 类型的条件单，普通条件单止损不被认可，会被当成没有止损而平仓 |
| F62 | 其余复核发现：开仓请求报错后的兜底平仓读到的是 15 秒持仓缓存（请求报错时缓存没清）；「连续被拒」计数从不归零；策略范围外与安全模式拦截的决策不进决策记录；回撤平仓失败只打 Info；英文确认文案对平仓单也追加开仓风险说明；`peakPnLCache` 只在回撤平仓时清理，模型平仓、止盈止损或兜底平仓后同向再开仓会带着上一笔的峰值 | 视角 A 复核（2026-11-26），P3 与范围外观察 | 已修：报错返回前清持仓缓存，兜底读数最多 3 次（间隔 300ms、每次先清缓存）；每轮无条件更新被拒计数；拦截的决策写入 `ExecutionLog`；回撤平仓失败升为 Error；英文文案只对开仓追加；每轮清理已不在交易所的仓位的峰值 |
| F63 | 旧的 OPEN 持仓记录从不与交易所对账：停机期间仓位被平掉、平仓成交落在同步窗口之外（停机超过 24 小时，或 24 小时内成交超过 100 笔）时记录永久残留；之后同合约同向出现一个不是本 trader 开出的仓位，旧记录会把它「收养」成自己的——模型能看到它，每轮保护兜底还可能把它市价平掉；本 trader 自己的下一次同向开仓也会被合并进旧记录，数量/入场价/PnL 失真且记录关不掉 | 最终独立复核（2026-11-26），P2；用 overlay 探针实证了「残留记录 + 外来仓位 → 兜底平仓」 | 已修：`buildTradingContext` 每次成功读到持仓后对账（`reconcileOpenPositionRecords`）——交易所上没有该仓位 → 关闭记录；仓位在但入场价与记录相差超过 0.5% → 视为不可归属、关闭记录（宁可不管，不可误平）；入场价一致、只有数量不同（错过了部分平仓成交）→ 把记录数量对齐到交易所读数（`store.SyncPositionSnapshot`）。只在持仓读取成功时执行，读取失败不删除任何记录 |
| F64 | 网格 trader 与 AI trader 共用账户时，网格的 `StartOrderSync` 也会把 AI trader 的 `t-nofx-ai` 开仓记到网格名下（备注无法区分 trader）；同一轮内「先全平再同向开仓」或兜底平仓后同轮再开仓时，旧峰值缓存不会被清理，新仓位可能被回撤监控提前平掉 | 最终独立复核（2026-11-26），P3 | 网格不消费 `trader_positions`，其归属按网格品种判定：网格 trader 不再启用订单同步。峰值缓存改为在平仓成功时立即清理（模型全平、兜底平仓、保护丢失后的紧急平仓、失败开仓后的兜底平仓），不再只依赖每轮开始时清理已消失的仓位 |
| F65 | 交易员配置弹窗的「实际节奏」提示在真实数据下不显示：策略配置的线上形状是 `{strategy_type, ai_config:{indicators,...}}`，组件读的是顶层 `indicators`（旧扁平形状） | 送测候选 `570cea30` 的浏览器实测（2026-11-26）：编辑交易员弹窗只见回退扫描间隔标签与说明，没有实际节奏行 | 已修：`TraderConfigModal` 先读 `config.ai_config.indicators.klines`，回落到 `config.indicators.klines`；弹窗测试夹具改为线上真实形状（此前扁平夹具掩盖了这个问题） |
| F66 | 风控编辑器的「开仓金额百分比」对旧配置（存着 0/缺失）直接显示 0，与运行时「0 → 默认 10%」和界面说明「默认 10」不一致；用户照着 0 保存时后端再静默夹回 10 | 送测候选 `570cea30` 的浏览器实测（2026-11-26）：测试策略的旧配置在风控参数里显示「开仓金额百分比 0」 | 已修：`RiskControlEditor` 对 ≤0/缺失显示默认 10（与服务端一致），新增组件用例（0→10、正值保留、最小开仓输入下限 10） |
| F67 | 最终候选上的执行路径只由单测/冒烟覆盖，`t-nofx-ai` 归属备注、平仓成交确认与终态零残留没有在最终代码上做真实交易所验证 | 送测候选的真实随单保护开平（2026-11-27，W2 授权，单笔名义 9.05 USDT）：探针 `probe/w2_final_livecheck.go`，证据 `probe/w2-final-livecheck-2026-11-27.jsonl` | `LIVECHECK_PASS`：随单保护开仓并核对条件单 → `my_trades` 中开仓成交 `text="t-nofx-ai"`（按下单 ID 匹配；注意成交 ID 按合约递增，不能跨合约比大小）→ 全平（成交确认路径）→ 终态零持仓/零委托/零条件单，可用余额差 −0.01065 即手续费。同步层的归属由单测覆盖（宿主机无 cgo，sqlite 无法在 darwin 探针中使用） |
| F68 | 最终候选的最后一轮独立检查（trellis-check，2026-11-27）发现：P1 订单同步对每条已存在的平仓成交无条件重放——分批平仓后记录仍为 OPEN，每次轮询（30 秒）重复扣数量/累加手续费与盈亏，约两轮后把记录误关，剩余仓位脱离归属；P2 对账关闭用 0 出场价，`GetRecentTrades` 按 `(0−Entry)/Entry×杠杆` 造出 −100%×杠杆 的假交易并压低胜率，且主周期先于同步读到「无仓位」时记录被提前关闭、随后同步因无 OPEN 记录丢弃合法平仓成交（退出价/盈亏永久丢失）；另有 P3：账户/成交数值解析忽略错误、峰值缓存清理符号口径（DOGEUSDT vs DOGE_USDT）、保护核对在方向未知时可能配对反向遗留条件单、`sz_decimals` 把 0.1 张截成 0、静态候选为空但有持仓时整轮跳过、归档证据尾部空行 | 最后一轮独立检查（受检候选 5469912e / 65263894） | 已修：同步改为按成交幂等——订单行存在即跳过，持仓写入失败（含成交行写入失败）回滚订单/成交行供下次重放，删除无条件重放分支（新增连跑两次断言数量/手续费/盈亏不变且记录仍 OPEN 的用例）；对账新增 `RECONCILED` 中间态（不编造出场价、不进入历史/统计），迟到的平仓成交由 `BackfillReconciledClose` 回填并按数量递减/加权出场价，归零转 CLOSED（新增回填用例）；当前 OPEN 记录晚于成交时间时按上一笔仓位的迟到成交处理；账户持仓 entry/mark 严格解析（<=0 报错、强平价/未实现盈亏空串按 0、非空值必须可解析），成交价无效整笔跳过；`ClearPeakPnLCache` 按符号比较键清理所有变体；保护核对方向未知时 fail-closed（不再任意方向配对）；`sz_decimals` 保留 Gate 最小张数的小数语义（ETH/SOL 0.1）；静态币池为空但有持仓时把持仓补进候选以继续管理；归档证据去尾部空行。一个账户一个 trader 维持 F52 的运维约束（代码不阻断多 trader，文档已声明） |
| F69 | 人工真实下单测试（AI 交易员 DeepSeek，BTC 2 张/ETH 0.8 张，随单 SL/TP）发现：`PositionBuilder`/`ReducePositionQuantity` 用固定 0.0001 数量容差判断「剩余为零」——BTC 记录 0.0002 BTC 平掉一半剩 0.0001 时被误判为全平，记录被关闭但交易所还剩 1 张，剩余仓位失去 OPEN 记录（重启后不可见、不可管理）；迟到回填同一容差也有同样问题 | 人工测试（2026-11-26，测试授权，最小仓位 13% 本金）：BTC 真实开仓 `order 36028841385881417`、`my_trades text=t-nofx-ai`、保护核对 2 个条件单 PASS；分批平仓探针 `live_close_probe partial` 后同步把 0.0001 BTC 的平仓误判为全平 | 已修：新增 `positionQuantityTolerance(scale)`（相对 0.01%、绝对下限 1e-9），`handleClose`、`ReducePositionQuantity`、`BackfillReconciledClose` 统一使用；新增 `TestPartialCloseKeepsSmallPositionOpen` 与 `TestBackfillReconciledCloseKeepsSmallRemainder`。现场按剩余 0.0001 BTC 修复被误关的记录，其余仓位已由探针全平（终态 0 持仓） |


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

已全部确认，结论见下表与 D40。

| 编号 | 问题 | 结论 |
|---|---|---|
| U1 | 账户里不是本 trader 开出的持仓（手动仓）是否归机器人管 | 不归，出现时直接无视（D40，2026-11-25 确认）：对模型不可见，不管理、不平仓，回撤监控也不处理（沿用原推荐）；与原推荐不同的是不因此拒绝 trader 启动，"trader 启动时账户里存在这类持仓则拒绝启动并说明"没有采用 |

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
| `trader/gate/trader_account.go` `GetPositions`、`trader/auto_trader_risk.go` 回撤监控 | 杠杆以整数类型返回；监控取不到杠杆时恒按 10 倍计；处理账户内全部持仓 | 按 D37：杠杆取实际值，只处理本 trader 开出的仓位（按 D40）——子任务 6 已交付 |

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
| 6 | `drawdown-monitor` 回撤监控 | 杠杆取持仓的实际值；只处理本 trader 开出的仓位，不是本 trader 开出的持仓直接无视、不因此拒绝启动（按 D40）；全平走张数换算 | 单测（杠杆取值、阈值判定、仓位归属）；带仓的只读真实读数 | 2、5 |
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

全部在交易子账户上执行，单笔测试名义价值不超过 10 USDT，每一步完成后立即还原，不在用户有手动持仓的合约上开新仓。W1–W4 均已授权（D33、D35），W5 于 2026-11-25 授权（D41）。账户在 2026-11-24 复核时无持仓（F27），原定在手动持仓上做的保护单验证并入 W2。

| 编号 | 操作 | 对应子任务 |
|---|---|---|
| W1 | 已并入 W2 | — |
| W2 | DOGE_USDT 开 17 张（约 9.5U，带随单止盈止损）→ 核对保护单 → 分批平仓 → 改止损止盈 → 全平 → 确认无残留委托。这里列的是验证步骤；实盘里是否分批、平多少由模型按行情决定（D36） | 4、10、11 |
| W3 | ETH_USDT 以 0.1 张（约 1.7U）开仓并平仓，验证最小步长与随单止盈止损 | 4 |
| W4 | 在一个未持仓的合约上设置全仓、逐仓与杠杆，从设置接口的返回值与持仓条目读回，结束后还原原设置 | 5 |
| W5 | 在一个从没有设置过仓位设置的合约上调用 `set_leverage`（全仓、逐仓与杠杆），从设置接口的返回值与持仓条目读回。会留下一条仓位设置条目，无法还原为"没有条目"，这是用户授权时知情的例外；不下单，其余约束同上 | 5 |

### 10.4 父任务收口

1. [x] 逐项对照本文件做对抗式审查并修正。W5 已实测 `LIVECHECK_PASS`（`probe/README.md`）；四视角审查 B、C、A、D 与最终独立复核均已回传并修复（F48–F66）；最后一轮独立检查（2026-11-27）已回传，P1/P2 与可修的 P3 已修复（F68），修复后 `make test`、`make build`、`make ci-local` 与串联冒烟（smoke-cb171385，SMOKE_PASS）在候选 aa98e6d6 上复跑通过。送测清单（F67 已在最终候选上完成真实随单保护开平：`t-nofx-ai`、成交确认、零残留）；启动 trader 后核对日志中是否有残留 OPEN 记录被对账清理；确认 `probe/closeout_smoke.py` 首跑与重跑均可通过。
2. [x] 串联全部子任务做端到端验收与完整项目门禁（`make ci-local`、`probe/closeout_smoke.py`）：候选 aa98e6d6 全过，冒烟 24 项 SMOKE_PASS。
3. [x] 合入 dev-offline 并重建送测栈；真实下单实测已执行（用户授权按最低金额试仓）：AI 开 BTC 2 张 + ETH 0.8 张、随单保护与 `t-nofx-ai` 均通过；同步连跑两次幂等；实测发现并修复小数量分批平仓误判（F69），修复后在停用 AI 的受控仓位上证明确实保持 OPEN/只关闭一次，终态零持仓；策略配置已恢复。
4. [x] 合入 main（merge cd21a5ab）并重建主栈：`make main-stack` 通过三段 D20（traders.is_running=0）、容器 healthy、无自动拉起；主栈 127.0.0.1:8888。
5. [x] 改写主栈两个 K哥策略提示词（波段 b0d9d935、短线 01a66a75）：删除头仓/加仓/后端硬校验盈亏比旧语义，改为一次决策一个仓位、名义由系统按本金×比例计算、update_stop_loss 推保护、close_*_partial 分批止盈、止损亏损≤本金 2% 为代码硬约束；配置 JSON 校验通过，后端重启 healthy。
6. [x] 删除未使用镜像（旧 nofx-dev-offline/nofx-local tag、nofx-golang、上游预览镜像与容器），只保留主栈 cd21a5ab 与送测栈 39f895fc；父任务归档到 `.trellis/tasks/archive/2026-11/11-24-gate-trading-foundation/`。

### 10.5 实施进度

- [x] 1 `gate-sdk-v7`：`32f63003`，全量 `go test` 通过，只读真实读数 19 项全部成功
- [x] 2 `contract-math`：`590ba577`，单测矩阵通过，7 个合约元数据真实读数全部一致（`failed=0 mismatched=0`），`make test` 与 `make build` 通过；独立检查通过
- [x] 3 `protective-order-adapter`：`12c2a249`（381d7f02 + 5f5160a4），单测矩阵通过，真实挂、查、撤与随单止盈止损全部成功（`LIVECHECK_PASS steps=10`，终态零残留）；改单被交易所拒绝（新路径 404 未部署、旧路径 code 1077 拒绝 API 触价单，research H.2），撤旧建新替代路径验证通过；仓位级条件单无持仓被拒（H.1），其真实验证并入 W2；独立检查 PASS（1 P2 + 6 P3 已修复），`make test` 与 `make build` 通过
- [x] 4 `open-position-safety`：`60db88da`（0df83b9a + 9156ec34 + 7850ab5d + 7e45d47f），门禁 `git diff --check`、`make test`、`make build` 全过；W2（DOGE_USDT 17 张随单保护开仓→核对→全平→零残留）与 W3（ETH_USDT 0.1 张最小步长开平）实测 `LIVECHECK_PASS`；独立检查 1 P1 + 1 P2 + 4 P3 全部修复并复核 PASS。实测发现：随单 tpsl 是委托级保护，小数张合约（ETH）以 `close-long-order` 出现在未触发条件单列表（可按 ID 撤销、分类正确），整数张合约（DOGE）不出现在列表——`VerifyOrderProtection` 以委托回读 + 列表双路径覆盖两种形态；子任务 10、11 的管理路径设计须先复核此行为（证据见归档任务的 `probe/README.md`）
- [x] 5 `unified-trading-params`（W4）：`ccd2f51c`（b8252754 + d92328b9 + 4eedd743 + d758d3ec + eb84f2e4 + e1b6fdc3 + aa06dc75），最终候选 `e1b6fdc3` 上 `git diff --check`、`make test`、`make build` 全过；W4（DOGE_USDT 全仓 / 逐仓 × 10x / 5x / 3x 设置并读回、重复设置、超上限被拒、两个旧方法）在 `d758d3ec` 上实测 `LIVECHECK_PASS`（23 步 + 1 条观察），DOGE_USDT 还原为全仓 75x，BTC_USDT、ETH_USDT 条目不变，可用余额前后一致。交付：`trader/gate/margin_settings.go`（`ApplyMarginSettings` 走新接口 `positions/{contract}/set_leverage`，以返回值或持仓条目读回一致为成功；`PositionMarginSettings`；重写的 `SetLeverage`、`SetMarginMode`）、能力接口 `types.MarginSettingsApplier`、`ProtectedOpenRequest.IsCrossMargin`；随单保护开仓在下单前生效设置、失败不开仓；网格初始化在设置生效后才标记已初始化。独立检查首轮 NEEDS FIX（无 P0/P1），修复后复核 PASS；变异实验的 53 个可编译变体最终全部被测试杀死（其中 3 个由主会话修复后复验）。实测发现：(a) `set_leverage` 线上已部署，单向持仓模式下不带 `dual_side`；(b) 逐仓条目的 `cross_leverage_limit` 残留旧值，读回用 `lever` 与同模式旧字段互相印证；(c) 重复设置返回 200；(d) 超上限的 label 为 `LEVERAGE_EXCEEDED`，不在文档 label 表（F37）；(e) `get_leverage` 在单向持仓模式下不能用；(f) 不显式设置时首次建仓的合约在本账户落到全仓 + 最大杠杆（F36）；(g) 账户仍是跨币种模式（`margin_mode=1`），逐仓在该模式下可用，单币种下的读回待账户切换后复验。未实测（归档任务 design.md 已知边界）：从没有设置过的合约上的 `set_leverage`（要留一条无法还原的条目；2026-11-25 已授权为 W5，2026-11-26 实测 `LIVECHECK_PASS`）、带仓或有挂单时能否修改设置。对后续子任务的提示：⑥ 回撤监控取实际杠杆读条目的 `lever`（`GetPositions` 现在仍返回整数 `leverage`，全仓为 0），`PositionMarginSettings` 可直接读回合约的设置；设置是合约级的，同一合约上已有反向持仓时，开仓前生效设置会同时改动那个仓位
- [x] 6 `drawdown-monitor`（带仓只读真实读数一次）：`c0eadc1f`，最终候选上 `git diff --check`、`make test`、`make build` 全过（Go 全量 + 前端 158 用例）；带仓只读真实读数 `LIVECHECK_PASS`（0 失败：17 张 DOGE_USDT @0.05472、名义 9.3024 USDT 随单保护开仓 → 原始 `lever="75"` 与适配层 float64 `75` 一致 → 归属判定（网格品种 true、非网格 false）→ 监控一轮峰值缓存与按实际杠杆 75 手算收益率差 3.5e-17 → 全平还原，终态零持仓零委托零条件单、设置条目与基线一致、余额差 −0.011 即双边 taker 手续费）。交付：`trader/gate/trader_account.go`（`positionLeverage`：`lever` → 同模式旧字段 → 0 不可用；`GetPositions` 返回 float64 杠杆）、`trader/auto_trader_risk.go`（`isOwnPosition` 归属过滤：网格品种子句 + DB OPEN 记录子句，其余一律无视；杠杆 ≤0 才回落 10 倍；全平维持 `close=true + size=0` 原语）、`trader/drawdown_monitor_probe.go`（livecheck tag 探针桥，正常构建与测试不编译）。单测 13 个监控用例 + 适配层 7 子用例解析矩阵；独立检查首轮 NEEDS FIX（1 P1 + 2 P2 + 2 P3，均为测试缺口），修复后复核 PASS（变异 M1/M3/M4 转杀，新增 M9/M10 边界变异被杀）。实测确认：gate 的持仓记录由 OrderSync → PositionBuilder 落库（`recordAndConfirmOrder` 对 gate 提前返回），归属查询与落库同源。下一步提示：⑧ `strategy-config` 无依赖可开工；⑨ 依赖 ⑥⑦ 均已合父
- [x] 7 `account-equity`：`831dfb5e`（13 个子提交，代码候选 `20a1c61c`），最终候选上 `git diff --check`、`make test`、`make build` 全过
      （Go 22 个包通过、前端 158 个用例）；第 4 轮真实读数 `LIVECHECK_PASS`（不带仓与带仓读数、加载路径的初始资金重建、
      提示词预览，W3 级开平仓一次、终态零残留）；第二轮界面实测核对换绑重置、改过才发送、创建失败原因与中英印尼文案；
      独立检查复审 PASS（首轮 NEEDS FIX 的 1 项 P1、3 项 P2、9 项 P3 全部修复，变异 0 存活）。因 U1 未决，第 7 步先于第 6 步实施。
      任务归档 `.trellis/tasks/archive/2026-11/11-24-account-equity/`
- [x] 8 `strategy-config`：`9fd74a2c`（子分支 `7d23b28a` + 修复 `324015a0`），最终候选上 `git diff --check`、`make test`（Go 22 包 + 前端 158 用例）、`make build`、`tsc --noEmit`、`vite build` 全过；独立检查 **PASS**（无 P0/P1；1 P2 + 4 P3——P2 测试断言收紧为 `"error"` JSON 约定、P3-1 删死代码 `trader.isBTCETH`、P3-4 英文标签改 "position size percent"，均已修复并定向补验 `go build ./...` + `go test ./trader/ ./agent/` 全绿；P3-2/P3-3 留档为 F41/F42）。交付：权益×仓位价值倍数的整套旧语义一次性删除（`store/strategy.go` 字段/clamp/diff、`kernel/engine_position.go` 上限块与 ratio 参数、`kernel/engine_prompt.go` 硬约束/仓位大小指引/输出示例、`engine_analysis.go` 透传、`trader/auto_trader_risk.go` 封顶、`agent/trade.go` 助手下单上限拒绝、助手锁定名单/摘要（中英）/关键词/编辑分支/不开单诊断与 planner 指令段、`skills/strategy_diagnosis.json`、`skills/trader_diagnosis.json`、`api/server.go` schema、`STRATEGY_MODULE.md/.zh-CN.md` 两份）；新增 `risk_control.position_percent`（1–20、步长 0.1、默认 10，存量缺失 0 值补默认；clamp 7 例矩阵单测）；`min_position_size` 默认 12→10 并解锁为可编辑项（编辑器输入框、助手目录/编辑分支、摘要去 System enforced、诊断「不能手动修改」文案中英翻转、四处兜底同步，旧「锁定拒绝」断言翻转为「生效」）；周期按 D25 收敛为 {10s,1m,5m,15m,30m,1h,4h,8h,1d,7d,1w,30d}（`normalizeTimeframe` 矩阵 + 别名、前端 `IndicatorEditor` 选项数组、api schema 行、旧配置兜底 `"3m"→"5m"`）。边界确认：kernel 12/60 硬编码最小与盈亏比校验按计划保留给 ⑨；`validKlineInterval`（行情工具输入）与 `market/timeframe.go` 属 ⑫⑮ 边界未动。任务归档 `.trellis/tasks/archive/2026-11/11-25-strategy-config/`
- [x] 9 `sizing-engine`：`ab2ca8d0`（子分支 `9eeaa72a`：`d7572d84` 实施 + `d85c98de` 指南口径 + `1daec733` 检查修复 + `9eeaa72a` 复核建议收口），最终候选上 `git diff --check`、`make test`（Go 全部包 + 前端 13 文件 / 158 用例）、`make build` 全过；独立检查首轮 NEEDS FIX（P1 符号变体回归、P2 架构文档签名、6 项 P3），修复后复核 **PASS**（preview ClampLimits 变异被杀；base 匹配分支已补用例）。交付：kernel `AccountInfo.Principal` 经 `types.ReadAccountBalance` 填充、`Decision` 删除 `position_size_usd`/`risk_usd` 并新增代码填充的 `NotionalUSD(json:"-")`、`openNotional` 精确十进制（F33：10000 × 1.13% = 113）、开仓校验新增止损止盈分居现价两侧与「到止损价亏损 ≤ 本金 2%（等于允许，D34）」、符号变体按执行层口径取价（精确 → `market.Normalize` → 去 `/` → base）；删除硬编码最小下单 12/60 与盈亏比 ≥3 代码校验（D12）；提示词改本金 × 百分比与 2% 上限（含 USDT 额）、输出契约去金额、示例占位提示；`preview-prompt` 字段改 `account_principal`、`test-run` 模拟本金，两者先 `ClampLimits`；助手诊断改本金口径与证据解析（含 `trade_execution.json` 旧 12 USDT 口径）；删除无生产调用方的 `kernel/prompt_builder.go`、`kernel/formatter.go` 及其测试；文档（STRATEGY_MODULE、prompt-guide、TROUBLESHOOTING 中英、store 注释、api schema）同步；提示词预览证据 `probe/sizing-prompt-{zh,en}.txt` 与 `probe/README.md`。未实测：真实成交价下的 2% 边界与真实下单（并入父任务 W2/E2E）。任务归档 `.trellis/tasks/archive/2026-11/11-25-sizing-engine/`
- [x] 10 `partial-close`（W2 分批平仓）：`3985f016`（子分支 `7cd7788f`；规划 `f902f7db`），最终候选上 `git diff --check`、`make test`（Go 全部包 + 前端 14 文件 / 160 用例）、`make build` 全过；独立检查首轮 NEEDS FIX（1 项 P2：证据日志行尾空格；5 项 P3），修复（换算矩阵用例、无缓存复核、编译期能力断言、弱测试段、示例语义、适配器规范）后复审 **PASS**。W2 在修复后重跑 `LIVECHECK_PASS`（2026-11-25 16:43）：DOGE_USDT 随单保护开 17 张（名义 9.2769U，SL/TP ±10%）→ `PartialCloseStatus` 可分批 → 50% 分批 8 张、剩 9 张（orderId=56013526826389199）→ 仓位 90 币、`supported=true contracts=9`、开仓委托 tpsl 仍可读回 → 100%/1% 被拒（未缓存读数仍 9 张）→ 全平（orderId=56013526826389504）→ 零残留，仓位设置不变，可用差 −0.01187U。交付：`types.PartialCloser` / `PartialCloseStatus` / `PartialCloseRequest` / `PartialCloseResult`；`trader/gate/partial_close.go`（状态、精确 `SizeForCloseRatio`、市价单最大张数校验、reduce-only 市价单、平多负/平空正）；`ContractSpec.validateMarketOrderSize`；kernel `Decision.ClosePercent` + `close_long_partial`/`close_short_partial` 与 `(0,100)` 校验（100 指向全平）；`PositionInfo.CanPartialClose` / 不可分批原因与提示词标记；中英动作清单/说明/单独格式示例；执行路径 fail-closed、排序优先级 1、`DecisionAction.close_percent`；前端 DecisionCard 映射与比例徽标；架构文档中英同步；规范新增 Partial close by ratio 场景。实测观察 F43（分批后条件单列表形态）留档给 ⑪。任务归档 `.trellis/tasks/archive/2026-11/11-25-partial-close/`
- [x] 11 `protective-order-management`（W2 改止损止盈）：子分支 `8dfbb326`（`9af310c1` 实施 + `e5e1e706` 检查修复 + `8dfbb326` 注释订正），合父 `d0072836`，归档 `.trellis/tasks/archive/2026-11/11-25-protective-order-management/`。最终候选上 `git diff --check`、`make test`（Go 全部包 598 个用例 + 前端 14 文件 / 162 用例）、`make build`、前端 `tsc --noEmit` 全过；独立检查首轮 NEEDS FIX（P1×2：无保护哨兵未进错误链、交易路径符号变体匹配；P2：执行侧 2% 上限缺 1e-12 容差；P3×4：恢复有效价核对、排序用例、证据路径、kernel 变体 fixture 非 Gate 格式），修复后定向复核 **PASS**（无 P0/P1/P2；剩 1 项 P3 注释边角，已随 `8dfbb326` 订正）。W2/W3 在修复候选上重跑 `LIVECHECK_PASS`（2026-11-25 18:30）：DOGE 17 张随单保护（对齐后 SL 0.04864 / TP 0.05945，列表各 1 条 `close-long-order`）→ 50% 分批剩 9 张（保护张数不变、reduce-only）→ 改止损（委托级旧单按 ID 撤销、新单位仓位级 `close-long-position`，0.04864→0.05242）→ 改止盈（0.05945→0.05675，向现价，D32 双向）→ SL/TP 各 1 条 → 全平零残留；ETH 0.1 张（列表 `close-long-order`、size=0/amount=-0.1）→ 改止损 1628.27→1636.49 → 全平零残留；余额 107.4230→107.3942（差为多轮手续费）。交付：`ContractSpec.PriceTick` 与 `AlignTriggerPrice`（F44：对齐后开仓随单保护才真正建单）、开仓对齐与 `VerifyOrderProtection` 以未触发条件单列表为准（3×300ms 重试、持仓已平视为无需保护、方向改由持仓推导）、`types.ProtectiveOrderManager`/`PositionProtection`/`ProtectionUpdate{Request,Result}`/`ErrPositionUnprotected`、`trader/gate/protection_management.go`（同类取最近与计数、改单先挂新核对→撤旧→核对、失败恢复并核对原有效价、2% 上限与 1e-12 容差、价格步长对齐）、kernel `update_stop_loss`/`update_take_profit` 与校验（持仓/保护可读/方向/2% 上限/符号变体）、`PositionInfo.StopLoss/TakeProfit/ProtectionUnavailableReason` 每轮读取、中英提示词、执行路径（能力缺失拒绝、保护丢失紧急平仓、决策记录实际价）、前端 DecisionCard UPDATE SL/TP、架构中英与适配器规范新场景
- [x] 12 `market-source`（7 标的 × 全部保留周期的真实取数）：子分支 `3be668b6`（`7c4e0267` 实施 + `3be668b6` 检查修复），合父 `0c1323e1`，归档 `.trellis/tasks/archive/2026-11/11-25-market-source/`。最终候选上 `git diff --check`、`make test`（Go 全部包 608 用例 + 前端 14 文件 / 162 用例）、`make build` 全过；独立检查 **PASS**（无 P0/P1/P2；6 项 P3：30d 月初仅在全历史 limit 成立、design 整除表述矛盾、最后一根 30d CloseTime 近似、ticker 请求形状与失败矩阵补强、注释/未用字段、列表 fail-closed 的新合约边界，均已修复或留档）。真实探针 `PROBE_PASS`（2026-11-25）：7 标的（BTC/ETH/SOL/ZEC/HYPE/DOGE/XAU）× 12 保留周期全部成功；非 30d 周期间距精确、`1w` 全为周一、`30d` 在 limit=200 时 85 根全为月初（相邻 28–31 天）、ticker 漂移 ≤0.0207%、Volume 与原始 `sum` 精确相等、全量 1042 合约解析通过、旧周期被拒；小 limit 的 30d 实测为 9/10/11 月 2 日（代码不依赖月初）。交付：`market/gate_market.go`（只走 Gate、无回退；`GateIntervalDuration` 保留集合 {10s..30d}；`GetGateKlines` 用 `sum` 成交额、毫秒时间、CloseTime 取下一根开盘、严格校验 OHLC/时间/数值；`GateTicker`/`GetGateTicker`/`ListGateTickers`），单测 389 行矩阵（字符串/数字混排、失败注入、非法参数零请求）
- [x] 13 `engine-klines-gate`（策略试运行显示 Gate 数据）：子分支 `89a3a97e`（`13246f0f` 实施 + `ad5fef58` 检查修复 + `bdc844ea` 探针证据 + `89a3a97e` P3 收口），合父 `0e498514`，归档 `.trellis/tasks/archive/2026-11/11-25-engine-klines-gate/`。最终候选上 `git diff --check`、`make test`（Go 全部包 620 用例 + 前端 14 文件 / 162 用例）、`make build` 全过；独立检查首轮 NEEDS FIX（1 项 P2：主周期与 1h/4h 窗口不匹配时读数失真；5 项 P3），修复后 **PASS**（无 P0/P1/P2；7 项 P3 已修复或留档 F46）。真实探针 `PROBE_PASS`：12 个保留周期逐一经 `GetWithTimeframes`（含 10s/7d/30d，F41 收口）、箱体为 Gate 1h × 500、无 AI 提示词渲染出 10s/5m/1h/1d/7d/30d 与 `Volume(USDT)`；`3m` 被拒。交付：引擎/网格/旧内嵌路径全部改 `GetGateKlines`（无 CoinAnk/Binance 回退，成交量=sum）；`calculatePriceChangeByBars` 改用 Gate 周期时长（F46 留档近似边界）；`isStaleData` 判定窗口改为至少 15 分钟（10s=90 根、1m=15 根）；删除 `getKlinesFromCoinAnk`、`parseTimeframeToMinutes`、`market/timeframe.go(+test)`、`coinank_api/kline_ws.go`、`depth_ws.go`（+tests）、`kernel.FetchMarketData`；提示词周期顺序改为保留集合、K 线表与说明标 `Volume(USDT)`（中英）、`Format` 文案 5m/USDT；`api/strategy.go` 的 `"3m"` 兜底改 `"5m"`；`provider/coinank/coinank_api/kline.go`、`api/handler_klines.go`、`agent/*` 保持未动（⑭⑮ 边界）
- [x] 14 `chart-klines-gate`：子分支 `35e9f508`（`374bd603` 实施 + `30e6a4c3` 检查修复 + `35e9f508` P3 收口），合父 `d6236560`，归档 `.trellis/tasks/archive/2026-11/11-25-chart-klines-gate/`。最终候选上 `git diff --check`、`make test`（Go 全部包 619 用例 + 前端 14 文件 / 162 用例）、`make build`、前端 `tsc --noEmit`、`vite build` 全过；独立检查 **PASS**（无 P0/P1/P2；6 项 P3：非保留周期 400、10m 用例、AdvancedChart 冗余 volume 字段、孤儿 i18n、设计文档矛盾、测试缝注释，均已修复）。交付：`/api/klines` 加密货币分支只调 `market.GetGateKlines`（`exchange` 不再影响路由、limit 1..2000、非保留周期 400、`QuoteVolume=Volume=USDT sum`），Alpaca/TwelveData 保留；删除 `provider/coinank/coinank_api` 全套与顶层 `provider/coinank/kline.go`（其他 CoinAnk 数据能力与 enum 保留，D10）；前端 AdvancedChart/ChartTabs/TraderDashboardPage 去 `exchange`、成交量统一 `V(USDT)`、删除无调用方的 `TradingViewChart`/`ChartWithOrders`/`ChartWithOrdersSimple`；`advancedChart.units` 孤儿 i18n 清理
- [x] 15 `assistant-market`：子分支 `2f7f25b0`（`30b8cc26` 实施 + `1b52234e` 30d 小 limit 修复 + `4e79c5dc` P2/P3 修复 + `eeac13fc` P3 收口 + `2f7f25b0` 复核收口），合父 `f9644929`，归档 `.trellis/tasks/archive/2026-11/11-25-assistant-market/`。最终候选上 `git diff --check`、`make test`（Go 全部包 625 用例 + 前端 14 文件 / 162 用例）、`make build`、前端 `tsc --noEmit`、`vite build` 全过；独立检查首轮 NEEDS FIX（1 P2：get_kline 把 HYPE 等 Gate 币种误判为股票；7 P3），修复后复核 **PASS**（无 P0/P1/P2；残余为非阻塞观察）。交付：`/api/agent/tickers` 改 Gate（字段与前端兼容、脏合约跳过）；`get_market_snapshot`/`get_kline` 全 Gate（USDT 成交额、OI USD、D25 周期枚举、Gate-only 币种不再误判股票）；Brain 简报与 Sentinel 告警用 Gate ticker；`agent.go` 用 `GetWithTimeframes` + 动态精度摘要；删除 `/api/agent/klines|ticker`、`proxyBinance`/`binanceClient`、全部 Binance 取数、`market.Get`/`GetWithExchange`/`Format`/`Intraday*`/`LongerTerm*`/`BuildDataFromKlines` 及 kernel 旧格式分支；grid ATR/EMA50 改从时间周期数据取（EMA50 优先 4h）。真实探针 `PROBE_PASS`（ticker/metrics/K 线 10s/15m/1d/7d/30d，末根与 ticker 偏离 ≤5%）；新发现 F47（Gate 30d 小 limit 返回占位根，已用全历史拉取+截取修复）
- [x] 16 `scan-alignment`：子分支 `4ddf671f`（`61cf7913` 实施 + `4ddf671f` P3 收口），合父 `ade6d9ea`，归档 `.trellis/tasks/archive/2026-11/11-25-scan-alignment/`。最终候选上 `git diff --check`、`make test`（Go 全部包 + 前端 14 文件 / 162 用例）、`make build` 全过；独立检查 **PASS**（无 P0/P1/P2；6 项 P3）。修复 4 项：safe 模式日志改为「下一个计划扫描」中性口径、Run 循环每轮重取主周期（引擎首轮归一化别名后自愈）、回退路径输出实际等待且启动告警用 1 分钟兜底真实值、补 30m/8h 与 1w/30d「恰在触发时刻」用例。留档 2 项：回退路径按「每轮结束后再等 ScanInterval」（design 已定，与旧 ticker 节拍差异可忽略）、`GetStatus.scan_interval` 仍返回配置值（PRD 明确不改周期配置/前端）。交付：`trader/scan_schedule.go`（`scanCloseDelay=5s`、`scanTimeframe` 复用保留周期、`nextScanTime` UTC 墙钟：1w 周一 / 30d 月初 / 其余含 7d 按 Unix 纪元网格、`After` 语义、非法周期 error、`scanFallbackInterval` 1 分钟兜底）、`trader/auto_trader.go` Run 循环改每轮定时器（启动立即一轮、跳过错过边界、stop 即时退出、每轮重取主周期）、`scan_schedule_test.go` 边界矩阵
- [x] 17 `infra-hygiene`：子分支 `f91e2d0d`（`fa67a9c1` 实施 + `319df460` 检查修复 + `3f300cfc` 复核修复 + `f91e2d0d` P3 收口），合父 `c6ace816`，归档 `.trellis/tasks/archive/2026-11/11-25-infra-hygiene/`。最终候选上 `git diff --check`、`make infra-check`、`make test`（Go 全部包 + 前端 14 文件 / 162 用例）、`make build` 全过；独立检查首轮 NEEDS FIX（1 P1 + 5 P2 + 11 P3）→ 修复后复核又发现 1 P1（主栈服务名）→ 最终 **PASS**（无 P0/P1/P2；3 项 P3 已修）。交付：`.dockerignore` 排除 `.nofx-local/`、`scratch/`、`.env`、`data/` 与本地工具目录；前端健康检查只保留 `Dockerfile.frontend` 一份且探测改 `127.0.0.1`（musl 下 `localhost` 解析为 `::1`、nginx 仅监听 IPv4，主栈前端容器此前常驻 unhealthy）；`Makefile` 新增 `main-stack`（git archive 干净检出构建、`nofx-local/backend|frontend:<sha>`、tag 同步并断言、重建前/构建后/重建后三次 `traders.is_running=0` 拦截、容器 healthy、真实自动拉起日志标记、失败即停）与 `infra-check`（compose 解析、dockerignore 条目、健康检查单份与 127.0.0.1、主栈服务名、标记目录实测上下文排除）；`up` 增加容器 healthy 等待。实测：D20 拦截（含 id:名称）、无表 fail-closed、tag 断言、上下文排除与正向对照；真实主栈重建留父任务收口执行。
