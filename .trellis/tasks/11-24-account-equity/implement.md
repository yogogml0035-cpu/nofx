# 执行计划：账户权益与本金（account-equity）

## 步骤

1. [x] **规划**：`prd.md`、`design.md`、本文件、`implement.jsonl` / `check.jsonl`，连同研究留档一并提交
   ——`c933a7c4`（研究留档由研究子智能体定向抓取文档站钱包分页与 Agent 文档页后落档，主会话通读核对并补规划期只读实测）
2. [x] **口径定义**：`trader/types/balance.go`——`AccountBalance`、`InitialBalance()`、`ParseAccountBalance`，
   外加两个读取函数 `ReadAccountBalance` / `ReadInitialBalance`（九处调用点共用，避免各处重复「读取 + 解析」）；
   单测 `trader/types/balance_test.go`
3. [x] **Gate 取数**：`trader/gate/trader_account.go` 的 `GetBalance` 改取钱包总额与合约账户 `available`；
   单测 `trader/gate/trader_account_test.go`（真实响应格式的 httptest）；既有套件的 mock 补钱包总额端点
4. [x] **交易路径**：决策上下文、账户信息、开仓上限、网格回撤检查与网格上下文改用 `types.ReadAccountBalance`；
   删除按键取数、权益公式与「权益退回可用余额」；初始资金的校验与重建拆成 `resolveInitialBalance` 并取本金；
   单测 `trader/auto_trader_account_test.go`（适配器桩 + 临时 sqlite 库）
5. [x] **API 与助手**：创建取本金（删除创建请求的 `initial_balance`）、更新拒绝负值、同步余额取本金与除零防护、
   账户状态改用解析函数（保留「请求失败按内容归类」）、加载失败的报错归类随新文案更新；助手创建 trader 取本金；
   删除失效的提取函数；单测 `api/handler_trader_initial_balance_test.go`、`agent/trader_initial_balance_test.go`
   （探测适配器构造函数改为可替换的变量，测试不连交易所）
   ——第 2–5 步与第 7 步在 `e73c3c06`
6. [x] **界面**：`TraderConfigModal` 编辑模式的初始资金输入与校验、创建模式提示、类型、三语文案；
   `TraderConfigModal.test.tsx`（12 个用例）；构建前后端镜像，在隔离的容器栈（`127.0.0.1:1189`）里实际操作浏览器核对，
   截图留档 `probe/ui/`，实测后隔离栈整体删除
   ——第一轮（11-24，修复前的代码）核对创建提示与编辑流程；第二轮（11-25，修复后）重做并补上换绑与「没改过就不发送」，
   共 9 张截图，见 `probe/ui/README.md`
7. [x] **文档与规格**：`docs/pnl.md` 按新口径重写；`exchange-adapters.md` 新增账户读数契约；
   `quality-guidelines.md` 新增禁止模式；索引同步
8. [x] **真实读数** `probe/livecheck.go`，带仓读数前沿用 W3 开最小测试仓位、读数后立即平仓，终态核对无残留；提示词预览
   ——四轮：试跑（只读步骤）通过；第 1 轮在第 3 步下单前被脚本自己的预检查拦下（目标名义价值漏乘张数，未下单）；
   第 2 轮开平仓完成、第 4 步两条核对因断言方式设错而失败；按实测改正断言后第 3 轮 `LIVECHECK_PASS`
   （修复前的候选 `88982a24`）；独立检查修复后，清理演练（`-cleanup-drill`，不开仓）与只读试跑通过，
   第 4 轮 `LIVECHECK_PASS`（`ed4aed64` 的干净检出，17 条事件，0 失败）。入库的证据是第 4 轮：
   `probe/livecheck-2026-11-25.*` 与 `probe/prompt-preview-2026-11-25.txt`。
   实测结论回写 design.md 与 research H 节：本金不含浮盈、手续费即时结算、钱包总额的未实现盈亏没有看到缓存滞后、
   「可用额度」公式不严格成立、合约账户 `total` 不是本金
9. [x] **门禁**：`git diff --check`、`make test`、`make build`
    ——在最终代码候选 `20a1c61c` 上（之后的提交只动任务目录与文档）：`git diff --check 230abd7d..HEAD` 干净；
    `make test` 退出码 0（Go 22 个包通过、18 个包没有测试、0 失败；前端 13 个文件 158 个用例）；`make build` 退出码 0。
    再前一次在 `ed4aed64` 上的结果相同
