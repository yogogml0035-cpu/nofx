# 设计：子任务 6 回撤监控（drawdown-monitor）

## 1. 边界

| 层 | 文件 | 变更 |
|---|---|---|
| Gate 适配层 | `trader/gate/trader_account.go` | `GetPositions` 的 `leverage` 改 float64 实际值（新辅助 `positionLeverage`） |
| 交易路径 | `trader/auto_trader_risk.go` | 归属过滤 `isOwnPosition` + 杠杆读取改 float64 实际值（回落 10 倍仅限不可用） |
| 单测 | `trader/gate/trader_account_test.go`（追加）、`trader/auto_trader_risk_test.go`（新增） | 杠杆解析矩阵、阈值判定、归属判定 |

其他交易所适配器不动（D3）。监控阈值、周期、峰值缓存结构不动。

## 2. 杠杆取实际值

### 字段语义（官方文档，留档于子任务 5 research/gate-leverage-margin-docs.md）

- `lever`：「表示仓位当前杠杆，逐仓和全仓都可以该字段表示，逐步替换当前的 leverage 和 cross_leverage_limit」。
- `leverage`：「逐仓模式下的杠杆倍数。如果是 0，表示当下为全仓模式」。
- `cross_leverage_limit`：「全仓模式下的杠杆倍数」。
- `pos_margin_mode`：`cross` / `isolated`。

### 适配层解析（`positionLeverage`）

```
lever 可解析为正整数      → float64(lever)
否则 pos_margin_mode==cross → cross_leverage_limit 可解析为正整数 → float64
否则                         → leverage（逐仓旧字段）可解析为正整数 → float64
否则                       → 0（不可用）
```

- 与 `marginSettingsFromPosition`（设置路径）的区别：设置路径是下单风险路径，两源矛盾即报错、不挑一个；
  持仓列表是展示/监控读数路径，按 F12 的设计「取不到时按 10 倍计」预留了不可用语义，因此这里只按
  优先级取值、取不到报 0，不让整条 `GetPositions` 失败（否则一个异常条目会打断整个交易循环）。
  `lever` 是文档钦定的权威字段，优先于旧字段；不读 `mode`（不校验单向——列表条目的 mode 不是本任务
  的影响面）。
- map 键仍是 `"leverage"`，类型 int → float64。trader 包内的消费方
  （`auto_trader_decision.go:125/194`、`auto_trader_loop.go:442`、`auto_trader_risk.go:60`、
  死代码 `position_snapshot.go:54`）全部是 `.(float64)` 断言，类型变化后全部开始读到实际值：
  监控的收益率、交易上下文的保证金占用估算都从「恒 10 倍」变为实际杠杆，这正是 F12 要修的缺陷。

### 监控侧

```go
leverage := 10.0 // F12 回落：仅当适配层返回 0（不可用）时
if lev, ok := pos["leverage"].(float64); ok && lev > 0 {
    leverage = lev
}
```

## 3. 仓位归属（D40）

### 判定函数 `isOwnPosition(symbol, side)`

1. **网格子句**：`IsGridStrategy()` 且 `market.Normalize(symbol) == market.Normalize(GridConfig.Symbol)`
   → 属于本 trader。理由：网格由自己的限价单成交建仓，不写 `trader_positions`；网格 trader 只在
   网格品种上交易，该品种上的持仓就是它的。
2. **AI 子句**：`at.store != nil` 且 `GetOpenPositionBySymbol(at.id, market.Normalize(symbol),
   strings.ToUpper(side))` 命中 OPEN 记录 → 属于本 trader。落库侧已确认（独立检查核实）：
   gate 的持仓记录由 `GateTrader.StartOrderSync` → `store.PositionBuilder.ProcessTrade` 写入
   （`trader/gate/order_sync.go` 以 `market.Normalize` 归一化符号、大写 `LONG/SHORT`、`Status: "OPEN"`），
   与查询同源；决策路径的 `recordAndConfirmOrder` 对 gate 提前返回（`auto_trader_decision.go` 的
   OrderSync 交易所分支），不落持仓。
3. **其余一律不属于**：无 store、DB 查询出错（记警告）、无 OPEN 记录、非网格品种。回撤监控会市价平仓，
   是高风险动作——归属不了就不处理，宁可漏处理也不误平。

### 不拒绝启动

D40 明确不采用「启动时存在不可归属持仓则拒绝启动」；现状也没有这样的检查，本任务不新增任何启动校验。

### 峰值缓存

不可归属的仓位在进入计算前就跳过：不读缓存、不更新峰值、不平仓（对模型不可见属子任务 9，这里只保证
监控不动它）。

## 4. 全平语义

`emergencyClosePosition` 维持 `CloseLong/CloseShort(symbol, 0)`。Gate 适配层的 quantity=0 路径是
`close=true + size=0`（研究 C.1：单向持仓模式下仓位级全平的文档语义；子任务 4 已交付并在 W2/W3 实测）。
不做「先换算张数再 reduce_only」的替代实现：向下取整会在全平场景留下残余仓位。单测钉住路由
（long → CloseLong(symbol,0)、short → CloseShort(symbol,0)）。平仓后不加保护单清理——决策平仓路径
同样不清理，随单保护随仓消亡（W2/W3 零残留）。

## 5. 兼容性

- `GetPositions` 的 map 契约只变 `leverage` 的类型（int → float64）；trader 包消费方全部兼容（见上）。
  其他适配器返回类型未统一核查，但本任务不改它们；监控代码对两者都兼容（断言失败回落 10 倍保持原有
  行为——其他适配器维持现状语义，Gate 得到实际值）。
- 网格 trader 原来会被监控处理（网格品种持仓 + 账户内全部持仓），改后网格品种持仓仍被处理，
  网格品种以外的持仓不再被处理——这正是 D40 的收敛。
- AI trader 在 DB 有 OPEN 记录的仓位行为不变；重启后 DB 记录仍在，归属判定跨重启稳定。

## 6. 回滚

单提交边界，涉及 2 个源文件 + 2 个测试文件；回滚按提交边界手动恢复文件。
