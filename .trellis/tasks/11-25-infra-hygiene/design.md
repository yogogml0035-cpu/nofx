# 技术设计：⑰ infra-hygiene

## 边界与职责

- `.dockerignore`：条目新增（本地部署状态、草稿、`.env`、`data/`、本地工具目录）。
- `docker-compose.yml`：删除 `nofx-frontend.healthcheck`。
- `docker/Dockerfile.frontend`：`HEALTHCHECK` 探测由 `localhost` 改为 `127.0.0.1`
  （musl 下 `localhost` 解析为 `::1`，nginx 只监听 IPv4；独立检查实测主栈前端容器因此常驻 unhealthy）。
- `Makefile`：新增 `main-stack` 与 `infra-check` 目标及主栈变量；`up` 增加容器 healthy 等待。
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
2. 前置 D20：`sqlite3 -readonly -cmd ".timeout 5000" "$$DB" "SELECT COUNT(*) FROM traders WHERE is_running=1"`
   必须为 `0`；查询失败同样中止（fail-closed）。busy timeout 避免运行中后端的写锁造成偶发误判。
   镜像构建完成后、改写 compose 之前再执行一次同样的检查（构建期间旧栈仍在服务）。
3. `TMP=$(mktemp -d)` + `trap` 清理；`git -C "$$MAIN_STACK_DIR" archive "$$SHA" | tar -x -C "$$TMP"`。
4. `docker build` 后端（`--build-arg GOPROXY=https://goproxy.cn,direct`）与前端，tag 带 SHA。
5. 用锚定正则改写 compose 的 `image: nofx-local/backend:...` / `frontend:...` 两行；
   写回后分别 `grep` 断言新 tag 已写入，`docker compose -f "$$COMPOSE" config -q` 通过后再删除 `.bak`。
6. `docker compose -f "$$COMPOSE" up -d`；轮询 `http://127.0.0.1:$$PORT/api/health` 与 `/`
   （最长 120s，curl 带 `--max-time 5`）；再等待 `nofx` 与 `nofx-frontend` 两个容器都达到 `healthy`。
7. 后置 D20：DB 计数仍为 0；从 `up -d` 前的时间戳起取日志，出现
   `Auto-starting trader` / `Trader auto-started successfully` / `Auto-restoring trader runtime` 即失败。

## infra-check

- `.dockerignore` 用 `grep -qx` 校验 `.nofx-local/`、`scratch/`、`.env`、`data/`、`web/node_modules/`。
- `docker compose config -q` 校验 dev 栈；主栈 compose 存在时同样校验。worktree 里没有 `.env`
  时使用 `--no-env-resolution`，仍校验文件结构与插值语法（主栈重建用的 `main-stack` 保持完整解析）。
- 前端健康检查：用 sed 截取 `nofx-frontend:` 服务段，断言段内无 `healthcheck:`；
  并断言 `Dockerfile.frontend` 的 `HEALTHCHECK` 存在且探测 `127.0.0.1`。
- 构建上下文：临时目录拷入 `.dockerignore` 并放置 `.nofx-local/`、`scratch/`、`data/`、`.trellis/`
  与 `.env` 标记，写 `FROM alpine:latest` + `COPY . /ctx` + `RUN test ! -e ...`，以该临时目录为上下文构建，
  构建成功即证明排除生效（不依赖 worktree 是否有这些目录）；随后 `docker rmi` 清理临时镜像。
- 全部检查只读业务数据；不启动/停止容器。

## 权衡与已知边界

- 默认路径指向本机主仓，与既有 `NOFX_DEV_OVERRIDE` 风格一致；从其他 worktree 调用时用变量覆盖。
- 不改 `.nofx-local/compose.yml` 的其它内容（数据卷、env_file、depends_on 条件保持）。
- `main-stack` 会更新镜像 tag 并重建容器，属高风险动作；本轮只做 `make -n` 静态验收，
  实际执行留到父任务收口，由用户确认后运行。
- bash 3.2 在 `$var` 后紧跟全角标点时会吞掉首字节，提示文案统一用 `${var}` 显式边界。
- `docker-compose.prod.yml` / `docker-compose.stable.yml` 仍保留 compose 级前端健康检查（探测 127.0.0.1，
  可用），不在本子任务范围。
