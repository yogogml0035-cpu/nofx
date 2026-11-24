# 子任务 5：杠杆与保证金模式（unified-trading-params）

## Goal

按 trader 界面的设置生效全仓或逐仓与杠杆，使用文档的新接口
（`POST /futures/{settle}/positions/{contract}/set_leverage`：`leverage` + `margin_mode`）；
删除旧的「恒为逐仓」的 `SetLeverage` 与「空操作」的 `SetMarginMode`。单测 + W4：闲置合约上真实设置并读回，
结束后还原原设置。

## Requirements

父任务：`11-24-gate-trading-foundation`（Gate 交易基座）。需求与决策以父分支
`docs/plans/2026-11-24-gate-trading-foundation.zh-CN.md` 为准；本文件是该子任务在计划文档第 10.2 节第 5 行的落地。
依赖子任务 1（SDK v7.2.184）与子任务 4（随单保护开仓），两者已合入父分支。

### 背景

- 现状（计划文档第 3.4 节、F16）：`SetLeverage` 走旧接口 `positions/{contract}/leverage` 且只传非零杠杆，
  结果恒为逐仓；`SetMarginMode` 是空操作，只打一行日志。用户在 trader 界面选了全仓也不生效。
- 两个方法分开调用时，保证金模式与杠杆各缺一半信息；AI 开仓路径对杠杆设置失败只告警不阻断。
- 实测（2026-11-24，只读）：本账户 `position_mode=single`、`margin_mode=1`（跨币种）。子任务 4 的 W2/W3
  开仓传的是杠杆 0、没有调用杠杆设置，之后 DOGE_USDT、ETH_USDT 的条目是全仓 + 该合约的最大杠杆
  （75x、200x）；BTC_USDT 是全仓 10x。也就是说不显式设置，首次建仓的合约在本账户下会落到全仓最大杠杆，
  这是本子任务要消除的风险。
- `GET /futures/{settle}/get_leverage/{contract}` 在单向持仓模式下必须同时带 `pos_margin_mode` 与
  `dual_side` 才返回 200，返回值三个合约都是 `10`，与持仓条目的 `lever`（75、200）不一致，不能用于读回。
  读回以 `set_leverage` 的返回值与持仓条目（`GET /futures/{settle}/positions/{contract}`）为准。

### 范围

1. **一次性生效**：新增能力接口，按「全仓 / 逐仓 + 杠杆」一次调用生效，调用后用接口返回值读回核对；
   不一致或被交易所拒绝即报错。
2. **接入 AI 开仓**：随单保护开仓在提交订单前按 trader 的设置生效保证金模式与杠杆；生效失败则不开仓。
   保证金模式取 `AutoTraderConfig.IsCrossMargin`（trader 界面的全仓 / 逐仓开关），杠杆取决策的杠杆。
3. **接入网格初始化**：网格开跑前按 trader 的设置生效；适配器支持一次性生效时，失败即终止初始化。
4. **重写两个旧方法**（`SetLeverage`、`SetMarginMode`）：不再恒为逐仓、不再空操作；语义见 design.md。
5. **删除旧语义**：旧的 `positions/{contract}/leverage` 调用、`RISK_LIMIT_EXCEEDED` 被吞成成功的分支、
   AI 开仓路径里的独立 `SetMarginMode` 调用与其「提示性实现」注释、开仓路径里「尽力而为、失败不阻断」的
   杠杆设置及相关测试桩。

### 约束

- 适配层只做「外部接口 ↔ 规范单位」的换算与校验，不含交易判断（父任务 design.md）
- 外部数据解析失败即报错，不回落猜测值（父任务 design.md 约定 3）
- 使用文档的新接口；账户的持仓模式保持单向（`single`），本子任务不改持仓模式
- NOFX 的修改对单币种与跨币种两种保证金模式都适用，代码不依赖账户的 `margin_mode`（D21）。
  账户当前仍是跨币种模式；切回单币种是账户级操作（当前 API key 没有 unified 权限），由用户在交易所侧完成
