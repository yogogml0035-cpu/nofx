# 执行计划：⑰ infra-hygiene

## 待办

- [x] 1. `.dockerignore` 增加 `.nofx-local/`、`scratch/`。
- [x] 2. `docker-compose.yml` 删除前端重复 healthcheck（保留 Dockerfile HEALTHCHECK）。
- [x] 3. `Makefile` 新增主栈变量与 `main-stack`（干净检出构建、tag 同步、健康检查、双向 D20 拦截、日志核对）；D20 前置拦截已用临时 `is_running=1` 数据库实测中止。
- [x] 4. `Makefile` 新增 `infra-check`（compose 解析、dockerignore 条目、前端健康检查单份、上下文真实断言）；在 worktree 内临时建 `.nofx-local`/`scratch` 后实测通过并清理。
- [ ] 5. 门禁：`git diff --check`、`make test`、`make build`、`make infra-check`、`make -n main-stack`。
- [ ] 6. 独立检查子智能体检查；修复并补验。
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
