# C2：策略市场与 vergex 语义全量清除

## Goal

删除「策略市场」的全部语义（iframe 页面、路由导航、发布链路、公开 API、Landing 跳转）与 vergex 品牌痕迹。一次性删除，不做兼容分支、不保留历史变量、不加重定向。

## Requirements（删除清单，逐项核对）

### 前端（web/src）

1. `pages/StrategyMarketPage.tsx` 整文件删除；`AppRoutes.tsx` 的 `ROUTES.strategyMarket` 路由块删除
2. `components/common/HeaderBar.tsx` 桌面 + 移动两处「策略市场」导航项删除
3. `router/paths.ts`：`Page` 联合类型 `'strategy-market'`、`ROUTES.strategyMarket`、`PAGE_PATHS`、`LEGACY_HASH_ROUTES`、`getCurrentPageForPath` 分支全删；`router/paths.test.ts` 同步
4. `components/landing/core/AgentGrid.tsx`：`handleInitialize` 已登录改跳 `/traders`（未登录 `/login` 不变）
5. `components/strategy/PublishSettingsEditor.tsx` 整文件删除；`pages/StrategyStudioPage.tsx` 中发布设置区块/引用删除
6. `types/strategy.ts`：isPublic / publish 相关类型字段删除
7. i18n：`strategy-translations.ts` 的 `publishSettings` 组、`translations.ts` 的 `strategyMarket` 键删除（`dataCenter` 键保留——C3 新看板沿用）
8. landing 其他组件中的 vergex/策略市场引导文案（实施时 grep 复核）

### 后端（Go）

9. `api/server.go:172` 的 `GET /strategies/public` 路由与 `api/strategy.go` 的 `handlePublicStrategies`、`attachPublishConfig` 删除
10. `api/strategy.go` 请求/响应 DTO 中 `is_public` / `config_visible` / `publish_config` 字段与 `hadPublishConfig` 分支删除
11. `store/strategy.go`：`Strategy.IsPublic`、`Strategy.ConfigVisible` 列与 `PublishStrategyConfig` 类型、`ListPublic()` 方法删除；`StrategyConfig.PublishConfig` 字段删除
12. agent 层 publish/is_public 语义清除：`skill_management_handlers.go`、`central_brain.go`、`strategy_field_catalog.go`、`tools.go`、`llm_flow_extractor.go`、`skill_domain_context.go`、`skill_execution_handlers.go`（实施时以 grep 逐文件核对，含工具 schema 描述文案）
13. 相关测试同步：`strategy_schema_test.go` 等断言 is_public/config_visible 列的用例删除或改写为"列不存在"

### 文档与营销

14. `README.md`：头部 "Backed by vergex.trade" 行、链接表 Website/Dashboard(@vergex.trade/explore) 行删除（Twitter 行见 15）
15. `web/src/constants/branding.ts` 的 twitter `@vergex_ai`；`components/landing/CommunitySection.tsx` 的 X 卡片——若无其他官方账号可指则整卡删除，导航布局相应收拢
16. `docs/i18n/{zh-CN,en→无,vi,ja,uk,ru,ko}/README.md` 六语言中的 vergex 链接与背书行同步删除
17. `.github/CLA.md` 中 vergex 字样（贡献者协议主体不涉及则只删涉vergex行；逐处核对）
18. 主工作区残留 `provider/vergex/` 空目录删除（untracked，rmdir 即可；worktree 内不存在）

### 明确保留（不得误删）

- `handler_competition.go` 全部 public 端点与 CompetitionPage（排行榜，内部功能）
- `store/visibility.go` 的 IsVisibleAIModel/IsVisibleExchange（与策略市场无关的可见性过滤）
- `DataPage.tsx` 与 `dataCenter` i18n 键（C3 重建对象）
- CHANGELOG 双文件历史记录

## Acceptance Criteria

- [ ] `rg -il "vergex|strategy-market|strategy_market|strategyMarket|publishToMarket|handlePublicStrategies|ListPublic|ConfigVisible|config_visible|PublishConfig|publish_config|strategies/public" --hidden --glob '!CHANGELOG.md' --glob '!CHANGELOG.zh-CN.md' --glob '!.trellis/**' --glob '!.git/**' --glob '!web/node_modules/**' --glob '!.nofx-local/**' --glob '!scratch/**' .` 归零
- [ ] `rg -n "IsPublic|\"is_public\"" --type go` 归零（含测试）
- [ ] `go build ./...`、`go test ./...`、`npm --prefix web run build`、`npm --prefix web run test` 全绿
- [ ] 浏览器探针：导航无「策略市场」；`/strategy-market` 直接访问 404；Landing Initialize（已登录态）落 `/traders`
- [ ] 策略编辑器无"发布到市场"区块；策略列表/详情无公开状态徽标
