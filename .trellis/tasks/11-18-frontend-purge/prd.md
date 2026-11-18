# C3：前端删除 Hyperliquid、claw402 与新手/老手模式

## Goal

删除前端两套体系与模式体系的全部实现：钱包连接组件、xyz 标的面板、快速交易、claw402 模型 UI 与二维码绑定、模式选择器与新手页；落地页/Footer 推荐位收缩为仅 Gate（D4 的前端半边）。登录后统一进入标准界面（原老手模式行为，恒 `/traders`）。

## 依赖

**必须在 C1、C2 完成后执行**：浏览器运行时验证需要删除后的后端（以 `go run` 起临时后端：独立 DB 路径、端口避开被占用的 8080，不中途构建镜像）；共享类型与 API client 需与后端契约同步（exchange 请求体无 hyperliquid 三字段、AIModel 无钱包 DTO 字段、`/wallet/*` 与 `/onboarding/beginner*` 路由消失、SourceType 仅 static/ai500/oi_top/oi_low）。

## Requirements

### 删除整文件

- `web/src/components/common/HyperliquidWalletConnect.tsx`（消费方：HeaderBar:14,212、ExchangeConfigModal:6,703）
- `web/src/components/agent/HyperliquidSymbolsPanel.tsx`（xyz 标的面板；**`web/src/components/agent/AI500Panel.tsx` 是独立组件，保留**——AgentChatPage :24-25,741,753 分别渲染两者，无隐式依赖）
- `web/src/lib/hyperliquidQuickTrade.ts`
- `web/src/lib/api/wallet.ts`（Hyperliquid 钱包 API client）
- `web/src/components/auth/OnboardingModeSelector.tsx`（模式选择器）
- `web/src/pages/BeginnerOnboardingPage.tsx`（`/welcome` 新手页）
- `web/src/components/trader/BeginnerGuideCards.tsx`（新手引导卡片）
- `web/public/icons/claw402.svg`、`web/public/icons/claw402.png`（ModelIcons.tsx:53 与 ModelConfigModal.tsx:249 引用的是 png）、`web/public/icons/hypeliquid.svg`（注意拼写少 r 的孤儿资产）、`web/public/exchange-icons/hyperliquid.png`

### 模式体系收敛

- `web/src/lib/onboarding.ts`：整文件删（全文 37 行均为模式体系：UserMode、`nofx_user_mode` / `nofx_beginner_wallet_address` / `nofx_beginner_onboarding_completed` 三键、getPostAuthPath）
- `web/src/contexts/AuthContext.tsx`（:6,117 getPostAuthPath）、`web/src/router/AppRoutes.tsx`：删 `/welcome` 路由（:489-500）、showBeginnerOnboarding 分支（:196-215）、**`/setup` 对已登录用户重定向改指 `/traders`（:443-444）**；登录成功后恒跳 `/traders`
- `web/src/components/modals/SetupPage.tsx` 模式相关分支清理
- `web/src/router/paths.ts`（:19 ROUTES.welcome）与 `web/src/router/paths.test.ts`（:12 断言 welcome→traders）同步修复

### 营销面收缩（D4 前端半边）

- `web/src/components/landing/FooterSection.tsx`（:34-44）：supporters 推荐位删 Binance/Bybit/OKX/Bitget/KuCoin/Hyperliquid/Aster/Lighter 八条，仅留 Gate
- `web/src/components/landing/FeaturesSection.tsx`（:42-43）：交易所列举文案改 9 所表述（无 Hyperliquid）
- `web/src/components/auth/LoginPage.tsx`（:119-122）：三语营销文案删 Hyperliquid 表述

### 组件与页面清理

- `web/src/components/trader/ExchangeConfigModal.tsx`：模板表删 hyperliquid 行（:25-36 共 10 行删后恰 9 所）、删钱包分支与 HyperliquidWalletConnect 引入、onSave（:49）删 hyperliquidWalletAddr 实参
- `web/src/components/trader/ModelConfigModal.tsx`：删 claw402 / blockrun-base / blockrun-sol 选项（:11-12,321-331）、featured card（:230-281）、Claw402ConfigForm 与二维码充值/绑定、usdcBalance 展示（:350-460）、QRCodeSVG import（:2）
- `web/src/components/common/HeaderBar.tsx` 钱包连接入口；`web/src/components/common/ModelIcons.tsx`、`ExchangeIcons.tsx` 图标映射；`web/src/components/trader/ConfigStatusGrid.tsx`（:141-150 claw402 状态、:241-260 hyperliquidWalletAddr 状态项）
- `web/src/components/trader/`（TraderConfigModal、AITradersPage、model-constants.ts :48,67,132）：文案与状态项清理，provider 列表仅 API Key
- `web/src/components/charts/`：AdvancedChart（:53,944-945）、ChartTabs（:19,30,87-88 xyz 逻辑 + **:49 无 exchangeId 时默认市场 'hyperliquid' 改为 'gate'**）、ChartWithOrders（:40）
- `web/src/components/strategy/CoinSourceEditor.tsx`：**重接而非保留现状**——删 hyper_rank 来源按钮（:204-216 现仅 hyper_rank 与 static 两按钮）、头部"Hyperliquid 原生标的"卡片（:180-201）改通用文案；static 模式标的清单数据源从 `/api/symbols?exchange=hyperliquid-xyz`（:73）改为 `/api/symbols?exchange=gate`（D7 默认；符号格式沿用现有 Normalize 归一链路，market/data.go:561+）；搜索占位符（:348 "SAMSUNG / TESLA / GOLD…"）改 crypto 语义（BTC / ETH / SOL…）；默认来源 static
- `web/src/types/strategy.ts`（:108）：SourceType 枚举删 `'hyper_all'|'hyper_main'|'hyper_rank'`，保留 `'static'|'ai500'|'oi_top'|'oi_low'`
- `web/src/pages/`（AgentChatPage、SettingsPage、StrategyStudioPage、TraderDashboardPage——**:60 kline 图表无交易所 fallback 'binance' 改 'gate'**）、`web/src/stores/tradersConfigStore.ts`、`web/src/types/config.ts`、`types/trading.ts`、`web/src/lib/api/data.ts`、`lib/api/config.ts` 清理
- `web/src/i18n/translations.ts`（76 处）、`strategy-translations.ts`（8 处）：删相关键值（删键前 `rg -n "<键名>" web/src` 确认无孤儿引用）

### UIUX 验证（浏览器探针 + 截图）

以 `go run` 临时后端（独立 DB、非 8080 端口）+ `npm --prefix web run dev` 逐页截图核验：登录页（无 Hyperliquid 文案）、`/traders` 列表、交易所配置弹窗（9 所模板）、模型配置（仅 API Key provider、无二维码/钱包入口）、策略工作室（CoinSource 无 xyz 来源、static 币源从 Gate 标的清单选币，D7）、agent 聊天页 + 侧栏 AI500 面板正常渲染、登录后无模式选择器与 `/welcome`、落地页 Footer 推荐位仅 Gate。

## Acceptance Criteria

- [ ] `npm --prefix web run build` 全绿（build = tsc && vite build）
- [ ] `npm --prefix web run test` 全绿（vitest，含修复后的 paths.test.ts）
- [ ] `rg -i "hyperliquid|claw402|blockrun|hypeliquid|hyper_rank|hyper_all|hyper_main" web/src web/public` 归零
- [ ] `rg -n "nofx_user_mode|/welcome|getPostAuthPath|UserMode" web/src` 归零
- [ ] localStorage 不再写入模式键
- [ ] 浏览器截图确认上述页面集全部符合终态
