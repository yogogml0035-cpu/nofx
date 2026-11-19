# C5 集成验收探针记录（2026-11-20）

环境：父 worktree 全量门禁 + 临时后端（容器化 linux/arm64，127.0.0.1:18090，独立 DB scratch/c5run/data.db）+ vite dev（3100 代理 /api）。账号 c5probe@example.com；模型 probe1_deepseek 指向不可达端点（https://c5-probe-nonexistent.invalid/v1，DNS NXDOMAIN fast-fail）；交易所为用户提供的真实 Gate key。

## 一、全量门禁（父 worktree @ 4991f7dc+）

| 项 | 结果 |
|---|---|
| go build ./...（Docker golang:1.25-alpine + CGO + goproxy.cn） | PASS（BUILD_OK） |
| go test -count=1 ./...（同上，22 个含测包） | PASS（全 ok） |
| npm --prefix web run build（tsc + vite） | PASS（2937 modules，WEB_BUILD_OK） |
| npm --prefix web run test（vitest） | PASS（10 文件 136 用例全绿） |
| 三条 grep 词表（父 PRD 排除集 + --hidden 纳入 .github/） | 全部归零（词表一 vergex/策略市场 0、词表二 IsPublic 0、词表三 nofxos 数据链 0） |

初跑词表曾出现 zsh 变量不分词导致的「No files were searched」假归零，已以内联引号重跑为真归零；初跑 web 门禁曾因 node_modules 缺失出现 tsc/vitest command not found 假绿，npm ci 后重跑为真绿。

## 二、curl 运行时探针（真实 MCP）

