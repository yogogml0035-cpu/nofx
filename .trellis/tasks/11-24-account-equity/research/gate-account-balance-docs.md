# Research: Gate 账户总资产文档原文（`GET /wallet/total_balance`）

- Query: 摘录 Gate「查询账户总资产 GET /wallet/total_balance」与 AccountBalance 模型的官方原文，并与官方 Go SDK v7.2.184 对照；摘录 Agent 文档页里账户、资产、余额、权益、保证金、持仓相关的工具说明
- Scope: external
- Date: 2026-11-24
- 性质: 只做原文摘录与对照。标注「原文」的是逐字引用；标注「归纳」的是整理者的概括，两者分开写。不含实现建议。

## 0. 取数过程与实际使用的地址

| 目标 | 实际使用的地址 | 结果 |
| --- | --- | --- |
| 文档站钱包分页 | `https://www.gate.com/docs/developers/apiv4/zh_CN/wallet` | 成功，第 1 次调用即返回 |
| 文档站根页 | `https://www.gate.com/docs/developers/apiv4/zh_CN/` | **未抓取**，原因见下 |
| Agent 文档页 | `https://www.gate.com/docs/developers/agent/zh_CN` | 成功，第 1 次调用即返回，无需重试 |
| SDK `WalletApi.md` | `https://raw.githubusercontent.com/gate/gateapi-go/v7.2.184/docs/WalletApi.md` | HTTP 200 |
| SDK `docs/model_account_balance.md`（任务给的地址） | 同上目录 | **HTTP 404**。实际文件名是 `docs/AccountBalance.md`（HTTP 200） |
| SDK `docs/TotalBalance.md` | 同上目录 | HTTP 200 |
| SDK `model_account_balance.go`、`model_total_balance.go` | 同 tag 根目录 | HTTP 200 |

取数说明（归纳）:

- `mcp__web_reader__webReader` 在本会话里已经是已加载工具，`ToolSearch select:...` 返回「No matching deferred tools found」，直接调用即可。参数为 `return_format: markdown`、`no_cache: true`、`retain_images: false`、超时 60 秒。
- 本机直连确认不可达：`curl` 返回 `curl: (35) Recv failure: Connection reset by peer`；`WebFetch` 抓根页返回 `fetch failed: read ECONNRESET`。
- **根页未抓取**。任务给出的回退条件之一「内容里没有『查询账户总资产』」在字面上成立（页面上的标题是「查询个人账户总额」），但同一路径 `GET /wallet/total_balance` 的接口原文已在钱包分页取到，并且与官方 SDK 逐项一致（见 D 节）。任务同时备注根页单次返回约数百万 token，直接抓取有撑爆上下文的风险，所以本次没有再抓根页。如果仍需根页内容（例如版本更新记录，或确认全站是否有「查询 Sub 账户总资产」），需要另行指派。
- 另用 `WebSearch` 做了两次快照探测，返回的摘要都只指向「Query personal account totals / 查询个人账户总额」，没有出现「查询账户总资产」标题或带 `time`/`value` 的结构。搜索摘要不是原文，仅作旁证。

### 先说结论（归纳）

1. 文档站钱包分页上，`GET /wallet/total_balance` 的标题是**「查询个人账户总额」**，响应模型是 **`TotalBalance`**，只有 `total`、`details` 两个属性。任务里预期的「查询账户总资产」标题、带 `time`/`value` 的顶层 `AccountBalance` 模型、`AccountBalanceTotal`/`AccountBalanceDetails`、「查询 Sub 账户总资产」，在钱包分页和官方 SDK 里都不存在（证据见 C、D、F 节）。
2. 文档站与 Go SDK v7.2.184 对这个接口的定义一致，没有实质出入。
3. 文档**没有写明** `total.amount` 是否包含未实现盈亏、是否扣除借款，也没有写 `details` 与 `total` 的加总关系。

---

## A. 「查询个人账户总额」`GET /wallet/total_balance` 全文

- 来源: 文档站钱包分页
- 性质: 文档站原文（阅读器返回的 markdown，逐字）

### A.1 接口标题、说明正文、参数、返回

