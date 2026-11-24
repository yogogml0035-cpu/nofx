# Research: Gate 合约杠杆 / 全仓逐仓 / 持仓模式 —— 文档站中文原文

- Query: 为「Gate USDT 永续合约：按 trader 设置生效全仓/逐仓与杠杆」取得 Gate 官方文档站的中文原文（逐字摘录），覆盖 Futures 分页的杠杆 / 保证金模式 / 持仓模式接口与模型字段、统一账户相关句子、错误 label 与限制、根页 changelog、Agent 页相关内容
- Scope: external（Gate 官方文档站 zh_CN 三个页面；文末附官方 Go SDK v7.2.184 英文定义的小范围对照，单独成节，不与中文原文混排）
- Date: 2026-11-24

## 来源与抓取方法

| # | 地址 | 抓取方法 | 结果 | 请求次数 | 返回内容是否看起来被截断 |
| --- | --- | --- | --- | --- | --- |
| 1 | `https://www.gate.com/docs/developers/apiv4/zh_CN/futures` | `mcp__web_reader__webReader`，`timeout=90`，`retain_images=false`，未禁用缓存 | 成功 | 1（未重试） | 有折叠，见下 |
| 2 | `https://www.gate.com/docs/developers/apiv4/zh_CN/` | 同上 | 成功 | 1（未重试） | 未见截断 |
| 3 | `https://www.gate.com/docs/developers/agent/zh_CN` | 同上，另加 `no_cache=true` | 成功 | 1（未重试） | 未见截断 |

页面 1（Futures 分页）的起止与折叠情况：

- 内容从「Futures / 查询所有的合约信息」开始，到页尾「模型」区的 `StopChaseOrderReq` 结束，最后一个模型块自身完整（含示例）。无法从返回内容判断其后是否还有未渲染的模型。
- 本文涉及的全部接口章节都在（清单见 A.0），参数表完整，参数表没有折叠。
- 折叠只出现在「返回格式」表，页面文字为「展开剩余 N 个字段」。与本题相关的有：Position 单对象响应「展开剩余 32 个字段」（获取单个仓位信息、更新仓位保证金、更新仓位杠杆、更新指定模式的杠杆、切换全逐仓模式、更新仓位风险限额），Position 数组响应「展开剩余 34 个字段」（获取用户仓位列表、双仓模式下的各接口），FuturesAccount「展开剩余 29 个字段」（获取合约账号、设置持仓模式、设置持仓模式,替换dual_mode接口），PositionTimerange「展开剩余 9 个字段」。折叠部分的内容未知。
- 本题要的字段（`leverage`、`cross_leverage_limit`、`lever`、`pos_margin_mode`、`mode`、`leverage_max`、`risk_limit`、`margin_mode`、`position_mode`、`in_dual_mode`、`enable_dual_plus`、`enable_new_dual_mode`）全部落在已展开的部分里。
- 页尾「模型」区的 `Position`、`FuturesAccount`、`FuturesLeverage`、`FuturesPositionCrossMode`、`UpdateDualCompPositionCrossModeRequest` 完整渲染，没有折叠标记。
- 示例代码区只渲染了 Python 与 Shell 两个标签页（页面标签栏列有 Python / Shell / JavaScript / Node.js / Go / Java / PHP / Ruby）。

页面 2（根页）：版本头「v4.106.184 · Stable」；changelog 从 `v4.106.184`（2026-11-18）一直到 `v1.0.0`（2018-12-30），其后「General」「限频规则」「异常 label 列表」「认证」「常见问题」齐全。

页面 3（Agent 页）：标题「Gate for AI | Gate API v4」，正文标题「Gate for AI 开发者指南」，止于「相关链接」和「Last Updated: 4/27/2026, 1:28:42 AM」。

### 阅读约定

- 来源标注：【文档站·中文·Futures】= 页面 1；【文档站·中文·根页】= 页面 2；【文档站·中文·Agent】= 页面 3。代码块与表格内的文字是原文照录；代码块和表格之外的中英文说明是整理者的话。
- 三个原文符号没有原样输出，替换方式如下，其余文字、标点照录：
  1. 接口标题里的锁形符号（表示需要认证）改写为标题后的「（需要认证）」。
  2. 正文里的警告符号（U+26A0 U+FE0F）改写为「〔警告〕」。
  3. Agent 页用例步骤前的键帽数字（数字 + U+FE0F U+20E3）改写为「1.」到「7.」。
- 已省略的抓取工具附加物：「返回」表里 `OK` 后面的 `(opens new window)`、代码块末尾的 `Copy`、示例代码的公共 import 行。
- Shell 示例里每个接口都相同的签名样板行（key / secret / timestamp / sign 计算）没有录入，只录与接口相关的行。
- 含 `object|null` 的两张 Position 表放在代码块内，避免表格竖线破坏单元格。
- 「合约交易下单」`text` 字段取值说明里有三个康熙部首区字符，按原样保留、没有换成常用汉字：`统⼀账户` 的 `⼀` 是 U+2F00，`⽼经典模式` 的 `⽼` 是 U+2F7C，`⾮对冲` 的 `⾮` 是 U+2FAE。

## A. Futures 分页：杠杆 / 保证金模式 / 持仓模式接口

来源均为【文档站·中文·Futures】。

### A.0 接口清单（按页面上出现的顺序）

| 章节标题 | 方法与路径 | 200 响应含义 | 格式 |
| --- | --- | --- | --- |
| 获取合约账号 | GET `/futures/{settle}/accounts` | 查询成功 | FuturesAccount |
| 获取用户仓位列表 | GET `/futures/{settle}/positions` | 列表查询成功 | [Position] |
| 根据时间获取用户历史仓位信息列表 | GET `/futures/{settle}/positions_timerange` | 列表查询成功 | [PositionTimerange] |
| 获取单个仓位信息 | GET `/futures/{settle}/positions/{contract}` | 仓位信息 | Position |
| 获取指定模式的杠杆信息 | GET `/futures/{settle}/get_leverage/{contract}` | 查询杠杆成功 | FuturesLeverage |
| 更新仓位保证金 | POST `/futures/{settle}/positions/{contract}/margin` | 仓位信息 | Position |
| 更新仓位杠杆 | POST `/futures/{settle}/positions/{contract}/leverage` | 仓位信息 | Position |
| 更新指定模式的杠杆 | POST `/futures/{settle}/positions/{contract}/set_leverage` | 仓位信息 | Position |
| 切换全逐仓模式 | POST `/futures/{settle}/positions/cross_mode` | 仓位信息 | Position |
| 双仓模式下切换全逐仓模式 | POST `/futures/{settle}/dual_comp/positions/cross_mode` | 查询成功 | [Position] |
| 更新仓位风险限额 | POST `/futures/{settle}/positions/{contract}/risk_limit` | 仓位信息 | Position |
| 设置持仓模式 | POST `/futures/{settle}/dual_mode` | 修改成功 | FuturesAccount |
| 设置持仓模式,替换dual_mode接口 | POST `/futures/{settle}/set_position_mode` | 修改成功 | FuturesAccount |
| 获取双仓模式下的持仓信息 | GET `/futures/{settle}/dual_comp/positions/{contract}` | 查询成功 | [Position] |
| 更新双仓模式下的保证金 | POST `/futures/{settle}/dual_comp/positions/{contract}/margin` | 查询成功 | [Position] |
| 更新双仓模式下的杠杆 | POST `/futures/{settle}/dual_comp/positions/{contract}/leverage` | 查询成功 | [Position] |
| 更新双仓模式下的风险限额 | POST `/futures/{settle}/dual_comp/positions/{contract}/risk_limit` | 查询成功 | [Position] |

以上接口页面均标注「该请求需要 API key 和 secret 认证」。每个接口的「枚举值列表」都只列了 `settle` = `btc` / `usdt` / `usd1`；返回 Position 的接口另列 `hedge_status` = `partial_hedged` / `full_hedged`、`mode` = `single` / `dual_long` / `dual_short`。

### A.1 更新指定模式的杠杆（需要认证）

```
POST`/futures/{settle}/positions/{contract}/set_leverage`

POST `/futures/{settle}/positions/{contract}/set_leverage`

_更新指定模式的杠杆_

为了简化leverage接口复杂逻辑，新增修改杠杆的接口
```

参数

| 名称 | 位置 | 类型 | 必选 | 描述 |
| --- | --- | --- | --- | --- |
| settle | URL | string | 是 | 永续合约结算货币 |
| contract | URL | string | 是 | 合约标识 |
| leverage | 请求参数 | string | 是 | 仓位杠杆倍数 |
| margin_mode | 请求参数 | string | 是 | 保证金模式 isolated/cross |
| dual_side | 请求参数 | string | 否 | dual_long - 多 dual_short - 空 |

返回

| 状态码 | 含义 | 描述 | 格式 |
| --- | --- | --- | --- |
| 200 | OK | 仓位信息 | Position |

返回格式（状态码 200，`_合约仓位详情_`），表后紧跟「展开剩余 32 个字段」：

```
| 名称 | 类型 | 描述 |
| --- | --- | --- |
| » user | integer(int64) | 用户ID |
| » contract | string | 合约标识 |
| » size | string | 头寸大小 |
| » hedge_status | string | 中性策略下仓位的对冲状态。包括：  - `partial_hedged`: 部分对冲 - `full_hedged`: 完全对冲 |
| » hedged_size | string | 中性策略下已对冲的仓位数量。 |
| » unhedged_size | string | 中性策略下未对冲的仓位数量，计算方式为 `max(abs(size) - abs(hedged_size), 0)`。 |
| » leverage | string | 逐仓模式下的杠杆倍数。如果是0，表示当下为全仓模式。全仓杠杆模式下的倍数请以“cross_leverage_limit”为准。 |
| » risk_limit | string | 风险限额 |
| » leverage_max | string | 在当前的仓位规模下，允许可开的最大杠杆倍数。当下仓位规模越大，可开的杠杆倍数越低。 |
| » maintenance_rate | string | 当前仓位规模所处的那一档的风险限额的维持保证金率要求。由于仓位维持保证金已启动梯度式计算，该仓位的实际维持保证金率要求以`average_maintenance_rate`为准 |
| » value | string | 按结算币种标记价格计算的合约价值 |
| » margin | string | 保证金 |
| » entry_price | string | 开仓价格 |
| » liq_price | string | 预估强平价，仅提供参考。实际强平触发以仓位保证金率或者账户维持保证金率为准。 |
| » mark_price | string | 合约当前标记价格 |
| » initial_margin | string | 仓位占用的起始保证金 |
| » maintenance_margin | string | 仓位所需的维持保证金 |
| » unrealised_pnl | string | 未实现盈亏 |
| » realised_pnl | string | 已实现盈亏，该仓位产生的所有平仓结算、资金费结算、手续费支出的资金流水之和 |
| » pnl_pnl | string | 已实现盈亏中的平仓结算盈亏 |
| » pnl_fund | string | 已实现盈亏中的资金费结算盈亏 |
| » pnl_fee | string | 已实现盈亏中的总手续费支出 |
| » history_pnl | string | 该合约市场下所有平仓结算盈亏，包括所有历史仓位 |
| » last_close_pnl | string | 最近一次平仓的盈亏 |
| » realised_point | string | 点卡已实现盈亏 |
| » history_point | string | 已平仓的点卡总盈亏 |
| » adl_ranking | integer | 自动减仓排名，共1-5个等级，`1` 最高，`5` 最低，特殊情况 `6` 是没有持仓或在爆仓中 |
| » pending_orders | integer | 当前未完成委托数量 |
| » close_order | object|null | 当前平仓委托信息，如果没有平仓则为`null` |
| »» id | integer(int64) | 委托ID |
| »» price | string | 委托价格 |
| »» is_liq | boolean | 是否为强制平仓 |
| » mode | string | 持仓模式。包括：  - `single`: 单向持仓模式 - `dual_long`: 双向持仓模式下的做多仓位 - `dual_short`: 双向持仓模式下的做空仓位 |
| » cross_leverage_limit | string | 全仓模式下的杠杆倍数 |
| » update_time | integer(int64) | 最后更新时间 |
| » update_id | integer(int64) | 更新id，仓位每更新一次，数值会+1 |
| » open_time | integer(int64) | 开仓时间 |
| » risk_limit_table | string | 风险限额梯度表id |
| » average_maintenance_rate | string | 平均维持保证金率 |
| » pid | integer(int64) | 分仓仓位id |
| » pos_margin_mode | string | 仓位保证金模式 isolated - 逐仓, cross - 全仓 |
| » lever | string | 表示仓位当前杠杆，逐仓和全仓都可以该字段表示 逐步替换当前的leverage和cross_leverage_limit |
```

