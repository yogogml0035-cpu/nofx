# 技术设计：杠杆与保证金模式（unified-trading-params）

## 边界

接入交易路径并删除旧语义。合入父分支后，AI 开仓与网格初始化按 trader 的设置生效保证金模式与杠杆；
设置生效失败则不开仓、不启动网格。

- `trader/gate/margin_settings.go`：新增。仓位设置的解析与读回、一次性生效、两个旧方法的新实现
- `trader/gate/protected_open.go`：提交订单前一次性生效；删除「尽力而为」的杠杆设置
- `trader/gate/trader_orders.go`：删除旧的 `SetLeverage`、`SetMarginMode`（迁到 margin_settings.go 重写）；
  `openMarket` 的杠杆设置改为失败即不开仓
- `trader/types/interface.go`：新增能力接口 `MarginSettingsApplier`；`ProtectedOpenRequest` 带上保证金模式
- `trader/auto_trader_orders.go`：删除独立的 `SetMarginMode` 调用，保证金模式随开仓请求传入
- `trader/auto_trader_grid.go`：网格初始化按 trader 的设置一次性生效，生效后才标记为已初始化
- 不动：其他交易所适配器、`GetPositions` 的杠杆字段（回撤监控取实际杠杆属于子任务 6）、
  `GridTraderAdapter`（各交易所共用，保持现状）、账户的持仓模式与保证金模式

## 文档依据

原文摘录见 `research/gate-leverage-margin-docs.md`（章节号指该文件）。与本设计直接相关的结论：

| 设计项 | 依据 | 性质 |
|---|---|---|
| 新接口 `POST /futures/{settle}/positions/{contract}/set_leverage`：`leverage`（必选，仓位杠杆倍数）、`margin_mode`（必选，`isolated` / `cross`）、`dual_side`（必选：否）；说明只有一句「为了简化leverage接口复杂逻辑，新增修改杠杆的接口」；返回 Position。changelog `v4.106.12` 引入 | A.1、D.1 | 文档站原文 |
| 文档没有为 `set_leverage` 写注意事项、`leverage` 的取值范围、持仓或挂单限制、逐接口的错误响应 | A.1、C.1、未找到的项 1–3 | 文档站原文 |
| `dual_side` 是否必填，文档自相矛盾：`set_leverage` 页面标「否」；changelog `v4.106.43` 写「全站复用的查询参数 `pos_margin_mode` 与 `dual_side` 在引用场景中改为必填要求」 | A.1、D.1 | 文档站原文 |
| 旧接口 `POST /futures/{settle}/positions/{contract}/leverage`：「leverage ≠ 0：逐仓模式」「leverage = 0：全仓模式（使用 cross_leverage_limit 设置杠杆倍数）」 | A.3 | 文档站原文 |
| Position：`leverage`「逐仓模式下的杠杆倍数。如果是0，表示当下为全仓模式。全仓杠杆模式下的倍数请以 cross_leverage_limit 为准」；`cross_leverage_limit`「全仓模式下的杠杆倍数」；`pos_margin_mode`「isolated - 逐仓, cross - 全仓」；`lever`「表示仓位当前杠杆，逐仓和全仓都可以该字段表示 逐步替换当前的leverage和cross_leverage_limit」；`mode` `single` / `dual_long` / `dual_short` | A.11 | 文档站原文 |
| 文档的 Position 响应示例自相矛盾：`leverage` 为 `"0"`、`cross_leverage_limit` 为 `"0"`，而 `pos_margin_mode` 为 `"isolated"`、`lever` 为 `"30"`。不能当测试夹具 | A.12 | 文档站原文（照录） |
| `GET /futures/{settle}/get_leverage/{contract}`：`pos_margin_mode`「分仓模式必传」与 `dual_side` 在页面上都标必选；响应只有 `Lever` | A.2、A.14 | 文档站原文 |
| 根页 label 表：`POSITION_NOT_FOUND` 仓位不存在、`NO_CHANGE` 没有改变发生、`LEVERAGE_TOO_HIGH` / `LEVERAGE_TOO_LOW`、`POSITION_HOLDING` 有持仓则不允许此操作、`ORDER_PENDING` 有委托存在则不允许此操作、`INVALID_PARAM_VALUE` | C.2 | 文档站原文 |
| 「统一账户永续合约的 API 操作与经典账户完全相同，当前只支持 USDT 结算」；三个页面都没有「某种账户保证金模式下不能设杠杆或切换全逐仓」的句子 | B | 文档站原文 |
| Agent 文档页：`cex_fx_update_fx_position_leverage`（调整杠杆）只有工具名，没有参数说明；页面上没有「全仓」「逐仓」字样 | E | 文档站原文 |
| `get_leverage` 在单向持仓模式下不带 `dual_side` 一律 `INVALID_PARAM_VALUE`；带上 `dual_side=dual_long` 返回 200，但三个合约都返回 `10`，与持仓条目的 `lever`（75、200）不一致 | 2026-11-24 只读实测 | 实测 |
| 持仓条目带 `leverage`、`cross_leverage_limit`、`lever`、`pos_margin_mode`；`size=0` 的合约也有条目 | 2026-11-24 只读实测（BTC 全仓 10x、DOGE 全仓 75x、ETH 全仓 200x，三个字段互相一致） | 实测 |
| 没有显式设置时，首次建仓的合约在本账户下落到全仓 + 该合约的最大杠杆 | 2026-11-24 只读实测：子任务 4 的 W2/W3 传杠杆 0、没有调用设置接口，之后 DOGE、ETH 的条目是全仓 75x、200x（research G.1） | 实测 |
| 合约元数据 `leverage_min` / `leverage_max` / `cross_leverage_default`：BTC 1 / 200 / 10，DOGE 1 / 75 / 10，ETH 1 / 200 / 10 | 2026-11-24 只读实测 | 实测 |