```markdown
## # 查询个人账户总额🔒 需要认证

GET`/wallet/total_balance`

GET `/wallet/total_balance`

_查询个人账户总额_

该查询接口返回的是各账户里所有币按照传入币种折算之后的总_估值_，折算价以及相关各账户的余额信息最长会有1分钟的缓存，
不推荐在即时计算时使用该接口的数据。

即时计算可根据账户类型查询对应的余额接口，如：

- `GET /spot/accounts` 查询现货账户
- `GET /margin/accounts` 查询杠杆账户
- `GET /futures/{settle}/accounts` 查询合约账户

### 参数

参数

| 名称 | 位置 | 类型 | 必选 | 描述 |
| --- | --- | --- | --- | --- |
| currency | 请求参数 | string | 否 | 用于统计换算的目标货币类型，可接受BTC，CNY，USD，USDT四个值。USDT是默认值 |

#### # 详细描述

__currency__: 用于统计换算的目标货币类型，可接受BTC，CNY，USD，USDT四个值。USDT是默认值

### 返回

返回

| 状态码 | 含义 | 描述 | 格式 |
| --- | --- | --- | --- |
| 200 | OK (opens new window) | 请求有效且成功返回 | TotalBalance |

### 返回格式

状态码 __200__

用户总资产信息

| 类型 | 约束 |
| --- | --- |
| TotalBalance |  |

__属性__

| 名称 | 类型 | 必填 | 约束 | 描述 |
| --- | --- | --- | --- | --- |
| total | object | Optional |  | 换算成目标币种的账户总额 |
| details | object | Optional |  | 各账户总额 - cross_margin: 全仓杠杆账户 - spot: 现货账户 - finance: 金融账户 - margin: 杠杆账户 - quant: 量化账户 - futures: 永续合约账户 - delivery: 交割合约账户 - warrant: warrant 账户 - cbbc: 牛熊证账户 - meme_box: alpha账户 - options: 期权账户 - payment: 支付账户 |

__total__

换算成目标币种的账户总额

| 类型 | 约束 |
| --- | --- |
| object |  |

__属性__

| 名称 | 类型 | 必填 | 约束 | 描述 |
| --- | --- | --- | --- | --- |
| amount | string | Optional |  | 账户总额数字 |
| currency | string | Optional | enum: ["BTC","CNY","USD","USDT"] | 币种 |
| unrealised_pnl | string | Optional |  | 未实现盈亏总和,这个字段只会在futures,options,delivery,total 账户中出现 |
| borrowed | string | Optional |  | 杠杆借贷总和,这个字段只会在margin,cross_margin账户中出现 |

__details__

各账户总额

- cross_margin: 全仓杠杆账户
- spot: 现货账户
- finance: 金融账户
- margin: 杠杆账户
- quant: 量化账户
- futures: 永续合约账户
- delivery: 交割合约账户
- warrant: warrant 账户
- cbbc: 牛熊证账户
- meme_box: alpha账户
- options: 期权账户
- payment: 支付账户

| 类型 | 约束 |
| --- | --- |
| object |  |

__details / additionalProperties__

换算成目标币种的账户总额

| 类型 | 约束 |
| --- | --- |
| object |  |

__属性__

| 名称 | 类型 | 必填 | 约束 | 描述 |
| --- | --- | --- | --- | --- |
| amount | string | Optional |  | 账户总额数字 |
| currency | string | Optional | enum: ["BTC","CNY","USD","USDT"] | 币种 |
| unrealised_pnl | string | Optional |  | 未实现盈亏总和,这个字段只会在futures,options,delivery,total 账户中出现 |
| borrowed | string | Optional |  | 杠杆借贷总和,这个字段只会在margin,cross_margin账户中出现 |

WARNING

该请求需要 API key 和 secret 认证
```

页面底部「模型」区的 `## # TotalBalance` 一节重复了同样的两个属性和 `__total__` 的四个子字段，这里不再重复抄录。

### A.2 逐项说明

| 项 | 结果 | 性质 |
| --- | --- | --- |
| 接口标题 | 「查询个人账户总额」，不是「查询账户总资产」 | 文档站原文 |
| 方法与路径 | `GET /wallet/total_balance` | 文档站原文 |
| 认证 | 「该请求需要 API key 和 secret 认证」 | 文档站原文 |
| 缓存注意事项 | 「折算价以及相关各账户的余额信息最长会有1分钟的缓存，不推荐在即时计算时使用该接口的数据。」 | 文档站原文 |
| 请求参数 | 只有 `currency`，`string`，必选「否」 | 文档站原文 |
| 限频 | **未取到**。阅读器返回的这个接口内容里没有任何限频字样 | 未找到 |
| 响应示例 JSON | **未取到**。阅读器返回内容里这个接口没有响应示例块 | 未找到 |
| 折叠字段 | 没有出现「展开剩余 N 个字段」之类的字样。`TotalBalance` 列出 2 个属性，嵌套对象列出 4 个属性，即阅读器返回的全部 | 文档站原文 |
| `time` 字段 | 页面上不存在 | 未找到 |
| `value` 字段 | 页面上不存在 | 未找到 |

关于响应示例的推测（归纳，未证实）: 钱包分页上所有接口都没有示例块，可能是示例由页面脚本动态加载，阅读器没有捕获到。

### A.3 任务点名的字段对照（归纳）

| 任务要的字段 | 页面是否存在 | 页面原文描述 |
| --- | --- | --- |
| `time` | 否 | — |
| `value` | 否 | — |
| `total` | 是，`object` | 换算成目标币种的账户总额 |
| `total.amount` | 是，`string` | 账户总额数字 |
| `total.currency` | 是，`string`，enum `BTC`/`CNY`/`USD`/`USDT` | 币种 |
| `total.unrealised_pnl` | 是，`string` | 未实现盈亏总和,这个字段只会在futures,options,delivery,total 账户中出现 |
| `total.borrowed` | 是，`string` | 杠杆借贷总和,这个字段只会在margin,cross_margin账户中出现 |
| `details` | 是，`object` | 各账户总额，后接 12 个键的清单 |
| `details.<业务线>.*` | 与 `total` 相同的 4 个子字段（`additionalProperties`） | 同上 |
| `details.account_futures` | **未列出** | — |
| `details.unified` | **未列出** | — |

---

## B. 钱包分页上与关键词有关的全部句子

- 来源: 文档站钱包分页（阅读器返回的页面全文，逐段人工通读核对，不是程序化 grep）
- 性质: 文档站原文

### B.1 统一账户

全部出现在 `GET /wallet/sub_account_futures_balances`「查询子账号永续合约账户余额信息」的返回结构里。

| 出处（字段） | 原文 |
| --- | --- |
| `available` | 指的是逐仓可用的转出或交易的额度，统一账户下包含授信额度的逐仓可用额度(有包含体验金,体验金无法转出,所以要转出,转出金额需要扣除体验金) |
| `enable_credit` | 是否开启统一账户模式 |
| `position_initial_margin` | 头寸占用的起始保证金，适用于统一账户模式 |
| `maintenance_margin` | 头寸占用的维持保证金，适用于新经典账户保证金模式和统一账户模式 |
| `history.cross_settle` | 代表统一账户模式下，合约账户盈利被结算到现货数值。负数代表从合约结算到现货的，正数代表从现货结算到合约的。此数值为累计值 |

### B.2 跨币种、单币种、组合保证金

同一句话，出处同上接口的 `margin_mode` 字段。

