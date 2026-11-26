# W4 真实验证证据（2026-11-24，D35 授权范围内：闲置合约上设置全仓、逐仓与杠杆并读回）

- `livecheck.go` — 验证脚本（独立 main，不进构建产物）。设置操作全部调用交付的适配器代码
  （`ApplyMarginSettings`、`SetLeverage`、`SetMarginMode`、`PositionMarginSettings`），与线上同一路径；
  只读用途（账户、持仓条目、委托与条件单）与第 2 步对「重复设置」的原始观察使用单独的 SDK 客户端
- `livecheck-2026-11-24.jsonl` — 通过运行的完整证据（23 步 + 1 条观察记录 + 结尾标记）：`LIVECHECK_PASS`
- `livecheck-2026-11-24.adapter-log.txt` — 同一次运行里适配器的日志：8 次设置都是经「接口返回值读回一致」通过的

## 运行的代码

入库的证据是第三轮，在提交 `d758d3ec`（独立检查修复之后的候选）的干净检出上编译并运行，本地时间 19:37。
此前两轮在独立检查之前的代码上也都 `LIVECHECK_PASS`，证据没有入库（第二轮的输出留在提交 `d92328b9` 里）。
三轮之间适配器的改动是读回解析与报错方式：加了持仓模式守卫，`lever` 与同模式旧字段矛盾时改为报错，
交易所错误改为留在错误链里。三轮的设置与读回读数一致。

运行方式：宿主机无 Go 且容器出口 IP 不在 Gate key 白名单（VPN TUN 出口），脚本在 Docker 内交叉编译
`CGO_ENABLED=0 GOOS=darwin GOARCH=arm64` 后于宿主机运行（宿主机直连出口在白名单内）。
凭据经环境变量 `GATE_KEY` / `GATE_SECRET` 传入，不落盘、不打印；`-out` 把 JSONL 证据写到文件，
适配器的日志留在 stdout。

```bash
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  golang:1.25-alpine sh -c 'CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o /tmp/w4livecheck \
  ./.trellis/tasks/archive/2026-11/11-24-unified-trading-params/probe/livecheck.go'
GATE_KEY=… GATE_SECRET=… /tmp/w4livecheck -out livecheck-2026-11-24.jsonl > livecheck-2026-11-24.adapter-log.txt
```

## 对象与基线

账户：`position_mode=single`、`margin_mode=1`（跨币种保证金）、`in_dual_mode=false`，可用 107.49769 USDT。

| 合约 | 基线（`pos_margin_mode` / `leverage` / `cross_leverage_limit` / `lever`） |
|---|---|
| DOGE_USDT（W4 对象，无持仓） | `cross` / `0` / `75` / `75` |
| BTC_USDT | `cross` / `0` / `10` / `10` |
| ETH_USDT | `cross` / `0` / `200` / `200` |

预检：全账户无未完结委托、无未触发条件单，DOGE_USDT 无持仓。从未设置过的合约（SOL_USDT）读不到条目：
`PositionMarginSettings` 返回 `ErrNoPositionSettings`。

## 结果

| 步 | 操作 | 读回（`pos_margin_mode` / `leverage` / `cross_leverage_limit` / `lever`） |
|---|---|---|
| 1 | 全仓 10x | `cross` / `0` / `10` / `10` |
| 2 | 再设一次全仓 10x | 适配器读回通过；原始调用返回 200，返回体与随后的条目逐字段一致（观察记录，不计成败） |
| 3 | 全仓 5x | `cross` / `0` / `5` / `5` |
| 4 | 逐仓 5x | `isolated` / `5` / `5` / `5` |
| 5 | 逐仓 3x | `isolated` / `3` / `5` / `3` |
| 6 | 全仓 100x（超过 DOGE_USDT 上限 75） | 被交易所拒绝：`label: LEVERAGE_EXCEEDED, message: limit [1, 75]`；读回不变 |
| 6 | 杠杆 0 | 适配器在发请求前拒绝：`leverage must be at least 1, got 0` |
| 7 | `SetLeverage` 10x | `isolated` / `10` / `5` / `10`（保持逐仓） |
| 8 | `SetMarginMode` 全仓 | `cross` / `0` / `10` / `10`（保持杠杆） |
| 9 | 还原 | `cross` / `0` / `75` / `75`，与基线一致 |
| 10 | 终态 | 三个合约的条目与基线一致；可用余额 107.49769 → 107.49769；全账户无未完结委托、无未触发条件单 |

## 实测结论

1. **新接口 `set_leverage` 线上已部署**：
   `POST /futures/{settle}/positions/{contract}/set_leverage?leverage=N&margin_mode=cross|isolated` 不带 `dual_side`，
   在单向持仓模式下返回 200，返回体是 Position（带 `mode: "single"`、`pos_margin_mode`、`leverage`、
   `cross_leverage_limit`、`lever`）。文档对 `dual_side` 必填与否的自相矛盾（research A.1 与 D.1），
   以实测为准：单向持仓模式下不带。
2. **字段形态**：全仓时 `leverage="0"`，杠杆在 `cross_leverage_limit`；逐仓时 `leverage` 是杠杆，
   **`cross_leverage_limit` 残留上一次全仓的值**（第 5、7 步都是 `5`）。`lever` 在两种模式下都是当前杠杆。
3. **重复设置同一组值返回 200**，不报 `NO_CHANGE`。「接口没有确认目标设置时读回持仓条目再核对」的路径
   因此只在请求超时等异常时用到。
4. **超过合约杠杆上限被拒**，label 是 `LEVERAGE_EXCEEDED`、message `limit [1, 75]`。这个 label 不在根页的
   label 表里（表里只有 `LEVERAGE_TOO_HIGH` / `LEVERAGE_TOO_LOW`）。适配器把它原样报出并读回确认设置没变。
5. **跨币种保证金模式下逐仓可设置并读回**。`SetMarginMode` 切换模式时是适配器先读回当前杠杆、再带着它设置
   （第 8 步：逐仓 10x → 全仓 10x）；交易所自身在切换模式时是否保留杠杆没有测。
6. **适配器对 8 次 `set_leverage` 的返回值都读回一致**：适配器日志里 8 行都是「Margin settings applied」，
   没有一次走「读回持仓条目」的回退，8 次解析都没有触发「矛盾即报错」。字段级的证据是第 2 步的原始
   返回体（全仓）与各步读回的条目，这两处 `lever` 与同模式的旧字段相等；逐仓的原始返回体没有留档。
7. **返回体与持仓条目逐字段比对只在第 2 步的原始调用上做过**（一致）。其余步骤是：返回值经适配器核对
   等于目标设置，条目经脚本读回等于目标设置。

## 没有测到的

- **从未设置过的合约上的 `set_leverage`**（AI 首次开新合约的常规路径）。W4 的对象是已有条目的合约；
  实测需要在一个可弃合约上留一条无法还原为「无条目」的条目，要用户授权后再做。单元测试按「交易所会建条目」模拟。
- 单币种保证金模式下的读回（账户当前是跨币种模式）。
- 带持仓或有挂单时能否修改设置。
- 双向持仓模式下返回体的形态。

## 账户影响

只有 DOGE_USDT 的仓位设置条目被临时改动，三轮结束时都还原为运行前的全仓 75x；BTC_USDT、ETH_USDT 的条目未变。
没有下单，可用余额前后一致（107.49769 USDT），没有残留的持仓、委托或条件单。