W4（2026-11-24，`probe/README.md`、research G.3）：

| 设计项 | 依据 | 性质 |
|---|---|---|
| `set_leverage` 线上已部署：不带 `dual_side`，单向持仓模式下返回 200，返回体是 Position（带 `mode: "single"`） | W4 第 1 步 | 实测 |
| 条目形态：全仓 `leverage="0"`、`cross_leverage_limit=N`、`lever=N`；逐仓 `leverage=N`、`lever=N`，`cross_leverage_limit` 残留上一次全仓的值 | W4 第 1–8 步 | 实测 |
| 重复设置同一组值返回 200，不报 `NO_CHANGE` | W4 第 2 步 | 实测 |
| 超过合约杠杆上限被拒：`LEVERAGE_EXCEEDED`（`limit [1, 75]`）；这个 label 不在根页的 label 表里 | W4 第 6 步 | 实测 |
| 从未设置过的合约 `GET positions/{contract}` 返回 HTTP 400 + `POSITION_NOT_FOUND` | 只读实测（SOL、LTC、XRP、ADA） | 实测 |
| 账户处于跨币种保证金模式时逐仓可设置并读回；切换模式时新接口把杠杆带过去 | W4 第 4–8 步 | 实测 |

不采用 `get_leverage` 做读回：单向持仓模式下它的入参与返回值都不反映该合约的实际设置（实测）。
SDK v7.2.184 的英文定义与文档站在路径、参数、字段上一致（research F 节）。

## 语义

### 一次性生效（核心）

```
ApplyMarginSettings(symbol, isCrossMargin, leverage)
  1. leverage < 1 → 报错，不发任何请求
  2. POST positions/{contract}/set_leverage?leverage={leverage}&margin_mode={cross|isolated}
     不带 dual_side（账户是单向持仓模式）
  3. 读回核对：返回的 Position 解析出的设置 == 目标设置 → 成功
  4. 否则读回持仓条目（GET positions/{contract}）再核对一次：
       条目 == 目标设置 → 成功（如请求超时但交易所已生效；W4 实测重复设置同一组值时交易所直接返回 200）
       否则 → 报错（带上请求值、读回值、交易所错误）
```

