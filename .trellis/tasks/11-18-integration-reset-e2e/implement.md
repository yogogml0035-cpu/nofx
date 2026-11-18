# C5 执行计划：数据重置、镜像重建与 Gate 端到端验收

前置：C1–C4 全部完成并 commit。

## 阶段 1：数据重置

- [ ] 停容器：`docker compose -f .nofx-local/compose.yml down`
- [ ] 备份：`cp .nofx-local/candidate-data/data.db scratch/data.db.bak`
- [ ] sqlite3 CLI：保留 users 表与登录行，drop 其余全部表（**禁止 `nofx reset-account`**：删 users、漏 12 张表、不清旧列）
- [ ] 验证：users 行仍在，其余表已删

## 阶段 2：镜像重建与运行时验证

- [ ] 确认 frontend `961e016d-fix1` 增量不再需要（否则先并入源码）
- [ ] 在 C4 终态 commit 的仓库根构建（禁止从 `.nofx-local/preview/source` 构建、禁止复用旧 tag）：
  ```bash
  docker build -f docker/Dockerfile.backend  -t nofx-local/backend:<新tag>  .
  docker build -f docker/Dockerfile.frontend -t nofx-local/frontend:<新tag> .
  ```
- [ ] 编辑 `.nofx-local/compose.yml` 两个 image 字段为新 tag；`docker compose -f .nofx-local/compose.yml up -d --force-recreate`
- [ ] 运行时探针：`GET /api/symbols`（无参）默认返回 Gate USDT 永续清单（非空、无 hyperliquid）、`GET /hyperliquid/account` 404、前端页面无 Hyperliquid 字样
- [ ] 浏览器探针：旧账密登录成功 → 直达 `/traders`（无模式选择器）
- [ ] 检查空态：交易所/模型/策略/trader 均为空；`PRAGMA table_info(exchanges)` 无 hyperliquid 三列
- [ ] 清除宿主机 `.env` 的 `CLAW402_WALLET_*` 残留变量

## 阶段 3：Gate 端到端

- [ ] 连通性预检：`scratch/gate_conn_test.py`（已验证通过：公开行情 200、签名私有接口 200、合约读取权限正常；签名格式见脚本内注释——URL 仅含路径，空 body 用 SHA512 空串；凭据经 `GATE_KEY/GATE_SECRET` 环境变量传入）
- [ ] UI 配置 Gate 子账户 key/secret（输出脱敏为 `e8b5****ed4`；禁止写入文件/日志/命令行参数）
- [ ] 只读：余额（available 基准）、持仓返回正确；再填一个无效 key 验证报错清晰后改回
- [ ] 交易：BTC_USDT 永续最小张数市价单（quantity < quanto_multiplier 时 size 自动取 1）→ 查成交 → ReduceOnly 全量平仓 → 持仓归零；前后 available 变动与手续费对账；全程截图
- [ ] 若 key 无合约交易权限，如实报告并降级为只读验收，通知用户补权限

## 阶段 4：UIUX 截图与终态验收

- [ ] 截图集：登录页 / `/traders` / 交易所配置（9 所）/ 模型配置（仅 API Key）/ 策略工作室 / agent 聊天 + AI500 面板 / 落地页 Footer（仅 Gate）
- [ ] 终态命令：
  ```bash
  rg -il "hyperliquid|claw402|blockrun|x402|hyper_all|hyper_main|hyper_rank" \
    --hidden --glob '!CHANGELOG.md' --glob '!CHANGELOG.zh-CN.md' --glob '!.git/**' \
    --glob '!.trellis/**' --glob '!web/node_modules/**' --glob '!.nofx-local/**' --glob '!scratch/**' .   # 归零
  # 凭据零落盘：-F 固定串、-f - 从 stdin 读 pattern（不进 argv/shell 历史/文件）、-uuu 关闭 ignore 过滤
  # （.nofx-local/preview 除外，但 candidate-data 运行日志必须搜——泄漏正是发生在那里）
  printf '%s' "$GATE_KEY"    | rg -F -n -f - -uuu --no-messages --glob '!.git/**' --glob '!.nofx-local/preview/**' . || echo "key 零落盘"
  printf '%s' "$GATE_SECRET" | rg -F -n -f - -uuu --no-messages --glob '!.git/**' --glob '!.nofx-local/preview/**' . || echo "secret 零落盘"
  ```
- [ ] 勾完父任务 PRD 跨子任务验收清单
- [ ] 提醒用户轮换 Gate key

## 阶段 5：收尾

- [ ] trellis-update-spec 更新 spec
- [ ] commit 本子任务；父任务做最终集成评审后归档

## 回滚点

- DB：`cp scratch/data.db.bak .nofx-local/candidate-data/data.db` 整文件恢复
- 镜像：compose image 字段改回旧 tag（961e016d / 961e016d-fix1）再 up
- 本任务无代码改动，失败重试成本低
