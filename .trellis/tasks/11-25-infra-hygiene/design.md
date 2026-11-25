# 技术设计：⑰ infra-hygiene

## 边界与职责

- `.dockerignore`：条目新增。
- `docker-compose.yml`：删除 `nofx-frontend.healthcheck`。
- `Makefile`：新增 `main-stack` 与 `infra-check` 目标及主栈变量；不改既有 test/build/up 目标。
- 不触碰业务代码与 dev-offline override。

## main-stack

变量（默认值面向本机主仓；可在命令行覆盖）：

```make
MAIN_STACK_DIR ?= /Users/hsikey/Trading/nofx
MAIN_STACK_COMPOSE ?= $(MAIN_STACK_DIR)/.nofx-local/compose.yml
MAIN_STACK_DATA ?= $(MAIN_STACK_DIR)/.nofx-local/candidate-data/data.db
MAIN_STACK_FRONTEND_PORT ?= 8888
```

流程（`set -e` 单 shell，失败即停）：

1. `SHA=$${MAIN_STACK_SHA:-$$(git -C "$$MAIN_STACK_DIR" rev-parse --short HEAD)}`；
   `git cat-file -e "$$SHA^{commit}"`。
2. 前置 D20：`sqlite3 -readonly "$$DB" "SELECT COUNT(*) FROM traders WHERE is_running=1"` 必须为 `0`；
   查询失败同样中止（fail-closed）。
3. `TMP=$(mktemp -d)` + `trap` 清理；`git -C "$$MAIN_STACK_DIR" archive "$$SHA" | tar -x -C "$$TMP"`。
4. `docker build` 后端（`--build-arg GOPROXY=https://goproxy.cn,direct`）与前端，tag 带 SHA。
5. 用锚定正则改写 compose 的 `image: nofx-local/backend:...` / `frontend:...` 两行；
   写回后 `docker compose -f "$$COMPOSE" config -q` 验证。
6. `docker compose -f "$$COMPOSE" up -d`；轮询 `http://127.0.0.1:$$PORT/api/health` 与 `/`（最长 120s）。
7. 后置 D20：DB 计数仍为 0；`docker compose logs --since 2m nofx | grep "✅ Running"` 无命中。

## infra-check

- `.dockerignore` 用 `grep -qx` 校验 `.nofx-local/`、`scratch/`、`web/node_modules/`。
- `docker compose config -q` 校验 dev 栈；主栈 compose 存在时同样校验。worktree 里没有 `.env`
  时使用 `--no-env-resolution`，仍校验文件结构与插值语法（主栈重建用的 `main-stack` 保持完整解析）。
- 前端健康检查：用 awk 截取 `nofx-frontend:` 服务段，断言段内无 `healthcheck:`。
- 构建上下文：临时目录写 `FROM alpine:3.20` + `COPY . /ctx` +
  `RUN test ! -e /ctx/.nofx-local && test ! -e /ctx/scratch`，以仓库根为上下文构建，
  构建成功即证明排除生效；随后 `docker rmi` 清理临时镜像。
- 全部检查只读业务数据；不启动/停止容器。

## 权衡与已知边界

- 默认路径指向本机主仓，与既有 `NOFX_DEV_OVERRIDE` 风格一致；从其他 worktree 调用时用变量覆盖。
- 不改 `.nofx-local/compose.yml` 的其它内容（数据卷、env_file、depends_on 条件保持）。
- `main-stack` 会更新镜像 tag 并重建容器，属高风险动作；本轮只做 `make -n` 静态验收，
  实际执行留到父任务收口，由用户确认后运行。
