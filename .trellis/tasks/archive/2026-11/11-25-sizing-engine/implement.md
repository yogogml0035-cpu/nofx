# 执行计划：⑨ sizing-engine

迭代期只跑定向测试；收敛后跑一次完整门禁。实施由主会话完成，独立检查子智能体检查后由主会话修复并补验。

## 清单

- [x] 1. kernel 契约：`AccountInfo.Principal`；`Decision` 删 `PositionSizeUSD`/`RiskUSD`、加 `NotionalUSD json:"-"`；`DecisionSizing`；`parseFullDecisionResponse`/`validateDecision` 签名改造；`buildTradingContext` 填充本金
- [x] 2. kernel 金额与校验：`openNotional`（F33 算法）、导出 `MaxLossPercentOfPrincipal`/`LossAtStop`/`ValidateStopLossLoss`；删除 12/60 与盈亏比校验；新增方向与 2% 亏损上限校验；`Prices` 从 `MarketDataMap` 构建
- [x] 3. kernel 提示词：`BuildSystemPrompt(principal)`；硬约束本金口径 + 2% 上限；输出示例/字段说明去金额；仓位指引重写；中英同步
- [x] 4. kernel 删除：`prompt_builder.go`、`prompt_builder_test.go`、`formatter.go`（无生产调用方）
- [x] 5. trader：`prepareOpen` 用 `NotionalUSD`；日志与注释同步；trader 单测字段替换
- [x] 6. api/web：preview-prompt `account_principal` + 前端字段；test-run 模拟账户本金；`server.go` schema 文案
- [x] 7. 助手：两个 diagnosis JSON；`skill_execution_handlers.go`（预期金额本金口径、证据解析、planner 指令）；`trader_scope_test.go` 样例与断言
- [x] 8. 文档：`STRATEGY_MODULE(.zh-CN).md`、`prompt-guide(.zh-CN).md`、`TROUBLESHOOTING(.zh-CN).md`、`store/strategy.go` 注释
- [x] 9. 定向测试：kernel 新增 `sizing_test.go`（金额/方向/上限/旧校验不拒绝）、提示词契约、解析忽略旧字段；api handler 测试；`go test ./kernel/ ./api/ ./trader/ ./agent/`
- [x] 10. 提示词预览证据：`probe/prompt_preview.go`（构建标签隔离）渲染中英提示词到 `probe/` 输出；`/strategies/preview-prompt`、`/strategies/test-run` handler 测试通过
- [x] 11. 完整门禁：`git diff --check`、`make test`、`make build`
- [x] 12. 独立检查子智能体；主会话修复 P0–P3 并定向补验
- [x] 13. 合父（`--no-ff` 合并提交）、父 worktree 归档、计划文档 10.5 勾选、父任务 prd/implement 地图同步、journal、push 父分支
- [x] 14. 确认父分支包含子提交、子 worktree 干净后删子分支与 worktree

## 独立检查修复（2026-11-25）

- [x] P1 符号变体回归：`currentPrice` 按执行层 `filterDecisionsToStrategyUniverse` 同口径解析（精确 → `market.Normalize` → 去 `/` → base 匹配）；新增 `SOL`/`sol-usdt`/`SOL/USDT`/小写单测
- [x] P2 架构文档：`BuildSystemPrompt(principal)`、`parseFullDecisionResponse(response, DecisionSizing)`、`validateDecisions(decisions, sizing)` 与检查清单（本金×百分比/方向/2% 上限）中英同步
- [x] P3-1 指南：去掉「系统强制盈亏比」「扩大仓位」措辞，改为提示词要求与开仓金额百分比
- [x] P3-2 预览/试运行先 `ClampLimits`（越界 25% 渲染为 20%）+ handler 测试
- [x] P3-3 探针 README：注明数据字典 map 顺序不定，复验比对关键行
- [x] P3-4 `agent/skills/trade_execution.json`：12 USDT 与权益倍数旧口径改为策略最小开仓金额（默认 10）与安全上限表述
- [x] P3-5 单标的示例：字段说明注明 stop_loss/take_profit 为占位，必须替换真实价格
- [x] P3-6 补单测：本金 110 × 10% = 11 USDT 的山寨与 BTC 开仓通过（锁定旧 12/60 门槛已删）
- [x] 自检项：止损止盈改为有限正数校验（NaN/Inf 拒绝）

## 验证命令

```bash
# 定向（宿主机无 Go，走 Docker；带 nofx-gocache 与 GOPROXY）
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  -e GOFLAGS=-buildvcs=false golang:1.25-alpine sh -c \
  'apk add --no-cache gcc musl-dev >/dev/null 2>&1; CGO_ENABLED=1 go test -count=1 ./kernel/ ./api/ ./trader/ ./agent/'

# 前端（若改 StrategyStudioPage）
cd web && npx tsc --noEmit && npx vitest run

# 完整门禁
make test && make build
```

## 回滚点

按清单步为单元手动回滚（不使用 git 回滚命令）；子分支未过门禁不合父。