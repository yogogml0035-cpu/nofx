# C3：前端 Gate 行情数据看板页

## Goal

重写 `DataPage`（C2 已将其置为空态占位）：以 Gate 数据构建多模块行情看板。维持 `/data` 路由、免登录（requiresAuth: false）、`dataCenter` 标题键。

## Requirements

1. **模块清单与数据源**（全部真实数据，无占位假值；字段集以 C1 design「探针结论」为准）：
   - 市场总览（总市值 + 24h 变化、总成交、BTC/ETH dominance、BTC 价格、ahr999、山寨季指数、活跃币数——**上游无板块数据，不做板块排行**）← `GET /api/market/overview`
   - 恐惧贪婪指数（数值 + 标签 + 24h 派生变化——后端进程内历史派生，**冷启动不足 24h 为 null，前端显示 "—"**；快照时间用响应 updated_at，不用本地时钟）← `GET /api/market/fear-greed`
   - 新闻情绪：BTC 新闻列表 ← `GET /api/market/news`；市场级公告/标题 ← `GET /api/market/announcements`；BTC 社交情绪（overall + 正负比例，页面标注币种）← `GET /api/market/sentiment`
   - 涨跌幅榜（涨 top 20 / 跌 top 20，可切换）← `GET /api/symbols` 前端按 change_24h_pct 排序（该字段 omitempty，排序按 undefined→0 处理）
   - 成交额榜 top 20 ← `GET /api/symbols` 按 volume_24h 排序
   - 资金费率榜 top 20（正/负可切换）← `GET /api/symbols` 按 funding_rate 排序。**口径：funding_rate 为 Gate 原始小数（0.000389 = 0.0389%/8h），展示 ×100 保留 4 位小数加 %；正榜 = 费率降序（多头付费最深），负榜 = 升序，排序在原值上进行**
   - 清算热点 top 20 ← `GET /api/market/liquidations`
2. **刷新**：30s 自动轮询（SWR `refreshInterval=30000` + `shouldRetryOnError:false`，沿用 AppRoutes 现有 SWR 轮询模式，不用裸 setInterval）+ 手动刷新按钮；轮询失败显示区块级「数据获取失败 + 上次成功时间」（上次成功时间由前端记录各模块最后一次成功响应的本地时刻），恢复后自动清除
3. **错误态**：每模块独立错误占位，单模块失败不拖垮整页（区块级 fast-fail 展示，与后端 502 语义一致；决策链的软失败容错是 C4 的另一层语义，见 C4 PRD Req 5）
4. **布局**：桌面双栏（左：榜单类；右：总览/恐惧贪婪/新闻情绪），移动单栏纵向；风格遵循现有 nofx 暗色金主题（Tailwind 现有 token）；时间显示转本地时区，大数复用现有紧凑格式化（CoinSourceEditor 的 formatCompactNumber 风格）
5. **组件结构**：`components/data/` 新目录（MarketOverviewCard、FearGreedCard、NewsCard（含公告）、SentimentCard、BoardTable 通用榜单表、LiquidationCard），DataPage 只做布局与数据编排
6. **i18n**：新增键进 `translations.ts` **同时补齐 en/zh/id 三语言**（`t()` 缺键回退是返回原始键名字符串、不回落英文——漏语言即漏展示）
7. **点击联动（轻量）**：本期纯只读看板，不做导航联动

## Constraints

- 不引入新前端依赖；图表不用重型库，数字/条形用现有样式能力
- 免登录页不做用户态个性化
- 不做 WebSocket，轮询即可（/api/symbols 上游有 C1 的 30–60s TTL 缓存挡上游放大）

## Acceptance Criteria

- [ ] `npm --prefix web run build`、`npm --prefix web run test` 全绿（新组件有 vitest 用例：数据映射、排序切片（含 undefined→0）、费率换算、错误占位）
- [ ] 浏览器探针（已授权）截图留档任务目录 `probe/`：桌面 + 移动视口，七类模块真实数据渲染
- [ ] 断链验收（与 C1 同法）：`GATEMCP_ENDPOINT=http://127.0.0.1:9` 起后端 → 总览/恐惧贪婪/新闻/公告/情绪模块错误占位、`/api/symbols` 与 `/api/market/liquidations` 正常，两组对比截图留档；单模块粒度错误占位由 vitest 覆盖
- [ ] 未登录状态直接访问 `/data` 可见全部模块
- [ ] grep `vergex` 于 web/src 归零（C2 已删 iframe 置占位，本任务重写后复核）