10. [ ] **独立检查**：trellis-check 子智能体检查，问题由主会话修复并补验
    ——首轮（候选 `cd9cfd88`）NEEDS FIX：无 P0，1 项 P1、3 项 P2、9 项 P3；变异实验 Go 56 个变体杀死 47、
    前端 15 个杀死 10。工作区未被改动，没有真实交易所调用
    - P1：编辑表单总是回传初始余额，换绑交易所后旧账户的初始余额不再被重置（本次引入的回归）
    - P2：加载失败的原因在 REST 层被文本脱敏抹掉，新加的两条按文案归类的分支走不到，对应测试是同义反复
    - P2：助手的运行时提示词（技能 JSON 与三处 Go 文案）仍按旧口径说「初始余额取账户净值、不能修改」
    - P2：验证脚本的兜底全平在失败路径上不执行（`finish()` 里的 `os.Exit` 让 defer 不跑）
    - P3：网格两处没有测试；`+Inf` 的权益被采用；创建与同步接受不正的本金；OKX 的开仓上限多算一次未实现盈亏
      （权益规则统一的副作用）；Gate 错误链与未实现盈亏来源没有钉住；前端 `parseFloat`、`min`、可访问性、新增行的格式；
      文档里说过头或不准确的表述（事件计数、「实时」、对合约账户 `total` 的解释、零值省略的说法、适配器数量）
    ——修复在 `ed4aed64`：表单只在改过时发送 + REST 与助手在换绑时重置；类型化错误 `ErrBalanceUnreadable` /
    `ErrPrincipalNotPositive` + `errors.Is` 归类；助手文案改为本金口径、取数函数去掉恒为真的 `found`；
    脚本改为 `run()` 返回码、清理在每条退出路径上先于结尾标记执行并撤遗留条件单、新增 `-cleanup-drill`；
    OKX 适配器补 `totalEquity`；其余测试缺口与文档逐项补齐
    - 收尾自查（首轮修复之后、复审之前）：助手换绑交易所成功后在回复里说明初始余额已重置（工具结果带 `initial_balance_reset`，
      中英文案，不换绑的更新不附带；`83d2dd95`）；更新接口的路由说明补上换绑时的初始余额规则与必填字段（`20a1c61c`）；
      更新处理器里一段描述修复前表单行为的注释更正
    - 复审：同一检查子智能体在新候选上复审，结果见下
11. [ ] **收口**：提交子分支 → 父 worktree 合入 → 归档子任务 → 推送父分支 → 删除子分支与子 worktree →
    同步计划文档 10.5 节与父任务的 `prd.md`、`implement.md`

## 迭代纪律

- 迭代期只跑定向测试（docker go 跑受影响的包、vitest 跑受影响的文件），不反复跑全量
- 独立检查修复期间只跑定向测试；收敛后 `make test`、`make build` 各跑一次
- 长命令放后台，等待不超过 60 秒一轮；等待期间只改文档，不动 Go 源文件

## 验证命令

宿主机没有 Go，go 命令统一经 Docker。`make` 里的 go 命令经 PATH 最前的转发脚本
`/Users/hsikey/Trading/nofx-wt/.bin/go` 转进 Docker；子 worktree 软链主仓已安装的 `web/node_modules`。

```bash
# 定向测试（迭代期）
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev && CGO_ENABLED=1 go test -count=1 ./trader/types/ ./trader/gate/ ./trader/ ./api/ ./agent/"
cd web && npx vitest run src/components/trader/TraderConfigModal.test.tsx

# 子任务门禁
make test     # go test -v ./... 与前端测试
make build    # go build -o nofx
```

真实读数脚本在 Docker 内交叉编译（`CGO_ENABLED=0 GOOS=darwin GOARCH=arm64`）后在宿主机运行，
API key 经环境变量传入，不落盘、不打印。

## 回滚点

- 提交边界即回滚单元：规划、实现与单测、界面、文档与规格、真实读数证据各自成提交；失败时手动恢复文件，
  不使用 git 回滚命令
- 带仓读数中途失败：脚本先记录基线，全平与核对不依赖前面步骤的结果；结束后读回仍有仓位或挂单时立即报告
- 界面实测用独立的 compose project、容器名、数据卷与端口，结束后整体删除，不触碰主栈与 dev-offline 送测栈
- 子任务失败不外溢：门禁未过的子分支不合入父分支
