# C1：后端删除 Hyperliquid 交易所实现

## Goal

删除 Hyperliquid 在 Go 后端的全部实现与引用：两个专属包、钱包与 xyz 路由、store 三列与迁移函数、策略 SourceType 的 hyper_* 枚举、agent/kernel/manager/market/auto_trader 分支、go-hyperliquid 依赖与全部相关测试；同时按 D7 以 Gate 重接 `/api/symbols`（该端点现状仅有 hyperliquid 实现，删除后即为空壳）。一次性删除旧语义，不做兼容分支。

## 依赖

无前置依赖（本父任务的第一个子任务）。注意与 C2 共享 `agent/tools.go`、`agent/skill_*_handlers.go`、`api/server.go`、`agent/skills/*.json`，必须在 C2 之前完成。

## Requirements

### 整包与专属文件删除

- `provider/hyperliquid/` 整包（coins.go、kline.go + kline_test.go、perp_dex_cache_test.go）
- `trader/hyperliquid/` 整包（client_init、order_sync、trader、trader_account、trader_orders、trader_positions、trader_sync 共 7 源文件 + builder_fee、client_init、symbol_conversion、sync、trader_race 共 5 测试文件）
- `api/handler_hyperliquid_wallet.go` 与 server.go 中 4 条 `/hyperliquid/*` 路由（connect-config、account、agent、submit-exchange）；`/wallet/validate`、`/wallet/generate` 属 C2，本任务不动
- `cmd/e2e_builder_fee/`（builder fee 为 Hyperliquid 专属概念）
- `market/data_hyperliquid_xyz_test.go`、`store/strategy_hyperliquid_defaults_test.go`

### store 层

- `store/exchange.go`：删三列 struct 字段与 gorm tag（:32-34）、种子列表 hyperliquid 项、`ensureHyperliquidBuilderApprovedColumn` 迁移函数及调用（:93-97、:60、:77）、`CreateLegacy` 整个 deprecated 方法（:371-400，含旧 ID 列表）、`cleanupIncompleteExchangeConfigs` 的钱包传参（:100-133）、`migrateToMultiAccount` 旧 ID 列表（:140,154）、`ExchangeStore.Create`/`.Update`（:241,292）的钱包字段映射
- `store/visibility.go`：`MissingRequiredExchangeCredentialFields`（:5，定义处）删三个 hyperliquid 参数，全部调用点同步收敛
- `store/strategy.go`：CoinSource 的 6 个 hyper_* 字段（:774-785）、normalize 三个 case（:178-223）、`normalizeCoinSourceType` 映射（:260-265）、`inferCoinSourceType`（:283-290）——**default 分支重定向 `static`**；`GetDefaultStrategyConfig`（:909）默认 SourceType 改 `static`、Hyperliquid 专属中英 RoleDefinition（:986-1008）换通用 crypto 文案；`HyperMainLimit` 等残余字段引用（:1462-1467）删
- `store/visibility.go` 其余 hyperliquid 引用同步清理

### api 层

- `api/handler_klines.go`：删两处 `"hyperliquid" / "hyperliquid-xyz" / "xyz"` case（:64、:473）与 xyz 板拼装（GetPerpDexCoins）；`handleSymbols` 新增 `case "gate"`——新建 `provider/gate/symbols.go`（gateapi-go 公开客户端，无需 API key）拉 `GET /futures/usdt/contracts` + `GET /futures/usdt/tickers` 映射 SymbolInfo（contract→symbol、last/mark_price→MarkPrice、volume_24h_settle→Volume24h、24h 涨跌幅→Change24hPct、leverage_max→MaxLeverage、order_size_min→SzDecimals；过滤 in_delisting；字段名以 SDK FuturesContract/FuturesTicker 为准），`hyperliquidCategoryOrder` 退化为按 Volume24h 降序；`DefaultQuery` 默认值 :453（symbols，现 hyperliquid）与 :33（klines，现 binance）统一改 `gate`
- `api/handler_user.go`：`createDefaultStrategies`（:223-299）的 3 个美股 USDC 预置策略及后续 definitions 整体删除（新用户初始无预置策略）
- `api/handler_exchange.go`、`handler_trader.go`、`handler_trader_status.go`、`exchange_account_state.go`、`utils.go` 的 hyperliquid 分支与枚举；`api/server.go` 路由文档注释（含 :294 凭证注释）交易所枚举改 9 所
- `api/traderid_test.go`、`utils_test.go`、`handler_trader_symbol_test.go`、`handler_user_default_strategy_test.go` 中引用修复

