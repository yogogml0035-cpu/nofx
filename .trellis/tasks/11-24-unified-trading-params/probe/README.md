# W4 真实验证证据（2026-11-24，D35 授权范围内：闲置合约上设置全仓、逐仓与杠杆并读回）

- `livecheck.go` — 验证脚本（独立 main，不进构建产物）。设置操作全部调用交付的适配器代码
  （`ApplyMarginSettings`、`SetLeverage`、`SetMarginMode`、`PositionMarginSettings`），与线上同一路径；
  只读用途（账户、持仓条目、挂单）与第 2 步对「重复设置」的原始响应观察使用单独的 SDK 客户端
- `livecheck-2026-11-24.jsonl` — 通过运行的完整证据（24 步 + 结尾标记）：`LIVECHECK_PASS`

运行方式：宿主机无 Go 且容器出口 IP 不在 Gate key 白名单（VPN TUN 出口），脚本在 Docker 内交叉编译
`CGO_ENABLED=0 GOOS=darwin GOARCH=arm64` 后于宿主机运行（宿主机直连出口在白名单内）。
凭据经环境变量 `GATE_KEY` / `GATE_SECRET` 传入，不落盘、不打印；`-out` 把 JSONL 证据写到文件。

```bash
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  golang:1.25-alpine sh -c 'CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o /tmp/w4livecheck \
  ./.trellis/tasks/11-24-unified-trading-params/probe/livecheck.go'
GATE_KEY=… GATE_SECRET=… /tmp/w4livecheck -out livecheck-2026-11-24.jsonl
```

## 对象与基线

账户：`position_mode=single`、`margin_mode=1`（跨币种保证金）、`in_dual_mode=false`，可用 107.49769 USDT。

| 合约 | 基线（`pos_margin_mode` / `leverage` / `cross_leverage_limit` / `lever`） |
|---|---|
| DOGE_USDT（W4 对象，无持仓） | `cross` / `0` / `75` / `75` |
| BTC_USDT | `cross` / `0` / `10` / `10` |
| ETH_USDT | `cross` / `0` / `200` / `200` |

预检：账户内无未完结委托，DOGE_USDT 无未触发条件单。从未设置过的合约（SOL_USDT）读不到条目：
`PositionMarginSettings` 返回 `ErrNoPositionSettings`。

## 结果

共运行两轮，均 `LIVECHECK_PASS`（24 步）。首轮在加「单向持仓模式」守卫之前的代码上通过；终轮在最终候选上
通过，入库的证据是终轮。

| 步 | 操作 | 读回（`pos_margin_mode` / `leverage` / `cross_leverage_limit` / `lever`） |
|---|---|---|
| 1 | 全仓 10x | `cross` / `0` / `10` / `10` |
| 2 | 再设一次全仓 10x | 交易所返回 200；读回不变 |
| 3 | 全仓 5x | `cross` / `0` / `5` / `5` |
| 4 | 逐仓 5x | `isolated` / `5` / `5` / `5` |
| 5 | 逐仓 3x | `isolated` / `3` / `5` / `3` |
| 6 | 全仓 100x（超过 DOGE_USDT 上限 75） | 被拒：`label: LEVERAGE_EXCEEDED, message: limit [1, 75]`；读回不变 |
| 6 | 杠杆 0 | 适配器在发请求前拒绝：`leverage must be at least 1, got 0` |
| 7 | `SetLeverage` 10x | `isolated` / `10` / `5` / `10`（保持逐仓） |
| 8 | `SetMarginMode` 全仓 | `cross` / `0` / `10` / `10`（保持杠杆） |
| 9 | 还原 | `cross` / `0` / `75` / `75`，与基线一致 |
| 10 | 终态 | 三个合约的条目与基线一致；无未完结委托、无条件单 |

## 实测结论

1. **新接口 `set_leverage` 线上已部署**：`POST /futures/{settle}/positions/{contract}/set_leverage?leverage=N&margin_mode=cross|isolated`
   不带 `dual_side`，在单向持仓模式下返回 200，返回体是 Position（带 `mode: "single"`、`pos_margin_mode`、
   `leverage`、`cross_leverage_limit`、`lever`）。文档对 `dual_side` 必填与否的自相矛盾（research A.1 与 D.1），
   以实测为准：单向持仓模式下不带。
2. **字段形态**：全仓时 `leverage="0"`，杠杆在 `cross_leverage_limit`；逐仓时 `leverage` 是杠杆，
   **`cross_leverage_limit` 残留上一次全仓的值**（第 5、7 步都是 `5`）。`lever` 在两种模式下都是当前杠杆。
   读回解析以 `lever` 优先、按模式取旧字段兜底，正是为了不受这个残留值影响。
3. **重复设置同一组值返回 200**，不报 `NO_CHANGE`。「接口报错后读回持仓条目再核对」的路径因此只在
   请求超时等异常时用到。
4. **超过合约杠杆上限被拒**，label 是 `LEVERAGE_EXCEEDED`、message `limit [1, 75]`。这个 label 不在根页的
   label 表里（表里只有 `LEVERAGE_TOO_HIGH` / `LEVERAGE_TOO_LOW`）。适配器把它原样报出并读回确认设置没变。
5. **跨币种保证金模式下逐仓可用**；切换模式时新接口把杠杆带过去（第 8 步：逐仓 10x → 全仓 10x）。
6. **返回值与持仓条目相互印证**：适配器对每次 `set_leverage` 的返回值做读回核对（模式与杠杆等于目标才返回成功），
   脚本再读回持仓条目（`GET positions/{contract}`）核对同一目标，24 步两处都一致；
   第 2 步还记录了原始返回体，与随后的条目逐字段一致。

## 账户影响

只有 DOGE_USDT 的仓位设置条目被临时改动，结束时还原为运行前的全仓 75x；BTC_USDT、ETH_USDT 的条目未变。
没有下单，没有资金变动，没有残留的持仓、委托或条件单。
