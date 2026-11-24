# 界面实测（2026-11-24，子任务 account-equity）

在隔离的容器栈里由主会话实际操作浏览器核对，前后端都运行本子任务的代码。

## 运行环境

- 镜像：从子 worktree 的代码构建（实现提交 `e73c3c06` 的内容），后端镜像与前端镜像各一个
- compose：独立的 project、容器名、数据目录与端口（前端 `127.0.0.1:1189`，后端无宿主端口、经前端 nginx 代理），
  与主栈、dev-offline 送测栈互不相干；数据库是全新的一次性 sqlite 文件；密钥是为本栈新生成的临时值
- 健康：两个容器 healthy；`http://127.0.0.1:1189/` 返回 200，经代理的 `/api/health` 返回 `{"status":"ok"}`
- 实测结束后容器、网络、镜像与临时数据、临时密钥已全部删除

## 步骤与结果

| 步 | 操作 | 结果 |
|---|---|---|
| 1 | 首次设置：注册唯一账号并登录 | 进入 AI 交易员页 |
| 2 | 新增 AI 模型（DeepSeek，占位 API Key） | 「Model config updated」 |
| 3 | 新增交易所（Gate.io，占位 Key / Secret） | 创建成功；交易所卡片显示「密钥无效」 |
| 4 | 新增策略（选 1 个市场后保存，**不激活**） | 保存成功 |
| 5 | 切换中文，打开「创建交易员」 | 提示为新口径：「创建时自动把交易所账户的当前本金（不含浮动盈亏）记为初始余额，之后可在编辑中修改」；没有初始余额输入框。见 `01-create-form-zh.png` |
| 6 | 创建 trader「界面实测-trader」（**不启动**） | 创建成功。服务端 `GET /api/traders/:id/config`：`initial_balance: 0`、`is_running: false` |
| 7 | 打开「编辑」 | 初始余额显示 `0`，下方是用途提示与告警「初始余额必须大于 0」，「保存修改」禁用。见 `02-edit-form-invalid-zh.png` |
| 8 | 初始余额依次输入 `-5`、`0`、清空 | 三种情况都保持告警、保存禁用 |
| 9 | 输入 `500.5` | 告警消失，保存可用。见 `03-edit-form-valid-zh.png` |
| 10 | 保存 | 服务端配置：`initial_balance: 500.5`、`is_running: false`、`show_in_competition: true`，其余字段不变 |
| 11 | 重新打开「编辑」 | 显示 `500.5`，无告警 |
| 12 | 切换英文，打开「Create Trader」 | 提示为「The exchange account's current principal (balance excluding unrealized P&L) is recorded as the initial balance at creation; you can change it later when editing the trader」；没有初始余额输入框 |

第 6 步对应「创建时读不到本金」的路径：占位密钥被 Gate 拒绝，创建没有被阻断，初始余额留 0。
后端日志（同一容器）：

```
⚠️ Failed to read the account principal for trader …, its initial balance will be rebuilt on load:
   failed to get account balance: failed to get wallet total balance: label: INVALID_KEY, …
failed to load trader: failed to create trader: [界面实测-trader] initial balance 0.00 is not valid
   and the account principal could not be read: …
✓ Trader created successfully
```

也就是创建路径确实调到了新的钱包总额接口，随后加载时 `resolveInitialBalance` 试图重建、读不到即按失败闭合处理。

## 接口终态（在已登录的页面里调用）

| 请求 | 响应 | 说明 |
|---|---|---|
| `GET /api/exchanges/account-state` | `status: invalid_credentials`、`error_code: INVALID_CREDENTIALS` | 余额请求失败按交易所错误归类（与「连上了但读数不符合口径」是两种状态） |
| `POST /api/traders/:id/sync-balance` | 500 `{"error":"Failed to query balance failed"}` | 读不到本金时同步失败，已保存的 `500.5` 没有被改动 |
| `PUT /api/traders/:id`，`initial_balance: -1` | 400 `{"error":"initial_balance must be greater than 0"}` | 负值被拒绝，值保持 `500.5` |
| `PUT /api/traders/:id`，不带 `initial_balance` | 200 | 值保持 `500.5` |
| `PUT /api/traders/:id`，`initial_balance: 0` | 200 | 值保持 `500.5` |

## 没有在界面里验证的

- 「创建时读到本金并记为初始余额」的成功路径：隔离栈用的是占位密钥，读不到本金。这条路径由真实读数验证的第 2 步
  （同一函数，真实密钥）与处理器级单元测试（适配器桩）覆盖。
- 印尼文界面只核对了文案键存在，没有逐屏操作。

## 顺带注意到的（不在本子任务范围，未改动）

- 编辑表单的「竞技场显示」显示为「隐藏」，而服务端的值是显示（`show_in_competition: true`）：
  `GET /api/traders/:id/config` 没有返回 `show_in_competition`，表单拿到的是空值。保存不会改动服务端的值
  （第 10 步核对过），只是显示与实际不符。