示例（Python 标签页，公共 import 行省略）

```python
host = "https://api.gateio.ws"
prefix = "/api/v4"
headers = {'Accept': 'application/json', 'Content-Type': 'application/json'}

url = '/futures/usdt/positions/BTC_USDT/set_leverage'
query_param = 'leverage=10&margin_mode=cross'
# `gen_sign` 的实现参考认证一章
sign_headers = gen_sign('POST', prefix + url, query_param)
headers.update(sign_headers)
r = requests.request('POST', host + prefix + url + "?" + query_param, headers=headers)
print(r.json())
```

示例（Shell 标签页，与接口相关的行）

```bash
method="POST"
url="/futures/usdt/positions/BTC_USDT/set_leverage"
query_param="leverage=10&margin_mode=cross"
body_param=''

full_url="$host$prefix$url?$query_param"
curl -X $method $full_url \
    -H "Timestamp: $timestamp" -H "KEY: $key" -H "SIGN: $sign"
```

返回示例见 A.12（与其他返回 Position 的接口是同一份 JSON）。

该接口章节的正文只有上面一句说明；页面没有给这个接口写注意事项、`leverage` 取值范围、持仓或挂单限制、错误响应，「枚举值列表」里也没有 `margin_mode` / `dual_side` 的取值表。

### A.2 获取指定模式的杠杆信息（需要认证）

```
GET`/futures/{settle}/get_leverage/{contract}`

GET `/futures/{settle}/get_leverage/{contract}`

_获取指定模式的杠杆信息_

获取指定模式的杠杆信息
```

参数

| 名称 | 位置 | 类型 | 必选 | 描述 |
| --- | --- | --- | --- | --- |
| settle | URL | string | 是 | 永续合约结算货币 |
| contract | URL | string | 是 | 合约标识 |
| pos_margin_mode | 请求参数 | string | 是 | 仓位保证金模式, 分仓模式必传，取值isolated/cross。 |
| dual_side | 请求参数 | string | 是 | dual_long - 多 dual_short - 空 |

返回

| 状态码 | 含义 | 描述 | 格式 |
| --- | --- | --- | --- |
| 200 | OK | 查询杠杆成功 | FuturesLeverage |

返回格式（状态码 200，`_返回结果包含Lever字段_`，无折叠）

| 名称 | 类型 | 描述 |
| --- | --- | --- |
| » Lever | string | 杠杆 |

示例（Python 标签页，与接口相关的行）

```python
url = '/futures/usdt/get_leverage/BTC_USDT'
query_param = 'pos_margin_mode=isolated&dual_side=dual_long'
# `gen_sign` 的实现参考认证一章
sign_headers = gen_sign('GET', prefix + url, query_param)
headers.update(sign_headers)
r = requests.request('GET', host + prefix + url + "?" + query_param, headers=headers)
print(r.json())
```

示例（Shell 标签页，与接口相关的行）

```bash
method="GET"
url="/futures/usdt/get_leverage/BTC_USDT"
query_param="pos_margin_mode=isolated&dual_side=dual_long"

full_url="$host$prefix$url?$query_param"
curl -X $method $full_url \
    -H "Timestamp: $timestamp" -H "KEY: $key" -H "SIGN: $sign"
```

返回示例

```json
{
  "Lever": "10"
}
```

### A.3 更新仓位杠杆（需要认证）

```
POST`/futures/{settle}/positions/{contract}/leverage`

POST `/futures/{settle}/positions/{contract}/leverage`

_更新仓位杠杆_

〔警告〕 仓位模式切换规则：

- leverage ≠ 0：逐仓模式（无论是否填写 cross_leverage_limit，该参数都会被忽略）
- leverage = 0：全仓模式（使用 cross_leverage_limit 设置杠杆倍数）

示例：

- 设置逐仓 10 倍杠杆：leverage=10
- 设置全仓 10 倍杠杆：leverage=0&cross_leverage_limit=10
- leverage=5&cross_leverage_limit=10 → 结果为逐仓 5 倍（cross_leverage_limit 被忽略）

〔警告〕 警告：错误设置可能导致仓位模式意外切换，影响风险管理。
```

参数

| 名称 | 位置 | 类型 | 必选 | 描述 |
| --- | --- | --- | --- | --- |
| settle | URL | string | 是 | 永续合约结算货币 |
| contract | URL | string | 是 | 合约标识 |
| leverage | 请求参数 | string | 是 | 该字段用于设置逐仓的杠杆倍数，且设置逐仓杠杆倍数时需要“cross_leverage_limit”字段设置为空 |
| cross_leverage_limit | 请求参数 | string | 否 | 该字段用于设置全仓的杠杆倍数，且设置全仓杠杆倍数时需同时将“leverage”字段设置为0 |
| pid | 请求参数 | integer(int32) | 否 | 产品ID |

返回

| 状态码 | 含义 | 描述 | 格式 |
| --- | --- | --- | --- |
| 200 | OK | 仓位信息 | Position |

返回格式：状态码 200，`_合约仓位详情_`，字段表与 A.1 的返回格式表逐行相同，表后「展开剩余 32 个字段」。返回示例见 A.12。

示例（与接口相关的行）

```python
url = '/futures/usdt/positions/BTC_USDT/leverage'
query_param = 'leverage=10'
sign_headers = gen_sign('POST', prefix + url, query_param)
r = requests.request('POST', host + prefix + url + "?" + query_param, headers=headers)
```

```bash
method="POST"
url="/futures/usdt/positions/BTC_USDT/leverage"
query_param="leverage=10"
```

### A.4 切换全逐仓模式（需要认证）

```
POST`/futures/{settle}/positions/cross_mode`

POST `/futures/{settle}/positions/cross_mode`

_切换全逐仓模式_
```

正文除斜体标题外没有其他说明。

参数

| 名称 | 位置 | 类型 | 必选 | 描述 |
| --- | --- | --- | --- | --- |
| settle | URL | string | 是 | 永续合约结算货币 |
| body | body | FuturesPositionCrossMode | 是 |  |
| » mode | body | string | 是 | 全逐仓模式，ISOLATED-逐仓，CROSS-全仓 |
| » contract | body | string | 是 | 合约市场 |

返回

| 状态码 | 含义 | 描述 | 格式 |
| --- | --- | --- | --- |
| 200 | OK | 仓位信息 | Position |

返回格式：状态码 200，`_合约仓位详情_`，字段表与 A.1 逐行相同，表后「展开剩余 32 个字段」。返回示例见 A.12。

示例（Python 标签页，与接口相关的行）

```python
url = '/futures/usdt/positions/cross_mode'
query_param = ''
body='{"mode":"ISOLATED","contract":"BTC_USDT"}'
# `gen_sign` 的实现参考认证一章
sign_headers = gen_sign('POST', prefix + url, query_param, body)
headers.update(sign_headers)
r = requests.request('POST', host + prefix + url, headers=headers, data=body)
print(r.json())
```

示例（Shell 标签页，与接口相关的行）

```bash
method="POST"
url="/futures/usdt/positions/cross_mode"
query_param=""
body_param='{"mode":"ISOLATED","contract":"BTC_USDT"}'

full_url="$host$prefix$url"
curl -X $method $full_url -d "$body_param" -H "Content-Type: application/json" \
    -H "Timestamp: $timestamp" -H "KEY: $key" -H "SIGN: $sign"
```

请求体示例

```json
{
  "mode": "ISOLATED",
  "contract": "BTC_USDT"
}
```

### A.5 双仓模式下切换全逐仓模式（需要认证）

```
POST`/futures/{settle}/dual_comp/positions/cross_mode`

POST `/futures/{settle}/dual_comp/positions/cross_mode`

_双仓模式下切换全逐仓模式_
```

参数

| 名称 | 位置 | 类型 | 必选 | 描述 |
| --- | --- | --- | --- | --- |
| settle | URL | string | 是 | 永续合约结算货币 |
| body | body | UpdateDualCompPositionCrossModeRequest | 是 |  |
| » mode | body | string | 是 | 全逐仓模式，ISOLATED-逐仓，CROSS-全仓 |
| » contract | body | string | 是 | 合约市场 |

返回

| 状态码 | 含义 | 描述 | 格式 |
| --- | --- | --- | --- |
| 200 | OK | 查询成功 | [Position] |

返回格式：`_None_ | array | [合约仓位详情]`、`_None_ | Position | 合约仓位详情`，其后字段表与 A.1 逐行相同（前缀多一级 `»`），表后「展开剩余 34 个字段」。

示例与 A.4 相同，仅 `url = '/futures/usdt/dual_comp/positions/cross_mode'`（Shell：`url="/futures/usdt/dual_comp/positions/cross_mode"`）；请求体示例同为 `{"mode": "ISOLATED", "contract": "BTC_USDT"}`；返回示例是只含一个元素的数组，元素内容与 A.12 相同。

### A.6 设置持仓模式,替换dual_mode接口（需要认证）

```
POST`/futures/{settle}/set_position_mode`

POST `/futures/{settle}/set_position_mode`

_设置持仓模式,替换dual_mode接口_

变更模式的前提是，所有仓位没有持仓，并且没有挂单
```

参数

| 名称 | 位置 | 类型 | 必选 | 描述 |
| --- | --- | --- | --- | --- |
| settle | URL | string | 是 | 永续合约结算货币 |
| position_mode | 请求参数 | string | 是 | 可选值：single, dual, dual_plus，分别表示单向、双向、分仓 |

返回

| 状态码 | 含义 | 描述 | 格式 |
| --- | --- | --- | --- |
| 200 | OK | 修改成功 | FuturesAccount |

返回格式：状态码 200，字段表与 A.13 的模型表逐行相同（名称列带 `»` 前缀），表后「展开剩余 29 个字段」。返回示例见 A.13。

示例（与接口相关的行）

```python
url = '/futures/usdt/set_position_mode'
query_param = 'position_mode=dual_plus'
sign_headers = gen_sign('POST', prefix + url, query_param)
r = requests.request('POST', host + prefix + url + "?" + query_param, headers=headers)
```

```bash
method="POST"
url="/futures/usdt/set_position_mode"
query_param="position_mode=dual_plus"
```

### A.7 设置持仓模式（需要认证）

```
POST`/futures/{settle}/dual_mode`

POST `/futures/{settle}/dual_mode`

_设置持仓模式_

变更模式的前提是，所有仓位没有持仓，并且没有挂单
```

参数

| 名称 | 位置 | 类型 | 必选 | 描述 |
| --- | --- | --- | --- | --- |
| settle | URL | string | 是 | 永续合约结算货币 |
| dual_mode | 请求参数 | boolean | 是 | 是否设置为双向持仓 |

返回

| 状态码 | 含义 | 描述 | 格式 |
| --- | --- | --- | --- |
| 200 | OK | 修改成功 | FuturesAccount |

返回格式与返回示例同 A.6。

示例（与接口相关的行）

```python
url = '/futures/usdt/dual_mode'
query_param = 'dual_mode=true'
sign_headers = gen_sign('POST', prefix + url, query_param)
r = requests.request('POST', host + prefix + url + "?" + query_param, headers=headers)
```

```bash
method="POST"
url="/futures/usdt/dual_mode"
query_param="dual_mode=true"
```

### A.8 更新双仓模式下的杠杆（需要认证）

```
POST`/futures/{settle}/dual_comp/positions/{contract}/leverage`

POST `/futures/{settle}/dual_comp/positions/{contract}/leverage`

_更新双仓模式下的杠杆_
```

正文除斜体标题外没有其他说明；没有 A.3 那样的切换规则段。

参数

