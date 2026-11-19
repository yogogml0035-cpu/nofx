# C2b 浏览器探针记录（2026-11-19）

环境：临时后端（容器化 linux/arm64，127.0.0.1:18090，独立 DB scratch/c2brun/data.db）+ vite dev（3100 代理 /api）。账号 c2bprobe@example.com（API 直注注册）。

| # | 验收项 | 操作 | 结果 | 证据 |
|---|--------|------|------|------|
| 1 | GET /api/ai500 404 | curl 直探 | 404 page not found | 本记录 |
| 2 | /api/klines 正常（Coinank 保留实证） | curl BTCUSDT 5m | HTTP 200 | 本记录 |
| 3 | Agent 页无 AI500 面板 | 登录进 /agent，遍历快照 | AI500 零出现；Trading Panel 仅 Market/Positions/Traders/Preferences；快捷卡仅 3 张（原 AI500 精选卡已删） | probe/agent-no-ai500.png |
| 4 | 策略编辑器币源仅静态 Gate 清单 | API 建策略（coin_source={} 纯净）+ 打开编辑器 | Coin Source 区 = Crypto markets + 1033 个 Gate 可视化标的卡，无 AI500/OI/量化选项 | probe/strategy-editor-static-only.png |
| 5 | 排行榜回归 | /competition 页 | AI Competition 页正常渲染（0 traders 为新库正常态） | probe/leaderboard-regression.png |

补充：新建策略默认 coin_source 为空对象（{}），无任何 ai500/oi 遗留字段——static-only 收敛生效。
