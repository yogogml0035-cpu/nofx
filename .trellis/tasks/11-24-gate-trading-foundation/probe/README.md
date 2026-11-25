# 父任务收口证据

## W5 真实验证（2026-11-26，D41 授权：从没有设置过仓位设置的合约上调用 `set_leverage` 并读回）

- `w5_livecheck.go` — 验证脚本（独立 main，不进构建产物）。设置操作全部调用交付的适配器代码
  （`ApplyMarginSettings`、`SetLeverage`、`SetMarginMode`、`PositionMarginSettings`），与线上同一路径；
  只读用途（账户、持仓条目、委托与条件单）使用单独的 SDK 客户端。
- `w5-livecheck-2026-11-26.jsonl` — 通过运行的完整证据：`LIVECHECK_PASS`
- `w5-livecheck-2026-11-26.adapter-log.txt` — 同一次运行里适配器的日志

运行方式与 W4 相同：在 Docker 内交叉编译 `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64` 后于宿主机运行
（宿主机直连出口在 Gate key 白名单内）。凭据经环境变量 `GATE_KEY` / `GATE_SECRET` 传入，不落盘、不打印。
编译产物不随证据入库。

```bash
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  golang:1.25-alpine sh -c 'CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o /tmp/w5livecheck \
  ./.trellis/tasks/11-24-gate-trading-foundation/probe/w5_livecheck.go'
GATE_KEY=… GATE_SECRET=… /tmp/w5livecheck -out w5-livecheck-2026-11-26.jsonl > w5-livecheck-2026-11-26.adapter-log.txt
```

## 对象与预检

账户 `position_mode=single`、`margin_mode=1`（跨币种保证金）、`in_dual_mode=false`，可用 107.39417 USDT；
全账户无未完结委托、无未触发条件单。候选合约依次为 SOL/XRP/LTC/LINK/AVAX/BCH/UNI/ADA/DOT/NEAR/APT/OP，
取第一个既无仓位设置条目（`ErrNoPositionSettings`）也无持仓条目（`POSITION_NOT_FOUND`）的合约：**SOL_USDT**
（`leverage_max=100`）。基线核对合约 BTC_USDT、DOGE_USDT、ETH_USDT（均为全仓 10x、无持仓）。

## 结果

| 步 | 操作 | 读回（`pos_margin_mode` / `leverage` / `cross_leverage_limit` / `lever` / `size`） |
|---|---|---|
| 1 | 全仓 10x（首次建条目） | `cross` / `0` / `10` / `10` / `0` |
| 2 | 逐仓 5x | `isolated` / `5` / `10` / `5` / `0` |
| 3 | `SetLeverage` 10x（保持逐仓） | `isolated` / `10` / `10` / `10` / `0` |
| 4 | `SetMarginMode` 全仓（保持 10x） | `cross` / `0` / `10` / `10` / `0` |
| 5 | 终态 | 目标条目留在全仓 10x；三个基线条目不变；无未完结委托与未触发条件单；可用 107.39417 → 107.39417 |

## 实测结论

1. **首次开新合约的路径成立**：没有仓位设置条目的合约上 `set_leverage`（全仓、逐仓与杠杆）直接返回 200，
   适配器从返回值读回一致；持仓条目随之出现，字段与 W4 的形态相同。
2. **逐仓条目的 `cross_leverage_limit` 残留上一次全仓的值**（第 2、3 步都是 `10`），与 W4 的观察一致；
   适配器以 `lever` 与同模式旧字段互相印证，不读这个残留值。
3. **留下的条目**：SOL_USDT 现为全仓 10x（授权内例外，无法还原为「没有条目」）。不下单，没有资金变动。

## 没有测到的

- 带仓或有挂单时能否修改设置（W4、W5 都要求账户空闲）。
- 单币种保证金模式下的读回（账户仍是跨币种模式）。