| 名称 | 位置 | 类型 | 必选 | 描述 |
| --- | --- | --- | --- | --- |
| settle | URL | string | 是 | 永续合约结算货币 |
| contract | URL | string | 是 | 合约标识 |
| leverage | 请求参数 | string | 是 | 新的杠杆倍数 |
| cross_leverage_limit | 请求参数 | string | 否 | 全仓模式下的杠杆倍数（即 `leverage` 为 0 时） |

返回

| 状态码 | 含义 | 描述 | 格式 |
| --- | --- | --- | --- |
| 200 | OK | 查询成功 | [Position] |

返回格式同 A.5（「展开剩余 34 个字段」）。示例：`url = '/futures/usdt/dual_comp/positions/BTC_USDT/leverage'`，`query_param = 'leverage=10'`。

### A.9 提到杠杆接口的保证金 / 风险限额接口（需要认证）

三个接口的说明正文相同：

```
在新的风险限额规则(https://www.gate.com/zh/help/futures/futures-logic/22162)下，持仓限额和杠杆倍数相关，设置越低的杠杆倍数即会获得越大的持仓限额。请使用调整杠杆倍数的接口来调整限额。
```

| 章节标题 | 方法与路径 | 参数（除 settle、contract 外） | 示例 |
| --- | --- | --- | --- |
| 更新仓位保证金 | POST `/futures/{settle}/positions/{contract}/margin` | `change` 请求参数 string 是：保证金变化数额，正数增加，负数减少 | `url = '/futures/usdt/positions/BTC_USDT/margin'`，`query_param = 'change=0.01'` |
| 更新仓位风险限额 | POST `/futures/{settle}/positions/{contract}/risk_limit` | `risk_limit` 请求参数 string 是：新的风险限额价值 | `url = '/futures/usdt/positions/BTC_USDT/risk_limit'`，`query_param = 'risk_limit=1000000'` |
| 更新双仓模式下的风险限额 | POST `/futures/{settle}/dual_comp/positions/{contract}/risk_limit` | `risk_limit` 请求参数 string 是：新的风险限额价值 | `query_param = 'risk_limit=1000000'` |

另一个保证金接口「更新双仓模式下的保证金」（POST `/futures/{settle}/dual_comp/positions/{contract}/margin`）正文为空，参数为 `change` 请求参数 string 是「保证金变化数额，正数增加，负数减少」、`dual_side` 请求参数 string 是「多头或空头仓位」，示例 `query_param = 'change=0.01&dual_side=dual_long'`。

### A.10 读取类接口

获取合约账号（需要认证）

```
GET`/futures/{settle}/accounts`

GET `/futures/{settle}/accounts`

_获取合约账号_

该接口支持查询经典合约账户和统一账户的合约业务相关信息
```

参数只有 `settle | URL | string | 是 | 永续合约结算货币`。返回「200 | OK | 查询成功 | FuturesAccount」，字段表见 A.13。

获取用户仓位列表（需要认证）

```
GET`/futures/{settle}/positions`

GET `/futures/{settle}/positions`

_获取用户仓位列表_
```

| 名称 | 位置 | 类型 | 必选 | 描述 |
| --- | --- | --- | --- | --- |
| settle | URL | string | 是 | 永续合约结算货币 |
| holding | 请求参数 | boolean | 否 | 只返回真实持仓-true,全部返回-false |
| limit | 请求参数 | integer | 否 | 列表返回的最大数量，不传时默认返回当前全部的仓位，传入时则只允许传入[1,100]的范围 |
| offset | 请求参数 | integer | 否 | 列表返回的偏移量，从 0 开始 |

返回格式同 A.5（「展开剩余 34 个字段」）。页面没有给这个接口列 `position_mode` / `pos_margin_mode` / `pid` 之类的筛选参数。

获取单个仓位信息（需要认证）

```
GET`/futures/{settle}/positions/{contract}`

GET `/futures/{settle}/positions/{contract}`

_获取单个仓位信息_

获取指定合约市场下的仓位信息。如果您在一个合约市场下持有双向的两个仓位，请使用此接口：/futures/{settle}/dual_comp/positions/{contract}
```

参数只有 `settle`、`contract | URL | string | 是 | 合约标识`。返回格式同 A.1（「展开剩余 32 个字段」）。

获取双仓模式下的持仓信息（需要认证）：GET `/futures/{settle}/dual_comp/positions/{contract}`，正文只有斜体标题，参数 `settle`、`contract`，返回 `[Position]`（「展开剩余 34 个字段」）。

根据时间获取用户历史仓位信息列表（需要认证）：GET `/futures/{settle}/positions_timerange`，参数 `settle`、`contract | 请求参数 | string | 是 | 合约标识`、`from`、`to`、`limit`、`offset`（后四个均「否」）。返回 `[PositionTimerange`，其中与本题有关的行：

| 名称 | 类型 | 描述 |
| --- | --- | --- |
| »» leverage | string | 杠杆倍数，0代表全仓，正数代表逐仓 |
| »» risk_limit | string | 风险限额 |
| »» leverage_max | string | 在当前的仓位规模下，允许可开的最大杠杆倍数。当下仓位规模越大，可开的杠杆倍数越低。 |
| »» mode | string | 持仓模式。包括： - `single`: 单向持仓模式 - `dual_long`: 双向持仓模式下的做多仓位 - `dual_short`: 双向持仓模式下的做空仓位 |
| »» cross_leverage_limit | string | 全仓模式下的杠杆倍数（即 `leverage` 为 0 时） |

### A.11 Position 模型（页尾「模型」区全文）

```
## # Position

_合约仓位详情_

### # 属性

属性

| 名称 | 类型 | 必选 | 限制 | 描述 |
| --- | --- | --- | --- | --- |
| user | integer(int64) | false | 只读 | 用户ID |
| contract | string | false | 只读 | 合约标识 |
| size | string | false | 只读 | 头寸大小 |
| hedge_status | string | false | 只读 | 中性策略下仓位的对冲状态。包括：  - `partial_hedged`: 部分对冲 - `full_hedged`: 完全对冲 |
| hedged_size | string | false | 只读 | 中性策略下已对冲的仓位数量。 |
| unhedged_size | string | false | 只读 | 中性策略下未对冲的仓位数量，计算方式为 `max(abs(size) - abs(hedged_size), 0)`。 |
| leverage | string | false | none | 逐仓模式下的杠杆倍数。如果是0，表示当下为全仓模式。全仓杠杆模式下的倍数请以“cross_leverage_limit”为准。 |
| risk_limit | string | false | none | 风险限额 |
| leverage_max | string | false | 只读 | 在当前的仓位规模下，允许可开的最大杠杆倍数。当下仓位规模越大，可开的杠杆倍数越低。 |
| maintenance_rate | string | false | 只读 | 当前仓位规模所处的那一档的风险限额的维持保证金率要求。由于仓位维持保证金已启动梯度式计算，该仓位的实际维持保证金率要求以`average_maintenance_rate`为准 |
| value | string | false | 只读 | 按结算币种标记价格计算的合约价值 |
| margin | string | false | none | 保证金 |
| entry_price | string | false | 只读 | 开仓价格 |
| liq_price | string | false | 只读 | 预估强平价，仅提供参考。实际强平触发以仓位保证金率或者账户维持保证金率为准。 |
| mark_price | string | false | 只读 | 合约当前标记价格 |
| initial_margin | string | false | 只读 | 仓位占用的起始保证金 |
| maintenance_margin | string | false | 只读 | 仓位所需的维持保证金 |
| unrealised_pnl | string | false | 只读 | 未实现盈亏 |
| realised_pnl | string | false | 只读 | 已实现盈亏，该仓位产生的所有平仓结算、资金费结算、手续费支出的资金流水之和 |
| pnl_pnl | string | false | 只读 | 已实现盈亏中的平仓结算盈亏 |
| pnl_fund | string | false | 只读 | 已实现盈亏中的资金费结算盈亏 |
| pnl_fee | string | false | 只读 | 已实现盈亏中的总手续费支出 |
| history_pnl | string | false | 只读 | 该合约市场下所有平仓结算盈亏，包括所有历史仓位 |
| last_close_pnl | string | false | 只读 | 最近一次平仓的盈亏 |
| realised_point | string | false | 只读 | 点卡已实现盈亏 |
| history_point | string | false | 只读 | 已平仓的点卡总盈亏 |
| adl_ranking | integer | false | 只读 | 自动减仓排名，共1-5个等级，`1` 最高，`5` 最低，特殊情况 `6` 是没有持仓或在爆仓中 |
| pending_orders | integer | false | 只读 | 当前未完成委托数量 |
| close_order | object|null | false | 只读 | 当前平仓委托信息，如果没有平仓则为`null` |
| » id | integer(int64) | false | none | 委托ID |
| » price | string | false | none | 委托价格 |
| » is_liq | boolean | false | none | 是否为强制平仓 |
| mode | string | false | none | 持仓模式。包括：  - `single`: 单向持仓模式 - `dual_long`: 双向持仓模式下的做多仓位 - `dual_short`: 双向持仓模式下的做空仓位 |
| cross_leverage_limit | string | false | none | 全仓模式下的杠杆倍数 |
| update_time | integer(int64) | false | 只读 | 最后更新时间 |
| update_id | integer(int64) | false | 只读 | 更新id，仓位每更新一次，数值会+1 |
| open_time | integer(int64) | false | none | 开仓时间 |
| risk_limit_table | string | false | 只读 | 风险限额梯度表id |
| average_maintenance_rate | string | false | 只读 | 平均维持保证金率 |
| pid | integer(int64) | false | 只读 | 分仓仓位id |
| pos_margin_mode | string | false | none | 仓位保证金模式 isolated - 逐仓, cross - 全仓 |
| lever | string | false | none | 表示仓位当前杠杆，逐仓和全仓都可以该字段表示 逐步替换当前的leverage和cross_leverage_limit |
```

模型区紧跟的「枚举值列表」：`hedge_status` = `partial_hedged` / `full_hedged`；`mode` = `single` / `dual_long` / `dual_short`。模型区共 39 个顶层字段，没有折叠标记。

### A.12 Position 响应示例

以下 JSON 在「获取单个仓位信息」「更新仓位保证金」「更新仓位杠杆」「更新指定模式的杠杆」「切换全逐仓模式」「更新仓位风险限额」六个接口下完全相同：

```json
{
  "user": 10000,
  "contract": "BTC_USDT",
  "size": "-9440",
  "hedge_status": "partial_hedged",
  "hedged_size": "12",
  "unhedged_size": "9428",
  "leverage": "0",
  "risk_limit": "100",
  "leverage_max": "100",
  "maintenance_rate": "0.005",
  "value": "3568.62",
  "margin": "4.431548146258",
  "entry_price": "3779.55",
  "liq_price": "99999999",
  "mark_price": "3780.32",
  "unrealised_pnl": "-0.000507486844",
  "realised_pnl": "0.045543982432",
  "pnl_pnl": "0.045543982432",
  "pnl_fund": "0",
  "pnl_fee": "0",
  "history_pnl": "0",
  "last_close_pnl": "0",
  "realised_point": "0",
  "history_point": "0",
  "adl_ranking": 5,
  "pending_orders": 16,
  "close_order": {
    "id": 232323,
    "price": "3779",
    "is_liq": false
  },
  "mode": "single",
  "update_time": 1684994406,
  "update_id": 1,
  "cross_leverage_limit": "0",
  "risk_limit_table": "BIG_HOT_COIN_50X_V2",
  "average_maintenance_rate": "0.005",
  "pos_margin_mode": "isolated",
  "lever": "30"
}
```

照录的事实：这份示例里 `leverage` 为 `"0"`（按字段描述表示全仓）、`cross_leverage_limit` 为 `"0"`，而 `pos_margin_mode` 为 `"isolated"`、`lever` 为 `"30"`，几个取值彼此对不上。

### A.13 FuturesAccount 模型（页尾「模型」区全文）与响应示例

