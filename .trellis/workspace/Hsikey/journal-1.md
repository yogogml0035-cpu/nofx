# Journal - Hsikey (Part 1)

> AI development session journal
> Started: 2026-11-18

---



## Session 1: 子任务 2 contract-math：Gate 合约张数换算底座
<!-- trellis-session: v=2 fp=a69afc9778e612af -->

**Date**: 2026-11-24
**Task**: 子任务 2 contract-math：Gate 合约张数换算底座
**Branch**: `feat/2026_1123_gate-trading-foundation`

### Summary

补登父任务 Trellis 记录；交付子任务 2（USDT 名义价值与分批比例换算为合约张数、币数量与张数互转、带缓存的合约元数据读取），独立检查通过，门禁通过，已快进合入父分支并归档

### Main Changes

- 新增 trader/gate/decimal.go 与 contract.go：精确有理数换算，按最小步长向下取整，不足一个步长、低于最小下单金额、超过市价单最大张数时不下单
- 查证并记入计划文档：步长取 order_size_min 是推论（官方无明文，由 W3 验证）；交易所层面没有最小下单金额；市价单另有最大张数 market_order_size_max；请求头计划废弃；条件单小数张走 amount
- 后端质量规范写入三条约定：禁止浮点除法截断推导下单数量、解析失败不得回落猜测值、交易所数量字段按十进制字符串处理

### Git Commits

| Hash | Message |
|------|---------|
| `daa8369a` | docs(trellis): 登记 Gate 交易基座父任务——prd、设计约定、子任务执行计划与上下文清单 |
| `2ae06cca` | docs(trellis): 张数换算子任务规划——需求、设计、执行计划与 Gate 合约张数文档原文 |
| `f33d78e3` | feat(gate): 合约张数换算——USDT 名义价值与分批比例换算为张数、合约元数据读取 |
| `62ab50a5` | fix(gate): 按独立检查修正张数换算——容差、取整除法与注释 |
| `5d41dde5` | docs(trellis): 张数换算子任务收口——检查结论、设计边界、规范更新与证据 |
| `590ba577` | docs(trellis): 张数换算子任务门禁结果——diff --check、make test、make build 通过 |
| `c5edd1ae` | chore(task): archive 11-24-contract-math |
| `75dfbedd` | docs(plans): 子任务 2 contract-math 收口——实施进度、张数换算的文档依据与留给后续子任务的事项 |

### Testing

- [OK] trader/gate 包内测试带竞态检测通过；计划文档 6.1 节两张表逐格复算一致
- [OK] 7 个合约元数据真实只读读数：contracts=7 failed=0 mismatched=0（证据在归档任务的 probe/）
- [OK] make test：后端 22 个含测试的包通过，前端 142 个用例通过；make build 通过；git diff --check 通过
- [OK] 独立检查子智能体：通过，23.1 万个用例独立对照零差异；10 条建议级发现已全部处理（62ab50a5）

### Status

[OK] **Completed**

### Next Steps

- 子任务 3 protective-order-adapter：保护单适配（真实挂、查、改、撤）；小数张用 amount 提交
- U1（手动持仓是否归机器人管）仍待用户确认，只阻塞子任务 6


## Session 2: 子任务 5 unified-trading-params：杠杆与保证金模式一次性生效（W4）
<!-- trellis-session: v=2 fp=b8687177271a7d7b -->

**Date**: 2026-11-24
**Task**: 子任务 5 unified-trading-params：杠杆与保证金模式一次性生效（W4）
**Branch**: `feat/2026_1123_gate-trading-foundation`

### Summary

按 trader 界面的设置生效全仓/逐仓与杠杆：走 Gate 新接口 set_leverage，以返回值或持仓条目读回一致为成功，失败不开仓；W4 在 DOGE_USDT 上实测通过并还原；独立检查首轮 NEEDS FIX（无 P0/P1），修复后复核 PASS

### Main Changes

