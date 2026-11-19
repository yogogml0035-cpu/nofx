# 父任务执行计划

父任务自身无代码实施；本文件是子任务执行顺序与交付节奏的单一权威记录（子任务地图见 prd.md，两者顺序一致）。

## 顺序与门禁

| 步 | 子任务 | 开工前置 | 完成门禁（全过后合父） |
|---|--------|----------|--------------------------|
| 1 | C1 backend-gate-market-data | 父分支最新提交 | Docker build/test + 六端点 curl 探针 + GATEMCP_ENDPOINT 断链 502 + funding_rate 抽查 |
| 2 | C2 purge-strategy-market-vergex | C1 合父 | 全门禁 + 词表一/二归零 + 浏览器探针（导航/404/跳转/占位页） |
| 3 | C2b purge-nofxos-data-chain | C2 合父 | 全门禁 + 词表三归零 + 排行榜/策略/klines 回归探针 |
| 4 | C3 frontend-data-dashboard | C1、C2 合父 | web build/test + 桌面/移动/断链分组截图 |
| 5 | C4 backend-market-context-injection | C1、C2、C2b 合父 | Docker build/test（含序列化往返）+ web build/test + 决策日志注入段实证 |
| 6 | C5 integration-e2e-probe | C1–C4 全部合父 | 父 PRD 跨子任务验收标准逐条打勾 |

## 交付节奏

- 每个子任务在专属 worktree（基于父分支最新提交）实施，通过自身完整门禁后合入父分支，删除子分支与子 worktree
- 父分支阶段性提交与最终候选推送同名远端 `feat/2026_1119_gate-market-data` 留档（规划完成即已推送；每合入一个子任务可推送一次阶段点）
- C5 阶段子分支已删：缺陷直接在父分支修复（commit 标注 `fix(integration): 回流 <slug> 的 <问题>`），重跑受影响门禁
- 最终验收通过后按 AGENTS.md 交付节：推送候选 → 用户确认 → 用户操作 dev-offline 集成

## 失败重试

- 重试单元 = 子任务 worktree + commit 边界（手动恢复文件，禁止 git 回滚）
- C1 若因 go-sdk/网络卡住，C2/C2b 无功能依赖可先行开工（调换顺序只需更新本表与父 PRD 地图）
- 探针留档只进各任务目录（.trellis/ 下，词表排除集内），不放仓库根 screenshots/