```
## # FuturesAccount

### # 属性

属性

| 名称 | 类型 | 必选 | 限制 | 描述 |
| --- | --- | --- | --- | --- |
| total | string | false | none | 钱包余额，只适用于经典合约账户。钱包余额为所有历史已发生的资金流水之和，包括历史转入转出、平仓结算、手续费支出等，不包含仓位的未实现盈亏。total = SUM(history_dnw, history_pnl, history_fee, history_refr, history_fund) |
| unrealised_pnl | string | false | none | 未实现盈亏 |
| position_margin | string | false | none | 已废弃 |
| order_margin | string | false | none | 所有未完成订单的起始保证金 |
| available | string | false | none | 指的是逐仓可用的转出或交易的额度，统一账户下包含授信额度的逐仓可用额度(有包含体验金,体验金无法转出,所以要转出,转出金额需要扣除体验金) |
| point | string | false | none | 点卡数额 |
| currency | string | false | none | 结算币种 |
| in_dual_mode | boolean | false | none | 是否为双向持仓模式 |
| enable_credit | boolean | false | none | 是否开启统一账户模式 |
| position_initial_margin | string | false | none | 头寸占用的起始保证金，适用于统一账户模式 |
| maintenance_margin | string | false | none | 头寸占用的维持保证金，适用于新经典账户保证金模式和统一账户模式 |
| bonus | string | false | none | 体验金 |
| enable_evolved_classic | boolean | false | none | 已废弃 |
| cross_order_margin | string | false | none | 全仓挂单保证金，适用于新经典账户保证金模式 |
| cross_initial_margin | string | false | none | 全仓初始保证金，适用于新经典账户保证金模式 |
| cross_maintenance_margin | string | false | none | 全仓维持保证金，适用于新经典账户保证金模式 |
| cross_unrealised_pnl | string | false | none | 全仓未实现盈亏，适用于新经典账户保证金模式 |
| cross_available | string | false | none | 全仓可用额度，适用于新经典账户保证金模式 |
| cross_margin_balance | string | false | none | 全仓保证金余额，适用于新经典账户保证金模式 |
| cross_mmr | string | false | none | 全仓维持保证金率，适用于新经典账户保证金模式 |
| cross_imr | string | false | none | 全仓初始保证金率，适用于新经典账户保证金模式 |
| isolated_position_margin | string | false | none | 逐仓仓位保证金，适用于新经典账户保证金模式 |
| enable_new_dual_mode | boolean | false | none | 已废弃 |
| margin_mode | integer | false | none | 保证金模式，0-经典保证金模式，1-跨币种保证金模式，2-组合保证金模式，3-单币种保证金模式 |
| enable_tiered_mm | boolean | false | none | 是否开启梯度式计算维持保证金 |
| enable_dual_plus | boolean | false | none | 是否支持分仓模式 |
| position_mode | string | false | none | 持仓模式 single-单向持仓 dual-双向持仓 dual_plus-分仓 |
| history | object | false | none | 累计统计数据 |
| » dnw | string | false | none | 累计转入转出 |
| » pnl | string | false | none | 累计交易盈亏 |
| » fee | string | false | none | 累计手续费 |
| » refr | string | false | none | 累计获取的推荐人返佣 |
| » fund | string | false | none | 累计资金费用 |
| » point_dnw | string | false | none | 累计点卡转入转出 |
| » point_fee | string | false | none | 累计点卡抵扣手续费 |
| » point_refr | string | false | none | 累计获取的点卡推荐人返佣 |
| » bonus_dnw | string | false | none | 累计体验金转入转出 |
| » bonus_offset | string | false | none | 累计体验金抵扣 |
| » cross_settle | string | false | none | 代表统一账户模式下，合约账户盈利被结算到现货数值。负数代表从合约结算到现货的，正数代表从现货结算到合约的。此数值为累计值 |
```

模型区共 28 个顶层字段加 11 个 `history` 子字段，没有折叠标记。

响应示例（「获取合约账号」「设置持仓模式」「设置持仓模式,替换dual_mode接口」三个接口下相同）

```json
{
  "user": 1666,
  "currency": "USDT",
  "total": "9707.803567115145",
  "unrealised_pnl": "3371.248828",
  "position_margin": "38.712189181",
  "order_margin": "0",
  "available": "9669.091377934145",
  "point": "0",
  "bonus": "0",
  "in_dual_mode": false,
  "enable_evolved_classic": false,
  "cross_initial_margin": "61855.56788525",
  "cross_maintenance_margin": "682.04678105",
  "cross_order_margin": "0",
  "cross_unrealised_pnl": "1501.178222634128",
  "cross_available": "27549.406108813951",
  "cross_margin_balance": "10371.77306201952",
  "cross_mmr": "797.2134",
  "cross_imr": "116.6097",
  "isolated_position_margin": "0",
  "history": {
    "dnw": "10000",
    "pnl": "68.3685",
    "fee": "-1.645812875",
    "refr": "0",
    "fund": "-358.919120009855",
    "point_dnw": "0",
    "point_fee": "0",
    "point_refr": "0",
    "bonus_dnw": "0",
    "bonus_offset": "0"
  },
  "enable_tiered_mm": true,
  "position_mode": "dual_plus",
  "enable_dual_plus": true
}
```

### A.14 FuturesLeverage / FuturesPositionCrossMode / UpdateDualCompPositionCrossModeRequest 模型

```
## # FuturesLeverage

_返回结果包含Lever字段_

### # 属性

属性

| 名称 | 类型 | 必选 | 限制 | 描述 |
| --- | --- | --- | --- | --- |
| Lever | string | false | none | 杠杆 |

{
  "Lever": "string"
}
```

```
## # FuturesPositionCrossMode

_FuturesPositionCrossMode_

### # 属性

属性

| 名称 | 类型 | 必选 | 限制 | 描述 |
| --- | --- | --- | --- | --- |
| mode | string | true | none | 全逐仓模式，ISOLATED-逐仓，CROSS-全仓 |
| contract | string | true | none | 合约市场 |

{
  "mode": "string",
  "contract": "BTC_USDT"
}
```

```
## # UpdateDualCompPositionCrossModeRequest

_UpdateDualCompPositionCrossModeRequest_

### # 属性

属性

| 名称 | 类型 | 必选 | 限制 | 描述 |
| --- | --- | --- | --- | --- |
| mode | string | true | none | 全逐仓模式，ISOLATED-逐仓，CROSS-全仓 |
| contract | string | true | none | 合约市场 |

{
  "mode": "string",
  "contract": "BTC_USDT"
}
```

### A.15 其他模型里的 leverage / cross_leverage_limit / mode 字段

FuturesAutoDeleverage（查询ADL自动减仓订单信息）

| 名称 | 类型 | 必选 | 限制 | 描述 |
| --- | --- | --- | --- | --- |
| leverage | string | false | 只读 | 逐仓模式下的杠杆倍数。如果是0，表示当下为全仓模式。全仓杠杆模式下的倍数请以“cross_leverage_limit”为准。 |
| cross_leverage_limit | string | false | 只读 | 全仓模式下的杠杆倍数 |

PositionTimerange（与 A.10 逐接口表的对应行文字相同）

| 名称 | 类型 | 必选 | 限制 | 描述 |
| --- | --- | --- | --- | --- |
| leverage | string | false | none | 杠杆倍数，0代表全仓，正数代表逐仓 |
| leverage_max | string | false | 只读 | 在当前的仓位规模下，允许可开的最大杠杆倍数。当下仓位规模越大，可开的杠杆倍数越低。 |
| mode | string | false | none | 持仓模式。包括： - `single`: 单向持仓模式 - `dual_long`: 双向持仓模式下的做多仓位 - `dual_short`: 双向持仓模式下的做空仓位 |
| cross_leverage_limit | string | false | none | 全仓模式下的杠杆倍数（即 `leverage` 为 0 时） |

FuturesLiquidate（查询强制平仓历史）：`leverage | string | false | 只读 | 杠杆倍数，公共接口无该字段返回`

页面上杠杆上下限只出现在合约与风险限额对象里：Contract 的 `leverage_min`「最小杠杆」、`leverage_max`「最大杠杆」；`risk_limit_tiers`（FuturesLimitRiskTiers）与 `risk_limit_table`（FuturesRiskLimitTier）的 `leverage_max`「最大杠杆」。

## B. 「统一账户 / 跨币种 / 单币种」与杠杆、全仓/逐仓、持仓模式的关系句

来源：【文档站·中文·Futures】。Futures 分页里出现「统一账户」「跨币种」「单币种」且与保证金模式、全仓/逐仓、持仓模式有关联的句子全部如下。

| 出处 | 原文 |
| --- | --- |
| 获取合约账号，说明正文 | 该接口支持查询经典合约账户和统一账户的合约业务相关信息 |
| FuturesAccount.`total` | 钱包余额，只适用于经典合约账户。钱包余额为所有历史已发生的资金流水之和，包括历史转入转出、平仓结算、手续费支出等，不包含仓位的未实现盈亏。total = SUM(history_dnw, history_pnl, history_fee, history_refr, history_fund) |
| FuturesAccount.`available` | 指的是逐仓可用的转出或交易的额度，统一账户下包含授信额度的逐仓可用额度(有包含体验金,体验金无法转出,所以要转出,转出金额需要扣除体验金) |
| FuturesAccount.`enable_credit` | 是否开启统一账户模式 |
| FuturesAccount.`position_initial_margin` | 头寸占用的起始保证金，适用于统一账户模式 |
| FuturesAccount.`maintenance_margin` | 头寸占用的维持保证金，适用于新经典账户保证金模式和统一账户模式 |
| FuturesAccount.`cross_order_margin` / `cross_initial_margin` / `cross_maintenance_margin` / `cross_unrealised_pnl` / `cross_available` / `cross_margin_balance` / `cross_mmr` / `cross_imr` | 描述均以「适用于新经典账户保证金模式」结尾（如「全仓挂单保证金，适用于新经典账户保证金模式」） |
| FuturesAccount.`isolated_position_margin` | 逐仓仓位保证金，适用于新经典账户保证金模式 |
| FuturesAccount.`margin_mode` | 保证金模式，0-经典保证金模式，1-跨币种保证金模式，2-组合保证金模式，3-单币种保证金模式 |
| FuturesAccount.`history.cross_settle` | 代表统一账户模式下，合约账户盈利被结算到现货数值。负数代表从合约结算到现货的，正数代表从现货结算到合约的。此数值为累计值 |
| FuturesAccount.`enable_dual_plus` | 是否支持分仓模式 |
| FuturesAccount.`position_mode` | 持仓模式 single-单向持仓 dual-双向持仓 dual_plus-分仓 |
| Contract.`enable_credit`（合约详情） | 是否支持统一账户 |
| 合约交易下单的 `text` 参数，以及订单类接口的 `text` 响应字段（查询合约订单列表、批量取消状态为 open 的订单、查询单个订单详情、修改单个订单、撤销单个订单、查询合约订单列表(时间区间)、档位bbo合约下单） | 见下方代码块 |
| 合约交易下单、bbo 下单的 `pid` 参数 | 仓位ID |
| 合约交易下单的 `pos_margin_mode` 参数，查询合约订单列表 / 批量取消状态为 open 的订单 / 查询单个订单详情 / 修改单个订单 / 撤销单个订单 / 查询合约订单列表(时间区间) / 档位bbo合约下单 的 `pos_margin_mode` 响应字段 | 仓位保证金模式 isolated - 逐仓, cross - 全仓 只在简易分仓模式下传递 |
| 合约交易下单说明里的单仓 / 双仓条目 | 见下方代码块 |
| 获取指定模式的杠杆信息，`pos_margin_mode` 参数 | 仓位保证金模式, 分仓模式必传，取值isolated/cross。 |
| 设置持仓模式,替换dual_mode接口，`position_mode` 参数 | 可选值：single, dual, dual_plus，分别表示单向、双向、分仓 |
| 查询自动订单列表 / 批量取消自动订单 / 查询单个自动订单详情 的 `pos_margin_mode` 响应字段，创建价格触发订单的 `pos_margin_mode` 参数（枚举值 `isolated`、`cross`） | 仓位保证金模式：`isolated`（逐仓）、`cross`（全仓）。 在简易分仓模式下服务端会返回；写入场景下请仅使用下列取值。 |
| 创建追踪委托的 `pos_margin_mode` / `position_mode` 参数；终止追踪委托、批量终止追踪委托、获取追踪委托列表、获取追踪委托详情、修改追踪委托 的同名响应字段 | `仓位保证金模式 逐仓isolated/全仓cross`；`持仓模式 single、dual和dual_plus` |
| 追踪委托响应的 `is_dual_mode` / `leverage` / `error_label` 字段 | `创建委托时是否双向持仓`；`杠杆`；`错误标签` |
| 创建追逐限价单的 `is_dual_mode` / `pos_margin_mode` / `position_mode` 参数 | `是否为双仓模式`；`仓位保证金模式，如逐仓 isolated、全仓 cross`；`持仓模式（如 single、dual、dual_plus）` |
| 批量终止追逐限价单的 `pos_margin_mode` 参数 | 可选保证金模式 |
| 追逐限价单各接口响应里的 `is_dual_mode` / `pos_margin_mode` / `position_mode` / `leverage` / `error_label` | 字段存在，描述列为空 |

