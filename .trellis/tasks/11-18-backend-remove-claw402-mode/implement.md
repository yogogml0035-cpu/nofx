# C2 执行计划：后端删除 claw402 钱包体系与新手模式

前置：C1 已完成并 commit。每阶段保持可编译；阶段即回滚检查点。

## 阶段 1：模型 provider 目录与调用轨道

- [ ] `agent/model_provider_catalog.go`：删 claw402 / blockrun-base / blockrun-sol 条目与全部钱包引导文案（中英两套）；删 UsesWalletCredential / Recommended / RecommendedModelHints 死字段
- [ ] 删除 `agent/model_wallet_fastpath.go` 及 agent.go :443,493 挂载点
- [ ] `agent/agent.go`：删 USDC 余额排序三函数、claw402 默认项、`nofx/wallet` import（:172 的 mcp.NewAIClientByProvider 保留）
- [ ] `agent/central_brain.go` provider 枚举提示词更新（仅 API Key）
- [ ] `agent/skill_dispatcher.go:281,290` 钱包调用与充值文案删
- [ ] 验证：`go build ./...`

## 阶段 2：agent 其余文件与 skills

- [ ] `agent/ai500.go`：删 Claw402WalletKeyForStoreUser（:28-48）与消费点（:106），保留直连
- [ ] `agent/config_validation.go`、`llm_flow_extractor.go`、`skill_domain_context.go`、`skill_management_handlers.go`、`tools.go`（含 :993-994 DTO 字段）清理
- [ ] `agent/skills/*.json`（model_management、model_diagnosis、trader_management、trader_diagnosis）provider 枚举清理
- [ ] `agent/onboard.go`：模型引导删钱包 provider 选项（setup 流保留）
- [ ] 验证：`go build ./...`

## 阶段 3：api 路由与 handler

- [ ] 删除 `api/handler_onboarding.go`；server.go 删 `/onboarding/beginner*` 路由（:203-204）
- [ ] 删除 `api/handler_wallet.go`；server.go 删 `/wallet/validate`、`/wallet/generate`（:149-150）
- [ ] `api/handler_ai_model.go`：钱包 CRUD/余额刷新分支（:37-38,102-111,250-252）与 handleGetSupportedModels 硬编码列表三条（:241-253）——**与 handler_onboarding.go 同批次编辑**（walletAddressFromPrivateKey 定义/使用链）
- [ ] `api/handler_ai500.go` 钱包透传删；server.go:217 注释清理
- [ ] `api/strategy.go`：删 resolveStrategyDataWalletKey（:572,763）与 :11 blank import；:582 的 NewStrategyEngine 调用收敛
- [ ] `kernel/engine.go`：NewStrategyEngine variadic 参数删除（:192）、CLAW402_WALLET_KEY/URL env 读取删（:206-209）
- [ ] 验证：`go build ./...`

## 阶段 4：trader/auto_trader.go 专属收敛

- [ ] 删 :9 `_ "nofx/mcp/payment"` blank import、:22 `nofx/wallet` import
- [ ] 删 Claw402WalletKey 配置字段（:121,176）、claw402WalletAddr 字段（:22）、NewStrategyEngine 传参（:362-368）
- [ ] 删 runPreLaunchChecks 钱包余额预检（:648-690）、deriveWalletAddress（:692+）、IsClaw402Config/EstimateRunway 调用（:650,675）
- [ ] go-ethereum import 若无存活使用则删
- [ ] 验证：`go build ./...`

## 阶段 5：wallet / mcp / nofxos 包收敛

- [ ] `wallet/` 整包删除（此时 7 个消费方已全部在前序阶段消失）
- [ ] `mcp/payment/` 子包删除；`main.go:12`、`telegram/bot.go:8` blank import 删
- [ ] `mcp/providers.go:16` ProviderClaw402 常量删；`mcp/client.go:49-66` Channel() claw402 分支删（mcp 其余部分保留）
- [ ] `provider/nofxos/`：删 claw402.go；client_resolve.go 恒直连；client.go 删 SetClaw402 与 doRequest claw402 分支；ai500_cache.go 注释更新
- [ ] 验证：`go build ./...`；`go vet ./...`

## 阶段 6：store 与外围清理

- [ ] `store/ai_model.go`：删 ResolveClaw402WalletKey、FindOrphanClaw402、AdoptModel、recover wallets 机制
- [ ] `store/ai_charge.go`：删 IsClaw402Config、EstimateRunway（表与通用统计保留）
- [ ] `store/strategy.go`：删 GetContextLimitForClient claw402 分支（:1249-1275，kernel/engine_analysis.go:67 同步）
- [ ] `trader/auto_trader_loop.go`：删钱包数据支付写入（:34,745）
- [ ] `telemetry/experience.go:45`、`config/config.go:161-167` Channel 链清理
- [ ] `telegram/bot.go`（:278-295,318-320）、`manager/trader_manager.go`（:747,787-808）清理
- [ ] `docker-compose.yml` :14-20 注释块删除（env_file 保留）
- [ ] 验证：`go build ./...`

## 阶段 7：测试修复与终验

- [ ] 修复 `agent/agent_model_selection_test.go`、`model_create_flow_test.go`、`model_provider_catalog_test.go`、`clear_memory_test.go`、`trader_scope_test.go`（已核实为全部命中集）
- [ ] 终验命令：
  ```bash
  go build ./... && go test ./...
  rg -i "claw402|blockrun" --glob '*.go'          # 归零
  rg -n "ProviderClaw402|mcp/payment" --glob '*.go' # 归零
  rg -n "wallet" api/server.go                     # 无 /wallet 路由
  ls wallet mcp/payment 2>/dev/null                # 输出为空即已删
  ```
- [ ] 更新 spec 并 commit 本子任务
- [ ] 备注：宿主机 `.env` 的 CLAW402_* 残留变量清理属 C5 运维步骤（client_resolve 恒直连后已惰性）

## 回滚点

每阶段存档（全量内容，禁用 --stat 作为唯一存档）：

```bash
git diff > scratch/c2-stage<N>.full.patch
git status --porcelain > scratch/c2-stage<N>.files.txt
```

恢复仅用文件编辑工具，禁止 git 回滚命令。
