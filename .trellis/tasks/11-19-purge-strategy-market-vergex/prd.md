# C2：策略市场与 vergex 语义全量清除

## Goal

删除「策略市场」的全部语义（iframe 页面、路由导航、发布链路、公开 API、Landing 跳转）与 vergex 品牌痕迹。一次性删除，不做兼容分支、不保留历史变量、不加重定向。

## Requirements（删除清单，逐项核对）

### 前端（web/src）

1. `pages/StrategyMarketPage.tsx` 整文件删除；`AppRoutes.tsx` 的 `ROUTES.strategyMarket` 路由块删除
2. `components/common/HeaderBar.tsx` 桌面 + 移动两处「策略市场」导航项删除
3. `router/paths.ts`：`Page` 联合类型 `'strategy-market'`、`ROUTES.strategyMarket`、`PAGE_PATHS`、`LEGACY_HASH_ROUTES`、`getCurrentPageForPath` 分支全删；`router/paths.test.ts` 同步
4. `components/landing/core/AgentGrid.tsx`：`handleInitialize` 已登录改跳 `/traders`（未登录 `/login` 不变）
5. `pages/DataPage.tsx`：删除 vergex iframe 与其 URL，页面置为最小空态占位（标题 + `dataCenter` 键 + 「数据看板升级中」类文案）——C3 重写内容
6. `components/strategy/PublishSettingsEditor.tsx` 整文件删除；`pages/StrategyStudioPage.tsx` 中发布设置区块/引用删除
7. `types/strategy.ts`：isPublic / publish 相关类型字段删除
8. i18n：`strategy-translations.ts` 的 `publishSettings` 组、`translations.ts` 的 `strategyMarket` 键删除（translations.ts 为 en/zh/id 三语言，同步删三处；`dataCenter` 键保留）
9. landing 其他组件中的 vergex/策略市场引导文案（实施时 grep 复核）

### 后端（Go）

10. `api/server.go` 的 `GET /strategies/public` 路由与 `api/strategy.go` 的 `handlePublicStrategies`、`attachPublishConfig` 删除
11. `api/strategy.go` 请求/响应 DTO 中 `is_public` / `config_visible` / `publish_config` 字段与 `hadPublishConfig` 分支删除
12. `store/strategy.go`：`Strategy.IsPublic`、`Strategy.ConfigVisible` 列与 `PublishStrategyConfig` 类型、`ListPublic()` 方法删除；`StrategyConfig.PublishConfig` 字段删除
13. agent 层 publish/is_public 语义清除：`skill_management_handlers.go`、`central_brain.go`、`strategy_field_catalog.go`、`tools.go`、`llm_flow_extractor.go`、`skill_domain_context.go`、`skill_execution_handlers.go`、`skills/strategy_management.json`（`config_visible` schema 定义与 `optional_slots` 的 `is_public`）、`config_visibility_test.go`、`trader_scope_test.go`（夹具名「AI500 做多策略」一并改名，属 C2b 词表的词），以及 `telegram/agent/apicall.go` bot 白名单中的 `^/api/strategies/public$` 条目（`/api/competition`、`/api/top-traders` 等排行榜端点白名单保留）
14. 相关测试同步：上述测试文件中 is_public/config_visible 断言删除或改写为「列不存在」（`strategy_schema_test.go` 实测不含这些断言，无需改）

### 文档与营销

15. `README.md`：头部 "Backed by vergex.trade" 行、链接表 Website/Dashboard(@vergex.trade/explore) 与 Twitter(@vergex_ai) 行删除
16. `web/src/constants/branding.ts` 的 twitter `@vergex_ai`；`components/landing/CommunitySection.tsx` 的 X 卡片——若无其他官方账号可指则整卡删除，导航布局相应收拢
17. `docs/i18n/{zh-CN,vi,ja,uk,ru,ko}/README.md` 六份中的 vergex 链接与背书行删除（`en/` 目录无 README.md，不涉及）
18. `.github/CLA.md` 中 vergex 字样（逐处核对，仅删涉 vergex 行）

### 明确保留（不得误删）

- `handler_competition.go` 全部 public 端点与 CompetitionPage（排行榜，内部功能，D11）
- `store/visibility.go` 的 IsVisibleAIModel/IsVisibleExchange（与策略市场无关的可见性过滤）
- `dataCenter` i18n 键与 DataPage 占位壳（C3 重建对象）
- CHANGELOG 双文件、`docs/legal/**`（对外维权报告）、`docs/plans/**`（历史规划快照）历史记录
- 主工作区 `provider/vergex/` 空目录的清理移至 C5 收口（子任务 worktree 不写主工作区）

## Acceptance Criteria

- [ ] `rg -il "vergex|strategy-market|strategy_market|strategyMarket|publishToMarket|handlePublicStrategies|ListPublic|ConfigVisible|config_visible|PublishConfig|publish_config|strategies/public" --hidden --glob '!CHANGELOG.md' --glob '!CHANGELOG.zh-CN.md' --glob '!.trellis/**' --glob '!.git/**' --glob '!web/node_modules/**' --glob '!.nofx-local/**' --glob '!scratch/**' --glob '!docs/legal/**' --glob '!docs/plans/**' --glob '!screenshots/**' .` 归零
- [ ] `rg -n "IsPublic|\"is_public\"" --type go` 归零（含测试；词表二在 Go 域对排行榜/visibility/AI model 零假阳性，已实证）
- [ ] `go build ./...`、`go test ./...`（Docker golang:1.25-alpine + GOPROXY=goproxy.cn）、`npm --prefix web run build`、`npm --prefix web run test` 全绿
- [ ] 浏览器探针：导航无「策略市场」；`/strategy-market` 直接访问 404；Landing Initialize（已登录态）落 `/traders`；「数据」导航指向空态占位页
- [ ] 策略编辑器无「发布到市场」区块；全库 isPublic 引用归零即自动满足（现列表/详情组件本无公开状态徽标）
