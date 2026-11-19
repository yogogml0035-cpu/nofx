# C5：父任务集成验收与端到端探针

## Goal

C1–C4 全部合入父分支后，以父 PRD「跨子任务验收标准」为唯一清单收口。此阶段子分支与子 worktree 已按流程删除，**发现缺陷直接在父分支修复**（commit 标注 `fix(integration): 回流 <子任务 slug> 的 <问题>`），重跑受影响子任务门禁与父验收清单；属需求变更时先回父 PRD 修订再实施。

## Requirements

1. 全量门禁在父 worktree 顺序执行并记录输出：`go build ./...`、`go test ./...`（Docker golang:1.25-alpine + GOPROXY=goproxy.cn）、`npm --prefix web run build`、`npm --prefix web run test`
2. 父 PRD **三条 grep 词表**（词表一 vergex/策略市场、词表二 IsPublic/is_public、词表三 nofxos 数据链）复跑归零；排除集与 `--hidden` 纳入 `.github/` 均按父 PRD 执行
3. 运行时探针（临时后端 + 前端，或本地 compose；断链统一用 `GATEMCP_ENDPOINT=http://127.0.0.1:9`）：
   - `/api/symbols` 无参默认 gate 且含 funding_rate（抽查 BTC_USDT 等 5 个合约非空，小数语义）
   - 六个 `/api/market/*` 真实数据（news/announcements 允许真实空列表视为合格态；fear-greed 的 change_24h 冷启动可为 null）；断网点 502 fast-fail 复测
   - 浏览器截图（含**未登录会话**直接访问 `/data`）：导航（无策略市场）、`/strategy-market` 直访 404、Landing Initialize（已登录）落 `/traders`、`/data` 七模块（桌面+移动）、策略编辑市场环境勾选组、策略编辑器币源仅静态 Gate 清单、Agent 页无 AI500 面板、排行榜回归（公开榜渲染、参赛开关、equity 曲线）
   - 决策日志：勾选策略含注入段、未勾选不含
   - `GET /api/ai500` 404；`/api/klines` 正常（Coinank 保留实证）
4. 探针产物统一留档父任务目录 `probe/`（不放仓库根 screenshots/——该目录不在词表排除集）
5. 主工作区收尾：`provider/vergex/` 空目录 rmdir（untracked，主工作区操作在此阶段做）
6. 验收结论对照父 PRD 清单逐条打勾，全部通过后父任务进入交付流程（最终候选 commit 后推送 `feat/2026_1119_gate-market-data` 同名远端 → 用户确认后集成 dev-offline，按 AGENTS.md 交付节执行）

## Acceptance Criteria

- [ ] 父 PRD「跨子任务验收标准」全部条目打勾（逐条引用父 PRD 条目，不二次转写）
- [ ] 探针产物齐备（截图 + curl 输出 + 决策日志摘录）
- [ ] 最终候选 commit 后推送 `feat/2026_1119_gate-market-data` 同名远端

## Notes

- 本任务无独立 design/implement（PRD-only）：执行清单即父 PRD 验收标准 + 交付流程
- Docker 终态镜像重建不在本任务（上次任务的 c4-final 流程可复用，是否重建镜像由用户在 dev-offline 集成阶段决定）