| 出处（字段） | 原文 |
| --- | --- |
| `margin_mode` | 保证金模式，0-经典保证金模式，1-跨币种保证金模式，2-组合保证金模式，3-单币种保证金模式 |

### B.3 借币、借款

字面上含「借币」「借款」的句子：**0 条，未找到**。页面上与借入有关的是下面的「借入」「借贷」字样，已并入 B.4。

### B.4 borrowed（含「借入」「借贷」）

| 出处 | 字段 | 原文 |
| --- | --- | --- |
| `GET /wallet/sub_account_margin_balances`「查询子账号逐仓杠杆账户余额信息」 | `available[]` 的 `base`、`quote` | 可用于杠杆交易的额度，available = 保证金 + borrowed |
| 同上 | `base.borrowed`、`quote.borrowed` | 借入资金 |
| `GET /wallet/sub_account_cross_margin_balances`「查询子账号全仓杠杆账户余额信息」 | `borrowed` | 折算成 USDT 的账户总借入数量，即所有币种(不包括点卡)的 `borrowed*price*discount` 之和 |
| 同上 | `borrowed_net` | 折算成 USDT 的账户总借入数量 * 放大系数 |
| 同上 | `risk` | 风险率，风险率小于 110% 会被爆仓，计算方式 `total / (borrowed+interest)` |
| 同上 | `balances.*.borrowed` | 借入额度 |
| `GET /wallet/total_balance`「查询个人账户总额」 | `total.borrowed`、`details.*.borrowed` | 杠杆借贷总和,这个字段只会在margin,cross_margin账户中出现 |

### B.5 unrealised_pnl（含中文「未实现盈亏」）

| 出处 | 字段 | 原文 |
| --- | --- | --- |
| `GET /wallet/sub_account_futures_balances` | `total` | 钱包余额，只适用于经典合约账户。钱包余额为所有历史已发生的资金流水之和，包括历史转入转出、平仓结算、手续费支出等，不包含仓位的未实现盈亏。total = SUM(history_dnw, history_pnl, history_fee, history_refr, history_fund) |
| 同上 | `unrealised_pnl` | 未实现盈亏 |
| 同上 | `cross_unrealised_pnl` | 全仓未实现盈亏，适用于新经典账户保证金模式 |
| `GET /wallet/total_balance` | `total.unrealised_pnl`、`details.*.unrealised_pnl` | 未实现盈亏总和,这个字段只会在futures,options,delivery,total 账户中出现 |

### B.6 Spot / spot

| 出处 | 原文 |
| --- | --- |
| `GET /wallet/total_balance`，`details` 描述 | spot: 现货账户 |
| `GET /wallet/sub_account_futures_balances`，`history.cross_settle` | 代表统一账户模式下，合约账户盈利被结算到现货数值。负数代表从合约结算到现货的，正数代表从现货结算到合约的。此数值为累计值 |
| `GET /wallet/transfers`「查询交易账户划转订单」，`from_account` | 转出账户类型： - `spot`：现货账户 - `margin`：杠杆账户 - `futures`：永续合约账户 - `delivery`：交割合约账户 - `options`：期权账户 - `unknown`：无法识别的账户类型 |
| 同上，`to_account` | 转入账户类型： - `spot`：现货账户 - `margin`：杠杆账户 - `futures`：永续合约账户 - `delivery`：交割合约账户 - `options`：期权账户 - `unknown`：无法识别的账户类型 |
| `POST /wallet/transfers`「交易账户互转」，请求体结构 | 转入转出账户列表: - `spot`: 现货账户 - `margin`: 杠杆账户 - `futures`: 永续合约账户 - `delivery`: 交割合约账户 - `options`: 期权账户 |
| `GET`、`POST /wallet/sub_account_transfers`，`sub_account_type` | 操作的子账号交易账户， spot - 现货账户， futures - 永续合约账户， delivery - 交割合约账户, options - 期权账户 |
| `POST /wallet/sub_account_to_sub_account`，`sub_account_from_type`、`sub_account_to_type` | 转出的子账号交易账户, spot - 现货账户, futures - 永续合约账户, delivery - 交割合约账户 / 转入的子账号交易账户, spot - 现货账户, futures - 永续合约账户, delivery - 交割合约账户 |
| `GET /wallet/withdrawals`「获取提现记录」，`asset_class` | 提现记录币种类型，默认为空。即支持用户按需查询主区和创新区的提现记录。 取值范围：SPOT、PILOT SPOT ： 主区 PILOT： 创新区 |

最后一行的大写 `SPOT` 指提现分区「主区」，与账户资产无关。

### B.7 cross_margin

| 出处 | 原文 |
| --- | --- |
| `GET /wallet/total_balance`，`details` 描述 | cross_margin: 全仓杠杆账户 |
| `GET /wallet/total_balance`，`borrowed` 描述 | 杠杆借贷总和,这个字段只会在margin,cross_margin账户中出现 |
| `GET /wallet/sub_account_cross_margin_balances` | 仅路径里出现，标题「查询子账号全仓杠杆账户余额信息」 |

### B.8 account_futures、unified

字面命中：**均为 0 条，未找到**。

### B.9 同页其他与「总额/净资产/保证金」直接相关的句子（文档站原文）

出处均为 `GET /wallet/sub_account_cross_margin_balances`：

| 字段 | 原文 |
| --- | --- |
| `total` | 折算成 USDT 的账户总资产，即所有币种(不包括点卡)的 `(available+freeze)*price*discount` 之和 |
| `net` | 折算成 USDT 的净资产 |
| `interest` | 折算成 USDT 的账户未接利息的总和，即所有币种(不包括点卡)的 `interest*price*discount` 之和 |
| `total_available_margin` | 可用的保证金额度 |
| `balances.*.available` | 可用额度 |
| `balances.*.freeze` | 被锁定的额度 |
| `balances.*.interest` | 未还利息 |

### B.10 对任务「特别关注」四问的回答（归纳）

