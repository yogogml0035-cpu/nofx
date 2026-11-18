# C3 执行计划：前端删除 Hyperliquid、claw402 与模式体系

前置：C1、C2 已完成并 commit。构建命令 `npm --prefix web run build`（tsc && vite build）、测试 `npm --prefix web run test`（vitest）。

## 阶段 1：删除整文件与路由

- [ ] 删除 9 个文件/资产：HyperliquidWalletConnect.tsx、HyperliquidSymbolsPanel.tsx、hyperliquidQuickTrade.ts、lib/api/wallet.ts、OnboardingModeSelector.tsx、BeginnerOnboardingPage.tsx、BeginnerGuideCards.tsx、claw402.svg + claw402.png + hypeliquid.svg + hyperliquid.png
- [ ] `lib/onboarding.ts` 整删；`router/AppRoutes.tsx` 删 /welcome 路由与 showBeginnerOnboarding，`/setup` 重定向（:443-444）改 `/traders`；`router/paths.ts` 删 ROUTES.welcome
- [ ] `contexts/AuthContext.tsx`：登录成功后恒跳 `/traders`
- [ ] `router/paths.test.ts` 同步修复
- [ ] 验证：`npm --prefix web run build`（此时期望报引用错误，进入阶段 2）

## 阶段 2：交易所配置链路

- [ ] `ExchangeConfigModal.tsx`：模板表 9 所、删钱包分支、去 HyperliquidWalletConnect import、onSave 删钱包实参
- [ ] `HeaderBar.tsx` 删钱包入口；`ConfigStatusGrid.tsx` 删 claw402 与 hyperliquidWalletAddr 状态项
- [ ] `types/config.ts`、`stores/tradersConfigStore.ts`、`lib/api/config.ts`：删 hyperliquid 三字段与相关类型
- [ ] 验证：build

## 阶段 3：模型配置链路

- [ ] `ModelConfigModal.tsx`：删 claw402/blockrun 选项、featured card、钱包创建、二维码充值/绑定、余额展示、QRCodeSVG import
- [ ] `model-constants.ts`（CLAW402_MODELS、BLOCKRUN_MODELS、AI_PROVIDER_CONFIG.claw402）、`ModelIcons.tsx`：provider 列表仅 API Key、删 claw402 图标
- [ ] 验证：build

## 阶段 4：图表 / 策略 / 营销面 / 页面 / i18n

- [ ] `charts/` 三文件 xyz 逻辑；ChartTabs :49 与 `pages/TraderDashboardPage.tsx` :60 默认市场统一改 `gate`
- [ ] `CoinSourceEditor.tsx`：删 hyper_rank 按钮、头部卡片改通用文案、static 数据源重接 `/api/symbols?exchange=gate`、占位符改 crypto 语义（符号经现有 Normalize 链路归一）；`types/strategy.ts` 枚举删 hyper_*；`IndicatorEditor.tsx` 清理
- [ ] `pages/` 四页、`SetupPage.tsx`、`AITradersPage.tsx`、`TraderConfigModal.tsx`、`types/trading.ts`、`lib/api/data.ts`
- [ ] 营销面：`FooterSection.tsx` supporters 仅留 Gate；`FeaturesSection.tsx` 交易所列举；`LoginPage.tsx` 营销文案
- [ ] i18n 两文件删键：每个键名删除前 `rg -n "<键名>" web/src` 确认无孤儿引用
- [ ] 验证：build + test

## 阶段 5：本地运行 + 浏览器截图验收

- [ ] `go run` 起临时后端（独立 DB 路径、端口避开被占用的 8080，不构建镜像）+ 前端 dev server
- [ ] 浏览器探针（browser-use）逐页截图：登录页 / `/traders` / 交易所配置弹窗 / 模型配置 / 策略工作室 / agent 聊天 + AI500 面板 / 落地页 Footer
- [ ] 核验点：无 Hyperliquid 字样与图标、9 所模板、模型仅 API Key、无二维码弹窗、无模式选择器、登录直达 `/traders`、AI500 面板正常、Footer 推荐位仅 Gate
- [ ] 终验命令：
  ```bash
  npm --prefix web run build && npm --prefix web run test
  rg -i "hyperliquid|claw402|blockrun|hypeliquid|hyper_rank|hyper_all|hyper_main" web/src web/public   # 归零
  rg -n "nofx_user_mode|/welcome|getPostAuthPath|UserMode" web/src                                    # 归零
  ```
- [ ] 更新 spec 并 commit 本子任务

## 回滚点

每阶段存档（全量内容，禁用 --stat 作为唯一存档）：

```bash
git diff > scratch/c3-stage<N>.full.patch
git status --porcelain > scratch/c3-stage<N>.files.txt
```

恢复仅用文件编辑工具，禁止 git 回滚命令。