### agent 与 kernel

- `agent/tools.go`（28 处：工具 schema、枚举、description）、`onboard.go`（别名表 `"hyperliquid": "hyperliquid", "7": "hyperliquid"` :242、`state.Exchange == "hyperliquid"` 分支 :442、saveSetupExchange 钱包参数 :436-475）、`trade.go`、`planner_runtime.go`、`config_validation.go`、`entity_field_catalog.go`、`llm_flow_extractor.go`、`skill_execution_handlers.go`、`skill_management_handlers.go`、`skill_semantic_gate.go`
- `agent/skills/exchange_management.json`、`exchange_diagnosis.json`、`trader_management.json`
- `kernel/engine.go`（HyperMain 标签逻辑 + hyper_* SourceType 引擎分支：getHyperAll/getHyperMain/getHyperRankCoins、usesHyperliquidNativeUniverse 的 4 个 NofxOS 跳过点 :228,567,588,637）、`engine_prompt.go`（HyperMain 5 处）、`engine_position.go`（:39 注释）

### manager / market / trader 运行时

- `manager/trader_manager.go` 四处：工厂注册 case（:717）、`ensureHyperliquidNativeStrategy` 整函数（:414）及调用（:659）、builder approved 启动拦截（:664-665）、TraderConfig 字段填充（:677-678、:718-720）
- `market/data.go`：`useHyperliquidAPI`（:44-71、:176-188）、xyzDexAssets 资产表（:500-539）、`IsXyzDexAsset`（:542）、Normalize 的 USDC/xyz 分支（:561-570）；`market/data_klines.go`：`getKlinesFromHyperliquid`（:115）及调用（:412）、coinank 映射（:67-68）
- `trader/auto_trader.go`（TraderConfig 四字段 :95-98、工厂调用 :284）、`auto_trader_decision.go`、`auto_trader_loop.go`（xyz 符号规范化段 :306-393）、`auto_trader_risk.go`、`trader/types/interface.go`（:42 注释）
- `provider/coinank/coinank_enum/exchange.go` 删枚举项（string 常量无 iota，无序列化位移风险）

### 依赖

- `go mod tidy` 移除 `github.com/sonirico/go-hyperliquid`（go.mod:21）

## 约束

- 删除即终态：不留 deprecated 标记、不留空壳函数、不留"预留将来恢复"的注释
- 仅删 hyperliquid 语义；aster/lighter 的 DEX 分支与其各自凭证字段不动
- 本任务不触碰 claw402 / blockrun / 新手模式（属 C2）

## Acceptance Criteria

- [ ] `go build ./...` 成功；`go test ./...` 全绿
- [ ] `rg -i "hyperliquid|hyper_all|hyper_main|hyper_rank|UseHyperMain|HyperMainLimit|HyperRank" --glob '*.go'` 归零（go.sum 由 tidy 消除）
- [ ] `rg -n "hyperliquid" go.mod go.sum` 归零
- [ ] `api/server.go` 无 `/hyperliquid/` 路由，文档注释枚举为 9 所
- [ ] `store` 的 exchanges 模型无 hyperliquid 三列字段（DB 层列清除由 C5 drop 表重建达成）
- [ ] SourceType normalize/infer 的 default 重定向 static 有测试覆盖；`GetDefaultStrategyConfig` 默认 static
- [ ] `/api/symbols?exchange=gate` 返回非空 Gate USDT 永续清单（公开接口，无 key 可用）；`/api/symbols` 与 `/api/klines` 无参默认均为 gate