`text` 字段里与保证金模式有关的内部保留取值（「合约交易下单」详细描述一节，原文的破折号前是两个空格加半角连字符）：

```
- auto_deleveraging: 自动减仓
- liquidation: ⽼经典模式仓位强制平仓
- liq-xxx: a. 新经典模式仓位强制平仓，包含逐仓、单向全仓、双向全仓⾮对冲仓位强平。 b. 统⼀账户单币种保证金模式逐仓强制平仓
- hedge-liq-xxx: 新经典模式双向全仓对冲部分强制平仓，即同时平多空仓位
- pm_liquidate: 统⼀账户跨币种保证金模式强制平仓
- comb_margin_liquidate: 统⼀账户组合保证金模式强制平仓
- scm_liquidate: 统⼀账户单币种保证金模式仓位强制平仓
- insurance: 保险
- clear: 合约下架清退
```

「合约交易下单」说明里与持仓模式有关的条目：

```
- 单仓模式下，如果需要平仓，需要设置 `size` 为 0 ，`close` 为 `true`
- 双仓模式下，
- 减仓：reduce_only=true，size 为正数表示减空仓，负数表示减多仓
- 加仓：reduce_only=false，size 为正数表示加多仓，负数表示加空仓
- 平仓：size=0，根据 auto_size 设置平仓的方向，并同时设置 `reduce_only` 为 true
```

来源：【文档站·中文·根页】通用章节，与统一账户下的永续合约有关：

```
### # 永续合约交易

统一账户永续合约的 API 操作与经典账户完全相同，当前只支持 USDT 结算
```

```
目前统一账户支持开通`多币种保证金`模式，后续将推出`组合保证金`模式，可根据自身需求进行账户模式切换，敬请期待。
```

