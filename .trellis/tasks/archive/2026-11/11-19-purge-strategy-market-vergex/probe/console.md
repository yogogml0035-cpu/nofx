# C2 浏览器探针记录（2026-11-19）

环境：临时后端（容器化 linux/arm64，127.0.0.1:18090，独立 DB scratch/c2run/data.db）+ vite dev（127.0.0.1:3100，代理 /api → 18090）。注册首个账号 c2probe@example.com 建立登录态。

| # | 验收项 | 操作 | 结果 | 证据 |
|---|--------|------|------|------|
| 1 | 导航无「策略市场」 | 登录后查看顶部导航 | 导航为 Agent / Data / Config / Dashboard / Strategy / Leaderboard / FAQ，无 Market/策略市场项（桌面与移动两处均已删） | 导航快照（下方原文） |
| 2 | `/strategy-market` 直接访问 404（无重定向兼容） | 登录态直接访问 | 显示 PageNotFound 404 页（SIGNAL_LOST）；通配路由 `*` 从 Navigate home 改为挂载既有 PageNotFound 组件（消除未知路径静默回首页的兼容行为） | probe/strategy-market-404.png |
| 3 | Landing Initialize（已登录）跳 `/traders` | 已登录访问 `/`，点击 INITIALIZE | URL 落 `/traders`（未登录 `/login` 分支不变） | 本记录 |
| 4 | 「数据」导航指向空态占位页 | 点击 Data 导航 | 页面为占位壳：`Data Center` + "Dashboard upgrade in progress, stay tuned"，无任何 iframe | probe/data-placeholder.png |
| 5 | 策略编辑器无「发布到市场」区块 | New Strategy 创建后查看左侧分区 | 分区为 Strategy Type / Coin Source（Gate USDT 永续清单语义）/ Indicators / Risk Control / Strategy prompt，无 Publish Settings | probe/strategy-editor-no-publish.png |

## 导航快照原文（第 1 项证据）

```
- navigation:
  - img "NOFX Logo"
  - generic: NOFX
  - button "AgentBeta"
  - button "Data"
  - button "Config": Config
  - button "Dashboard"
  - button "Strategy"
  - button "Leaderboard"
  - button "FAQ"
  - link "GitHub" …
  - link "Telegram" …
```

## 实施期修复

- `/strategy-market` 首次探针落 `/`（既有通配路由 `*` → Navigate home 的静默兼容），与 PRD「404（无重定向兼容）」不符 → 挂载从未接线的 `PageNotFound` 组件为通配路由，复探显示 404 页
