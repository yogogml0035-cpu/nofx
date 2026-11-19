# C5：父任务集成验收与端到端探针

## Goal

C1–C4 全部合入父分支后，以父 PRD「跨子任务验收标准」为唯一清单收口；不改产品代码（发现缺陷回对应子任务修复后重新集成）。

## Requirements

1. 全量门禁在父 worktree 顺序执行并记录输出：`go build ./...`、`go test ./...`、`npm --prefix web run build`、`npm --prefix web run test`
2. 父 PRD 两条 grep 词表复跑归零
3. 运行时探针（临时后端 + 前端，或本地 compose）：
   - `/api/symbols` 无参默认 gate 且含 funding_rate
   - 五个 `/api/market/*` 真实数据；断网点 fast-fail 复测
   - 浏览器截图：导航（无策略市场）、`/data` 七模块（桌面+移动）、策略编辑市场环境勾选组、排行榜回归
   - 决策日志：勾选策略含注入段、未勾选不含
4. 探针产物统一留档父任务目录 `probe/`
5. 验收结论对照父 PRD 清单逐项打勾，全部通过后父任务进入交付流程（推送同名远端 feature → 用户确认后集成 dev-offline，按 AGENTS.md 交付节执行）

## Acceptance Criteria

- [ ] 父 PRD「跨子任务验收标准」全部条目打勾
- [ ] 探针产物齐备（截图 + curl 输出 + 决策日志摘录）
- [ ] 最终候选 commit 后推送 `feat/2026_1119_gate-market-data` 同名远端

## Notes

- 本任务无独立 design/implement（PRD-only）：执行清单即父 PRD 验收标准 + 交付流程
- Docker 终态镜像重建不在本任务（上次任务的 c4-final 流程可复用，是否重建镜像由用户在 dev-offline 集成阶段决定）
