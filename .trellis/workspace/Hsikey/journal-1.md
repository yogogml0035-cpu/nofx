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


## Session 3: 子任务 7 account-equity 收口：复审 PASS、合入父分支并归档
<!-- trellis-session: v=2 fp=4cdc84be88ff4d0b -->

**Date**: 2026-11-25
**Task**: 子任务 7 account-equity 收口：复审 PASS、合入父分支并归档
**Branch**: `feat/2026_1123_gate-trading-foundation`

### Summary

代码候选 20a1c61c：Gate 取钱包总额本金、口径唯一定义、初始资金可编辑与存量重建；门禁 make test/make build 全过；第 4 轮真实读数 LIVECHECK_PASS；第二轮界面实测覆盖换绑重置与改过才发送；独立检查复审 PASS（首轮 NEEDS FIX 各项全部修复、变异 0 存活）。已合入父分支（831dfb5e）并归档（archive/2026-11/11-24-account-equity），计划文档 10.5/F38–F40/3.1 同步，父分支已推送 b343cace。下一步：子任务 8 strategy-config（子任务 6 等用户决定 U1）

### Git Commits

| Hash | Message |
|------|---------|
| `831dfb5e` | merge: 账户权益与本金子任务并入父分支（feat/2026_1123_gate-trading-foundation--account-equity c933a7c4 + e73c3c06 + 88982a24 + cd9cfd88 + ed4aed64 + b6da1239 + 9805ba50 + 83d2dd95 + 20a1c61c + abb6e79d + 4cdc05a3 + 65726249 + 3f3cba7a，代码候选 20a1c61c，独立检查复审 PASS，门禁 make test / make build 全过） |
| `09f2015b` | chore(task): archive 11-24-account-equity |
| `f5399125` | chore(task): 修正归档任务 jsonl 与证据说明里的自引用路径 |
| `b343cace` | docs(plans): 子任务 7 account-equity 收口——10.5 进度、F38–F40 实测发现、3.1 可用额度公式更正、父任务子任务地图 |

### Testing

- [OK] make test / make build（20a1c61c，全过）；检查子智能体变异 15 杀死 1 不可编译 0 存活

### Status

[OK] **Completed**

### Next Steps

- 子任务 8 strategy-config；U1（手动持仓是否归机器人管）仍待用户确认


## Session 4: 记录 U1 的确认结论（D40）与 W5 补测授权（D41）
<!-- trellis-session: v=2 fp=0709ccd96e1791eb -->

**Date**: 2026-11-25
**Task**: 记录 U1 的确认结论（D40）与 W5 补测授权（D41）
**Branch**: `feat/2026_1123_gate-trading-foundation`

### Summary

用户 2026-11-25 确认 U1：不是本 trader 开出的持仓不归机器人管，出现时直接无视，也不因此拒绝 trader 启动（交易子账户后续不再手动开仓）；授权 W5：在从没有设置过仓位设置的合约上调用 set_leverage 并读回（会留下一条无法还原的条目）。计划文档新增 D40、D41 与 W5 行，第 7 节改为结论，子任务 6 的行与父任务 prd.md、implement.md 同步。助手的对话内直接下单保持现状（不跟随 trader 的全仓/逐仓开关）。本轮只做记录，没有开工子任务 6，也没有执行补测。

### Git Commits

| Hash | Message |
|------|---------|
| `59defea8` | docs(plans): 记录 U1 的确认结论（D40）与 W5 补测授权（D41）——不是本 trader 开出的持仓直接无视、不因此拒绝启动 |

### Status

[OK] **Completed**

### Next Steps

- 子任务 6 drawdown-monitor（U1 已决，可开工）；做真实接口探测时执行 W5 并把结果记进计划文档 F37


## Session 5: 子任务 6 drawdown-monitor 收口：合父、归档、计划文档同步
<!-- trellis-session: v=2 fp=8ed857ae07a4e902 -->

**Date**: 2026-11-25
**Task**: 子任务 6 drawdown-monitor 收口：合父、归档、计划文档同步
**Branch**: `feat/2026_1123_gate-trading-foundation`

### Summary

回撤监控按实际杠杆（lever）与仓位归属（D40）收敛，独立检查两轮复核 PASS，带仓只读真实读数 LIVECHECK_PASS；门禁 diff-check/make test/make build 全过；计划文档 3.6/F12/文件表/10.5 与父任务 prd/implement 已同步

### Git Commits

| Hash | Message |
|------|---------|
| `c0eadc1f` | feat(trader): 回撤监控按实际杠杆与仓位归属收敛——Gate 杠杆读 lever 修复恒 10 倍（F12）、非本 trader 持仓直接无视不拒启动（D40）、全平维持 close 原语；13 组监控单测 + 适配层杠杆解析矩阵；探针桥（livecheck tag）与带仓只读真实读数 LIVECHECK_PASS（子任务 6 drawdown-monitor） |

