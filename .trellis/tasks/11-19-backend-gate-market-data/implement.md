# C1 执行计划

前置：在子 worktree（基于父分支最新提交）开工；`go` 用 `~/sdk/go1.26.2/bin/go`，GOPROXY=goproxy.cn。

## 步骤

1. [ ] MCP 可达性探针：临时目录写探针 main（不进仓库），`go get github.com/modelcontextprotocol/go-sdk@<当前稳定版>`，对四个工具各发一次 CallTool，保存真实返回 JSON 到任务目录 `research/`（作为裁剪字段依据）
2. [ ] `go.mod` 引入 go-sdk（锁版本）；新建 `provider/gatemcp/`：Client（两个 server 连接）+ 四个取数函数 + TTL 缓存 + 并发合并
3. [ ] 裁剪视图类型 + 纯函数裁剪器（依据步骤 1 的真实返回）
4. [ ] `provider/gate/symbols.go`：SymbolInfo 加 `funding_rate`，映射 `ticker.FundingRate`
5. [ ] `provider/gate/liquidations.go`：聚合器（StartLiquidationAggregator + LiquidationHotspots）+ 单测
6. [ ] `api/handler_market.go` 五端点 + `api/server.go` 公开注册；main.go/服务启动处拉起聚合 goroutine
7. [ ] 单测补齐（缓存/裁剪/映射/聚合/handler 错误传递）
8. [ ] 门禁：`go build ./... && go test ./...`（GOPROXY 环境）
9. [ ] 运行时探针：临时起后端，curl 五端点 + `/api/symbols`（验 funding_rate）；断网点验 fast-fail；输出留档任务目录 `probe/`

## 验证命令

```bash
~/sdk/go1.26.2/bin/go build ./...
~/sdk/go1.26.2/bin/go test ./provider/gatemcp/... ./provider/gate/... ./api/...
curl -s localhost:<port>/api/market/overview | head -c 400   # 其余端点同理
```

## 回滚点

- 每步一 commit；探针 main 与中间产物放任务目录或 gitignore 的 scratch，不进主干
- 出错时按 commit 边界手动恢复文件（禁止 git 回滚命令）
