# 子任务 ⑰ PRD：基础设施卫生（infra-hygiene）

父任务 `11-24-gate-trading-foundation`。依据：计划 D13、D14、D16、D20。本文件列需求与验收；
技术设计见 `design.md`，执行步骤见 `implement.md`。

## Goal

消除部署与构建面的三个隐患：构建上下文携带本地状态、前端健康检查两份、主栈重建靠手工。
主栈重建固化为 Makefile 命令，且在任何阶段都不允许拉起 trader。

## Requirements

1. **`.dockerignore`**：新增 `.nofx-local/` 与 `scratch/`；`web/node_modules/` 已有条目保留。
   构建上下文不得包含本地部署状态（密钥、候选数据库、日志）与临时草稿。
2. **健康检查单份**：前端健康检查只保留 `docker/Dockerfile.frontend` 的 `HEALTHCHECK`；
   `docker-compose.yml` 的 `nofx-frontend` 不再重复声明。后端维持现状（D14 只要求前端）。
3. **Makefile `main-stack`**：从干净检出构建并重建主栈：
   - 构建上下文由 `git archive <SHA>` 解包到临时目录，不使用工作区；
   - 镜像 `nofx-local/backend:<sha>`、`nofx-local/frontend:<sha>`；
   - 同步 `.nofx-local/compose.yml` 的 image tag 到该 SHA；
   - `docker compose config -q` 通过后 `up -d`；
   - 健康检查 `127.0.0.1:8888/` 与 `/api/health`；
   - **D20**：重建前 `.nofx-local/candidate-data/data.db` 的 `traders.is_running=1` 计数必须为 0，
     否则拒绝重建；重建后再核对仍为 0，并检查启动日志没有 `✅ Running`；
   - 任一步失败立即退出，不碰 dev-offline 栈。
4. **Makefile `infra-check`**：配置解析与上下文卫生的确定性检查：
   - `docker compose -f docker-compose.yml config -q`；存在 `.nofx-local/compose.yml` 时同样检查；
   - `.dockerignore` 必需条目齐全；
   - 构建上下文确不含 `.nofx-local`/`scratch`（临时 Dockerfile `COPY .` 后断言不存在）；
   - 前端健康检查在 compose 中不重复。
5. **不改**：业务代码、数据库结构、dev-offline 容器与数据卷、trader/策略运行状态。

## Acceptance Criteria

1. `make infra-check` 通过（含真实构建上下文断言）。
2. `make -n main-stack` 展示完整步骤；本轮不实际重建主栈（留到父任务收口按 D20 执行）。
3. `git diff --check`、`make test`、`make build` 通过；独立检查子智能体 PASS。
