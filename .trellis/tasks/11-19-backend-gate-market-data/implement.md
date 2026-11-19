# C1 执行计划

前置：在子 worktree（基于父分支最新提交）开工。**MCP 探针已在规划阶段完成**（结论见 design.md「探针结论」，原始输出 `research/` 四份：mcp_probe、news_matrix、news_tools_list、news_alternatives）；宿主机无 Go，全部 go 命令走 Docker。

## 步骤

1. [x] MCP 可达性、协议与参数矩阵探针（规划阶段完成）：`/mcp/info`、`/mcp/news` 直连 200；overview 含恐惧贪婪（字段集实证，无板块数据）；**news 参数定稿：仅传 coin 不传 query（query 全矩阵 total:0）**；市场级标题用 exchange_announcements；/mcp/news 共 17 工具实测后采用 4 个；go-sdk v1.8.0 经 goproxy.cn 可取
2. [ ] `go.mod` 引入 `github.com/modelcontextprotocol/go-sdk@v1.8.0`；SDK 形态冒烟（Docker 内最小 main 对 /mcp/info 调一次）：v1.8.0 真实 API（`StreamableClientTransport` 结构体 → `NewClient` → `Connect` → `session.CallTool`）；**验证 standalone SSE GET 行为（405/重连风暴则 `DisableStandaloneSSE:true`）与 session Close/重建无泄漏**
3. [ ] 新建 `provider/gatemcp/`：Client（两个 server 懒连接 + `GATEMCP_ENDPOINT` 覆盖）+ 五个取数函数 + TTL 缓存 + singleflight 合并 + 恐惧贪婪 24h 历史环
4. [ ] 裁剪视图类型 + 纯函数裁剪器（依据 research/ 真实返回；news 用 coin-only 定稿参数）
5. [ ] `provider/gate/symbols.go`：SymbolInfo 加 `funding_rate`（小数语义、无 omitempty）+ ListSymbols TTL 缓存
6. [ ] `provider/gate/liquidations.go`：聚合器（ListLiquidatedOrders + From 水位重叠 + 方向拆分 + 去重键）+ fake puller 单测
7. [ ] `api/handler_market.go` 六端点（502 语义）+ `api/server.go` 公开注册；`main.go` 在 server.Start() 前拉起聚合 goroutine（ctx 接 Shutdown；启动先拉一次再进 60s tick）
8. [ ] 单测补齐（缓存/历史环/裁剪/映射/聚合/handler 502 传递）
9. [ ] Docker 门禁：build + test 全绿
10. [ ] 运行时探针：Docker 内交叉编译 `GOOS=darwin GOARCH=arm64 go build -o scratch/nofx-bin .` 后宿主起临时后端，curl 六端点 + `/api/symbols`（验 funding_rate 抽查 5 合约）；断网点：`GATEMCP_ENDPOINT=http://127.0.0.1:9` 重起复测六端点 502；输出留档任务目录 `probe/`

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