| 问题 | 回答 |
| --- | --- |
| 文档是否写明 `amount`/`total` 包含未实现盈亏 | **针对 `/wallet/total_balance` 没有写**。`amount` 的描述只有「账户总额数字」。页面上唯一明确谈「是否含未实现盈亏」的，是 B.5 第一行，说的是子账号永续合约账户的 `total` 字段，而且注明「只适用于经典合约账户」，不是 `/wallet/total_balance` 的 `total.amount` |
| 文档是否写明是否扣除借款 | **针对 `/wallet/total_balance` 没有写**。`borrowed` 只描述了是什么和出现在哪些账户。页面上相关的明确公式只有两处，都是别的接口：杠杆账户「available = 保证金 + borrowed」；全仓杠杆账户 `total` 是 `(available+freeze)` 之和，借入量另由 `borrowed` 给出，`net` 才是净资产 |
| `value` 的含义 | **未找到**。页面上没有 `value` 字段 |
| `details` 各业务线字段与 `total` 是否有加总关系 | **没有写**。`total` 的描述是「换算成目标币种的账户总额」，`details` 是「各账户总额」加键清单，没有任何公式或加总说明 |

另一处值得注意的原文细节（归纳）: `unrealised_pnl` 的描述把 `total` 列在会出现的账户里（「futures,options,delivery,total」），而 `borrowed` 的描述只列了「margin,cross_margin」，没有列 `total`。文档没有解释 `total.borrowed` 是否一定有值。

---

## C. 「查询 Sub 账户总资产」

- 性质: **未找到**

- 文档站钱包分页没有这个接口。该页共 23 个接口，没有标题或路径含「Sub 账户总资产」的。与子账户余额最接近的三个是：
  - 「查询子账号余额信息」`GET /wallet/sub_account_balances`
  - 「查询子账号永续合约账户余额信息」`GET /wallet/sub_account_futures_balances`
  - 「查询子账号全仓杠杆账户余额信息」`GET /wallet/sub_account_cross_margin_balances`
- Go SDK v7.2.184 的 `WalletApi.md` 共 1752 行、23 个方法，没有 `GetSubAccountTotalBalance`，全文对 `total_balance` 的命中只有 `GetTotalBalance` 相关的 7 行。
- Go SDK 主干、Python SDK 主干、Java SDK 主干的 `WalletApi.md` 同样只有 `get_total_balance` / `getTotalBalance`。
- Go SDK v7.2.184 的 `AccountApi.md`（10 个方法）里也没有相关接口。

所以「与主接口的差异点」无从对照。根页未抓，不能排除根页的其他分页有此接口。

---

## D. 官方 Go SDK v7.2.184 英文定义与一致性对照

- 来源: `github.com/gate/gateapi-go`，tag `v7.2.184`
- 性质: SDK 原文（逐字）

### D.1 版本事实（归纳）

- 项目 `go.mod` 第 12 行锁定 `github.com/gate/gateapi-go/v7 v7.2.184`。
- `v7.2.184` 是该仓库当前最新的 tag，提交时间 `2026-11-19T13:14:05Z`。主干上这两个模型的定义与 v7.2.184 逐字相同。

### D.2 `GetTotalBalance` 函数说明与参数表

路径表首行说明与对应行:

```markdown
All URIs are relative to *https://api.gateio.ws/api/v4*

[**GetTotalBalance**](WalletApi.md#GetTotalBalance) | **Get** /wallet/total_balance | Query personal account totals
```

函数一节（`Example` 小节是 Go 调用示例代码，未摘录）:

```markdown
## GetTotalBalance

> TotalBalance GetTotalBalance(ctx, optional)

Query personal account totals

This query endpoint returns the total *estimated value* of all currencies in each account converted to the input currency. Exchange rates and related account balance information may be cached for up to 1 minute. It is not recommended to use this interface data for real-time calculations.  For real-time calculations, query the corresponding balance interface based on account type, such as:  - `GET /spot/accounts` to query spot account - `GET /margin/accounts` to query margin account - `GET /futures/{settle}/accounts` to query futures account

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**optional** | **GetTotalBalanceOpts** | optional parameters | nil if no parameters

### Optional Parameters

Optional parameters are passed through a pointer to a GetTotalBalanceOpts struct

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**currency** | **optional.String**| Target currency type for statistical conversion. Accepts BTC, CNY, USD, and USDT. USDT is the default value | [default to USDT]

### Return type

[**TotalBalance**](TotalBalance.md)

### Authorization

[apiv4](../README.md#apiv4)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json
```

SDK 文档里同样没有限频说明。

### D.3 模型属性表

`docs/TotalBalance.md`:

```markdown
# TotalBalance

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Total** | [**AccountBalance**](AccountBalance.md) |  | [optional]
**Details** | [**map[string]AccountBalance**](AccountBalance.md) | Total Balances of All Accounts  - cross_margin: Cross Margin Account - spot: Spot Account - finance: Finance Account - margin: Margin Account - quant: Quantitative Account - futures: Futures Account (Perpetual Contracts) - delivery: Delivery Account (Delivery Contracts) - warrant: Warrant Account - cbbc: CBBC Account (Covered Bull/Bear Contract) - meme_box: Alpha Account - options: Options Account - payment: Payment Account | [optional]
```

`docs/AccountBalance.md`（任务给的 `model_account_balance.md` 返回 404，实际文件名首字母大写）:

```markdown
# AccountBalance

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Amount** | **string** | Account total balance amount | [optional]
**Currency** | **string** | Currency | [optional]
**UnrealisedPnl** | **string** | Unrealised_pnl, this field will only appear in futures, options, delivery, and total accounts | [optional]
**Borrowed** | **string** | Total borrowed amount, this field will only appear in margin and cross_margin accounts | [optional]
```

Go 源码 `model_total_balance.go`:

