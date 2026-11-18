# C4：文档与营销层清理

## Goal

清除 Hyperliquid、claw402、x402 在全部文档、README、营销文案、安装脚本、GitHub 配置中的痕迹；README 推荐位仅保留 Gate；产品叙事收缩为 crypto 永续。CHANGELOG 保留历史记录。

## 依赖

**在 C3 完成后执行（弱依赖：可与 C3 并行，须在 C5 前完成）**——以代码终态为准撰写文案，避免二次返工。无代码改动（前端营销面 Footer/Features/Login 归 C3，本任务不碰代码）。

## Requirements

### README 与英文主文档

- `README.md`：
  - 推荐表仅保留 Gate 一行（现有 `gatenode.xyz/share/VQBGUAxY` referral，:58），删除 binance / bybit / okx / hyperliquid / bitget / kucoin / aster / lighter 八行（:52-60 现为 9 行）
  - tagline 收缩为 crypto 永续核心表述（参考："AI trading terminal for crypto perpetual markets — research, strategy generation, execution, and monitoring."）；:48 的 "US stock, FX, and commodity derivative markets" 执行叙事收缩（research 能力 search_stock / AI500 文案保留）；:102 交易所列举句同步
  - :55 引用的 `web/public/exchange-icons/hyperliquid.png` 行随表格删除
  - hyperliquid / claw402 / x402 提及全清
- `CONTRIBUTING.md`、`SECURITY.md`（根目录）、`install.sh`（:269）、`docs/README.md`、`docs/MIGRATION_GUIDE.md`

### GitHub 配置（rg 默认不搜隐藏目录，须显式覆盖）

- `.github/labeler.yml:33`（`**/hyperliquid*.go` CI labeler 规则）
- `.github/ISSUE_TEMPLATE/bug_report.md:105`（"Binance / Hyperliquid / Aster"）
- `.github/SECURITY.md:71,75`（"Private Keys (Hyperliquid/Aster)"、"Use agent wallets (Hyperliquid)"）

### 删除整文件

- `docs/architecture/X402_STREAMING_PAYMENT.md`
- `docs/community/bounty-hyperliquid.md`
- `docs/getting-started/hyperliquid-agent-wallet.md`

### docs 子树与 i18n

- 7 语种 8 份 README：ja / ko / ru / uk / vi / zh-CN 六语种目录各一份 + `docs/i18n/README.md` 索引（英文主文档即根 README.md，`docs/i18n/en/` 无 README）；各语种 PRIVACY / TERMS / CONTRIBUTING 一并检查
- 编辑：`docs/guides/faq.en.md`、`faq.zh-CN.md`、`docs/guides/README*.md`、`docs/architecture/README*.md`、`docs/roadmap/README*.md`、`docs/getting-started/README*.md`、`docs/community/README.md`（Hyperliquid Bounty 链接行）、`docs/community/bounty-guide.md`、`docs/plans/2026-01-17-grid-market-regime-impl.md`、`docs/i18n/README.md`、`docs/agent-skills/`（diagnostic-skills 等若含 Hyperliquid 排障条目）
- 交易所列举文案（如 "Binance, Hyperliquid, etc."）改为不含 Hyperliquid 的 9 所表述
- **不动**：`CHANGELOG.md`、`CHANGELOG.zh-CN.md`、`web/CHANGELOG.md`、`web/README.md`（后两者已实测零命中）

## Acceptance Criteria

- [ ] `rg -il "hyperliquid|claw402|blockrun|x402|hyper_all|hyper_main|hyper_rank" --hidden --glob '!CHANGELOG.md' --glob '!CHANGELOG.zh-CN.md' --glob '!.git/**' --glob '!.trellis/**' --glob '!web/node_modules/**' --glob '!.nofx-local/**' --glob '!scratch/**' .` 归零（`--hidden` 确保覆盖 `.github/`；rg 默认尊重 .gitignore/.git/info/exclude，排除项作冗余保险）
- [ ] README 推荐表仅 Gate 一行，渲染正常（表格对齐、链接有效）
- [ ] `install.sh` 提示文案无 Hyperliquid
