# 保护单适配：仓位级止损止盈构造、随单附带止盈止损、条件单查询改单撤销

## Goal

纯新增：仓位级止损止盈条件单构造（rule 按文档语义、order_type=close-long/short-position、全平 size=0+close、小数张走 amount）、开仓单随单附带止盈止损、查询未触发条件单（按合约与类型过滤）、条件单改单（PUT price_orders/amend/{order_id}）、按 ID 撤销；单测构造矩阵 + 真实挂查改撤

## Requirements

父任务：`11-24-gate-trading-foundation`（Gate 交易基座）。需求与决策以父分支
`docs/plans/2026-11-24-gate-trading-foundation.zh-CN.md` 为准；本文件是该子任务在计划文档
第 10.2 节第 3 行的落地。

### 背景

- 现有 `SetStopLoss`/`SetTakeProfit` 的条件单触发规则四个方向全部与 Gate 文档相反（F6）：
  文档语义是 1 = 价格 ≥ 触发价（触发价须高于最新价）、2 = 价格 ≤ 触发价（触发价须低于最新价）
- 保护单 `close: true` 同时带非零 `size`，且未设仓位级 `order_type`（F8）
- 撤条件单不区分止损与止盈，一次撤光（F10）
- 条件单的小数张数量必须走 `amount` 字符串字段（F32）
- Gate 下单接口支持随单附带止盈止损（`tpsl_tp_trigger_price`、`tpsl_sl_trigger_price`），
  现状是开仓后另发两次调用补挂、失败只记日志（F14；补挂的删除在子任务 4）

### 范围（纯新增，不接入交易路径）

不修改任何现有函数；合入父分支后运行中的 trader 行为不变。交付五个能力：

1. **仓位级止损止盈构造**：按「持仓方向 × 保护类型」构造仓位级止盈止损条件单
   （`order_type` = `close-long-position` / `close-short-position`，全平 `size=0` + `close=true`，
   市价出场，触发 `rule` 按「方向 × 类型」矩阵填写）
2. **随单附带止盈止损**：把止损、止盈触发价写入开仓委托的 `tpsl_*_trigger_price` 字段
   （供子任务 4 的开仓路径使用，满足保护不变量「保护单随开仓单一并提交」）
3. **查询未触发条件单**：按合约列出 `status=open` 的条件单，解析为带类型与方向的视图
   （止损/止盈按 `order_type` + `rule` 判定，修正 F10 的判定基础）
4. **条件单改单**：`PUT /futures/{settle}/price_orders/amend/{order_id}`，路径与请求体
   同时携带订单 ID；改触发价；张数修改经 `amount` 提交（F32）
5. **撤销**：按订单 ID 撤销；按合约 + 类型撤销（只撤止损或只撤止盈）

### 约束

- 适配层只做「外部接口 ↔ 规范单位」的换算与校验，不含交易判断（父任务 design.md）
- 外部数据解析失败即报错，不回落猜测值（父任务 design.md 约定 3）
- 触发价与止盈止损价以十进制字符串承载语义，float64 入参按最短十进制表示取值
  （与子任务 2 的 `decimalFromFloat` 约定一致）
- 旧 `SetStopLoss`/`SetTakeProfit`/`cancelTriggerOrders` 保持原样，由子任务 4 在接入时删除
- 真实写操作在 D35 授权范围内：单笔名义价值 ≤ 10 USDT，每一步立即还原，不在用户有手动
  持仓的合约上验证（2026-11-24 复核账户无持仓）

## Acceptance Criteria

- [ ] 单测构造矩阵：方向（多/空）× 保护类型（止损/止盈）→ `rule`、`order_type`、`initial`
      （`size`/`close`/`price`/`tif`）逐项断言；非法方向、非法类型、非正价格、随单止损止盈
      顺序颠倒全部报错
- [ ] 查询视图解析：触发价字符串解析失败即报错；类型判定矩阵（方向 × 规则 → 止损/止盈）正确
- [ ] 改单请求体：路径与请求体的订单 ID 一致；张数经 `amount` 提交；不带 `rule`
- [ ] 门禁：`git diff --check`、`make test`、`make build` 全部通过
- [ ] 真实验证：挂 → 查 → 改单尝试 → 撤旧建新 → 查 → 撤 → 确认无残留，全程在同一合约上、
      立即还原；过程与读数留档 `probe/`

### 真实验证对交易所行为的实测结论（2026-11-24）

原验收中的「改单后查询核对」在当前交易所环境下不可达成，实证记录见
`research/gate-price-order-docs.md` H 节：

- 新改单路径 `PUT price_orders/amend/{order_id}` 线上 404（未部署，尽管 changelog
  v4.106.160 与 SDK v7.2.184 已采用）
- 旧改单路径对 API 创建的触价单一律拒绝（code 1077 `APIOrderNotSupportUpdateTouchOrder`）
- 仓位级条件单无持仓时被拒（`AUTO_USER_NO_LONG_POSITION`）

据此验收调整为：改单封装按文档实现并由单测覆盖请求体构造；真实验证以「改单尝试 +
记录交易所拒绝 + 撤旧建新替代路径全链路通过」为准；仓位级保护单的挂、查、改、撤
真实验证并入 W2（子任务 4，有测试仓位时）。

## Notes

- 文档依据留档：`research/gate-price-order-docs.md`（文档站直连不可达，内容以父任务留档的
  ZH-FUT 原文摘录 + 官方 SDK v7.2.184 源码双源核对，另附 H 节实测结论）
- 修改止损止盈的实际路径是撤旧建新；改单封装保留（规范路径），由子任务 11 决定接入方式