```go
// User's total balance information
type TotalBalance struct {
	Total AccountBalance `json:"total,omitempty"`

	// Total Balances of All Accounts  - cross_margin: Cross Margin Account - spot: Spot Account - finance: Finance Account - margin: Margin Account - quant: Quantitative Account - futures: Futures Account (Perpetual Contracts) - delivery: Delivery Account (Delivery Contracts) - warrant: Warrant Account - cbbc: CBBC Account (Covered Bull/Bear Contract) - meme_box: Alpha Account - options: Options Account - payment: Payment Account
	Details map[string]AccountBalance `json:"details,omitempty"`
}
```

Go 源码 `model_account_balance.go`:

```go
// Total balances calculated with specified currency unit
type AccountBalance struct {

	// Account total balance amount
	Amount string `json:"amount,omitempty"`

	// Currency
	Currency string `json:"currency,omitempty"`

	// Unrealised_pnl, this field will only appear in futures, options, delivery, and total accounts
	UnrealisedPnl string `json:"unrealised_pnl,omitempty"`

	// Total borrowed amount, this field will only appear in margin and cross_margin accounts
	Borrowed string `json:"borrowed,omitempty"`
}
```

### D.4 任务预期的嵌套结构：不存在

以下文件在 `v7.2.184` 下均返回 HTTP 404（性质: 未找到）:

- `model_account_balance_total.go`
- `model_account_balance_details.go`
- `model_account_balance_details_spot.go`
- `model_account_balance_details_unified.go`
- `model_account_balance_details_account_futures.go`
- `model_account_balance_details_cross_margin.go`
- `docs/model_account_balance.md`
- `docs/AccountBalanceTotal.md`
- `docs/AccountBalanceDetails.md`

`docs/` 目录的文件清单里，名字含 `balance` 的模型文档只有 `AccountBalance.md`、`TotalBalance.md`、`UnifiedBalance.md`、`CrossMarginBalance.md`、`DualGetBalance.md`、`MockSpotBalance.md`、`SubAccountBalance.md`、`SubAccountCrossMarginBalance.md`、`SubAccountFuturesBalance.md`、`SubAccountMarginBalance.md`、`SmallBalance.md`、`SmallBalanceHistory.md`、`BalanceRecordItem.md`、`BalanceRecordList.md`、`BalanceRecordListData.md`，以及三个 `AiHubRebalance*.md`。

也就是说，在 v7.2.184 里 `AccountBalance` 是嵌在 `TotalBalance` 里的 4 字段结构，不是带 `time`/`value`/`total`/`details` 的顶层模型。

### D.5 与 A 的一致性对照（归纳）

| 对照项 | 文档站钱包分页（中文） | Go SDK v7.2.184（英文） | 是否一致 |
| --- | --- | --- | --- |
| 标题 | 查询个人账户总额 | Query personal account totals | 一致 |
| 方法与路径 | `GET /wallet/total_balance` | `Get /wallet/total_balance`，相对 `https://api.gateio.ws/api/v4` | 一致 |
| 参数 `currency` | `string`，非必选，四个值，USDT 默认 | `optional.String`，四个值，`[default to USDT]` | 一致 |
| 响应模型名 | `TotalBalance` | `TotalBalance` | 一致 |
| 顶层属性 | `total`（object）、`details`（object） | `Total`（`AccountBalance`）、`Details`（`map[string]AccountBalance`） | 一致，类型表述不同 |
| `amount` | 账户总额数字 | Account total balance amount | 一致 |
| `currency` | 币种，enum 四个值 | Currency | 一致，SDK 属性表未标 enum |
| `unrealised_pnl` | 未实现盈亏总和,这个字段只会在futures,options,delivery,total 账户中出现 | Unrealised_pnl, this field will only appear in futures, options, delivery, and total accounts | 一致 |
| `borrowed` | 杠杆借贷总和,这个字段只会在margin,cross_margin账户中出现 | Total borrowed amount, this field will only appear in margin and cross_margin accounts | 一致，措辞略有差异 |
| `details` 键 | 12 个 | 12 个，同名 | 一致 |
| `time`、`value` | 无 | 无 | 一致，均不存在 |
| 字段必填 | 全部 `Optional` | 全部 `[optional]` | 一致 |
| 缓存说明 | 最长会有1分钟的缓存 | may be cached for up to 1 minute | 一致 |
| 认证 | 需要 API key 和 secret | Authorization: apiv4 | 一致 |
| 限频 | 未取到 | 未写 | 均无 |

**出入**：文档站与 SDK 之间没有实质出入，只有三处表述差异（`borrowed` 中英文措辞、响应 `currency` 的 enum 只在文档站出现、`details` 的类型表述）。

**任务预期与实际的出入**（逐条，归纳）:

1. 接口标题不是「查询账户总资产」，是「查询个人账户总额」。
2. 顶层响应模型不是 `AccountBalance`，是 `TotalBalance`。
3. 没有 `time` 字段。
4. 没有 `value` 字段。
5. 没有 `AccountBalanceTotal`、`AccountBalanceDetails` 及各业务线子结构。
6. `details` 没有 `account_futures`、`unified` 键。
7. 没有 `GetSubAccountTotalBalance`。
8. SDK 模型文档路径不是 `docs/model_account_balance.md`，是 `docs/AccountBalance.md`。

---

## E. Agent 文档页里与账户、资产、余额、权益、保证金、持仓有关的工具

- 来源: `https://www.gate.com/docs/developers/agent/zh_CN`（页面标注「Last Updated: 4/27/2026, 1:28:42 AM」）
- 性质: 文档站原文（逐字）。页面只有「工具名 + 一句说明」的表格，**没有任何工具的参数说明或返回字段说明**。

### E.1 核心能力与端点

```markdown
- __钱包管理__ — 资产查询、转账、DApp 交互
```