- 不启动 trader、不激活策略（D20）；W4 是授权范围内的真实写操作（D35）
- 其他交易所的适配器保持 `types.Trader` 的两个方法与现有行为（D3）

## Acceptance Criteria

- [x] 一次性生效：全仓 / 逐仓 × 杠杆 → 请求路径 `positions/{contract}/set_leverage`、
      参数 `leverage` 与 `margin_mode`、不带 `dual_side`；返回值读回一致才算成功，
      不一致、交易所拒绝、杠杆非正都报错
      ——`trader/gate/margin_settings_test.go` 逐项覆盖；W4 在真实接口上读回一致
- [x] 失败不开仓：设置生效失败或读回不一致时，随单保护开仓返回错误且没有提交订单
      ——`trader/gate/protected_open_test.go`；普通市价原语在调用方给了杠杆时同样失败即不开仓
- [x] 接入：AI 开仓与网格初始化都按 trader 的设置生效；旧的独立 `SetMarginMode` 调用、
      「尽力而为」的杠杆设置与 `RISK_LIMIT_EXCEEDED` 分支已删除
      ——`trader/auto_trader_orders_test.go`、`trader/auto_trader_grid_margin_test.go`；网格在设置生效后才标记已初始化
- [x] 两个旧方法按新语义工作：`SetLeverage` 保持该合约当前的保证金模式、只改杠杆；
      `SetMarginMode` 保持当前生效杠杆、只改模式
      ——单测覆盖三种起始状态；W4 第 7、8 步在真实接口上读回一致
- [ ] 门禁：`git diff --check`、`make test`、`make build` 全部通过
      ——第一次全量门禁在独立检查修复之前的代码上通过（后端 22 个含测试的包、前端 11 个文件 142 个用例；
      `make build` 通过）。修复之后定向测试、vet、`git diff --check` 通过；最终候选上的全量门禁
      在独立检查复核通过后执行，结果记在 implement.md 第 7 步
- [x] W4 真实验证：在一个未持仓的合约上设置全仓、逐仓与杠杆，从设置接口的返回值与持仓条目读回，
      结束后还原原设置并读回一致；其他合约的设置条目不变；全程不产生持仓与挂单。证据留档 `probe/`
      ——`LIVECHECK_PASS`（23 步 + 1 条观察记录），在最终候选 `d758d3ec` 上运行，
      `probe/livecheck-2026-11-24.jsonl` 与适配器日志；DOGE_USDT 还原为全仓 75x，BTC_USDT、ETH_USDT
      条目不变，可用余额前后一致

### 真实验证对交易所行为的实测结论（2026-11-24）

详见 `research/gate-leverage-margin-docs.md` G 节与 `probe/README.md`：

- `set_leverage` 线上已部署；单向持仓模式下不带 `dual_side` 返回 200
- 逐仓时条目的 `cross_leverage_limit` 残留上一次全仓的值，`lever` 才是两种模式通用的当前杠杆。
  读回解析用 `lever` 与同模式的旧字段互相印证：只有一个能读时用它，都能读时必须相等。
  真实响应里两者一致（适配器日志里 8 次设置都是经返回值读回通过的）
- 重复设置同一组值返回 200；超过合约上限被拒的 label 是 `LEVERAGE_EXCEEDED`（不在文档的 label 表里）
- 账户处于跨币种保证金模式时逐仓可用。单币种模式下的读回、带仓或有挂单时能否修改设置，
  以及从未设置过的合约上的 `set_leverage`，都没有在真实接口上测（见 design.md 已知边界）

## Notes

- 文档原文留档：`research/gate-leverage-margin-docs.md`（文档站 Futures 分页 / 根页 changelog / Agent 文档页的中文原文，
  与官方 SDK v7.2.184 的英文定义对照）
- 读回仓位的实际杠杆（`lever`）供回撤监控使用，属于子任务 6 `drawdown-monitor`