### Status

[OK] **Completed**

### Next Steps

- 子任务 8 strategy-config 开工（无依赖）；随后 9 sizing-engine（依赖 6、7，均已合父）


## Session 6: ⑧ strategy-config 收口：百分比开仓配置与周期收敛合父归档
<!-- trellis-session: v=2 fp=30dbe71910e61488 -->

**Date**: 2026-11-25
**Task**: ⑧ strategy-config 收口：百分比开仓配置与周期收敛合父归档
**Branch**: `feat/2026_1123_gate-trading-foundation`

### Summary

子任务 8（strategy-config）完成：一次性删除权益×仓位价值倍数整套旧语义（store 字段/clamp/diff、kernel 校验与提示词、trader/助手封顶、锁定名单/摘要/关键词/诊断文案/skills JSON/api schema/架构文档两份）；新增 risk_control.position_percent（1–20，步长 0.1，默认 10，legacy 0 值补默认，clamp 矩阵单测）；min_position_size 默认 12→10 并解锁为前端/助手可编辑；策略周期按 D25 收敛为 Gate 文档枚举加 1w/30d（normalizeTimeframe/前端选项/schema/兜底 3m→5m）。门禁：git diff --check、make test（Go 22 包+前端 158 用例）、make build、tsc、vitest、vite build 全绿；独立检查 PASS（1 P2+4 P3：P2 断言收紧、删 isBTCETH 死代码、en 标签对齐已修复补验；P3-2/P3-3 留档 F41/F42）。合父 9fd74a2c、归档 febd8d78、文档同步 1404b8eb。

### Git Commits

| Hash | Message |
|------|---------|
| `1404b8eb` | docs(task): 子任务 8 strategy-config 收口——计划文档 10.5 进度与 F41/F42 留档、父任务 prd/implement 子任务地图同步归档路径 |

### Status

[OK] **Completed**

### Next Steps

- ⑨ sizing-engine（本金×百分比金额计算与强制、D34 亏损上限、删 12/60 硬编码与盈亏比校验、提示词契约去金额字段）；随后 10→17 串行


## Session 7: 子任务 9 sizing-engine 收口：金额由代码计算、亏损上限校验与提示词去金额契约
<!-- trellis-session: v=2 fp=9265d5948250b5c1 -->

**Date**: 2026-11-25
**Task**: 子任务 9 sizing-engine 收口：金额由代码计算、亏损上限校验与提示词去金额契约
**Branch**: `feat/2026_1123_gate-trading-foundation`

### Summary

开仓名义价值改由 kernel 按本金 × position_percent 精确计算（big.Rat，F33）；Decision 去 position_size_usd/risk_usd、NotionalUSD 代码填充；新增止损止盈方向与 2% 亏损上限校验（D34），删除 12/60 与盈亏比 ≥3 代码校验（D12）；提示词/API/助手/文档同步；独立检查首轮 NEEDS FIX 修复后 PASS；make test/build 全绿；合父 ab2ca8d0 并归档，下一步 ⑩ partial-close

### Git Commits

| Hash | Message |
|------|---------|
| `9eeaa72a` | test+docs(sizing): 检查复核建议收口——base 匹配与 test-run 越界用例、NaN/Inf 保护价、指南残留与探针来源标注 |

### Status

[OK] **Completed**


## Session 8: 子任务 10 partial-close 收口：决策契约、持仓可分批标记与 W2 真实分批平仓
<!-- trellis-session: v=2 fp=8924e4ad168f3caa -->

**Date**: 2026-11-25
**Task**: 子任务 10 partial-close 收口：决策契约、持仓可分批标记与 W2 真实分批平仓
**Branch**: `feat/2026_1123_gate-trading-foundation`

### Summary

新增 close_long_partial/close_short_partial 与 close_percent（(0,100)，100 指向全平）；Gate 适配器 PartialCloser 精确按 SizeForCloseRatio 换算并提交 reduce-only 市价单；持仓信息逐仓标记可分批；执行 fail-closed、提示词中英、前端卡片与架构/规范同步；W2 实测通过并经独立检查复审 PASS；合父、归档、清理子分支树

### Main Changes

- trader/types：PartialCloser/PartialCloseStatus/PartialCloseRequest/PartialCloseResult
- trader/gate：partial_close.go（状态、精确换算、市价单最大张数、reduce-only）；编译期能力断言；单测矩阵
- kernel：Decision.ClosePercent 与两动作校验；PositionInfo 可分批字段；提示词动作清单/示例/持仓标记
- trader：executePartialCloseWithRecord、能力断言、排序优先级、DecisionAction.close_percent；前端 DecisionCard 映射
- 文档与规范：STRATEGY_MODULE 中英、exchange-adapters 新增 Partial close by ratio 场景