| 端点 | 工具数 | 认证方式 | 说明 |
| --- | --- | --- | --- |
| `/mcp` | 58 | 无需认证 | 全品类行情数据（现货/合约/期权/交割/理财/Alpha） |
| `/mcp/exchange` | 400+ | OAuth2 | CEX 全功能交易（现货/合约/期权/闪兑/跟单/子账户等） |
| `/mcp/dex` | 33 | Google / Gate OAuth | DEX Swap、钱包管理、DApp 交互 |

### E.2 工具清单

Account（需要 OAuth2 认证）整表:

```markdown
| cex_wallet_get_total_balance | 账户总资产 |
| cex_wallet_create_transfer | 账户间划转 |
| cex_wallet_list_deposits | 充值记录 |
| cex_wallet_list_withdrawals | 提现记录 |
| cex_wallet_get_deposit_address | 充值地址 |
| cex_wallet_list_sa_balances | 子账户余额 |
| cex_unified_get_unified_accounts | 统一账户信息 |
| cex_unified_get_unified_mode | 账户模式 |
| cex_unified_create_unified_loan | 借款 |
| cex_sa_list_sas | 子账户列表 |
| cex_sa_create_sa | 创建子账户 |
| cex_sa_lock_sa / cex_sa_unlock_sa | 锁定/解锁子账户 |
| cex_account_get_account_detail | 账户详情 |
```

Spot Trading（需要 OAuth2 认证）里相关的:

```markdown
| cex_spot_get_spot_accounts | 查询现货账户余额 |
```

Futures Trading（需要 OAuth2 认证）里相关的:

```markdown
| cex_fx_get_fx_position | 查询单向持仓 |
| cex_fx_list_fx_positions | 查询所有持仓 |
| cex_fx_get_fx_dual_position | 查询双向持仓 |
| cex_fx_update_fx_position_leverage | 调整杠杆 |
| cex_fx_update_fx_position_margin | 调整保证金 |
| cex_fx_get_fx_accounts | 查询合约账户 |
```

DEX（需要 Google / Gate OAuth 认证）里相关的:

```markdown
| dex_wallet_get_addresses | 获取钱包地址列表 |
| dex_wallet_get_token_list | 查询钱包余额 |
```

工具表的收尾说明:

```markdown
注意：所有类别包括但不局限于以上 Tools，详见 Gate MCP (opens new window)。
```

### E.3 OAuth2 Scopes 表里相关的行

```markdown
| profile | 用户基本信息 | cex_unified_get_unified_accounts、VIP 等级查询 |
| wallet | 钱包与充提 | cex_wallet_create_transfer、cex_wallet_get_deposit_address |
| account | 账户资产查询 | cex_wallet_get_total_balance、cex_spot_get_spot_accounts |
```

### E.4 Skills 表里相关的行

```markdown
| gate-exchange-assets | 资产与余额查询：总资产、现货、合约、杠杆、期权、理财、Alpha、TradFi | OAuth2 |
| gate-exchange-assets-manager | 多账户资产总览、保证金风险、SimpleEarn 收益、联盟佣金、统一账户借贷与杠杆配置 | OAuth2 |
| gate-exchange-unified | 统一账户操作：资产权益、借贷额度、借还款、模式切换、杠杆与抵押配置 | OAuth2 |
| gate-exchange-subaccount | 子账户管理：查询、创建、锁定、解锁 | OAuth2 |
| gate-exchange-transfer | 同 UID 内部转账：现货、合约、杠杆账户间资金划转 | OAuth2 |
| gate-exchange-collateralloan | 多币种抵押借贷：查询贷款、固定利率、还款、增减抵押 | OAuth2 |
| gate-dex-wallet | 钱包账户管理：Google/Gate OAuth 认证、余额查询、地址获取、转账、交易历史、x402 支付、DApp 交互、CLI 工具 | Google/Gate OAuth |
```

### E.5 CLI 示例里相关的两行

```markdown
# 查询账户余额
gate-cli cex spot account list

# 查询合约持仓
gate-cli cex futures position list --settle usdt
```

### E.6 对任务问题的明确回答（归纳）

**该页面是否提供了任何能读出账户未实现盈亏、借款或合约可用额度的工具？**

- **从本页无法确认。** 页面没有任何工具的参数或返回字段说明，也没有一处出现「未实现盈亏」「可用额度」这类字段描述。
- 页面上出现的「借款」只有一处，是 `cex_unified_create_unified_loan`（「借款」），这是借入动作，不是读取负债。
- 只按工具名和一句说明推测，可能相关的有：`cex_wallet_get_total_balance`（账户总资产）、`cex_unified_get_unified_accounts`（统一账户信息）、`cex_fx_get_fx_accounts`（查询合约账户）、`cex_fx_list_fx_positions` / `cex_fx_get_fx_position` / `cex_fx_get_fx_dual_position`（持仓）。**这是推测，页面没有依据。**
- 工具参数原文：**未找到**，页面不含。

---

## F. 未找到的项

