# 父任务执行计划

父任务自身没有代码实施（组合审查与最终验收除外）。本文件记录子任务的执行顺序与交付节奏；
进度勾选以计划文档第 10.5 节为准，每个子任务收口时两处同步。

## 顺序与门禁

子任务串行推进，每个子任务从父分支最新提交切出。闭环验证的完整表述见计划文档第 10.2 节。

| 步 | 子任务 slug | Trellis 目录 | 开工前置 | 完成门禁（全过后合父） |
|---|---|---|---|---|
| 1 | `gate-sdk-v7` | 无（Trellis 记录建立前已交付） | — | 全量 `go test`；只读真实读数 19 项 |
| 2 | `contract-math` | `archive/2026-11/11-24-contract-math`（已归档） | 1 合父 | 单测矩阵；7 个合约元数据真实读数 |
| 3 | `protective-order-adapter` | `archive/2026-11/11-24-protective-order-adapter`（已归档） | 1 合父 | 单测构造矩阵；真实挂、查、改、撤 |
| 4 | `open-position-safety` | `archive/2026-11/11-24-open-position-safety`（已归档） | 2、3 合父 | 单测；真实最小开仓全流程（W2 开仓与全平、W3） |
| 5 | `unified-trading-params` | `archive/2026-11/11-24-unified-trading-params`（已归档） | 1、4 合父 | 单测；闲置合约上真实设置并读回（W4） |
| 6 | `drawdown-monitor` | `archive/2026-11/11-25-drawdown-monitor`（已归档） | 2、5 合父 | 单测；带仓的只读真实读数 |
| 7 | `account-equity` | `archive/2026-11/11-24-account-equity`（已归档） | 1 合父，且 4 已合父 | 单测；带仓与不带仓的只读真实读数；提示词预览 |
| 8 | `strategy-config` | `archive/2026-11/11-25-strategy-config`（已归档） | 无 | 单测；前端测试与构建 |
| 9 | `sizing-engine` | `archive/2026-11/11-25-sizing-engine`（已归档） | 6、7 合父 | kernel 单测；提示词预览；`/strategies/test-run` |
| 10 | `partial-close` | `archive/2026-11/11-25-partial-close`（已归档） | 4、8 合父 | 契约解析与换算单测；真实分批平仓（W2） |
| 11 | `protective-order-management` | `archive/2026-11/11-25-protective-order-management`（已归档） | 3、4、8 合父 | 单测；真实改单（W2） |
| 12 | `market-source` | `archive/2026-11/11-25-market-source`（已归档） | 1 合父 | 单测；7 个标的 × 全部保留周期的真实取数 |
| 13 | `engine-klines-gate` | `archive/2026-11/11-25-engine-klines-gate`（已归档） | 12 合父 | 单测；策略试运行显示 Gate 数据 |
| 14 | `chart-klines-gate` | `archive/2026-11/11-25-chart-klines-gate`（已归档） | 12 合父 | 单测；前端测试与构建 |
| 15 | `assistant-market` | `archive/2026-11/11-25-assistant-market`（已归档） | 12 合父 | 单测；工具真实调用 |
| 16 | `scan-alignment` | 开工时创建 | 无 | 单测 |
| 17 | `infra-hygiene` | 开工时创建 | 无 | 配置解析检查；构建上下文不含本地文件 |

## 每个子任务的闭环

1. 从父分支最新提交建子分支 `feat/2026_1123_gate-trading-foundation--<slug>` 与专属 worktree，
   在子 worktree 创建并激活子任务（子分支只在本地使用）。
2. 实施由主会话完成；收敛后由独立检查子智能体检查，发现的问题由主会话修复并补验。
3. 门禁：`git diff --check`、`make test`、`make build`，以及该子任务规定的验证。
   迭代期只跑定向测试，不反复跑全量。
4. 在父 worktree 合入子提交，核对父分支包含子提交、子 worktree 干净、证据已保存；
   归档子任务，推送父分支留档，然后删除子分支与子 worktree。
5. 同步本文件、计划文档第 10.5 节与父任务 `prd.md` 的子任务地图。

## 父任务收口

全部子任务合父后执行（计划文档第 10.4 节）：

- [ ] 多智能体对抗式审查：逐项对照计划文档，修正发现的问题
- [ ] 最终完整本地门禁 `make ci-local`，串联全部子任务的端到端验收与不使用 AI 的冒烟
- [ ] 核对最新 `main`，集成到 dev-offline 并重建送测栈；交接为 awaiting_test
- [ ] 用户在容器内启动智能体做真实下单实测（D27）；测试通过后由用户选定合入 main
- [ ] 合入 main、重建主栈并核对没有 trader 被自动拉起

## 验证命令

宿主机没有 Go，go 命令统一经 Docker（首次运行需拉取镜像）：

```bash
# 定向测试（迭代期）
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev && CGO_ENABLED=1 go test -count=1 ./trader/gate/"

# 子任务门禁
make test     # go test -v ./... 与前端测试
make build    # go build -o nofx

# 父任务最终门禁
make ci-local
```

## 回滚点

- 回滚单元是一个子任务的提交边界：手动恢复文件，不使用 git 回滚命令。
- 子任务失败不外溢：未通过门禁的子分支不合入父分支，父分支始终保持上一个已验证的状态。
- 子任务 15 已完成、合父并归档（2026-11-25，独立检查首轮 NEEDS FIX 修复后 PASS，合父 `f9644929`，详见计划文档 10.5）。下一步 ⑯ `scan-alignment`：无依赖；调度改为墙钟网格，收线后固定延迟触发（1w 周一、30d 月初、其余按 Unix 网格）。然后 ⑰ 串行。
