# C1 执行计划

前置：在子 worktree（基于父分支最新提交）开工。**MCP 探针与 go-sdk 验证已在规划阶段完成**（结论见 design.md「探针结论」、原始输出 `research/mcp_probe_output.txt`）；宿主机无 Go，全部 go 命令走 Docker。

## 步骤

1. [x] MCP 可达性与协议探针（规划阶段完成）：`/mcp/info`、`/mcp/news` 直连 200；overview 含恐惧贪婪；news search 参数待调优；go-sdk v1.8.0 经 goproxy.cn 可取
2. [ ] `go.mod` 引入 `github.com/modelcontextprotocol/go-sdk@v1.8.0`；SDK 形态冒烟：Docker 内跑最小 main 对 `/mcp/info` initialize+tools/call 各一次（确认 v1.8.0 Go API 封装形态，探针脚本为裸 JSON-RPC）
3. [ ] 新建 `provider/gatemcp/`：Client（两个 server 懒连接）+ 四个取数函数 + TTL 缓存 + 并发合并
4. [ ] 裁剪视图类型 + 纯函数裁剪器（依据 research/mcp_probe_output.txt 真实返回；news 参数在此步调优）
5. [ ] `provider/gate/symbols.go`：SymbolInfo 加 `funding_rate`，映射 `ticker.FundingRate`
6. [ ] `provider/gate/liquidations.go`：聚合器（StartLiquidationAggregator + LiquidationHotspots）+ 单测
7. [ ] `api/handler_market.go` 五端点 + `api/server.go` 公开注册；服务启动处拉起聚合 goroutine
8. [ ] 单测补齐（缓存/裁剪/映射/聚合/handler 错误传递）
9. [ ] Docker 门禁：build + test 全绿
10. [ ] 运行时探针：临时起后端，curl 五端点 + `/api/symbols`（验 funding_rate）；断网点验 fast-fail；输出留档任务目录 `probe/`

## 验证命令

```bash
docker run --rm -v "$PWD":/w -w /w -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine go build ./...
docker run --rm -v "$PWD":/w -w /w -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine go test ./...
curl -s localhost:<port>/api/market/overview | head -c 400   # 其余端点同理
```

## 回滚点

- 每步一 commit；冒烟 main 与中间产物放 gitignore 的 scratch，不进主干
- 出错时按 commit 边界手动恢复文件（禁止 git 回滚命令）