| # | 项 | 情况 | 查过的地方 |
| --- | --- | --- | --- |
| 1 | 「查询账户总资产」这个标题 | 不存在。同路径接口的标题是「查询个人账户总额」 | 文档站钱包分页；Go SDK v7.2.184、主干；Python、Java SDK 主干 |
| 2 | 顶层 `AccountBalance` 模型（含 `total`、`details`） | 不存在。顶层模型是 `TotalBalance` | 同上 |
| 3 | `time` 字段 | 未找到 | 文档站钱包分页；Go SDK |
| 4 | `value` 字段及其含义 | 未找到 | 同上 |
| 5 | `AccountBalanceTotal`、`AccountBalanceDetails` 及各业务线子结构 | v7.2.184 与主干均无，相关文件 404 | Go SDK |
| 6 | `details` 的 `account_futures`、`unified` 键 | 页面和 SDK 的键清单里都没有 | 文档站钱包分页；Go SDK |
| 7 | `/wallet/total_balance` 的响应示例 JSON | **未取到**。阅读器返回内容里没有示例块 | 文档站钱包分页 |
| 8 | `/wallet/total_balance` 的限频 | **未取到**。阅读器返回内容里没有限频字样，SDK 文档也没写 | 文档站钱包分页；Go SDK |
| 9 | 「查询 Sub 账户总资产」 | **未找到**。钱包分页、Go/Python/Java SDK 的 `WalletApi`、Go SDK `AccountApi` 均无 | 见 C 节 |
| 10 | 文档是否写明 `amount` 不含未实现盈亏 | **没有写** | 文档站钱包分页 |
| 11 | 文档是否写明 `amount` 是否扣除借款 | **没有写** | 文档站钱包分页 |
| 12 | `details` 与 `total` 的加总关系 | **没有写** | 文档站钱包分页 |
| 13 | 字面「借币」「借款」 | 钱包分页 0 条命中 | 文档站钱包分页 |
| 14 | 字面 `account_futures`、`unified` | 钱包分页 0 条命中 | 文档站钱包分页 |
| 15 | 统一账户（UNIFIED 模式）的资产在 `/wallet/total_balance` 的 `total`/`details` 里如何体现 | **没有写**。键清单里没有统一账户对应的键 | 文档站钱包分页 |
| 16 | 文档站根页 | **未抓取**，见第 0 节 | — |
| 17 | Agent 页各工具的参数原文 | **未找到**，页面不含参数说明 | Agent 文档页 |
| 18 | Agent 页哪个工具能读出未实现盈亏、借款、合约可用额度 | **无法确认**，页面没有返回字段说明 | Agent 文档页 |
| 19 | SDK 文档 `docs/model_account_balance.md` | 404，实际是 `docs/AccountBalance.md` | Go SDK v7.2.184 |

### 取数局限（归纳）

- 文档站内容经第三方网页阅读器转成 markdown 后摘录，强调符、换行、`(opens new window)` 等是阅读器的渲染结果，可能与页面原始排版略有差别，文字本身按返回内容逐字保留。
- 页面全文没有落盘，B 节的关键词核对是人工通读，不是程序化匹配。
- 没有调用任何需要 API key 的交易所接口，没有读取任何密钥。

## G. 2026-11-24 只读实测（规划期，主会话）

- 性质: 实测。交易子账户、USDT 永续；宿主机上的 Python 脚本（签名 GET 请求），凭据经环境变量传入。
- 与上面文档条目的对应关系写在每张表后面；这里只记录读数，不做设计。

### G.1 账户状态

`GET /futures/usdt/positions` 中 `size` 不为 0 的条目 0 个（共 3 个条目）；`GET /futures/usdt/orders?status=open` 0 条；
`GET /futures/usdt/price_orders?status=open` 0 条。即：无持仓、无未完结委托、无未触发条件单。

### G.2 `GET /futures/usdt/accounts`（节选，`user` 已脱敏）

```json
{
 "total": "0.000000005",
 "unrealised_pnl": "0",
 "available": "107.49769",
 "bonus": "0",
 "order_margin": "0",
 "position_initial_margin": "0",
 "maintenance_margin": "0",
 "currency": "USDT",
 "enable_credit": true,
 "in_dual_mode": false,
 "margin_mode": 1,
 "margin_mode_name": "multi_currency",
 "position_mode": "single",
 "cross_available": "107.49769",
 "cross_unrealised_pnl": "0",
 "history": { "dnw": "0", "pnl": "-2.13338", "fee": "-0.3623596", "fund": "-0.00656618154", "cross_settle": "2.50230578654" }
}
```

对应：`total` 只有 `0.000000005`（A.13 节所在归档文件里文档写它「只适用于经典合约账户」）；`available` 是 5 位小数的 `107.49769`；
`enable_credit=true`、`margin_mode=1`（跨币种）、`position_mode=single`。`history.dnw` 为 0 而 `history.pnl`、`history.fee` 非 0、
`history.cross_settle` 非 0，与计划文档 3.1「盈亏结算到现货」一致。

### G.3 `GET /wallet/total_balance?currency=USDT`（全文，无敏感字段）

```json
{
 "details": {
  "cfd": { "currency": "USDT", "amount": "0", "unrealised_pnl": "0" },
  "delivery": { "currency": "USDT", "amount": "0", "unrealised_pnl": "0" },
  "event_contract": { "currency": "USDT", "amount": "0" },
  "finance": { "currency": "USDT", "amount": "0" },
  "futures": { "currency": "USDT", "amount": "0", "unrealised_pnl": "0" },
  "margin": { "currency": "USDT", "amount": "0", "borrowed": "0" },
  "meme_box": { "currency": "USDT", "amount": "0" },
  "options": { "currency": "USDT", "amount": "0", "unrealised_pnl": "0" },
  "payment": { "currency": "USDT", "amount": "0" },
  "pre_ipos": { "currency": "USDT", "amount": "0" },
  "quant": { "currency": "USDT", "amount": "0" },
  "spot": { "currency": "USDT", "amount": "107.49769421", "borrowed": "0" },
  "tradfi_spot": { "currency": "USDT", "amount": "0", "unrealised_pnl": "0" }
 },
 "total": { "amount": "107.49769421", "borrowed": "0", "currency": "USDT", "unrealised_pnl": "0" }
}
```

对应与观察（归纳）:

| 观察 | 对应的文档条目 |
| --- | --- |
| 顶层只有 `total` 与 `details`，没有 `time`、`value` | A.1、D.3 |
| `total.amount` 是 8 位小数的字符串，`total.borrowed`、`total.unrealised_pnl` 在数值为 0 时仍然返回（`"0"`） | A.1 写这三个字段都是 Optional；数值为 0 时是否省略，文档没说，实测是没有省略 |
| `total.borrowed` 有值，而文档写 `borrowed`「只会在 margin、cross_margin 账户中出现」 | B.10 末段已指出文档没有解释 `total.borrowed` 是否一定有值 |
| `details` 实际 13 个键：文档的 12 个键里 `cross_margin`、`warrant`、`cbbc` 没有返回，另外多出 `cfd`、`event_contract`、`pre_ipos`、`tradfi_spot` | A.1 的 12 键清单；SDK 把 `Details` 定义成 `map[string]AccountBalance`，多出来的键不会解析失败 |
| `details.event_contract` 没有 `unrealised_pnl` 与 `borrowed` | 同上，Optional 字段可以缺失 |
| 数值为 0 的条目里 `amount` 是 `"0"`，不是空串 | — |
| 资金全部在 `details.spot`（`107.49769421`），`details.futures.amount` 为 `"0"` | 计划文档 3.1「资金位置」 |
| `total.amount`（`107.49769421`）与合约账户 `available`（`107.49769`）在后 3 位小数上不同：钱包总额给 8 位小数，合约账户给 5 位小数 | — |
| 不带仓时 `total.unrealised_pnl` 与合约账户 `unrealised_pnl` 都是 0，无法比较两者是否同步 | 带仓读数见任务 `probe/` |

## H. 2026-11-24 带仓读数的实测结论（主会话，证据见任务 `probe/README.md`）

- 性质: 实测。对象是 ETH_USDT 0.1 张的多头仓位（约 1.7 USDT，全仓 200x，开仓请求带的是该合约现有设置），
  持仓约 80 秒内采样 7 次，间隔约 13 秒；读数经交付的代码取得，并与两个原始接口的返回值逐项比对。
- 与 A、B 两节「文档没写的项」的对应：文档没有写明 `total.amount` 是否包含未实现盈亏、是否扣除借款，
  下面的 1、2 是对前者的实测回答；借款与 `details`、`total` 的加总关系仍然没有测到。

| # | 结论 | 读数 |
| --- | --- | --- |
| 1 | **`total.amount` 不包含未实现盈亏，也不随持仓变动** | 7 次采样 `total.amount` 恒为 `107.49519799`；同一时段 `total.unrealised_pnl` 在 −0.00134 到 +0.00150 之间变动 |
| 2 | **手续费与已实现盈亏在开平仓那一刻就从 `total.amount` 里结算**，没有看到 A.1 节所说「最长 1 分钟缓存」造成的滞后 | 开仓那一刻本金降 0.00084795 = 名义 1.69589 × 0.05%；平仓后本金变化 −0.00125612 = 已实现盈亏 +0.00044 − 两边手续费 0.00084795 与 0.00084817；平仓后 5 秒与 70 秒的读数相差 0.00000001 |
| 3 | **`total.unrealised_pnl` 没有可见的缓存滞后** | 7 次采样每次都在变，没有一次等于更早采样时合约账户的 `unrealised_pnl`；与同一轮合约账户的值 6 次相等、1 次相差 0.00007（两个请求先后发出之间的价格变动）。采样间隔 13 秒，只能排除 1 分钟量级的缓存 |
| 4 | **合约账户的 `total` 在统一账户下不是本金** | 不带仓时是 `-0.001648275`（此前一轮开平仓的手续费记在合约侧），持仓时是 `-0.00249622` |
| 5 | **「可用额度 = `amount` + `unrealised_pnl` − `position_initial_margin`」不严格成立**（计划文档 3.1 节把它写成了等式） | 7 次采样的残差在 −0.00162 到 +0.00029 之间且不恒定，挂单保证金为 0；交易所内部的计算方式没有文档 |
| 6 | **只有现货有余额时 `total.amount` 等于 `details.spot.amount`** | G.3：两者都是 `107.49769421`。这是单个账户的一次读数，不是一般规律 |

由 3 可见，权益里的未实现盈亏按计划文档取 `total.unrealised_pnl` 是可行的，没有换成合约账户的 `unrealised_pnl`。
由 5，计划文档 3.1 节的这条等式需要改为量级上的近似关系（在父分支收口时回写）。

### Files Found

| 文件路径 | 说明 |
| --- | --- |
| `/Users/hsikey/Trading/nofx-wt/feat-2026-1123-gate-trading-foundation--account-equity/go.mod` | 第 12 行锁定 `gateapi-go/v7 v7.2.184` |
| `.trellis/tasks/11-24-account-equity/probe/README.md` | 带仓读数的完整证据说明与运行历史 |

### External References

- [文档站钱包分页](https://www.gate.com/docs/developers/apiv4/zh_CN/wallet) - `GET /wallet/total_balance` 中文原文
- [Agent 文档页](https://www.gate.com/docs/developers/agent/zh_CN) - MCP 工具与 Skills 清单
- [gateapi-go v7.2.184 WalletApi.md](https://raw.githubusercontent.com/gate/gateapi-go/v7.2.184/docs/WalletApi.md) - `GetTotalBalance` 英文定义
- [gateapi-go v7.2.184 TotalBalance.md](https://raw.githubusercontent.com/gate/gateapi-go/v7.2.184/docs/TotalBalance.md) - 响应模型属性表
- [gateapi-go v7.2.184 AccountBalance.md](https://raw.githubusercontent.com/gate/gateapi-go/v7.2.184/docs/AccountBalance.md) - 嵌套结构属性表
- [gateapi-go v7.2.184 model_account_balance.go](https://raw.githubusercontent.com/gate/gateapi-go/v7.2.184/model_account_balance.go) - Go 结构体
- [gateapi-go v7.2.184 model_total_balance.go](https://raw.githubusercontent.com/gate/gateapi-go/v7.2.184/model_total_balance.go) - Go 结构体

### Related Specs

- 本次是外部文档检索，没有读取 `.trellis/spec/` 下的规范。