「成功」的定义：设置接口的返回值或持仓条目之一显示的正是目标设置。
解析时杠杆优先取 `lever`（文档指定的统一表示）；响应里没有可用的 `lever` 时按模式取旧字段
（全仓 `cross_leverage_limit`、逐仓 `leverage`）。两处都对不上就报错，调用方不得继续下单。

### 接入 AI 开仓

`ProtectedOpenRequest` 新增 `IsCrossMargin`。`openProtected` 在全部入参校验通过之后、提交订单之前调用
`ApplyMarginSettings(symbol, req.IsCrossMargin, req.Leverage)`；失败即返回错误，不提交订单。

- 保证金模式来自 `AutoTraderConfig.IsCrossMargin`（trader 界面的开关），杠杆来自决策
- 交易路径不再单独调用 `SetMarginMode`：实现 `ProtectedOpener` 的适配器负责按开仓请求生效保证金设置。
  当前只有 Gate 实现了 `ProtectedOpener`，其他适配器在 `protectedOpener()` 处已被拒绝开仓

### 接入网格初始化

适配器实现 `MarginSettingsApplier` 时一次性生效，失败终止初始化，网格不被标记为已初始化
（原实现先标记已初始化再尽力设置）。其余适配器沿用原来的两次调用，设置失败不阻断（行为不变）。

### 两个旧方法（`types.Trader` 接口，其他交易所适配器仍在用）

| 方法 | Gate 上的语义 | 当前模式未知时 |
|---|---|---|
| `SetLeverage(symbol, leverage)` | 保持该合约当前的保证金模式，只改杠杆 | 从未设置过的合约（没有条目）按逐仓：这类调用不带模式信息，逐仓把亏损限制在仓位保证金内，也与这两个原语过去的行为一致 |
| `SetMarginMode(symbol, isCrossMargin)` | 保持当前生效的杠杆，只改模式 | 没有条目就没有可保持的杠杆，报错并提示带上杠杆一并设置 |

`OpenLong` / `OpenShort` 普通市价原语在 `leverage > 0` 时调用 `SetLeverage`，失败即不开仓
（调用方明确给了杠杆，就必须生效）；`leverage = 0` 表示不设置，沿用该合约在交易所的现有设置。

## 数据结构与函数

```go
// trader/types/interface.go

// MarginSettingsApplier 按 trader 的设置一次性生效仓位的保证金模式与杠杆。
// 保证金模式与杠杆在交易所上是一次设置，分开设置时各缺一半信息；自动交易的开仓与网格
// 在交易前调用它，返回错误即不交易。
type MarginSettingsApplier interface {
    ApplyMarginSettings(symbol string, isCrossMargin bool, leverage int) error
}

type ProtectedOpenRequest struct {
    ...
    IsCrossMargin bool // 全仓为 true、逐仓为 false；提交订单前与 Leverage 一并生效
}

// trader/gate/margin_settings.go

// ErrNoPositionSettings 合约还没有仓位设置条目（从未设置过杠杆，也没有开过仓）
var ErrNoPositionSettings = errors.New("no position settings for contract")

// MarginSettings 合约当前的仓位设置
type MarginSettings struct {
    Cross    bool // 全仓为 true，逐仓为 false
    Leverage int  // 当前杠杆：优先取 lever，旧响应没有 lever 时按模式取设置字段
}

// PositionMarginSettings 读回合约的仓位设置（GET positions/{contract}）；
// 没有条目返回 ErrNoPositionSettings；模式未知、杠杆缺失或不是正整数都报错
func (t *GateTrader) PositionMarginSettings(symbol string) (MarginSettings, error)

// ApplyMarginSettings 一次性生效并读回核对
func (t *GateTrader) ApplyMarginSettings(symbol string, isCrossMargin bool, leverage int) error
```

### 设计要点

1. **解析**：`marginSettingsFromPosition` 的模式只接受 `cross` / `isolated`；杠杆经 `parseDecimal`
   精确解析，必须是正整数且在 `int` 范围内。`lever` 取不到正整数时才按模式取旧字段。
2. **没有条目是明确的状态**：`GET positions/{contract}` 返回 `POSITION_NOT_FOUND` 时返回
   `ErrNoPositionSettings`，调用方按语义决定；其他错误原样返回。