- trader/gate/margin_settings.go：ApplyMarginSettings（能力接口 types.MarginSettingsApplier）、PositionMarginSettings，重写 SetLeverage/SetMarginMode；删除恒为逐仓的旧实现、空操作的 SetMarginMode 与被吞成成功的 RISK_LIMIT_EXCEEDED 分支
- 随单保护开仓在下单前生效设置、失败不开仓（ProtectedOpenRequest.IsCrossMargin）；交易路径不再单独调用 SetMarginMode；网格初始化在设置生效后才标记已初始化；普通市价原语在调用方给了杠杆时失败即不开仓
- 读回解析：lever 与同模式旧字段只有一个能读时用它，都能读时必须相等（矛盾即报错）；返回体的持仓模式必须是 single；交易所错误留在错误链里
- 规格：新增 backend/exchange-adapters.md；quality-guidelines 增加「交易所侧设置失败不得只告警继续下单」与测试桩错误响应须带 JSON Content-Type；计划文档 10.5 收口并新增 F36/F37

### Git Commits

| Hash | Message |
|------|---------|
| `b8252754` | feat(trader): 保证金模式与杠杆一次性生效——按 trader 设置走新接口 set_leverage、读回核对、失败不开仓 |
| `d92328b9` | chore(task): W4 实测证据入库（LIVECHECK_PASS 24 步，DOGE_USDT 还原全仓 75x）并回写实测结论 |
| `4eedd743` | docs(spec): 适配层契约——下单前生效并读回交易所侧设置；适配层测试桩的错误响应须带 JSON Content-Type |
| `d758d3ec` | fix(trader): 按独立检查收口杠杆与保证金模式——读回矛盾即报错、补齐回退分支与 label 区分的测试 |
| `eb84f2e4` | chore(task): W4 第三轮证据入库（在 d758d3ec 上，LIVECHECK_PASS 23 步 + 1 条观察）并按独立检查修正文档表述 |
| `e1b6fdc3` | test(trader): 按复核的 P3 钉住次要分支的错误链；规格与文档措辞对齐证据 |
| `aa06dc75` | docs(task): 子任务 5 记录最终门禁与独立检查结果——e1b6fdc3 上 diff --check、make test、make build 全过，复核 PASS |
| `ccd2f51c` | merge: 杠杆与保证金模式子任务并入父分支（feat/2026_1123_gate-trading-foundation--unified-trading-params b8252754 + d92328b9 + 4eedd743 + d758d3ec + eb84f2e4 + e1b6fdc3 + aa06dc75） |

### Testing

- [OK] W4：DOGE_USDT 全仓/逐仓 × 10x/5x/3x 设置并读回、重复设置、超上限被拒、两个旧方法，在 d758d3ec 上 LIVECHECK_PASS（23 步 + 1 条观察），还原为全仓 75x，BTC/ETH 条目不变，可用余额前后一致
- [OK] 最终候选 e1b6fdc3：git diff --check 无输出；make test 后端 22 个含测试的包 ok、前端 11 个文件 142 个用例通过；make build 通过
- [OK] 独立检查：首轮 4 项 P2 + 9 项 P3（变异实验 31 个变体杀死 26 个）→ 修复 → 复核 PASS（53 个可编译变体杀死 50 个）→ 余下 3 个 P3 修复并由主会话变异复验全部杀死

### Status

[OK] **Completed**

### Next Steps

- 子任务 6 drawdown-monitor（等 U1）：回撤监控取实际杠杆读条目的 lever——GetPositions 现在仍返回整数 leverage（全仓为 0）；设置是合约级的，同一合约上已有反向持仓时开仓前生效设置会同时改动那个仓位
- 待用户决定：是否授权在可弃合约上补测「从没有设置过的合约上的 set_leverage」（会留下无法还原为无条目的条目）；账户切回单币种保证金模式后做一次只读复验；DOGE_USDT、ETH_USDT 目前是全仓 75x/200x
- U1（手动持仓是否归机器人管）仍待用户确认，只阻塞子任务 6
