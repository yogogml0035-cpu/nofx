# 真实验证证据（2026-11-24，D35 授权范围内）

全部写操作在 DOGE_USDT 上执行，单笔名义 ≤ 1 张（≈0.56 USDT），触发价远离现价 −50%/−45%，
每一步立即还原；结束后终态核查 `open trigger orders: 0 / open orders: 0`（CLEAN）。

- `livecheck.go` — 主验证脚本（挂 → 查 → 改单尝试 → 撤旧建新 → 查 → 撤 → 随单止盈止损 → 终态）
- `livecheck-2026-11-24.jsonl` — 通过运行的完整输出（逐条 JSON 读数 + 过程日志）：
  `LIVECHECK_PASS steps=10`；终态核查 `open trigger orders: 0 / open orders: 0`
  见提交后的独立核查（whoami 探针），文件内 step 10 为 `no open trigger orders left`
- `amend-boundary-probe.go` — 改单边界实验脚本（新旧路径 × text × order_id 类型，
  结论见 research H.2；实测 401 签名错误的一次为签名串漏查询段所致，修正后见 H.2 表）

运行方式：宿主机无 Go 且容器出口 IP 不在 Gate key 白名单（VPN TUN 出口），
脚本在 Docker 内交叉编译 `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64` 后于宿主机运行
（宿主机直连出口 114.94.212.18 在白名单内）。凭据经环境变量传入，不落盘。