### Git Commits

| Hash | Message |
|------|---------|
| `3985f016` | merge: 分批平仓并入父分支（feat/2026_1123_gate-trading-foundation--partial-close 7cd7788f，独立检查 PASS，make test 全部包 + 前端 160 用例、make build、W2 LIVECHECK_PASS） |

### Testing

- [OK] git diff --check（含 f902f7db..7cd7788f）
- [OK] make test：后端全部包 + 前端 14 文件 / 160 用例
- [OK] make build
- [OK] W2 LIVECHECK_PASS：DOGE 17 张 → 50% 平 8 留 9 → 保护回读 → 100%/1% 拒绝 → 全平零残留
- [OK] 独立检查首轮 NEEDS FIX 修复后复审 PASS

### Status

[OK] **Completed**

### Next Steps

- ⑪ protective-order-management：先读归档 probe/README.md 与 F43（分批后条件单列表形态），再设计改单/撤旧建新
- 其后按 12→17 串行；父任务收口前做组合审查、E2E 与 make ci-local


## Session 9: 子任务 11 protective-order-management：止损止盈管理与触发价对齐
<!-- trellis-session: v=2 fp=495b9dfe8f4efb47 -->

**Date**: 2026-11-25
**Task**: 子任务 11 protective-order-management：止损止盈管理与触发价对齐
**Branch**: `feat/2026_1123_gate-trading-foundation`

### Summary

交付每轮读取当前止损止盈、update_stop_loss/update_take_profit 决策动作与校验、撤旧建新改单（先挂新核对→撤旧→核对、失败恢复）、触发价对齐与保护核对收紧；检查修复后独立复核 PASS，W2/W3 实测通过，已合父归档

### Main Changes

- Gate：ContractSpec 价格步长与 AlignTriggerPrice；开仓对齐随单 tpsl；VerifyOrderProtection 以未触发条件单列表为准（3×300ms 重试、持仓已平视为无需保护、方向由持仓推导）
- trader/gate/protection_management.go：保护读取（同类取最近/计数）、改单执行与失败恢复、2% 上限 1e-12 容差、恢复有效价核对
- types：ProtectiveOrderManager/PositionProtection/ProtectionUpdate*/ErrPositionUnprotected；ProtectedOpenResult 回报实际保护价
- kernel：update_stop_loss/update_take_profit 校验（持仓/可读/方向/2%/符号变体）、PositionInfo 当前 SL/TP、提示词中英；交易路径每轮读取、执行与紧急平仓、符号变体路由、排序 2/3/4
- 前端 DecisionCard UPDATE SL/TP；架构中英与适配器规范新场景；计划文档 F43 辨明、F44/F45 留档、10.5 进度

### Git Commits

| Hash | Message |
|------|---------|
| `9af310c1` | feat(protection): 止损止盈管理——每轮读取当前保护、修改动作与撤旧建新、触发价对齐（子任务 11） |
| `e5e1e706` | fix(protection): 检查修复——无保护哨兵进错误链、符号变体匹配、执行侧 2% 容差与恢复有效价核对 |
| `8dfbb326` | docs(protection): 修正符号比较键注释的适用范围（不涉及行为） |
| `d0072836` | merge: 止损止盈管理并入父分支（子任务 11：9af310c1 实施 + e5e1e706 检查修复 + 8dfbb326 注释订正；独立检查 PASS，W2/W3 LIVECHECK_PASS，make test/build 全过） |

### Testing

- [OK] git diff --check；make test（Go 全部包 598 PASS + 前端 14 文件/162 用例）；make build；tsc --noEmit
- [OK] 独立检查首轮 NEEDS FIX（P1×2/P2/P3）修复后定向复核 PASS
- [OK] W2/W3 LIVECHECK_PASS：DOGE 17 张开仓→分批→改止损/止盈→全平；ETH 0.1 张→改止损→全平；终态零残留，余额差手续费量级

### Status

[OK] **Completed**

### Next Steps

- ⑫ market-source（依赖 ① 已合父），其后 13→17 串行；父任务收口前组合审查、E2E 与 make ci-local


## Session 10: 子任务 12 market-source：Gate 合约 K 线与 ticker 取数
<!-- trellis-session: v=2 fp=d90e3c2652b44869 -->

**Date**: 2026-11-25
**Task**: 子任务 12 market-source：Gate 合约 K 线与 ticker 取数
**Branch**: `feat/2026_1123_gate-trading-foundation`

### Summary

纯新增 Gate 合约 K 线与 ticker 取数（保留周期含 1w/30d、成交量取 sum、严格响应校验），独立检查 PASS，7 标的 × 12 周期真实探针通过，已合父归档

