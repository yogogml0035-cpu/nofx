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

## 只读探针：成交记录的下单备注（2026-11-26）

- `mytrades_text_probe.go` — 只读探针：列出 `GET /futures/usdt/my_trades` 最近 55 笔成交的 `text`（下单备注）。
- `mytrades-text-2026-11-26.jsonl` — 逐笔输出；`mytrades-text-2026-11-26.summary.txt` — 备注分布。

用途：确认仓位归属可以按下单备注判定（对抗审查复核的 P1：同步层此前把账户内全部成交记到 trader 名下）。

结果：`text` 原样带回下单备注，分布为 `t-nofx` 18、`t-nofx-close` 19、`t-nofx-partial-close` 11、
`t-nofxe2e` 2（e2e 探针）、`app` 3、`app-auto-trail-p-s8qc` 1、`ao-2125517580666933248` 1。
账户里确有其他入口的成交，所以归属必须按备注区分；AI trader 的开仓此后使用专属备注 `t-nofx-ai`。
不下单、不改任何设置；凭据经环境变量传入，编译产物不入库。

## 最终候选的真实随单保护开平（2026-11-27，W2 授权范围）

- `w2_final_livecheck.go` — 最终候选上的随单保护开平验证：预检账户空闲 → 经适配器 `OpenLongProtected`
  开 16 张 DOGE（名义 9.05 USDT，随单 SL/TP ±10%）→ `VerifyOrderProtection` 核对条件单 →
  在 `my_trades` 中按下单 ID 核对开仓成交的 `text` → `CloseLong(0)` 全平 → 终态核对。
- `w2-final-livecheck-2026-11-27.jsonl` — `LIVECHECK_PASS`；开仓成交 `trade_id=237951294`、
  `order_id=56013526833151996`、`text="t-nofx-ai"`；终态 `open_positions=0 open_orders=0 open_price_orders=0`，
  可用余额 107.37448 → 107.36383（差 −0.01065，即双边 taker 手续费）。
- `w2-final-livecheck.adapter-log.txt` — 同次运行的适配器日志。

结算层归属（同步只认 `t-nofx-ai` 的开仓、平仓只作用于已有 OPEN 记录、持仓写入失败回滚）由
`trader/gate/order_sync_ownership_test.go` 覆盖：宿主机没有 cgo，darwin 交叉编译下 store/sqlite 不可用，
这一层无法在宿主探针里跑真实库；探针改为只验证真实交易所侧的备注与执行路径。

运行方式：Docker 内 `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64` 交叉编译，宿主机运行（出口在白名单内）；
凭据经 `GATE_KEY` / `GATE_SECRET` 环境变量传入，不落盘、不打印。`w2_cleanup.go` 是失败时的立即平仓工具
（读持仓→全平→打印最近成交），本轮第一次失败后用它平掉了残留仓位。

## 最后一轮独立检查与修复复验（F68，2026-11-26）

- 独立检查在 5469912e/65263894 上报 P1（同步重复应用已存在的平仓成交，分批平仓后记录被误关）
  与 P2（对账用 0 出场价造假交易；对账先于同步时迟到平仓成交被丢弃）。
- 修复见 `cb171385`：同步按成交幂等并在持仓/成交行写入失败时回滚重放；对账新增 RECONCILED 中间态，
  迟到成交回填出场价与盈亏后转 CLOSED；同批修复账户/成交严格解析、峰值缓存符号变体、保护核对方向未知
  fail-closed、`sz_decimals` 小数语义、空静态币池但有持仓时继续管理、归档证据尾部空行。
- 复验：`trader/gate/order_sync_ownership_test.go` 新增「连跑两次不重复」「RECONCILED 不进历史/统计」
  「迟到平仓回填」用例；全量 `make test`、`make build` 通过；隔离冒烟 `smoke-cb171385/` `SMOKE_PASS`
  （24 项，与首跑 `smoke-6e04f795/` 的 PASS 一起构成首跑+重跑证据，两份 `summary.json` 入库）。

## 人工真实下单测试与分批平仓修复复验（2026-11-26，用户授权「按最低金额试仓」）

测试在 dev-offline 送测栈（`127.0.0.1:1188`，交易员 DeepSeek，Gate 子账户）上由主会话执行；
为让 10% 本金（≈10.7U）能开出真实订单，临时把策略最小仓位 12→10、仓位比例 10%→13%，
测完已恢复（min 12 / percent 10）。

- AI 真实交易：模型先 `wait`，第二轮开 BTC 2 张（`order 36028841385881417`，随单 SL/TP，
  保护核对 2 个条件单通过），下一轮再开 ETH 0.8 张（小数张，`order 63894861922056703`）；
  `my_trades` 开仓备注均为 `t-nofx-ai`。
- 首次同步只记新成交，30 秒后的重复同步 `0 new trades`，开仓记录数量/手续费不变。
- **现场发现缺陷（F69）**：BTC 记录 0.0002 BTC 分批平掉一半（0.0001）时，固定的 0.0001 数量容差
  把「剩一半」当成零，记录被误关、交易所剩余 1 张脱离 OPEN 记录。已改为按持仓规模缩放的相对容差
  （0.01%，绝对下限 1e-9），并补单测。
- 修复后复验（dev-offline `d277f0d7`，交易员停用，用 `sync_probe` 按需同步）：
  受控开 2 张（`order 36028841388027215`）→ 分批平 50%（`order 36028841388140768`）→ 同步后记录
  `OPEN 0.0001 / entry_quantity 0.0002`（正确保留）；再同步 `0 new trades` 数据不变；
  全平剩余 1 张（`order 36028841388422213`）→ 同步后 `CLOSED 0.0002`、加权出场价 60903.3、
  第二次同步 `0 new trades`。证据：`live-partial-close-2026-11-26.txt`。
- 终态：交易员 `is_running=0`，`OPEN`/`RECONCILED` 0 条，交易所无持仓；可用余额 107.33652 USDT。
- 工具：`live_close_probe.go`（open/partial/full 三种受控动作，宿主运行）、
  `sync_probe.go`（容器内按需同步 SQLite + Gate 适配器，不启动 AI/不下单）。
- 历史遗留：该交易员 `initial_balance` 为 3.46e-09（旧数据），页面总收益率显示异常大；
  不影响开仓（仓位按账户本金计算），记录为已知数据遗留。

- 最终候选（f23d8267，含 F69）复核：隔离冒烟 `smoke-f23d8267/` `SMOKE_PASS`（24 项，summary 入库）。