| # | 验收项 | 结果 | 证据 |
|---|--------|------|------|
| 1 | GET /api/symbols 无参默认 gate 且含 funding_rate | 200，count=1033，582 个含非空 funding_rate；抽查 BTC_USDT=0.0001、ETH_USDT=4.5e-06、XAU_USDT=0.000139、XAG_USDT=7.82e-05、SOL_USDT=0.0001（Gate 原始小数语义） | curl/symbols.json |
| 2 | 六个 /api/market/* 真实数据 | overview 200（总市值 $1.267T、BTC $63100.1、dominance 58.89%、山寨季 52）；fear-greed 200（value=11 Extreme Fear，change_24h=null 冷启动允许态）；news 200 真实条目；announcements 200 真实条目；sentiment 200（-0.363 negative）；liquidations 200（ETH_USDT 热点 $61335） | curl/market-*.json |
| 3 | 断链 502 fast-fail（GATEMCP_ENDPOINT=http://127.0.0.1:9 重启） | 五个 MCP 端点全部 502 {"error":"Gate MCP upstream unreachable"}；/api/symbols 与 /api/market/liquidations 200 不受影响 | curl/broken-overview.json |
| 4 | GET /api/ai500 | 404（路由已删） | curl/ai500.json |
| 5 | GET /api/klines 正常返回 | BTC_USDT 1h×5 真实 OHLCV（11-19 20:00 O:63919.9 H:63948.2 L:63542 C:63849.7 V:16.7M…，末根为成型中 K 线）；BTCUSDT/Landing hero 形态与显式 exchange=gate 均非空 | curl/klines.json |

### 5 的回流缺陷修复（本任务直接修父分支）

缺陷：`/api/klines` 默认 gate 路径曾恒返回空数组——handler 对 symbol 无条件 `market.Normalize`（BTC_USDT→BTCUSDT），而 Coinank 上游 Gate 源要求下划线形态（实证：Gate+BTC_USDT 有数据、Gate+BTCUSDT success:true 空 data 且不触发回退；C2b 当时「HTTP 200」同为空数组）。market 层决策链走 binance 硬编码不受影响，但 HTTP 端点与 gate 交易者的 `market.GetWithExchange` 订单路径取数受影响。

修复（fix(integration) 提交）：Coinank 各交易所合约符号归一收敛进 `coinank_api.Kline` 入口（新 provider/coinank/coinank_api/symbol.go：Gate→BTC_USDT、OKX→BTC-USDT-SWAP、其余紧凑形态；幂等、未知后缀 fast-fail 原样返回），api handler 删除冗余 Normalize/OKX 特判并对齐 market 层的空数据回退 Binance；13 子用例单测 symbol_test.go 全绿。

## 三、决策循环注入探针（真实 Gate 账户 + 真实 MCP + 不可达模型）

两个 trader 各跑一轮决策（AI 调用按设计在 DNS 失败重试 3 次后落库，决策链组装全程真实）：

| # | 验收项 | 结果 | 证据 |
|---|--------|------|------|
| 1 | 勾选策略决策日志含注入段 | `🌐 [c5-probe-trader-on] Market environment context assembled (overview=true fear_greed=true news_sentiment=true)`（loop :585，真实 MCP 取数、零 unavailable 告警） | curl/decision_all.log |
| 2 | 未勾选策略不含注入段 | 无 🌐 行，6.5 段守卫整段跳过（零 gatemcp 调用） | curl/decision_all.log |
| 3 | 决策记录落库（AI 失败路径） | 两 trader 各 1 条，error_message 含 "AI API call failed: still failed after 3 retries" | curl/decisionsA.json / decisionsB.json |
| 4 | market_context API 序列化往返 | POST /api/strategies 创建（ai_config.market_context 三勾选）→ GET 回读 `{"overview":true,"fear_greed":true,"news_sentiment":true}` | 本记录（探针脚本输出） |

真实 AI 决策一轮（需真实模型 key）留待送测阶段由用户执行；本任务以真实取数链 + 真实决策循环 + 确定性 AI 失败覆盖全部可确定性路径。

## 四、浏览器探针（未登录 + 登录态）

| # | 验收项 | 结果 | 证据 |
|---|--------|------|------|
| 1 | 未登录直访 /data：导航无策略市场 + 七模块 | 导航 Agent/Data/Config/Dashboard/Strategy/Leaderboard/FAQ + Sign In（无策略市场）；Top Gainers(20)/Top Volume(20)/Funding Rates/Liquidation Hotspots/Market Overview/Fear & Greed/News & Announcements/Social Sentiment 全渲染，真实 Gate 数据 | desktop-anon.png |
| 2 | 移动视口 390×844 | 单栏纵向布局正常 | mobile-anon.png |
| 3 | /strategy-market 直访 404 | URL 不变 + SIGNAL_LOST 404 页（无重定向兼容） | strategy-market-404.png |
| 4 | Landing INITIALIZE（已登录）落 /traders | 登录态点击 INITIALIZE → /traders；导航显示用户芯片、无 Sign In | 本记录（URL 断言） |
| 5 | 策略编辑市场环境勾选组 | Market Environment Context 三勾选组渲染；未勾选策略三框空、勾选策略三框 [checked]（与存储一致） | strategy-editor-mc.png / strategy-editor-mc-on.png |
| 6 | 币源仅静态 Gate 清单 | Coin Source 「Gate USDT perpetual list, sorted by 24h volume」1033 市场、无 AI500/OI 选项 | strategy-editor-mc.png |
| 7 | Agent 页无 AI500 面板 | /agent 快照与截图零 AI500/NetFlow | agent-page.png |
| 8 | 排行榜回归 | AI Competition 双 trader 对比、Performance Comparison equity 曲线区（1D/3D/7D/30D/All）、LIVE 榜单、Head-to-Head 全渲染；参赛开关 PUT /traders/:id/competition 关→开回环 200；公开榜 /api/competition count=2 | leaderboard.png |

探针操作备注：本会话 IAB 的 playwright click 因页面动画永不满足 stability 而挂起、页面级/locator evaluate 均不生效，统一改用 cua 坐标点击与 fill+Enter 提交（登录表单首次 dom_cua.type 曾把密码拼进 email 框被 HTML5 校验静默拦截，改 fill 后 Enter 提交成功）。

## 五、主工作区收尾

- `provider/vergex/` 空目录 rmdir（untracked，主工作区执行）——见收尾提交前核验。