3. **失败闭合**：设置没有读回一致就不继续。旧的 `RISK_LIMIT_EXCEEDED` 被吞成成功的分支删除。
4. **不做本地杠杆范围校验**：文档没有给出 `set_leverage` 的取值范围；范围以交易所为准，
   被拒绝（`LEVERAGE_TOO_HIGH` / `LEVERAGE_TOO_LOW`）或被截断都会在读回核对时暴露。
5. **不带 `dual_side`**：文档页面标它为可选（changelog 另有「改为必填」的说法），W4 实测单向持仓模式下
   不带它返回 200。NOFX 的下单模型建立在单向持仓模式上（张数带方向、`close=true` 全平），
   所以返回体带 `mode` 时必须是 `single`，否则报错。

## 真实写操作验证（W4，D35 授权范围内）

对象：DOGE_USDT（当前无持仓、无挂单；已有设置条目，原设置可以精确还原）。
运行前只读记录基线：账户的 `position_mode` / `margin_mode`、三个合约的设置条目、挂单与条件单数。
脚本调用交付的适配器代码（与线上同一路径），每步打印读数（`pos_margin_mode`、`leverage`、
`cross_leverage_limit`、`lever`、`mode`），结束时还原并核对：

| 步 | 操作 | 核对 |
|---|---|---|
| 1 | 全仓 10x | 返回值与持仓条目：`cross`、`cross_leverage_limit=10`、`lever=10` |
| 2 | 再设一次全仓 10x（重复设置） | 记录交易所的响应；最终读回仍是全仓 10x |
| 3 | 全仓 5x | 读回 5 |
| 4 | 逐仓 5x | 返回值与持仓条目：`isolated`、`leverage=5`、`lever=5` |
| 5 | 逐仓 3x | 读回 3 |
| 6 | 非法值：杠杆 0、超过合约上限（100 > 75） | 预期被拒且读回不变；记录错误 label |
| 7 | `SetLeverage` 10x | 保持逐仓，只改杠杆：`isolated`、`leverage=10` |
| 8 | `SetMarginMode` 全仓 | 保持杠杆：`cross`、`cross_leverage_limit=10` |
| 9 | 还原：全仓 75x | 读回与基线一致 |
| 10 | 终态 | 三个合约的条目与基线一致；无持仓、无挂单、无条件单 |

另做只读探测：对一个从未设置过的合约 `GET positions/{contract}`，确认返回 `POSITION_NOT_FOUND`。

脚本放 `probe/livecheck.go`（独立 main，不进构建产物），证据输出留档。容器出口 IP 不在 Gate key 的白名单内，
脚本在 Docker 内交叉编译后在宿主机运行，凭据经环境变量传入，不落盘。

## 已知边界

- W4 在账户当前的跨币种保证金模式下完成；单币种模式下的读回需在账户切换后复验（只读即可）。
  代码不依赖账户的 `margin_mode`，依据是文档「统一账户永续合约的 API 操作与经典账户完全相同」
- 新接口 `set_leverage` 线上已部署（W4 实测；子任务 3 实测过改单接口在线上是 404，两者不同），
  没有用到旧接口 `positions/{contract}/leverage` 的回退
- 文档没有说明带仓或有挂单时能否修改设置；label 表里有 `POSITION_HOLDING` 与 `ORDER_PENDING`。
  W4 只在未持仓的合约上做，这种情况没有测；运行中被交易所拒绝时按失败闭合处理，不开新仓
- 双向与分仓持仓模式下 `dual_side` 是否必填没有测（账户是单向持仓模式）；返回体的 `mode` 不是
  `single` 时适配器报错
- 助手手动交易与网格共用的 `GridTraderAdapter` 仍以「失败只告警」的方式调用 `SetLeverage`（各交易所共用，本子任务不动）
- 助手手动交易（`agent/trade.go` 的 `execute_trade`）走普通市价原语，不带保证金模式：沿用该合约当前的模式，
  从未设置过的合约按逐仓。它不读取 trader 界面的全仓 / 逐仓开关，是否跟随该开关不在本子任务范围内
