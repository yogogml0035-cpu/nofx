# 界面实测（2026-11-25，子任务 account-equity）

在隔离的容器栈里由主会话实际操作浏览器核对，前后端都运行本子任务的代码。
这是第二轮：第一轮（2026-11-24，独立检查修复前的代码）只覆盖了创建提示与编辑表单的基本流程；
独立检查指出「编辑表单总是回传初始余额，换绑交易所后旧账户的初始余额不再被重置」之后，表单与后端都有改动，
这一轮在修复后的代码上重做了基本流程，并补上换绑与「没改过就不发送」两条路径。本文件取代第一轮的说明。

## 运行环境

- 镜像：从子 worktree 构建，tag 是提交 `83d2dd95`（后端镜像 `120b4334f804`、前端镜像 `122bed1a261a`）。
  `83d2dd95` 之后只有一处代码改动：`api/server.go` 里更新接口的路由说明文字（见「顺带注意到的」第 2 条）。
  按包含这处改动的提交 `20a1c61c` 重建镜像（后端 `109bf1e4491a`、前端 `8751e436e0f1`）后做了一次收尾探针，见文末。
- compose：独立的 project `nofx-accteq`、容器名 `nofx-accteq-backend` 与 `nofx-accteq-frontend`、数据目录与端口
  （前端 `127.0.0.1:1189`，后端无宿主端口、经前端 nginx 代理），与主栈、dev-offline 送测栈互不相干；
  数据库是全新的一次性 sqlite 文件；密钥是为本栈新生成的临时值。
- 健康：两个容器 healthy；`http://127.0.0.1:1189/` 返回 200，经代理的 `/api/health` 返回 `{"status":"ok"}`。
- 实测结束后容器、网络、镜像、临时数据与临时密钥已全部删除。

前置数据经接口创建，不是验证对象：一个 AI 模型（DeepSeek，占位 API Key）、两个 Gate 交易所账户「账户A」「账户B」
（都是占位密钥，交易所回 `INVALID_KEY`，页面显示「密钥无效」）、一个策略「界面实测策略」（没有激活）。
交易所账户用的是占位密钥，所以本轮读不到真实的本金：凡是「以新账户的本金重建」的地方，重建都因为读不到而失败，初始余额停在 0。

## 步骤与结果

「请求体」是在页面里记录下的 XHR 请求体原文；「服务端」指 `GET /api/traders/:id/config` 的返回值。

| 步 | 操作 | 结果 |
|---|---|---|
| 1 | 首次设置：注册唯一账号并登录 | 进入 AI 交易员页 |
| 2 | 中文，打开「创建交易员」 | 提示为新口径：「创建时自动把交易所账户的当前本金（不含浮动盈亏）记为初始余额，之后可在编辑中修改」；没有初始余额输入框。见 `01-create-form-zh.png` |
| 3 | 创建 trader「界面实测-trader」（账户A，不启动） | 请求体没有 `initial_balance`。响应 201，`startup_warning`：「机器人「界面实测-trader」已经保存，但当前暂时还不能启动，原因是：系统暂时无法从交易所读取账户余额。请先检查模型、策略和交易所配置，修正后再点击启动。」服务端 `initial_balance: 0`、`is_running: false` |
| 4 | 打开「编辑」 | 初始余额显示 `0`，告警「初始余额必须大于 0」，「保存修改」禁用。输入框 `aria-invalid="true"`，`aria-describedby` 指向用途提示与告警两条，`min="0.01"`、`step="0.01"`。见 `02-edit-form-invalid-zh.png` |
| 5 | 输入 `-5` | 告警保留，保存禁用。见 `04-edit-form-negative-zh.png` |
| 6 | 清空输入框 | 告警保留，保存禁用。见 `05-edit-form-cleared-zh.png` |
| 7 | 依次输入 `0`、`-0`、`1e3`、`12.345` | 前三种都判无效（科学计数法不接受），保存禁用；`12.345` 判有效 |
| 8 | 输入 `500.5` | 告警消失，`aria-invalid="false"`，`aria-describedby` 只剩用途提示，保存可用。见 `03-edit-form-valid-zh.png` |
| 9 | 保存 | 请求体带 `"initial_balance":500.5`。服务端 `initial_balance: 500.5`，未启动 |
| 10 | 重新打开「编辑」 | 显示 `500.5`，无告警 |
| 11 | 只把扫描间隔改成 5，不动初始余额，保存 | 请求体**没有** `initial_balance`。服务端初始余额仍是 `500.5`，扫描间隔是 5 |
| 12 | 把交易所换成「账户B」，不动初始余额，保存 | 请求体带账户B 的 `exchange_id`、没有 `initial_balance`。服务端初始余额从 `500.5` 重置为 `0`，交易所是账户B。保存前的表单见 `06-edit-form-rebind-zh.png`（交易所已是账户B，初始余额仍是旧账户的 500.5） |
| 13 | 重新打开「编辑」 | 初始余额显示 `0`（重置后的值），告警，保存禁用；输入 `777` 后保存，服务端 `777` |
| 14 | 同一次编辑里把交易所换回「账户A」、初始余额填 `888.5`，保存 | 请求体同时带账户A 的 `exchange_id` 和 `"initial_balance":888.5`。服务端交易所是账户A、初始余额 `888.5`：明确给出的值生效，没有被重置。见 `07-edit-form-rebind-with-value-zh.png` |
| 15 | 英文界面：创建表单；编辑表单输入 `-5` | 创建提示：「The exchange account's current principal (balance excluding unrealized P&L) is recorded as the initial balance at creation; you can change it later when editing the trader」，没有输入框（`08-create-form-en.png`）。编辑表单告警「Initial balance must be greater than 0」，「Save Changes」禁用（`09-edit-form-invalid-en.png`） |
| 16 | 印尼文界面：创建表单；编辑表单输入 `-5` | 创建提示：「Prinsipal akun exchange saat ini (saldo tanpa PnL belum terealisasi) dicatat sebagai saldo awal saat pembuatan; dapat diubah nanti saat mengedit trader」。编辑表单告警「Saldo awal harus lebih besar dari 0」，「Simpan Perubahan」禁用。只读了页面文字，没有截图 |
| 17 | 关闭表单，核对终态 | 交易员：账户A、`initial_balance: 888.5`、未启动、`show_in_competition: true`；策略未激活；全程没有点过「启动」 |