```
- 使用 `/api/v4/futures/**` 接口进行永续合约相关操作（下单、修改订单、查询订单等）
```

「统一帐户（旧）」一节：「统一账户（旧）永续合约的 API 操作与经典账户完全相同，不过当前只支持 USD 结算」。

未找到（B 部分）：Futures 分页和根页都没有「统一账户 / 跨币种 / 单币种 / 组合保证金模式下可以或不可以设置杠杆、可以或不可以切换全仓逐仓、支持哪些持仓模式」这类句子；也没有「带持仓或挂单时能否修改杠杆或切换全逐仓」的句子。

## C. 错误 label 与限制说明

### C.1 Futures 分页里能找到的限制说明

来源：【文档站·中文·Futures】。

| 出处 | 原文 |
| --- | --- |
| 设置持仓模式、设置持仓模式,替换dual_mode接口 | 变更模式的前提是，所有仓位没有持仓，并且没有挂单 |
| 更新仓位杠杆 | 〔警告〕仓位模式切换规则与警告句，全文见 A.3 |
| 更新仓位保证金、更新仓位风险限额、更新双仓模式下的风险限额 | 在新的风险限额规则(…)下，持仓限额和杠杆倍数相关，设置越低的杠杆倍数即会获得越大的持仓限额。请使用调整杠杆倍数的接口来调整限额。全文见 A.9 |
| 获取指定模式的杠杆信息 | `pos_margin_mode`：仓位保证金模式, 分仓模式必传，取值isolated/cross。 |
| 合约交易下单、价格触发订单等 | `pos_margin_mode`：…只在简易分仓模式下传递 / 在简易分仓模式下服务端会返回；写入场景下请仅使用下列取值。 |

Futures 分页里这些接口的「返回」表都只列了 200 一行，没有逐接口的错误响应、错误 label、限频说明，也没有给 `set_leverage` / `leverage` 的 `leverage` 参数写取值范围。更新指定模式的杠杆、切换全逐仓模式、双仓模式下切换全逐仓模式、更新双仓模式下的杠杆、获取指定模式的杠杆信息这五个接口章节没有任何注意事项。

### C.2 根页的合约相关 label

来源：【文档站·中文·根页】「异常 `label` 列表 → 合约相关」。页面没有写哪个 label 对应哪个接口，以下是整张表：

| `label` | 含义 |
| --- | --- |
| USER_NOT_FOUND | 用户无合约账户 |
| CONTRACT_NO_COUNTER | 没有匹配的对手单 |
| CONTRACT_NOT_FOUND | 合约未找到 |
| NOT_FOUND | 请求路径不存在 |
| RISK_LIMIT_EXCEEDED | 委托超出风险限额 |
| INSUFFICIENT_AVAILABLE | 余额不足 |
| LIQUIDATE_IMMEDIATELY | 操作可能导致爆仓 |
| LEVERAGE_TOO_HIGH | 杠杆倍数设置过高 |
| LEVERAGE_TOO_LOW | 杠杆倍数设置过低 |
| ORDER_NOT_FOUND | 委托不存在 |
| ORDER_NOT_OWNED | 委托不存在 |
| ORDER_FINISHED | 委托已结束 |
| TOO_MANY_ORDERS | 过多未完成的委托 |
| POSITION_CROSS_MARGIN | 全仓不支持更新保证金 |
| POSITION_IN_LIQUIDATION | 仓位在强制平仓中 |
| POSITION_IN_CLOSE | 仓位正在平仓中 |
| POSITION_EMPTY | 仓位为空 |
| REMOVE_TOO_MUCH | 保证金超过可调范围 |
| RISK_LIMIT_NOT_MULTIPLE | 风险限额未按照步长调整 |
| RISK_LIMIT_TOO_HIGH | 超出最大风险限额 |
| RISK_LIMIT_TOO_lOW | 风险限额设置过低 |
| PRICE_TOO_DEVIATED | 下单价与标记价格相差过大 |
| SIZE_TOO_LARGE | 下单数量超过上限 |
| SIZE_TOO_SMALL | 下单数量不足下限 |
| PRICE_OVER_LIQUIDATION | 增加仓位时价格不能超过平仓价 |
| PRICE_OVER_BANKRUPT | 减少仓位时价格不能超过破产价 |
| ORDER_POC_IMMEDIATE | 被动委托会立即成交 |
| INCREASE_POSITION | 只减仓委托会增加仓位 |
| CONTRACT_IN_DELISTING | 当前合约市场处于下线过渡期，只允许创建只减仓委托或者平仓委托 |
| POSITION_NOT_FOUND | 仓位不存在 |
| POSITION_DUAL_MODE | 双向持仓模式不允许此操作 |
| ORDER_PENDING | 有委托存在则不允许此操作 |
| POSITION_HOLDING | 有持仓则不允许此操作 |
| REDUCE_EXCEEDED | 双向持仓模式下，减仓单超过仓位大小 |
| NO_CHANGE | 没有改变发生 |
| AMEND_WITH_STOP | 有止盈止损单时不能修改委托 |
| ORDER_FOK | FOK 订单无法全部成交 |

「请求参数或格式问题」一组里与本题接口可能相关的行：`INVALID_PARAM_VALUE` 参数输入值无效、`INVALID_ARGUMENT` 参数无效、`MISSING_REQUIRED_PARAM` 缺少必选参数。

异常处理说明的原文：

```
- `label` 用于标识某种错误的类型，格式 `string` ，它的值是一个固定的列表（见下方）。程序处理可以使用 `label` 字段的内容来设定和捕获异常
- `message` (或 `detail`) 表示详细的错误信息，方便 API 对接时，理解具体是因为什么样的参数设置导致请求出现了异常。
  该字段内容不建议用于异常的捕获或识别。
```

常见问题里的一条：

```
- 合约操作返回 `USER_NOT_FOUND`

  原因是因为合约账户没有开户，只需要执行一笔账户划转操作即可。注意不同结算币种有不同的合约账户
```

### C.3 根页的限频、权限与传参规则

来源：【文档站·中文·根页】。

限频规则表里「永续合约」一行：

| 市场 | 入口 | 限速 | 依据 | 包含 |
| --- | --- | --- | --- | --- |
| 永续合约 | 私有接口 | 合约批量/单个下单/修改单个订单接口一共 100r/s  合约批量/单个撤单接口一共 200r/s  永续合约其他单个接口 200r/10s | UID | 合约下单、撤单  成交历史、费率查询等 |

```
> 限速是每个子账号或主账号单独计算的

__限流字段__

每次请求API响应头(response header)中都会包含如下字段：

- X-Gate-RateLimit-Requests-Remain - 该接口当前时间窗口剩余可用请求数
- X-Gate-RateLimit-Limit - 该接口当前频率限制上限
- X-Gate-RateLimit-Reset-Timestamp - 如果您已超过该接口当前窗口频率限制，该字段表示下个可用时间窗口的时间戳（秒），即什么时候可以恢复访问；如果您未超过该接口当前窗口频率限制，该字段表示返回的是当前服务器时间（秒).
```

HTTP 通用格式里与本题接口的传参方式有关的原文：

```
- 所有读操作都是 `GET` 方法，只接受请求参数，不读取任何请求体。
- 更新操作使用 `POST`, `PUT` 或 `PATCH` 方法，不同的请求有不同的参数传递方式，
  但是他们要么是通过请求体，要么是请求参数，不会二者混用。
```

APIv4 权限表里的一行：

| 产品 | 权限 |
| --- | --- |
| `永续合约` | `只读`查询订单 `读写`查询订单&下单 |

其后原文：「所有请求方法为 `GET` 的都是读操作，其他的则是写请求。每个权限组可以设置为禁用、只读或读写。」

## D. 根页 changelog 相关条目

来源：【文档站·中文·根页】。版本号后是根页给出的日期；写「根页未标注日期」的是根页该版本条目下没有日期行。

### D.1 直接命中本题的条目

`v4.106.12`（根页未标注日期）—— 引入 `get_leverage`、`set_leverage`、`set_position_mode`、`lever`、`pos_margin_mode`、`FuturesLeverage` 的版本，整条照录：

```
- 新增 `GET /earn/dual/balance` 接口，查询双币理财资产
- 新增 `GET /futures/{settle}/get_leverage/{contract}` 接口，获取指定模式的杠杆信息
- 新增 `POST /futures/{settle}/positions/{contract}/set_leverage` 接口，更新指定模式的杠杆，简化leverage接口复杂逻辑
- 新增 `POST /futures/{settle}/set_position_mode` 接口，设置持仓模式，替换dual_mode接口
- `POST /futures/{settle}/positions/{contract}/reverse` 接口，新增 `pos_margin_mode` 参数（仓位保证金模式，分仓模式必传，取值isolated/cross）
- `FuturesAccount` 模型，新增 `enable_dual_plus` 字段，表示是否支持分仓模式
- `FuturesAccount` 模型，`position_mode` 字段描述更新，明确持仓模式：single-单向持仓，dual-双向持仓，dual_plus-分仓
- `FuturesOrder` 模型，新增 `pos_margin_mode` 字段，表示仓位保证金模式（isolated - 逐仓, cross - 全仓，只在简易分仓模式下传递）
- `Position` 模型，新增 `pos_margin_mode` 字段，表示仓位保证金模式（isolated - 逐仓, cross - 全仓）
- `Position` 模型，新增 `lever` 字段，表示仓位当前杠杆，逐仓和全仓都可以该字段表示，逐步替换当前的leverage和cross_leverage_limit
- 新增 `FuturesLeverage` 模型定义，用于杠杆查询响应
```

`v4.106.83`（2026-05-18）

```
- __`PUT /futures/{settle}/price_orders/amend`__ 不再使用路径参数 `order_id`，请在请求体 JSON 中使用 __`order_id`__ 指定待修改的自动订单（其他语义不变）
- 补充 __`pos_margin_mode`__（仓位保证金模式），取值 __`isolated`__（逐仓）__ / __`cross`__（全仓）__；移除 enum 中无效的中文占位项
```

`v4.106.43`（2026-03-19）

```
- 合约价格触发委托（期货自动下单）接口：文档中响应里的订单标识字段增补 __`format: int64`__ 说明；全站复用的查询参数 __`pos_margin_mode`__ 与 __`dual_side`__ 在引用场景中改为必填要求
```

`v4.106.20`（2026-02-02），该条目里与本题有关的一条（该条目另有追踪委托、资金费率等无关条目，未录）：

```
- `FuturesAccount` 模型：删除 `cross_settle` 字段
```

`v4.106.3`（根页未标注日期），与本题有关的条目（该版本另有期权、交割、杠杆账户等无关条目，未录）：

```
- `GET /futures/{settle}/accounts` 接口说明更新，支持查询经典合约账户和统一账户
- `PUT /futures/{settle}/accounts` 接口，`enable_evolved_classic` 字段标记为已废弃
- `GET /futures/{settle}/positions/{contract}` 接口说明更新，明确在同一合约市场下持有双向仓位时的查询方式
- `POST /futures/{settle}/positions/{contract}/margin` 接口说明更新，附加新风险限额规则链接，说明应使用杠杆调整接口而非直接修改保证金
- `POST /futures/{settle}/positions/{contract}/leverage` 接口参数说明更新，明确 `leverage` 参数（用于逐仓，需要 `cross_leverage_limit` 为空）和 `cross_leverage_limit` 参数（用于全仓，需要 `leverage` 设为 0）
- `POST /futures/{settle}/dual_comp/positions/{contract}/risk_limit` 接口说明更新，附加风险限额规则链接
- `Position` 模型字段描述更新，优化杠杆和风险管理相关说明：`leverage`（逐仓杠杆倍数，0 表示全仓模式）、`leverage_max`（基于当前仓位规模的最大杠杆）、`maintenance_rate`（梯度式维持保证金率计算）、`liq_price`（预估强平价，仅供参考）、`initial_margin` 和 `maintenance_margin`（扩大适用范围说明）、`realised_pnl`（详细说明包含平仓结算、资金费和手续费）、`pnl_pnl`（平仓结算盈亏）、`pnl_fund`（资金费盈亏）、`pnl_fee`（总手续费）、`history_pnl`（所有历史平仓结算盈亏）、`cross_leverage_limit`（简化说明）
- `FuturesAccount` 和 `DeliveryAccount` 模型更新：`total` 字段更新说明"仅适用于经典合约账户"，`position_margin` 标记为已废弃，`order_margin` 更新为"所有未完成订单的起始保证金"，`enable_evolved_classic` 和 `enable_new_dual_mode` 标记为已废弃，`margin_mode` 枚举值扩展新增值 3（单币种保证金模式）
- `PositionTimerange` 模型更新：`leverage_max` 和 `maintenance_rate` 描述优化
- `FuturesAutoDeleverage` 模型更新：`leverage` 和 `cross_leverage_limit` 描述明确，更好理解保证金模式
```

`v4.106.1`（根页未标注日期）

```
- `Position` 和 `FuturesAutoDeleverage` 模型中 `leverage` 和 `cross_leverage_limit` 字段描述更新，明确全仓/逐仓模式下的使用说明
- `POST /futures/{settle}/positions/{contract}/leverage` 接口中 `leverage` 和 `cross_leverage_limit` 参数描述更新，明确使用要求
- `PUT /sub_accounts/{user_id}/keys/{key}` 接口说明更新，提示此接口无法修改 `mode` 账户类型属性
```

`v4.106.0`（根页未标注日期），与 Position 有关的条目：

```
- 为支持小数张数，Futures 相关字段在 OpenAPI 和 SDK 模型中使用 `string` 类型。REST 请求携带 `X-Gate-Size-Decimal: 1` 时，响应以 JSON 字符串返回这些字段。直接发送 HTTP 请求时，将请求头设为 `0` 或省略该头，响应返回截去小数部分的整数。受影响的模型包括：`FuturesOrder`（`size`、`iceberg`、`left`）、`BatchFuturesOrder`（`size`、`iceberg`、`left`）、`Position`（`size`、`trade_long_size`、`trade_short_size`）、`PositionTimerange`（`size`）、`MyFuturesTrade`（`size`、`close_size`）、`MyFuturesTradeTimeRange`（`size`、`close_size`）、`FuturesTrade`（`size`）、`FuturesOrderBook`（`asks` 和 `bids` 中的 `size`）、`FuturesCandlestick`（`v`）、`FuturesLiquidate`（`size`、`order_size`）、`FuturesLiqOrder`（`position_size`、`size`、`order_size`）、`FuturesAutoDeleverage`（`size`、`entry_size`）、`FuturesCollusionOrder`（`size`、`left`）、`FuturesBatchAmendOrderRequest`（`size`）、`Contract`（`order_size_min`、`order_size_max`、`trade_size`、`position_size`）、`ContractStat`（`long_liq_size`、`short_liq_size`、`open_interest`）
- `Position` 模型中新增 `pid` 字段，用于分仓仓位 ID
- Go、C#、Java、Python、PHP 和 TypeScript SDK 默认发送 `X-Gate-Size-Decimal: 1` 请求头。
```

`v4.105.32`（根页未标注日期）

```
- `POST /futures/{settle}/positions/{contract}/leverage` 接口描述更新，新增仓位模式切换规则详解、使用示例和风险警告，提升接口使用的安全性和清晰度
```

`v4.105.8`（根页未标注日期）

```
- `GET /unified/accounts` 接口，返回值增加 `margin_mode` 字段，标识账户保证金模式
```

`v4.105.0`（根页未标注日期）

```
- `POST /futures/{settle}/positions/mode` 接口，请求参数新增 `position_mode` 字段，设置持仓模式
```

`v4.97.0`（根页未标注日期）

```
- 新增 `POST /futures/{settle}/positions/cross_mode` 接口, 切换全逐仓模式
```

`v4.88.0`（2024-12-24）

```
- `PUT /unified/unified_mode` 接口，新增单币种保证金模式
```

`v4.85.0`（2024-11-11）

```
- `POST /futures/{settle}/dual_mode`
  接口，返回增加 `cross_order_margin`、`cross_initial_margin`、`cross_maintenance_margin`、`cross_unrealised_pnl`、`cross_available`、`isolated_position_margin`
  字段
```

`v4.23.3`（2022-04-01）

```
4. 永续合约双仓模式下增加缺失的全仓杠杆 `cross_leverage_limit` 参数
```

`v4.22.2`（2021-09-29）

```
2. 合约下单新增 `auto_size` 只写字段用于双向持仓模式的平仓操作
```

`v4.21.2`（2021-06-07）

```
1. 合约 API 新增对全仓杠杆调整的支持
```

`v4.16.1`（2020-12-10）

```
- 修复 Position 模型定义里的错误字段名称 `dual_mode` 。正确应该是 `mode`
```

`v4.16.0`（2020-12-09）「合约」部分：

```
- 永续合约新增双仓支持 API ，`/futures/{settle}/dual_mode` 接口设置是否开启双仓。
  双仓相关的仓位操作参照 `/futures/{settle}/dual_comp/positions` 接口组
- 永续合约账户返回新增 `in_dual_mode` ，仓位返回新增 `dual_mode`
```

`v4.91.0`（2025-02-10），根页该条目下只有这一句，没有列出接口名：

```
`2025-04-01` 之后我们将移除以下接口，请尽快迁移至新接口
```

### D.2 边缘命中（字面含关键字，或含全仓/逐仓/持仓模式，但不是本题的接口）

`v4.106.82`（2026-05-14），追逐限价单接口，整条照录：

```
- 合约新增 __追逐限价单（Chase Limit Order）__ 接口，均在 `/futures/{settle}/autoorder/v1/chase/*` 路径下：
  - __`POST /futures/{settle}/autoorder/v1/chase/create`__（`createChaseOrder`）—— 创建追逐限价单，支持 `contract`、`amount`、`price_limit`（或 `offset_limit`）、可选 `reduce_only`、`text`、`is_dual_mode`、`price_type` / `price_gap_type` / `price_gap_value`、`pos_margin_mode`、`position_mode`
  - __`POST /futures/{settle}/autoorder/v1/chase/stop`__（`stopChaseOrder`）—— 终止指定追逐限价单，按 `id` 或 `text` 任一确定委托
  - __`POST /futures/{settle}/autoorder/v1/chase/stop_all`__（`stopAllChaseOrders`）—— 批量终止追逐限价单，可按 `contract`、`pos_margin_mode` 过滤
  - __`GET /futures/{settle}/autoorder/v1/chase/list`__（`getChaseOrders`）—— 获取追逐限价单列表，支持 `contract`、`is_finished`、`start_at` / `end_at`、`page_num` / `page_size`、必填 `sort_by`（1 `ORDER_SORT_CREATED_AT`，2 `ORDER_SORT_FINISHED_AT`）、`hide_cancel`、`reduce_only`、`side` 等筛选
  - __`GET /futures/{settle}/autoorder/v1/chase/detail`__（`getChaseOrderDetail`）—— 按 `id` 获取追逐限价单详情
- REST 文档为追逐限价单补充结构化请求与响应说明：`CreateChaseOrderReq`、`CreateChaseOrderResp`、`StopChaseOrderReq`、`StopChaseOrderResp`、`StopAllChaseOrdersReq`、`StopAllChaseOrdersResp`、`GetChaseOrdersResp`、`GetChaseOrderDetailResp`，以及列表/详情条目 `ChaseOrder`（含 `chase_price`、`interval_sec`、`suborder_*`、`price_type`、`price_gap_type`、`price_gap_value` 等字段）
```

其余边缘命中条目：

| 版本（日期） | 条目原文 |
| --- | --- |
| `v4.106.14`（根页未标注日期） | `Order` 模型，`text` 字段描述更新，补充强平订单场景说明（`pm_liquidate`、`comb_margin_liquidate`、`scm_liquidate` 这三种场景代表全仓强平订单，`liquidate` 代表逐仓强平订单） |
| `v4.106.45`（2026-03-20）、`v4.106.42`（2026-03-19） | `UnifiedAccount` 模型：新增 `mode` 字段（统一账户模式：经典账户/跨币种保证金/组合保证金/单币种保证金） |
| `v4.106.109`（2026-07-22） | 新增 __`POST /unified/leverage/user_setting`__（`setUserLeverage`）接口，用于设置用户所有借贷币种的杠杆倍数；存在借贷的币种不可修改，超出币种杠杆阈值时按阈值设置，响应返回设置失败的币种及原因 |
| `v4.106.97`（2026-06-17） | 新增 __`POST /unified/delta_neutral`__（`setUnifiedDeltaNeutral`）—— 设置账户 Delta 中性策略模式；条件：VIP 等级 ≥ 4 且账户处于跨币种保证金模式；请求体与响应体为 `DeltaNeutralEnabled`，含必填 __`enabled`__（布尔）字段 |
| `v4.106.157`（2026-10-14）、`v4.106.131`（2026-08-24） | __`GET /crossex/history_positions`__（`listCrossexHistoryPositions`）响应项新增可选字段 __`margin_mode`__（`CROSS`/`ISOLATED`）；__`POST /crossex/positions/margin`__（`updateCrossexPositionsMargin`）响应模型 `CrossexIsolatedMarginResponse` 新增可选字段 __`position_side`__（`NONE`/`LONG`/`SHORT`） |
| `v4.106.130`（2026-08-22） | __`GET /crossex/rule/symbols`__（`listCrossexRuleSymbols`）响应模型 `Symbol` 新增可选字符串字段 __`support_cross`__（`"true"`/`"false"`），用于标识交易对是否支持全仓下单 |
| `v4.106.171`（2026-10-30，股票接口） | `GET /stock/symbols` 和 `GET /stock/symbols/detail` 新增响应字段 `margin_mode`（整数：`0`、`1`、`2` 或 `3`），以及字符串字段 `long_im`、`long_mm`、`short_im`、`short_mm`、`long_borrowing_rate` 和 `short_borrowing_rate`。借贷利率为空字符串表示暂无有效利率。 |
| `v4.106.138`（2026-09-10，TradFi 接口） | __`POST /tradfi/orders`__（`createTradFiOrder`）的请求体 `TradFiOrderRequest` 新增可选整数字段 `leverage`，用于指定订单杠杆倍数。 |
| `v4.105.4`（根页未标注日期） | `GET /unified/accounts` 接口，返回值增加 `cross_leverage` 字段，显示全仓杠杆倍数 |
| `v4.105.3`（根页未标注日期） | `GET /futures/{settle}/positions` 接口，查询参数新增 `position_side` 字段，支持对冲模式 |
| `v4.104.6`（根页未标注日期） | `GET /futures/{settle}/positions` 接口，返回值增加 `isolated_margin` 字段，显示逐仓保证金 |
| `v4.104.3`（根页未标注日期） | `GET /futures/{settle}/positions` 接口，查询参数新增 `hedge_mode` 字段，支持对冲模式查询 |
| `v4.104.3`（根页未标注日期） | `GET /unified/accounts` 接口，返回值增加 `cross_margin_leverage` 字段，显示全仓杠杆倍数 |
| `v4.96.0`（根页未标注日期） | `GET /futures/{settle}/accounts` 接口, 返回值新增`cross_margin_balance`,`cross_mmr`,`cross_imr`字段 |
| `v4.83.0`（2024-10-28） | 新增 `GET /unified/leverage/user_currency_config` 接口, 查询用户最大、最小可设置币种杠杆倍数 |
| `v4.83.0`（2024-10-28） | 新增 `GET /unified/leverage/user_currency_setting` 接口, 获取用户币种杠杆倍数 |
| `v4.83.0`（2024-10-28） | 新增 `POST /unified/leverage/user_currency_setting` 接口, 设置币种杠杆倍数 |
| `v4.83.0`（2024-10-28） | `GET /unified/currency_discount_tiers` 接口，返回值增加 `leverage` 字段 |

changelog 里的日期分布：`v4.106.184` 到 `v4.106.15` 都有日期；`v4.106.14` 到 `v4.94.0` 都没有日期；`v4.92.1`、`v4.92.0`、`v4.91.0` 有日期；`v4.88.0` 到 `v4.51.0` 之间只有 `v4.87.0` 没有日期。根页任何地方都没有写 `set_leverage` 上线的日期。

## E. Agent 文档页

来源：【文档站·中文·Agent】。

### E.1 Futures Trading 工具表（整表）

标题「Futures Trading（需要 OAuth2 认证）」：

| 工具名 | 说明 |
| --- | --- |
| cex_fx_create_fx_order | 创建合约订单 |
| cex_fx_cancel_fx_order | 取消合约订单 |
| cex_fx_amend_fx_order | 修改合约订单 |
| cex_fx_create_fx_batch_orders | 批量创建订单 |
| cex_fx_cancel_fx_batch_orders | 批量取消订单 |
| cex_fx_amend_fx_batch_orders | 批量修改订单 |
| cex_fx_get_fx_order | 查询单个订单 |
| cex_fx_list_fx_orders | 查询订单列表 |
| cex_fx_list_fx_my_trades | 查询成交记录 |
| cex_fx_get_fx_position | 查询单向持仓 |
| cex_fx_list_fx_positions | 查询所有持仓 |
| cex_fx_get_fx_dual_position | 查询双向持仓 |
| cex_fx_update_fx_position_leverage | 调整杠杆 |
| cex_fx_update_fx_position_margin | 调整保证金 |
| cex_fx_set_fx_dual | 设置双向持仓模式 |
| cex_fx_create_fx_price_triggered_order | 创建条件单 |
| cex_fx_create_trail_order | 创建追踪止损单 |
| cex_fx_create_fx_bbo_order | 创建 BBO 订单 |
| cex_fx_get_fx_accounts | 查询合约账户 |

表后原文：「注意：所有类别包括但不局限于以上 Tools，详见 Gate MCP (opens new window)。」

永续合约行情表里与杠杆档位有关的一行：`cex_fx_list_fx_risk_limit_tiers | 风险限额档位`。Account 表里与账户模式有关的一行：`cex_unified_get_unified_mode | 账户模式`。

端点表里的一行：`/mcp/exchange | 400+ | OAuth2 | CEX 全功能交易（现货/合约/期权/闪兑/跟单/子账户等）`。

### E.2 Skills 表里的相关行

| Skill 名称 | 说明 | 认证 |
| --- | --- | --- |
| gate-exchange-futures | USDT 永续合约：开平仓、条件单、止盈止损 | OAuth2 |
| gate-exchange-assets | 资产与余额查询：总资产、现货、合约、杠杆、期权、理财、Alpha、TradFi | OAuth2 |
| gate-exchange-assets-manager | 多账户资产总览、保证金风险、SimpleEarn 收益、联盟佣金、统一账户借贷与杠杆配置 | OAuth2 |
| gate-exchange-unified | 统一账户操作：资产权益、借贷额度、借还款、模式切换、杠杆与抵押配置 | OAuth2 |

### E.3 用例 4：合约开仓

| 项目 | 内容 |
| --- | --- |
| __认证要求__ | OAuth2 |
| __用户提示词__ | BTC 永续做多 1 张 10x |
| __端点__ | `/mcp/exchange` |
| __工具 / Skill__ | gate-exchange-futures Skill |

__执行流程：__（原文每步行尾有两个空格的 Markdown 换行，未保留）

```
1. 解析意图：合约 = BTC_USDT，方向 = 做多（buy），数量 = 1 张，杠杆 = 10x
2. 调用 cex_fx_get_fx_contract 确认合约参数（最小下单量、面值、维持保证金率）
3. 调用 cex_fx_update_fx_position_leverage 设置杠杆倍数为 10x
4. 调用 cex_fx_get_fx_tickers 获取当前标记价格，计算所需保证金
5. 向用户展示订单预览（预估开仓价、保证金、强平价），等待确认
6. 用户确认后调用 cex_fx_create_fx_order 执行开多
7. 查询持仓确认开仓成功，输出持仓详情
```

__预期结果：__

```
成功开仓 BTC_USDT 永续多单 1 张，杠杆 10x，输出开仓均价、已用保证金、预估强平价。
```

### E.4 CLI 与限速

原文：「现货、合约等 CEX 相关子命令已统一挂在 `gate-cli cex` 下（例如 `gate-cli cex spot ...`、`gate-cli cex futures ...`）。可用 `gate-cli cex -h` 查看完整子命令树。」

「交易操作（需要授权）」代码块里与合约有关的一行：

```bash
# 查询合约持仓
gate-cli cex futures position list --settle usdt
```

常见问题：

```
__Q: 有速率限制吗？__
A: 行情端点遵循 Gate 公开 API 速率限制（通常 300 次/秒）。交易端点按用户账户维度限速，与 API 交易一致。遇到 429 错误时 Agent 会自动重试。
```

OAuth2 Scopes 表里的一行：`trade | 现货与合约交易 | cex_spot_create_spot_order、cex_fx_create_fx_order`。

未找到（E 部分）：Agent 页没有 `cex_fx_update_fx_position_leverage` 的参数说明、对应的 REST 路径、全仓/逐仓的选择方式；没有切换全仓/逐仓的工具；没有杠杆相关的 CLI 命令；页面上没有出现「全仓」「逐仓」字样。

## F. SDK 英文对照（Go SDK v7.2.184，仅作对照，不是文档站中文原文）

对照材料：`/tmp/gate-sdk-w4/` 下的 `api_futures_excerpt.go`、`doc_Position.md`、`doc_FuturesAccount.md`、`doc_FuturesLeverage.md`、`doc_FuturesPositionCrossMode.md`、`model_position.go`、`model_futures_account.go`、`model_futures_leverage.go`、`model_futures_position_cross_mode.go`。`api_futures_excerpt.go` 自身有被裁掉的片段（第 172–173、572–573、713–714 行处跳行）。

| 对照项 | 文档站中文 | SDK 英文 | 结论 |
| --- | --- | --- | --- |
| `set_leverage` 路径与方法 | POST `/futures/{settle}/positions/{contract}/set_leverage` | `localVarPath := ... + "/futures/{settle}/positions/{contract}/set_leverage"`，`http.MethodPost` | 一致 |
| `set_leverage` 参数与必填 | `leverage` 是、`margin_mode` 是、`dual_side` 否 | `UpdateContractPositionLeverage(ctx, settle, contract, leverage, marginMode string, localVarOptionals *UpdateContractPositionLeverageOpts)`，Opts 只有 `DualSide optional.String` | 一致 |
| `set_leverage` 说明 | 为了简化leverage接口复杂逻辑，新增修改杠杆的接口 | `To simplify the complex logic of the leverage interface, added a new interface for modifying leverage` | 一致；两边都没有注意事项段 |
| `set_leverage` 参数描述 | 仓位杠杆倍数 / 保证金模式 isolated/cross / dual_long - 多 dual_short - 空 | `Position Leverage Multiple` / `Margin Mode isolated/cross` / `dual_long - Long, dual_short - Short` | 一致 |
| `get_leverage` 参数与必填 | `pos_margin_mode` 是、`dual_side` 是 | `GetLeverage(ctx, settle, contract, posMarginMode, dualSide string)`，两个都是必填形参 | 一致 |
| `get_leverage` 的 `pos_margin_mode` 描述 | 仓位保证金模式, 分仓模式必传，取值isolated/cross。 | `Position Margin Mode, required for split position mode, values: isolated/cross.` | 一致 |
| `FuturesLeverage` | `Lever | string | 杠杆`，示例 `{"Lever": "10"}` | `Lever string json:"Lever,omitempty"`，注释 `Leverage` | 一致，JSON 键是首字母大写的 `Lever` |
| `leverage` 接口参数 | `leverage` 是、`cross_leverage_limit` 否、`pid` 否（integer(int32)） | `leverage string` 必填，`UpdatePositionLeverageOpts{CrossLeverageLimit optional.String; Pid optional.Int32}` | 一致 |
| `leverage` 接口说明 | A.3 的切换规则与警告 | 注释开头同样是一段 `Position Mode Switching Rules` 加 `Warning` 句，条目与示例和 A.3 对应（`leverage &#x3D; 0: Cross Margin Mode (Use cross_leverage_limit to set the leverage multiple)` 等） | 内容对应 |
| `positions/cross_mode` | body `FuturesPositionCrossMode`，`mode` 是、`contract` 是 | `UpdatePositionCrossMode(ctx, settle, futuresPositionCrossMode FuturesPositionCrossMode)`；`Mode string json:"mode"`、`Contract string json:"contract"` | 字段一致；SDK 结构体没有必填或枚举约束 |
| `dual_comp/positions/cross_mode` | body `UpdateDualCompPositionCrossModeRequest`，返回 `[Position]` | `UpdateDualCompPositionCrossMode(ctx, settle, updateDualCompPositionCrossModeRequest UpdateDualCompPositionCrossModeRequest) ([]Position, ...)` | 路径与返回类型一致；请求结构体不在摘录里，字段未能对照 |
| `set_position_mode` | 见 A.6 | `SetPositionMode(ctx, settle, positionMode string)`；`Set Position Holding Mode, replacing the dual_mode interface`；`The prerequisite for changing mode is that all positions have no holdings and no pending orders`；`Optional Values: single, dual, dual_plus, representing Single Direction, Dual Direction, Split Position respectively` | 一致 |
| `dual_mode`、`dual_comp/positions/{contract}/leverage` | 见 A.7、A.8 | 摘录未包含 | 无法对照 |
| Position 字段集 | 页尾模型表 39 个顶层字段 | `model_position.go` 39 个属性 | 一致 |
| Position.`leverage` | 逐仓模式下的杠杆倍数。如果是0，表示当下为全仓模式。全仓杠杆模式下的倍数请以“cross_leverage_limit”为准。 | `leverage for isolated margin. 0 means cross margin. For leverage of cross margin, please refer to cross_leverage_limit.` | 一致 |
| Position.`cross_leverage_limit` | 全仓模式下的杠杆倍数 | `leverage for cross margin` | 一致 |
| Position.`lever` | 表示仓位当前杠杆，逐仓和全仓都可以该字段表示 逐步替换当前的leverage和cross_leverage_limit | `Indicates the current leverage of the position, applicable to both isolated and cross margin, gradually replacing the current leverage and cross_leverage_limit` | 一致 |
| Position.`pos_margin_mode` | 仓位保证金模式 isolated - 逐仓, cross - 全仓 | `Position Margin Mode isolated - Isolated Margin, cross - Cross Margin` | 一致 |
| Position.`mode` | 持仓模式。包括：`single`: 单向持仓模式、`dual_long`: 双向持仓模式下的做多仓位、`dual_short`: 双向持仓模式下的做空仓位 | `Position mode, including: single: One-way Mode - dual_long: Long position in Hedge Mode - dual_short: Short position in Hedge Mode` | 一致 |
| Position.`leverage_max` / `risk_limit` | 见 A.11 | `the maximum permissible leverage given to the current positon value: the higher positon value, the lower maximum permissible leverage` / `Position risk limit` | 一致 |
| Position.`pid` | integer(int64)，只读，分仓仓位id | `Pid int64`，`Split-position ID.` | 一致 |
| FuturesAccount 字段集 | 页尾模型表 28 个顶层字段 | `model_futures_account.go` 28 个属性 | 字段集一致 |
| FuturesAccount.`margin_mode` | 保证金模式，0-经典保证金模式，1-跨币种保证金模式，2-组合保证金模式，3-单币种保证金模式 | `Margin mode of the account 0: classic future account or Classic Spot Margin Mode of unified account; 1: Multi-Currency Margin Mode; 2: Portoforlio Margin Mode; 3: Single-Currency Margin Mode` | 有出入：英文对 0 的说明更具体，中文只写「经典保证金模式」 |
| FuturesAccount.`enable_credit` | 是否开启统一账户模式 | `Whether portfolio margin account mode is enabled` | 有出入：中文写「统一账户模式」，英文写 `portfolio margin account mode` |
| FuturesAccount.`position_mode` | 持仓模式 single-单向持仓 dual-双向持仓 dual_plus-分仓 | `Position Holding Mode single - Single Direction Position, dual - Dual Direction Position, dual_plus - Split Position` | 一致 |
| FuturesAccount.`in_dual_mode` | 是否为双向持仓模式 | `Whether Hedge Mode is enabled` | 一致 |
| FuturesAccount.`enable_dual_plus` | 是否支持分仓模式 | `Whether to Support Split Position Mode` | 一致 |
| FuturesAccount.`enable_new_dual_mode` | 已废弃 | `Deprecated` | 一致 |
| FuturesAccount.`history` 子字段 | 页面列了 11 个子字段，含 `cross_settle` | 摘录未包含 `FuturesAccountHistory` 的定义 | 未能对照 |
| 逐接口响应表折叠的字段 | Position「展开剩余 32 / 34 个字段」，FuturesAccount「展开剩余 29 个字段」 | SDK 两个模型分别是 39 和 28 个属性，数目与文档站页尾模型表相同 | 折叠部分内容未知，SDK 模型里没有多出来的字段 |

## 未找到的项

1. `set_leverage` 的注意事项、`leverage` 取值范围、持仓或挂单限制、错误响应：未找到。找过【文档站·中文·Futures】全文、【文档站·中文·根页】changelog 与 label 表、【文档站·中文·Agent】。
2. 任何页面上的 `set_leverage` / `leverage` 的 `leverage` 参数取值范围（最小值、最大值、是否整数）：未找到。页面只有 Contract 的 `leverage_min` / `leverage_max` 和风险限额档位的 `leverage_max`。
3. 带持仓或挂单时能否修改杠杆或切换全逐仓（`set_leverage`、`leverage`、`cross_mode`）：未找到。只有持仓模式切换（`dual_mode`、`set_position_mode`）写了「变更模式的前提是，所有仓位没有持仓，并且没有挂单」。
4. `set_leverage` 的 `dual_side` 何时不传：未找到说明。`set_leverage` 页面把它标为「否」，`get_leverage` 把它标为「是」；根页 changelog `v4.106.43` 写「全站复用的查询参数 `pos_margin_mode` 与 `dual_side` 在引用场景中改为必填要求」，与 `set_leverage` 页面上的「否」并存。
5. 统一账户、跨币种、单币种、组合保证金模式下对杠杆设置、全仓/逐仓切换、持仓模式的限制或支持：未找到。Futures 分页只有 B 表里那些字段描述。
6. Futures 分页里逐接口的错误 label、4xx 响应、限频：未找到。只有根页的通用 label 表和限频表（C.2、C.3）。
7. `POST /futures/{settle}/positions/{contract}/reverse`（changelog `v4.106.12` 提到）和 `POST /futures/{settle}/positions/mode`（changelog `v4.105.0` 提到）：在 Futures 分页的接口章节和页尾模型区都未找到。
8. `v4.106.12`（引入 `set_leverage` 等的版本）的日期：根页未标注。
9. Agent 页上 `cex_fx_update_fx_position_leverage` 的参数、对应 REST 路径、全仓/逐仓选择方式，以及全仓/逐仓切换工具、杠杆 CLI 命令：未找到。
10. Position、FuturesAccount 逐接口响应表里折叠的字段内容：未取到。`trade_long_size`、`trade_short_size`（changelog `v4.106.0` 提到属于 `Position`）不在页尾 Position 模型表里，也不在 SDK 的 `model_position.go` 里。
11. SDK 摘录里没有 `dual_mode`、`dual_comp/positions/{contract}/leverage`、`FuturesAccountHistory`、`UpdateDualCompPositionCrossModeRequest` 的定义，这几项未能做 SDK 对照。

## 抓取异常

无。三个页面都在第一次请求成功，没有重试。

需要知道的事实：

- Futures 分页与根页 changelog 有不同步的先例：分页里「修改单个自动订单」写的是 `PUT /futures/{settle}/price_orders/amend`，根页 changelog `v4.106.160`（2026-10-16）写的是「由 `PUT /futures/{settle}/price_orders/amend` 改为 `PUT /futures/{settle}/price_orders/amend/{order_id}`」。这与本题无关，只说明分页可能滞后于 changelog。
- 根页 changelog `v4.106.20` 写「`FuturesAccount` 模型：删除 `cross_settle` 字段」，Futures 分页页尾的 FuturesAccount 模型表仍然列着 `» cross_settle`。
- Agent 页正文里有一段面向 AI 客户端的一键安装指令文本（让客户端输入一句话自动配置 Gate Skills 和 MCP），以及 CLI 安装命令。它们是页面正文，本次没有执行，也没有摘录。

## G. 实测补充（2026-11-24，非文档原文，仅用于核对文档说法）

交易子账户，USDT 永续。只读探测用宿主机上的 Python 脚本（签名请求），写操作见任务 `probe/`。
以下结论都只在本账户上验证过，标注了适用范围。

### G.1 账户与持仓设置条目（只读）

`GET /futures/usdt/accounts`：`position_mode=single`、`margin_mode=1`（跨币种保证金）、`in_dual_mode=false`、
`enable_credit=true`、`available=107.49769`、`total≈0`。账户内无持仓、无未完结委托、无未触发条件单。

`GET /futures/usdt/positions` 返回 3 个条目，都是 `size=0`、`mode=single`：

| 合约 | `pos_margin_mode` | `leverage` | `cross_leverage_limit` | `lever` | `leverage_max` |
|---|---|---|---|---|---|
| BTC_USDT | cross | 0 | 10 | 10 | 200 |
| DOGE_USDT | cross | 0 | 75 | 75 | 75 |
| ETH_USDT | cross | 0 | 200 | 200 | 200 |

DOGE_USDT、ETH_USDT 的条目是子任务 4 的 W2/W3 开仓留下的：条目的 `update_time` 是 2026-11-24T09:23:38Z
与 09:23:45Z，与 W2/W3 的运行时刻吻合；当时的脚本传的是杠杆 0，没有调用任何杠杆设置接口
（BTC_USDT 的 `update_time` 是 2026-11-23T19:37:32Z，对应用户手动 BTC 持仓平掉的时刻）。
也就是说，本账户下首次建仓的合约在没有显式设置时落到「全仓 + 该合约的最大杠杆」。这是从这两条读数
得出的结论，不是文档说法；合约元数据里的 `cross_leverage_default=10` 没有体现在这两条读数上。合约元数据实测：BTC `leverage_min=1`、`leverage_max=200`、`cross_leverage_default=10`；
DOGE 1 / 75 / 10；ETH 1 / 200 / 10。

`GET /futures/usdt/positions/{contract}`：有条目的合约返回 200；从未设置过的合约（SOL_USDT、LTC_USDT、
XRP_USDT、ADA_USDT）返回 **HTTP 400**、`Content-Type: application/json`、`{"label":"POSITION_NOT_FOUND"}`。

### G.2 `get_leverage/{contract}` 在单向持仓模式下的行为（只读）

| 查询串 | 结果 |
|---|---|
| （无参数） | 400 `INVALID_PARAM_VALUE`：`Invalid request parameter 'pos_margin_mode' value: ` |
| `pos_margin_mode=cross` 或 `isolated` | 400 `INVALID_PARAM_VALUE`：`Invalid request parameter 'dual_side' value: ` |
| `pos_margin_mode=&dual_side=` | 400，先报 `pos_margin_mode` |
| `pos_margin_mode=cross&dual_side=dual_long` | 200 `{"Lever":"10"}` |

带齐两个参数才返回 200，但 BTC、DOGE、ETH 三个合约都返回 `10`，与各自条目的 `lever`（10、75、200）不一致。
页面把 `dual_side` 标为必选（A.2），实测也确实必填；它的返回值在单向持仓模式下不反映该合约的设置。
本任务不用它读回。

### G.3 W4：`set_leverage` 真实设置并读回

完整证据见任务 `probe/README.md` 与 `probe/livecheck-2026-11-24.jsonl`。对象 DOGE_USDT，结束后还原为全仓 75x。

1. `POST /futures/usdt/positions/DOGE_USDT/set_leverage?leverage=10&margin_mode=cross` **不带 `dual_side`**，
   在单向持仓模式下返回 200。返回体是 Position，带 `mode: "single"`、`pos_margin_mode`、`leverage`、
   `cross_leverage_limit`、`lever`。——「未找到的项 4」（`dual_side` 何时不传）在单向持仓模式下有了答案；
   双向与分仓模式下是否必填没有测。
2. 字段形态：

   | 设置 | `pos_margin_mode` | `leverage` | `cross_leverage_limit` | `lever` |
   |---|---|---|---|---|
   | 全仓 10x | cross | 0 | 10 | 10 |
   | 全仓 5x | cross | 0 | 5 | 5 |
   | 逐仓 5x | isolated | 5 | 5 | 5 |
   | 逐仓 3x | isolated | 3 | **5** | 3 |
   | `SetLeverage` 10x（保持逐仓） | isolated | 10 | **5** | 10 |
   | 切回全仓 | cross | 0 | 10 | 10 |

   逐仓时 `cross_leverage_limit` 残留上一次全仓的值。A.11 的字段描述没有提这一点；`lever` 在两种模式下
   都是当前杠杆，与 A.11 / D.1 对 `lever` 的描述一致。
3. 重复设置同一组值（全仓 10x 再设一次）返回 200，不报 `NO_CHANGE`。
4. 全仓 100x（超过 DOGE_USDT 的 `leverage_max=75`）被拒：`label: LEVERAGE_EXCEEDED`、`message: limit [1, 75]`，
   条目不变。**`LEVERAGE_EXCEEDED` 不在 C.2 的 label 表里**，表里相近的只有 `LEVERAGE_TOO_HIGH` / `LEVERAGE_TOO_LOW`。
   message 里的范围下限是 1。文档没有写 `set_leverage` 的取值范围（「未找到的项 2」），这是实测得到的边界。
5. 账户处于跨币种保证金模式（`margin_mode=1`）时，逐仓可以设置并读回。单币种模式下没有测。

### G.4 仍然没有答案的

- 带持仓或有挂单时 `set_leverage` 是否被拒（label 表里的 `POSITION_HOLDING`、`ORDER_PENDING` 是否用于这个接口）：
  没有测。W4 只在未持仓的合约上做。
- 单币种保证金模式下的读回：没有测（账户切换需要 unified 权限或由用户在交易所侧操作）。
