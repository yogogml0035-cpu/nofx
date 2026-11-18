# C1 执行计划：后端删除 Hyperliquid

按序执行，每个阶段结束保持 `go build ./...` 可编译；阶段编号即回滚检查点（失败时以全量 patch 为参照，用文件编辑工具恢复该阶段改动文件）。

## 阶段 1：删除整包与专属文件

- [ ] 删除 `provider/hyperliquid/`（4 文件：coins.go、kline.go、kline_test.go、perp_dex_cache_test.go）
- [ ] 删除 `trader/hyperliquid/`（12 文件：7 源 + 5 测试）
- [ ] 删除 `api/handler_hyperliquid_wallet.go`、`cmd/e2e_builder_fee/`
- [ ] 删除 `market/data_hyperliquid_xyz_test.go`、`store/strategy_hyperliquid_defaults_test.go`
- [ ] 验证：`go build ./...`（此时引用方会报错，属预期，立即进入阶段 2）

## 阶段 2：store 层收敛

- [ ] `store/exchange.go`：删三列字段与 gorm tag、种子列表项、`ensureHyperliquidBuilderApprovedColumn` 及两处调用、`CreateLegacy` 整方法、`cleanupIncompleteExchangeConfigs` 钱包传参、`migrateToMultiAccount` 旧 ID 列表、`ExchangeStore.Create`/`.Update` 钱包映射
- [ ] `store/visibility.go`：`MissingRequiredExchangeCredentialFields` 三参收敛，同步调用点（`api/handler_exchange.go`、`api/utils.go`、`agent/skill_management_handlers.go`、`agent/onboard.go` 等）；`rg -n "hyperliquidWalletAddr|HyperliquidWalletAddr|hyperliquidUnified|hyperliquidBuilder|hyperliquid_builder"` 逐个清除
- [ ] `store/strategy.go`：CoinSource 6 字段、normalize case（default 重定向 static）、normalizeCoinSourceType、inferCoinSourceType（default 重定向 static）、`GetDefaultStrategyConfig` 默认 static + 通用 crypto RoleDefinition、HyperMainLimit 残余
- [ ] 验证：`go build ./...`

## 阶段 3：api 层路由与分支

- [ ] `api/server.go`：删 4 条 `/hyperliquid/*` 路由注册；文档注释 exchange 枚举改 9 所；:294 凭证注释清理
- [ ] `api/handler_klines.go`：删两处三别名 case 与 xyz 板拼装；新建 `provider/gate/symbols.go`（gateapi-go 公开客户端，contracts + tickers 映射 SymbolInfo，过滤 in_delisting，字段名以 SDK 为准）并在 handleSymbols 增加 `case "gate"`；`:453` symbols 默认值改 `gate`、`:33` klines 默认值 binance→`gate`；`hyperliquidCategoryOrder` 退化为 Volume24h 降序
- [ ] `api/handler_user.go`：删 createDefaultStrategies 的 3 个美股预置策略与 definitions
- [ ] `api/handler_exchange.go`、`handler_trader.go`、`handler_trader_status.go`、`exchange_account_state.go`、`utils.go` 清分支
- [ ] 验证：`go build ./...`

## 阶段 4：agent 与 kernel

- [ ] `agent/tools.go`（28 处）、`onboard.go`（别名表、state 分支、saveSetupExchange 钱包参数）、`trade.go`、`planner_runtime.go`、`config_validation.go`、`entity_field_catalog.go`、`llm_flow_extractor.go`、`skill_execution_handlers.go`、`skill_management_handlers.go`、`skill_semantic_gate.go`
- [ ] `agent/skills/exchange_management.json`、`exchange_diagnosis.json`、`trader_management.json`
- [ ] `kernel/engine.go`（HyperMain 标签 + hyper_* 引擎分支 + usesHyperliquidNativeUniverse 跳过点）、`engine_prompt.go`、`engine_position.go`
- [ ] 验证：`go build ./...`

## 阶段 5：manager / market / auto_trader / 依赖

- [ ] `manager/trader_manager.go`：工厂 case、ensureHyperliquidNativeStrategy、builder approved 拦截、TraderConfig 字段填充
- [ ] `market/data.go`（useHyperliquidAPI、xyzDexAssets、IsXyzDexAsset、Normalize xyz/USDC 分支）、`market/data_klines.go`（getKlinesFromHyperliquid、coinank 映射）
- [ ] `trader/auto_trader.go`、`auto_trader_decision.go`、`auto_trader_loop.go`（xyz 符号规范化段）、`auto_trader_risk.go`、`trader/types/interface.go`
- [ ] `provider/coinank/coinank_enum/exchange.go` 删枚举项
- [ ] `go mod tidy`
- [ ] 验证：`go build ./...`

## 阶段 6：测试修复与终验

- [ ] 修复 `trader/exchange_sync_test.go`、`manager/trader_manager_test.go`、`api/traderid_test.go`、`api/utils_test.go`、`api/handler_trader_symbol_test.go`、`api/handler_user_default_strategy_test.go`、`agent/config_visibility_test.go` 中的引用
- [ ] 新增/调整测试（纯函数级，不 mock 外网）：SourceType default 重定向 static、`GetDefaultStrategyConfig` 默认 static、provider/gate 的响应→SymbolInfo 映射与 in_delisting 过滤、symbols/klines 默认值断言（gate）；真实接口连通性由 C5 运行时探针覆盖
- [ ] 终验命令：
  ```bash
  go build ./... && go test ./...
  rg -i "hyperliquid|hyper_all|hyper_main|hyper_rank|UseHyperMain|HyperMainLimit|HyperRank" --glob '*.go'   # 归零
  rg -n "hyperliquid" go.mod go.sum                                                                          # 归零
  rg -n "/hyperliquid/" api/server.go                                                                        # 归零
  ```
- [ ] 更新 spec（trellis-update-spec）并 commit 本子任务

## 回滚点

每阶段完成时执行检查点存档（全量内容，禁用 --stat 作为唯一存档）：

```bash
git diff > scratch/c1-stage<N>.full.patch
git status --porcelain > scratch/c1-stage<N>.files.txt
```

scratch 目录已 gitignore；恢复仅用文件编辑工具，禁止 git 回滚命令。
