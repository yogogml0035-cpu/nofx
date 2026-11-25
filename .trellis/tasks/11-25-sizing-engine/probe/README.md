# 提示词预览证据（⑨ sizing-engine）

- `prompt_preview.go`（构建标签 `probe`）用固定的模拟本金渲染中英文系统提示词。
- `sizing-prompt-zh.txt` / `sizing-prompt-en.txt` 是 2026-11-25 在候选 `d7572d84` 上渲染的留档。

渲染命令（宿主机无 Go，走 Docker）：

```bash
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  golang:1.25-alpine go run -tags probe ./.trellis/tasks/11-25-sizing-engine/probe/prompt_preview.go \
  -out-dir .trellis/tasks/11-25-sizing-engine/probe
```

注意：`kernel/schema.go` 的数据字典是按 map 迭代生成的，两次渲染的条目顺序可能不同，文件不能做字节级 golden。复验时比对关键行：

- `开仓名义价值: 10.75 USDT (= 本金 107.50 × 10.0%, 由代码计算并强制)`
- `价格到止损价的亏损: ≤ 本金的 2% (≈2.15 USDT, 代码强制, 超过即拒绝开仓)`
- `Open Position Notional: 10.75 USDT (= principal 107.50 × 10.0%, computed and enforced by code)`
- `Loss at Stop Loss: ≤ 2% of principal (≈2.15 USDT, code enforced, the decision is refused otherwise)`

两份提示词都不得出现 `position_size_usd` / `risk_usd`。