# C2：后端删除 claw402 钱包体系与新手模式

## Goal

删除 claw402 钱包相关体系的全部后端逻辑：claw402 / blockrun-base / blockrun-sol 钱包付费模型 provider、x402 支付轨道、EVM 钱包工具接口、新手 onboarding API 与模式分支。模型层收敛为纯 API Key provider，AI500 收敛为 nofxos 直连。

## 依赖

**必须在 C1（`11-18-backend-remove-hyperliquid`）完成后执行**：两者共同编辑 `agent/tools.go`、`agent/skill_*_handlers.go`、`api/server.go`、`agent/skills/*.json`、`trader/auto_trader*.go`。

## Requirements

### 模型目录与调用轨道

- `agent/model_provider_catalog.go`：删 claw402（:32）、blockrun-base（:44）、blockrun-sol（:54）三个 provider 条目及全部钱包引导文案（中英两套，含"扫码充值/支付"、"一键创建钱包"话术）；`UsesWalletCredential` / `Recommended` / `RecommendedModelHints` 字段随之无消费方，删除（不留死字段）
- 删除 `agent/model_wallet_fastpath.go` 及其挂载点 `agent/agent.go:443,493`（handleModelWalletBalanceQuestion）
- `agent/agent.go`：删 `agentProviderSupportsUSDCBalance` / `agentModelUSDCBalance` / `rankAgentModelCandidates` 的 USDC 余额优先排序（:27,58-59,281,309）、`resolveModelRuntimeConfig` 的 claw402 默认项（:345）、`nofx/wallet` import；`agent.go:172` 的 `mcp.NewAIClientByProvider` 通用调用保留
- `agent/central_brain.go`、`config_validation.go`（:26）、`llm_flow_extractor.go`、`skill_domain_context.go`、`skill_management_handlers.go`（:2368）、`tools.go`、`skills/*.json`（model_management、model_diagnosis、trader_management、trader_diagnosis）：provider 枚举与钱包引用清理；`tools.go:993-994` 的 WalletAddress/BalanceUSDC DTO 字段删
- `agent/skill_dispatcher.go:281,290`：`agentWalletAddressFromPrivateKey` 调用与"钱包余额 0 USDC，需充值"文案删
- `agent/onboard.go`：模型引导删钱包 provider 选项（setup 流本身保留；实证无 beginner 分支，除钱包引导外零改动）

### api 路由与 handler

- 删除 `api/handler_onboarding.go` 与 server.go 的 `/onboarding/beginner`、`/onboarding/beginner/current` 两条路由（:203-204，无其他 onboarding 路由）；persistBeginnerWalletEnv（:241-270）随之消失
- 删除 `api/handler_wallet.go` 与 `/wallet/validate`、`/wallet/generate` 路由（server.go:149-150；含 checkClaw402Health）
- `api/handler_ai_model.go`：删钱包 CRUD/余额刷新分支（:37-38,102-111,250-252）与 **`handleGetSupportedModels` 硬编码列表中的 claw402/blockrun 三条**（:241-253）——该列表是 provider 枚举的第二数据源，漏删则 /supported-models 仍暴露已删 provider。与 handler_onboarding.go **同批次编辑**（`walletAddressFromPrivateKey` 定义在 onboarding :224、调用在 ai_model :104）
- `api/strategy.go`：删 `resolveStrategyDataWalletKey`（:572,763）与 :11 的 `_ "nofx/mcp/payment"` blank import
- `api/handler_ai500.go`：删钱包透传（:29）与 server.go:217 "routed through claw402" 注释
- `kernel/engine.go`：`NewStrategyEngine` variadic 参数删除（:192）与 `CLAW402_WALLET_KEY/URL` env 读取（:206-209）；带参调用仅两处：`api/strategy.go:582`、`trader/auto_trader.go:368`，同步收敛

### trader/auto_trader.go（专属清单，先于 wallet 整删完成）

- :9 的 `_ "nofx/mcp/payment"` blank import、:22 的 `nofx/wallet` import
- `Claw402WalletKey` 配置字段（:121,176）、`claw402WalletAddr` 字段（:22）、key→NewStrategyEngine 传递（:362-368）
- `runPreLaunchChecks` 的钱包余额预检（:648-690）与 `deriveWalletAddress`（:692+）
- `IsClaw402Config` / `EstimateRunway` 调用点（:650,675）
- go-ethereum import 若仅服务上述逻辑则同步删（aster/lighter 的保留）

