# C4 执行计划：文档与营销层清理

前置：C3 已完成并 commit（或与 C3 并行，须在 C5 前收口）。

## 阶段 1：README 与英文主文档 + GitHub 配置

- [ ] `README.md`：推荐表仅 Gate；tagline 与正文叙事收缩；提及全清
- [ ] `CONTRIBUTING.md`、`SECURITY.md`、`install.sh`、`docs/README.md`、`docs/MIGRATION_GUIDE.md`
- [ ] `.github/labeler.yml`、`.github/ISSUE_TEMPLATE/bug_report.md`、`.github/SECURITY.md`
- [ ] 验证：
  ```bash
  rg -in "hyperliquid|claw402|blockrun|x402|hyper_all|hyper_main|hyper_rank" \
    --hidden README.md CONTRIBUTING.md SECURITY.md install.sh docs/README.md docs/MIGRATION_GUIDE.md .github/   # 归零
  ```

## 阶段 2：删除整文件

- [ ] `docs/architecture/X402_STREAMING_PAYMENT.md`
- [ ] `docs/community/bounty-hyperliquid.md`
- [ ] `docs/getting-started/hyperliquid-agent-wallet.md`

## 阶段 3：docs 子树与 i18n

- [ ] `docs/architecture/README*.md`、`docs/roadmap/README*.md`、`docs/getting-started/README*.md`、`docs/guides/README*.md` + faq 双语
- [ ] `docs/i18n/`（ja / ko / ru / uk / vi / zh-CN 六语种 + 索引）README、PRIVACY、TERMS、CONTRIBUTING（en/ 无 README，英文主文档即根 README）
- [ ] `docs/community/README.md`、`docs/community/bounty-guide.md`、`docs/plans/2026-01-17-grid-market-regime-impl.md`、`docs/agent-skills/`
- [ ] 验证：
  ```bash
  rg -il "hyperliquid|claw402|blockrun|x402|hyper_all|hyper_main|hyper_rank" \
    --hidden --glob '!CHANGELOG.md' --glob '!CHANGELOG.zh-CN.md' --glob '!.git/**' \
    --glob '!.trellis/**' --glob '!web/node_modules/**' --glob '!.nofx-local/**' --glob '!scratch/**' .   # 归零
  ```

## 阶段 4：README 渲染检查 + 收尾

- [ ] README 表格渲染、Gate 链接可点击（`gatenode.xyz/share/VQBGUAxY`）
- [ ] 更新 spec 并 commit 本子任务

## 回滚点

单阶段文本改动，存档（全量内容，禁用 --stat 作为唯一存档）：

```bash
git diff > scratch/c4-stage<N>.full.patch
git status --porcelain > scratch/c4-stage<N>.files.txt
```

恢复仅用文件编辑工具，禁止 git 回滚命令。
