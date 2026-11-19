# C1 执行计划

前置：在子 worktree（基于父分支最新提交）开工。**MCP 探针已在规划阶段完成**（结论见 design.md「探针结论」，原始输出 `research/` 四份：mcp_probe、news_matrix、news_tools_list、news_alternatives）；宿主机无 Go，全部 go 命令走 Docker。

## 步骤

1. [x] MCP 可达性、协议与参数矩阵探针（规划阶段完成）：`/mcp/info`、`/mcp/news` 直连 200；overview 含恐惧贪婪（字段集实证，无板块数据）；**news 参数定稿：仅传 coin 不传 query（query 全矩阵 total:0）**；市场级标题用 exchange_announcements；/mcp/news 共 17 工具实测后采用 4 个；go-sdk v1.8.0 经 goproxy.cn 可取
2. [x] `go.mod` 引入 `github.com/modelcontextprotocol/go-sdk@v1.8.0`（已锁定）；SDK 冒烟通过：`StreamableClientTransport` 结构体 → `NewClient` → `Connect(ctx, t, nil)` → `session.CallTool`；standalone SSE GET 无报错/无重连风暴（无需 DisableStandaloneSSE），Close 幂等正常。实证差异：Client 侧 Transport 无导出 Logger 字段，日志统一走 ClientOptions.Logger
3. [x] 新建 `provider/gatemcp/`：Client（两个 server 懒连接 + `GATEMCP_ENDPOINT` 覆盖 + 出错 Close 重建）+ 五个取数函数 + TTL 缓存 + singleflight 合并 + 恐惧贪婪 24h 历史环
4. [x] 裁剪视图类型 + 纯函数裁剪器（依据 research/ 真实返回；news 用 coin-only 定稿参数；时间戳 flexInt64 兼容数字/数字字符串——运行时探针实证 announcements 的 publish_time 全为数字字符串）
5. [x] `provider/gate/symbols.go`：SymbolInfo 加 `funding_rate`（小数语义、无 omitempty）+ ListSymbols 45s TTL 缓存（singleflight 合并）
6. [x] `provider/gate/liquidations.go`：聚合器 + fake puller 单测。**实施修订（运行时探针实证）**：gateapi-go v6.104.3 的 FuturesLiqOrder 模型与真实返回不兼容（left/size 均为字符串且 size 可带小数如 "-1.4"，int64 模型整批反序列化失败），改为直接 HTTP GET + rawLiqOrder 字符串字段解析；方向语义实证 size>0 = 多头被清算
7. [x] `api/handler_market.go` 六端点（502 语义）+ `api/server.go` 公开注册；`main.go` 在 server.Start() 前拉起聚合 goroutine（ctx 接 Shutdown；启动先拉一次再进 60s tick）
8. [x] 单测补齐（缓存 TTL/singleflight/标注、历史环、裁剪器+flexInt64、funding_rate 映射+无 omitempty、symbols 缓存、聚合去重/窗口/水位/重试、handler 路由/200/502 类别/400 参数/limit 钳制）——三包 -race 全绿
9. [x] Docker 门禁：CGO_ENABLED=1 `go build ./...` + `go test -count=1 ./...` 全模块 23 包全绿（trader/store 等含 sqlite 的包需 CGO；alpine 需 apk add gcc musl-dev）
10. [x] 运行时探针：宿主无 Go 且 sqlite 需 CGO，改为容器内 CGO 编译 linux/arm64 并容器化运行（端口 18090、DB 隔离）。正向：六端点全 200 真实数据，funding_rate 1033 合约全量携带（BTC_USDT=-3.82e-05 等 5 合约抽查非空）；断网点 `GATEMCP_ENDPOINT=http://127.0.0.1:9`：五个 MCP 端点 502 + `{"error":"Gate MCP upstream unreachable"}`，`/api/symbols` 与 `/api/market/liquidations` 不受影响（200）。留档 `probe/ok/` 与 `probe/broken/`

## 验证命令

```bash
docker run --rm -v "$PWD":/w -w /w -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine go build ./...
docker run --rm -v "$PWD":/w -w /w -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine go test ./...
curl -s localhost:<port>/api/market/overview | head -c 400   # 其余端点同理
```

首次运行需 `docker pull golang:1.25-alpine`（网络失败重试）。

## 回滚点

- 每步一 commit；冒烟 main 与中间产物放 gitignore 的 scratch，不进主干
- 出错时按 commit 边界手动恢复文件（禁止 git 回滚命令）
