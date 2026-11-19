# C3：前端 Gate 行情数据看板页

## Goal

重写 `DataPage`：删除 iframe，以 Gate 数据构建多模块行情看板。维持 `/data` 路由、免登录（requiresAuth: false）、`dataCenter` 标题键。

## Requirements

1. **模块清单与数据源**（全部真实数据，无占位假值）：
   - 市场总览（板块涨跌、BTC/ETH 摘要）← `GET /api/market/overview`
   - 恐惧贪婪指数（数值 + 变化）← `GET /api/market/fear-greed`
   - 新闻情绪（top 新闻列表 + 社交情绪分）← `GET /api/market/news`、`GET /api/market/sentiment`
   - 涨跌幅榜（涨 top 20 / 跌 top 20，可切换）← `GET /api/symbols` 前端按 change_24h_pct 排序
   - 成交额榜 top 20 ← `GET /api/symbols` 按 volume_24h 排序
   - 资金费率榜 top 20（正/负可切换）← `GET /api/symbols` 按 funding_rate 排序（C1 新增字段）
   - 清算热点 top 20 ← `GET /api/market/liquidations`
2. **刷新**：30s 自动轮询 + 手动刷新按钮；轮询失败显示区块级"数据获取失败 + 上次成功时间"，恢复后自动清除
3. **错误态**：每模块独立 try/catch 渲染错误占位，单模块失败不拖垮整页（区块级 fast-fail 展示，与后端 fast-fail 一致）
4. **布局**：桌面双栏（左：榜单类；右：总览/恐惧贪婪/新闻情绪），移动单栏纵向；风格遵循现有 nofx 暗色金主题（Tailwind 现有 token）
5. **组件结构**：`components/data/` 新目录（MarketOverviewCard、FearGreedCard、NewsCard、BoardTable 通用榜单表、LiquidationCard），DataPage 只做布局与数据编排
6. **i18n**：新增键进 `translations.ts`（zh/en 为主，沿用现有多语言键结构）
7. **点击联动（轻量）**：榜单行点击跳 `/dashboard` 不做（无标的上下文）——本期纯只读看板，不做导航联动

## Constraints

- 不引入新前端依赖；图表不用重型库，数字/条形用现有样式能力
- 免登录页不做用户态个性化
- 不做 WebSocket，轮询即可

## Acceptance Criteria

- [ ] `npm --prefix web run build`、`npm --prefix web run test` 全绿（新组件有 vitest 用例：数据映射、排序切片、错误占位）
- [ ] 浏览器探针（已授权）截图留档任务目录 `probe/`：桌面 + 移动视口，七类模块真实数据渲染
- [ ] 断开后端某端点（改 URL 模拟）时对应模块显示错误占位，其余模块正常
- [ ] 未登录状态直接访问 `/data` 可见全部模块
- [ ] grep `vergex` 于 web/src 归零（C2 已删 DataPage iframe，本任务重写后复核）
