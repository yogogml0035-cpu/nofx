# C5：数据重置、镜像重建与 Gate 端到端验收

## Goal

完成父任务的集成验收：保留登录账号重置本地数据、从终态代码重建 `.nofx-local` 镜像并验证部署物、以用户提供的 Gate 子账户 API Key 走通端到端链路、浏览器截图验收关键页面，并做全仓终态 grep 验收。

## 依赖

**必须在 C1–C4 全部完成后执行**（本任务是整个父任务的集成验收节点）。

## Requirements

### 数据重置（D3）

- `.nofx-local/compose.yml` 已确认：sqlite、`DB_PATH=/app/data/data.db`、卷挂载 `./candidate-data:/app/data`、journal_mode=delete（冷拷贝即一致）
- 停容器后：`cp candidate-data/data.db scratch/data.db.bak` 全量备份；用 sqlite3 CLI 保留 `users` 表与登录行，drop 其余全部表
- **禁止用 `nofx reset-account` 代替手工 drop**：它连 users 一起删（违反 D3）、只清 5 张表漏掉 ai_charges/grid_*/trader_orders 等 12 张、不清 exchanges 的 hyperliquid 旧列（cli.go:169-178）
- 启动终态镜像后由 GORM AutoMigrate 以终态 schema 重建（exchanges 的 hyperliquid 三列随重建消失；users 表仅 5 列，无脏列）
- 验证：旧账密可登录；交易所/模型/策略/trader 全部空态

### 镜像重建与运行时验证（部署物即终态代码）

`.nofx-local/compose.yml` 无 build 段、当前引用旧 tag `nofx-local/backend:961e016d` / `frontend:961e016d-fix1`。重建步骤钉死：

1. 重建前确认 frontend `-fix1` 增量（22:10 构建、不在 git 历史）不再需要，否则先把该增量并入源码
2. 在 C4 终态 commit 的**仓库根**执行 `docker build -f docker/Dockerfile.backend -t nofx-local/backend:<新tag> .` 与 frontend 同理（需要时带 `--build-arg GOPROXY=…`）；**禁止从 `.nofx-local/preview/source`（upstream worktree，含未删除的 Hyperliquid 代码）构建，禁止直接复用旧 tag**
3. 编辑 `.nofx-local/compose.yml` 两个 image 字段为新 tag，`docker compose -f .nofx-local/compose.yml up -d --force-recreate`
4. 运行时探针三件套（证明部署物即终态代码）：`GET /api/symbols`（无参）默认返回 Gate USDT 永续标的清单（非空、无 hyperliquid 枚举）、`GET /hyperliquid/account` 返回 404、前端页面无 Hyperliquid 字样
5. 冒烟：登录 → `/traders` 空态 → 交易所配置弹窗 9 所 → 模型配置仅 API Key
6. 宿主机 `.env` 的 `CLAW402_WALLET_*` 残留变量（persistBeginnerWalletEnv 曾回写）清除；`.nofx-local/compose.yml` 已核实无 CLAW402 引用

### Gate 端到端（D5）

- UI 中配置用户提供的 Gate 子账户 API Key/Secret（凭据仅在此使用；日志与输出脱敏为 `e8b5****ed4` 形式，禁止写入任何文件；key/secret 经环境变量传入，不进命令行参数与 shell 历史）
- 只读验证：余额、持仓正确返回（余额以 available 为基准——此前实测 futures `total` 字段异常小而 `available≈48.5`，`GetBalance` 直接透传两者）；无效 key 的错误路径报错清晰
- 交易链路：BTC_USDT 永续**最小张数**市价单（`trader/gate/trader_orders.go:64-68`：quantity 小于 quanto_multiplier 时 size 自动取 1 张）→ 确认成交 → 立即平仓（quantity=0 自动 ReduceOnly 全量，:162-203）→ 持仓归零；下单前后 available 变动与手续费对账一致
- 若子账户 key 无合约交易权限，如实报告缺失权限并降级为只读验收，通知用户补权限
- 建议用户验收后在 Gate 后台轮换该 key（会话中已明文出现过）

### UIUX 截图验收

浏览器探针截图：登录页、`/traders`、交易所配置、模型配置、策略工作室、agent 聊天 + AI500 面板、落地页 Footer（仅 Gate）。本阶段验证部署形态，与 C3 的临时后端验证互不替代。

### 终态 grep 总验收（父任务 PRD 跨子任务标准）

全仓 `hyperliquid / claw402 / blockrun / x402 / hyper_all / hyper_main / hyper_rank` 仅剩 CHANGELOG；凭据零落盘。

## Acceptance Criteria

- [ ] 旧登录账号在重置后的环境可正常登录，其余数据空态；重建的 exchanges 表无 hyperliquid 三列（`PRAGMA table_info(exchanges)`）
- [ ] 运行时探针三件套通过（部署物即终态代码）
- [ ] Gate 余额/持仓读取正确（available 基准）；最小下单→成交→平仓链路走通且对账一致；无效 key 报错正常
- [ ] 全部截图符合终态（9 所、纯 API Key 模型、无模式体系、AI500 正常、Footer 仅 Gate）
- [ ] `rg -il "hyperliquid|claw402|blockrun|x402|hyper_all|hyper_main|hyper_rank" --hidden --glob '!CHANGELOG.md' --glob '!CHANGELOG.zh-CN.md' --glob '!.git/**' --glob '!.trellis/**' --glob '!web/node_modules/**' --glob '!.nofx-local/**' --glob '!scratch/**' .` 归零
- [ ] Gate 凭据零落盘（`-F -uuu` 固定串全仓搜索无命中）
- [ ] 父任务 PRD 全部跨子任务验收项勾完

## 回滚点

- DB：`cp scratch/data.db.bak candidate-data/data.db` 整文件恢复
- 镜像：compose image 字段改回旧 tag（`961e016d` / `961e016d-fix1`）再 up
- 本任务无代码改动，失败重试成本低
