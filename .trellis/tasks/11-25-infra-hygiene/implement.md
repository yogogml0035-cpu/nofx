# 执行计划：⑰ infra-hygiene

## 待办

- [x] 1. `.dockerignore` 增加 `.nofx-local/`、`scratch/`。
- [x] 2. `docker-compose.yml` 删除前端重复 healthcheck（保留 Dockerfile HEALTHCHECK）。
- [x] 3. `Makefile` 新增主栈变量与 `main-stack`（干净检出构建、tag 同步、健康检查、双向 D20 拦截、日志核对）；D20 前置拦截已用临时 `is_running=1` 数据库实测中止。
- [x] 4. `Makefile` 新增 `infra-check`（compose 解析、dockerignore 条目、前端健康检查单份且探测 127.0.0.1、主栈服务名断言、上下文真实断言）；上下文断言在临时目录放标记文件并保留 `keep.txt` 作正向对照，实测通过。
- [x] 5. 门禁：`git diff --check`、`make test`、`make build`、`make infra-check`、`make -n main-stack`（修复后复跑）。
- [x] 6. 独立检查首轮 **NEEDS FIX**（无 P0；1 P1 + 5 P2 + 11 P3）。修复：P1 镜像内健康检查探测 `localhost` 在 musl 下解析为 `::1`（nginx 只监听 IPv4，主栈前端容器实测常驻 unhealthy）→ 改 `127.0.0.1`，`infra-check` 增加 Dockerfile HEALTHCHECK 断言，`main-stack` 增加容器 healthy 断言，`up` 增加容器 healthy 等待；P2-1 日志标记改为真实文案（`Auto-starting trader` 等三条）；P2-2 日志命令改为命令替换（失败即中止）；P2-3 镜像构建后、改 compose 前再做一次 D20 计数检查；P2-4 tag 写入后逐项 grep 断言，`.bak` 留到 `config -q` 通过后删除；P2-5 `.dockerignore` 追加 `.env`、`data/`。P3 已修：`--since` 用 `up -d` 前时间戳、curl `--max-time 5`、`MAIN_STACK_SHA` 支持 make 变量、上下文断言改为标记目录实测（不依赖 worktree）、Dockerfile HEALTHCHECK 断言、design 文档 sed 口径、`make help` 列出两个目标、`set -o pipefail`、D20 提示带 trader id 与名称（并修正 bash 3.2 `$var` 后接全角标点吞字节的问题，统一 `${var}`）。复核发现 1 项新 P1（容器健康断言误用送测栈服务名 `nofx-frontend`，主栈 compose 实际为 `frontend`）→ 改为 `docker compose config --services` 动态取服务名，并在 `infra-check` 增加主栈服务名断言；P3 已修：三次 D20 检查分别用「重建前/构建后/重建后」文案、上下文断言加正向对照、Dockerfile 探测断言锚定 HEALTHCHECK 的 CMD 行、design 变量表与正则表述、日志读取失败提示。留档：`docker-compose.prod.yml`/`stable` 的 compose 级前端健康检查（范围外，探测 127.0.0.1 可用）；真实主栈重建未执行（留父任务收口）。
- [ ] 7. 合父、归档、推送、清理；更新地图与 10.5 进度。

## 验证命令

```bash
git diff --check
make infra-check
make -n main-stack
PATH=/tmp/nofx-bin:$PATH make test && make build
```

## 审查门禁

- `main-stack` 任一步失败即停；前置/后置 `is_running=1` 必须为 0，查询失败 fail-closed。
- 构建上下文只来自 `git archive`，不读取工作区。
- 不触碰 dev-offline 容器、数据卷和业务数据。
