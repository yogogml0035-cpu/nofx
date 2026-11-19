# C3 浏览器探针记录（2026-11-19）

环境：临时后端（容器化 linux/arm64，127.0.0.1:18090，独立 DB scratch/c3run/data.db）+ vite dev（3100 代理 /api）。注册 c3probe@example.com 初始化系统（AppRoutes :418 未初始化时显示 SetupPage，属单用户系统正常首启流程）。

| # | 验收项 | 操作 | 结果 | 证据 |
|---|--------|------|------|------|
| 1 | 未登录直接访问 /data 可见全部模块 | 清空 localStorage 后直接访问 | Data Center 页全渲染，导航含 Sign In（无用户态） | probe/desktop-anon.png |
| 2 | 七类模块真实数据渲染 | 遍历 DOM 快照核对 | Market Overview / Fear & Greed / News & Announcements / Social Sentiment / Liquidation Hotspots / Top Volume (20) / Funding Rates 全部出现，涨幅榜真实 Gate 数据（BITO_USDT +101.79% 等） | probe/desktop-anon.png |
| 3 | 移动视口单栏纵向 | 390×844 视口截图 | 单栏纵向布局正常 | probe/mobile-anon.png |
| 4 | 断链：MCP 模块错误占位 | GATEMCP_ENDPOINT=http://127.0.0.1:9 重启后端，刷新页面 | 5 个 MCP 数据源（overview/fear-greed/news/announcements/sentiment）全部区块级错误占位（news+announcements 合并卡片共享一个占位 = 4 个占位标记）；**/api/symbols 与 /api/market/liquidations 正常渲染**（服务端 curl 复核 200/200） | probe/broken-mcp.png |
| 5 | 单模块错误占位（vitest） | BoardTable.test | loading/error+上次成功时间/空态占位用例 4 过 | vitest 输出 |

## 补充说明

- 轮询采用 SWR `refreshInterval: 30000` + `shouldRetryOnError: false` + `keepPreviousData: true`（沿用 CompetitionPage 模式）
- "上次成功时间"来自 fetcher 成功回调写入的本地时刻（SWR 2.2.5 类型未暴露 dataUpdatedAt）
- 实施期修正：sortByChangeDesc/Asc 比较器方向曾反置（测试抓出），已修正