### Main Changes

- market/gate_market.go：GateIntervalDuration 保留集合 {10s..30d}；GetGateKlines 用 sum 成交额、毫秒时间、CloseTime 取下一根开盘、严格校验 OHLC/时间/数值；GateTicker/GetGateTicker/ListGateTickers；只走 Gate 无回退
- 实测 30d 是自然月（limit 覆盖全历史时月初；小 limit 为月内窗口），CloseTime 最后一根 30 天近似；1w 为周一
- 单测矩阵：字符串/数字混排、失败注入、非法参数零请求、ticker 请求形状、OHLC 四方向、limit 上界
- 计划 §3.3 留档 30d 事实；依赖列更正 13-15 → 12

### Git Commits

| Hash | Message |
|------|---------|
| `7c4e0267` | feat(market): Gate 合约 K 线与 ticker 取数（子任务 12） |
| `3be668b6` | docs+test(market): 检查修复——30d 月初/收线近似口径限定、ticker 请求形状与失败矩阵补强（子任务 12） |
| `0c1323e1` | merge: 行情数据源并入父分支（子任务 12：7c4e0267 实施 + 3be668b6 检查修复；独立检查 PASS，7 标的 × 12 周期 PROBE_PASS，make test/build 全过） |

### Testing

- [OK] git diff --check；make test（Go 全部包 608 用例 + 前端 162 用例）；make build；独立检查 PASS
- [OK] 真实探针 PROBE_PASS：7 标的 × 12 周期、1w 周一、30d 月历、ticker 漂移 ≤0.0207%、Volume=原始 sum、1042 合约列表、旧周期被拒

### Status

[OK] **Completed**

### Next Steps

- ⑬ engine-klines-gate：决策 K 线与网格箱体切 Gate，删除 CoinAnk K 线/旧内嵌路径（保持编译），其后 14→17


## Session 11: 子任务 13 engine-klines-gate：决策 K 线与网格箱体切 Gate
<!-- trellis-session: v=2 fp=a96b0d20ac38902f -->

**Date**: 2026-11-25
**Task**: 子任务 13 engine-klines-gate：决策 K 线与网格箱体切 Gate
**Branch**: `feat/2026_1123_gate-trading-foundation`

### Summary

引擎决策 K 线、网格箱体、旧内嵌路径全部切 Gate；删除 CoinAnk K 线与无调用方实时/深度代码；提示词周期与 USDT 成交量口径同步；独立检查 PASS，12 周期真实探针通过，已合父归档

### Main Changes

- market：GetWithTimeframes/GetBoxData/GetWithExchange 改 GetGateKlines；calculatePriceChangeByBars 用 Gate 周期时长；isStaleData 15 分钟窗口；删除 getKlinesFromCoinAnk/parseTimeframeToMinutes/timeframe.go/coinank kline_ws+depth_ws/kernel.FetchMarketData
- 提示词：周期顺序 {10s..30d}（F41）、K 线表与说明 Volume(USDT)（中英）、Format 5m/USDT；api test-run 兜底 3m→5m
- 保留 ⑭⑮ 边界：coinank_api/kline.go、api/handler_klines.go、agent/* 未动
- F46：主周期与 1h/4h 窗口不匹配时涨跌读数退回可用窗口，属已知近似

### Git Commits

| Hash | Message |
|------|---------|
| `13246f0f` | feat(engine): 决策 K 线与网格箱体切 Gate，删除 CoinAnk K 线与旧周期表（子任务 13） |
| `ad5fef58` | fix(market): 检查修复——Format 5m/USDT 口径、stale 15 分钟窗口、箱体请求与失败语义用例（子任务 13） |
| `bdc844ea` | chore(probe): 在修复候选上重跑子任务 13 真实探针（PROBE_PASS） |
| `89a3a97e` | fix(market): 检查 P3 收口——文档口径统一、Format/失败/涨跌窗口用例、F46 留档（子任务 13） |
| `0e498514` | merge: 引擎 K 线切 Gate 并入父分支（子任务 13：13246f0f 实施 + ad5fef58/bdc844ea/89a3a97e 修复；独立检查 PASS，12 周期 PROBE_PASS，make test/build 全过） |

### Testing

- [OK] git diff --check；make test（Go 620 用例 + 前端 162 用例）；make build；独立检查 PASS（首轮 P2+5P3 修复）
- [OK] 真实探针 PROBE_PASS：12 周期含 10s/7d/30d、箱体 1h×500、无 AI 提示词渲染 Volume(USDT)、3m 被拒

### Status

[OK] **Completed**

### Next Steps

- ⑭ chart-klines-gate：/api/klines crypto 改 Gate、删交易所映射与秒级周期、前端去 exchange；其后 ⑮→⑰