第 3 步的 `startup_warning` 是界面创建 trader 时后端给出的失败原因，其中「系统暂时无法从交易所读取账户余额」
由 `classifyTraderSetupError` 按错误类型归类得到（独立检查的修复项之一，见 `implement.md` 第 10 步）。
界面本身不展示 `startup_warning`，这一条是在接口响应里核对的。

后端日志（同一个容器）与上面的步骤对应：

```
⚠️ No valid initial balance for trader …, it will be rebuilt from the account principal on load:
   account balance unavailable: failed to get account balance: failed to get wallet total balance: label: INVALID_KEY, …
failed to load trader: failed to create trader: [界面实测-trader] initial balance 0.00 is not valid: account balance unavailable: …
🔄 Exchange changed for trader …, resetting stale initial_balance to 0        （出现两次：第 12 步与接口核对里的换绑）
```

创建路径确实调到了钱包总额接口；加载失败的原因带着类型化的错误链，`startup_warning` 里的「系统暂时无法从交易所读取账户余额」由它归类得到。

## 接口核对（在已登录的页面里直接调用）

除特别说明外，请求体带全 `name`、`ai_model_id`、`exchange_id`、`strategy_id`，当时保存的初始余额是 `888.5`。

| 请求 | 响应 | 保存的初始余额 |
|---|---|---|
| `PUT /api/traders/:id`，`initial_balance: -1` | 400 `{"error":"initial_balance must be greater than 0"}` | 保持 `888.5` |
| 同上，`initial_balance: 0` | 200 | 保持 `888.5` |
| 同上，`initial_balance: 888.5`（原值回传） | 200 | 保持 `888.5` |
| 同上，`exchange_id` 换成账户B 并回传 `888.5` | 200 | 交易所变成账户B，初始余额重置为 `0`（回传的旧值不算给出了新值） |
| 同上，只带 `initial_balance` | 400 `{"error":"Invalid request parameters"}` | 不变（`name`、`ai_model_id`、`exchange_id` 必填） |
| 同上，`exchange_id` 换回账户A 并带 `initial_balance: 888.5` | 200 | 账户A，`888.5` |
| `POST /api/traders/:id/sync-balance` | 500 `{"error":"Failed to query balance failed"}` | 保持 `888.5` |
| `GET /api/exchanges/account-state` | 两个账户都是 `status: invalid_credentials`、`error_code: INVALID_CREDENTIALS` | — |

## 收尾探针（镜像 `20a1c61c`）

重建镜像后数据库与登录状态都还在。重新打开「编辑」，把初始余额从 `888.5` 改成 `999.5` 并保存：请求体带 `"initial_balance":999.5`，
服务端 `999.5`。再用接口做两次换绑：换到账户B 并回传 `999.5`，初始余额重置为 `0`；换回账户A 并带 `888.5`，得到账户A、`888.5`。
交易员全程未启动。

## 没有在界面里验证的

- 「创建时读到本金并记为初始余额」的成功路径，以及换绑后「加载时以新账户的本金重建」的成功路径：隔离栈用的是占位密钥，读不到本金。
  这两条路径由真实读数的第 2 步（同一个函数，真实密钥）与处理器级单元测试（适配器桩）覆盖。
- 助手路径（在对话里让助手创建 trader、换绑交易所）：需要真实的模型调用，界面实测不经过它；
  由单元测试覆盖（`agent/trader_initial_balance_test.go`，含换绑后回复里的重置说明）。
- 印尼文界面只读了页面文字，没有截图。
- 自动化工具的限制：数字输入框里无法键入字母，所以没有测「输入非数字字符」（用户在浏览器里也敲不进去）。

## 顺带注意到的（不在本子任务范围）

1. 编辑表单的「竞技场显示」显示为「隐藏」，而服务端的值是显示（`GET /api/my-traders` 的 `show_in_competition: true`）：
   `GET /api/traders/:id/config` 没有返回这个字段，表单拿到的是空值。表单保存时不发送这个字段，所以不会改动服务端的值
   （实测前后都是 `true`），只是显示与实际不符。没有改动。
2. `PUT /api/traders/:id` 的 `name`、`ai_model_id`、`exchange_id` 是必填字段，路由说明原先写的是「只带要改的字段」，与实际不符。
   本子任务改这条说明时一并更正了。
3. 同步余额失败时返回给用户的文案是「Failed to query balance failed」，不通顺。没有改动。
