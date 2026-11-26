# 执行计划：⑪ protective-order-management

> 归档后回填（2026-11-26）：下列待办在交付时均已完成，原文件没有回填勾选。完成证据见计划文档 10.5 第 11 行、本任务的 probe/ 与父分支的合并提交。

## 待办（实施顺序，完成即勾选）

- [x] 1. `ContractSpec` 增加价格步长 `order_price_round` 与 `AlignTriggerPrice`（收紧/远离方向、
  边界、精度）；更新构造调用点与单测矩阵。
- [x] 2. 开仓路径对齐 SL/TP 并允许返回实际价格；`VerifyOrderProtection` 改为以未触发条件单
  列表为准（同侧重试、持仓已平视为无需保护）；单测覆盖静默未创建、重试、已平仓。
- [x] 3. `trader/types`：`ProtectionKind` / `PositionProtection` / `ProtectionUpdateRequest` /
  `ProtectionUpdateResult` / `ProtectiveOrderManager`；`ProtectedOpenResult` 加实际止损止盈。
- [x] 4. `gate` 适配器：保护读取（列表归类、多条取最近、未知原因）；改单执行（持仓/本金/
  2% 上限、对齐、先挂新核对、撤旧核对、失败恢复）与单测。
- [x] 5. `kernel`：Decision 动作、`DecisionSizing.Positions`、校验矩阵（未知保护、方向、
  2% 上限、字段匹配、无持仓）、`PositionInfo` 字段。
- [x] 6. 交易循环：每轮读取保护写入持仓信息；执行路径分发与 fail-closed；开仓记录实际价格。
- [x] 7. 提示词中英：动作清单、字段说明、示例、持仓行当前 SL/TP 与未知标记。
- [x] 8. 前端决策卡片动作映射与新价格展示、用例。
- [x] 9. 架构文档中英、规范场景、父任务计划文档（F44/F45、10.5 进度）同步。
- [x] 10. 定向测试：对齐矩阵、读取归类、改单路径、校验矩阵、提示词、前端。
- [x] 11. 真实 W2（DOGE 17 张：开仓 → 分批 → 改止损 → 改止盈 → 全平零残留）与
  W3 级 ETH 0.1 张（开仓列表核对 → 改止损 → 全平）；证据留档 `probe/`。
- [x] 12. 子任务门禁：`git diff --check`、`make test`、`make build`。
- [x] 13. 独立检查子智能体检查（范围、需求、规范、候选、验收规则），修复问题并定向补验。
- [x] 14. 合父、归档、推送父分支、清理子分支与 worktree；同步子任务地图与进度。

## 验证命令

```bash
# 定向（迭代期）
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev && CGO_ENABLED=1 go test -count=1 ./kernel/ ./trader/ ./trader/gate/ ./trader/types/"

# 真实探针/验收（宿主机，凭据经环境变量，不落盘）
docker run --rm -v "$PWD":/w -v /tmp:/hosttmp -w /w -v nofx-gocache:/go \
  -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine sh -c \
  'CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o /hosttmp/protective-livecheck \
   ./.trellis/tasks/11-25-protective-order-management/probe/livecheck.go'
GATE_KEY=… GATE_SECRET=… /tmp/protective-livecheck <out.jsonl> > <out.log>

# 门禁
git diff --check
make test
make build
```

## 审查门禁

- 保护不变量：任何代码路径都不得让持仓在无同类型保护单的情况下停留；改单失败可恢复。
- fail-closed：能力缺失、保护未知、持仓缺失、上限越界、对齐越界都不发请求。
- 无兼容分支：不保留“旧价格未对齐也不报错”的放行路径、不以委托 `tpsl_*` 字段单独判定保护。
- 数值精确：对齐与 2% 上限用十进制/有理数口径，容差只吸收浮点噪声。

## 回滚点

- 回滚单元是本子任务的提交边界；不使用 git 回滚命令。
- 未通过门禁的子分支不合入父分支，父分支保持已验证状态。
- 真实写操作失败时先还原（全平、撤条件单），再修复；每步完成后核对终态零残留。

## 证据

- `probe/explore.go` + `probe/probe-2026-11-25.jsonl`：探索探针的脚本与逐步证据
  （未对齐价格不建单、对齐后列表出现 2 条、撤销、仓位级撤旧建新；运行输出为
  `LIVECHECK_PASS`，日志未另存文件，结论见 jsonl 与 plan F43/F44）。
- `probe/trigger.go` + `probe/trigger-2026-11-25.log`：随单保护 reduce-only、分批后按剩余
  仓位收口、无反向持仓（触发探针的完整 stdout）。
- `probe/livecheck.go` + `probe/livecheck-2026-11-25.{log,jsonl}`：最终候选上的 W2/W3 验收
  （`LIVECHECK_PASS`，终态零残留）。