### wallet / mcp / nofxos 包收敛

- `wallet/` 整包删除（usdc.go + balance_cache.go；7 个消费方中 6 个已在前序阶段消失，auto_trader.go 在上一节完成后整删条件成立）
- `mcp/payment/` 子包删除（claw402.go、x402.go）；**`mcp/` 其余部分整包保留**（通用 LLM client 库，38 个消费文件，承载全部 API Key provider）——仅再删 `mcp/providers.go:16` 的 `ProviderClaw402` 常量、`mcp/client.go:49-66` `TokenUsage.Channel()` 的 claw402 分支
- `main.go:12`、`telegram/bot.go:8` 的 `_ "nofx/mcp/payment"` blank import 删
- `provider/nofxos/`：删 `claw402.go` 整文件；`client_resolve.go`（:14-32）收敛为恒直连；`client.go` 删 `SetClaw402`（:28,63-68）与 doRequest 的 claw402 分支（:97-109）；`ai500_cache.go:10` 注释更新
- 注意 `provider/nofxos/claw402.go` 对 `mcp.Logger` 的 import 随文件删除消失

### store 与外围

- `store/ai_model.go`：删 `ResolveClaw402WalletKey`（:299）、`FindOrphanClaw402`（:58）、`AdoptModel`（:70）、"recover wallets after account reset" 机制与注释（AIModel 结构本就无钱包列，无 schema 改动）
- `store/ai_charge.go`：**表与通用统计保留**；删 `IsClaw402Config`（:152-155）、`EstimateRunway`（:157-169）
- `store/strategy.go`：删 `GetContextLimitForClient` 的 claw402 模型名前缀分支（:1249-1275，调用方 kernel/engine_analysis.go:67 同步）
- `trader/auto_trader_loop.go`：删钱包数据支付写入（:34,745），通用 AICharge 成本记录保留
- `telemetry/experience.go:45` Channel 字段注释、`config/config.go:161-167` `usage.Channel()` 上报链随 mcp 分支删除收敛
- `telegram/bot.go`：isUSDCProvider 与 USDC payment 日志（:278-295,318-320）
- `manager/trader_manager.go`：`Claw402WalletKey` 注入（:747）、`resolveTraderDataWalletKey`（:787-808）
- `docker-compose.yml`：删 :14-20 的 `.env` bind mount 注释块与 persistBeginnerWalletEnv 说明（`env_file: .env` :23-24 保留）；`.nofx-local/compose.yml` 已核实无引用
- 修复全部相关测试（`agent/agent_model_selection_test.go`、`model_create_flow_test.go`、`model_provider_catalog_test.go`、`clear_memory_test.go`、`trader_scope_test.go`——已核实为全部命中集，mcp/api/store/kernel 包测试无命中）

## 保留边界（不得误删）

- AI500 功能与 `provider/nofxos` 直连客户端（DefaultAuthKey）
- `mcp/` 通用 LLM client 库（client.go、providers.go 其余部分、provider/ 注册表、intro/）——全部 API Key provider 依赖它
- `store/ai_charge.go` 的 ai_charges 表与通用成本统计 + `api/handler_ai_cost.go`
- 聊天 setup 流（`agent/onboard.go` handleSetupFlow）
- DeepSeek / OpenAI / Claude / Gemini / Qwen / Kimi / Grok / MiniMax 等 API Key provider 全链路
- `trader/aster`、`trader/lighter` 对 go-ethereum 的使用（其 API Key 模式自身形态）

## Acceptance Criteria

- [ ] `go build ./...`、`go test ./...` 全绿
- [ ] `rg -i "claw402|blockrun" --glob '*.go'` 归零
- [ ] `rg -n "ProviderClaw402|mcp/payment" --glob '*.go'` 归零
- [ ] `rg -n "onboarding/beginner|/wallet/validate|/wallet/generate" api/server.go` 归零
- [ ] 模型 provider 目录与 /supported-models 均仅含 API Key provider；agent 引导无钱包/扫码/充值话术
- [ ] `kernel.NewStrategyEngine` 单参数；`wallet/` 目录不存在；`mcp/payment/` 目录不存在且 mcp 其余部分完整可用
