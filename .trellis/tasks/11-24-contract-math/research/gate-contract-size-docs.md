# Research: Gate 合约张数相关文档原文

- Query: 合约张数、最小步长、小数张请求头、最小下单金额的官方文档依据
- Scope: external
- Date: 2026-11-24

摘录约定：

- 表格与引文均为原样摘录，未改写、未概括。
- 来源分四类并分开标注：【文档站·中文】【SDK 文档·英文】【公告】【实测】。SDK 英文原文不冒充文档站中文原文。
- 「未找到」= 在「来源与抓取方法」列出的已抓取页面与文件中通读未见，不代表 Gate 全部资料中不存在。

## 来源与抓取方法

| 代号 | 类别 | 地址 | 抓取方法 | 结果与截断情况 |
|---|---|---|---|---|
| ZH-ROOT | 文档站·中文 | https://www.gate.com/docs/developers/apiv4/zh_CN/ | 网页阅读器 `mcp__web_reader__webReader`，`return_format=markdown`，`timeout=60` | 成功。页面版本「v4.106.184 · Stable」，最新 changelog 条目 v4.106.184（2026-11-18）。返回内容为 changelog 与通用章节（匹配机制、HTTP 通用格式、限频、异常 label、认证、常见问题），止于「常见问题」，未见任何接口章节。无法区分是被阅读器截断，还是 v4.106.72 起「按 Tag 拆分展示」后根页本身不含接口章节。 |
| ZH-FUT | 文档站·中文 | https://www.gate.com/docs/developers/apiv4/zh_CN/futures | 同上，`timeout=90` | 成功。从「查询所有的合约信息」到模型 `StopChaseOrderReq`。接口的响应字段表有折叠行（合约列表接口显示「展开剩余 40 个字段」，单合约接口显示「展开剩余 38 个字段」），折叠内容未被展开；页尾「模型」章节的 Contract 表无折叠标记，列出 48 个字段。 |
| ZH-AGENT | 文档站·中文（Agent） | https://www.gate.com/docs/developers/agent/zh_CN | 同上 + `no_cache=true` | 第 3 次尝试才成功（前两次返回 MCP -500）。页面完整，末尾为「相关链接」与「Last Updated: 4/27/2026, 1:28:42 AM」。 |
| EN-SDK | SDK 文档·英文 | https://raw.githubusercontent.com/gate/gateapi-go/v7.2.184/ 下的 `docs/FuturesApi.md`、`docs/Contract.md`、`docs/FuturesInitialOrder.md`、`docs/FuturesPriceTriggeredOrder.md`、`docs/FuturesOrder.md`、`model_contract.go`、`model_futures_initial_order.go`、`model_futures_price_triggered_order.go`、`model_futures_order.go` | `curl` 直连 GitHub raw（本机可达，HTTP 200）。本机缓存 `/Users/hsikey/Trading/nofx/scratch/gate-sdk-docs/` 中除 `docs/Contract.md` 外的同名文件，已用 `cmp` 与上游逐字节比对，全部一致；`docs/Contract.md` 缓存中没有，单独从上游取到临时目录。 | 成功，无截断。SDK tag 为 `v7.2.184`。 |
| EN-ANN | 公告·英文 | https://www.gate.com/announcements/article/48788 | 网页阅读器，`timeout=90` | 成功。标题「Gate Announcement on USDT Perpetual Contract Order Quantity Field Type Changes and Feature Rollout Plan」，署名「Gate Team December 18, 2025」。该链接由 EN-SDK `GetFuturesContract` 的说明给出。 |
| ZH-ANN | 公告·中文 | https://www.gate.com/zh/announcements/article/50325 | 网页阅读器，`timeout=90` | 成功。标题「永续合约市场张数精度调整公告」，发布时间 2026-03-20 13:57 (UTC+8)。该链接由 EN-SDK `GetFuturesContract` 的说明给出。 |
| LIVE | 实测（非文档） | `GET https://api.gateio.ws/api/v4/futures/usdt/contracts/{contract}` | `curl`，公开只读，无鉴权 | 成功。仅用于核对文档说法，见「实测补充」。 |

补充说明：

1. 任务给出的 Agent 地址 `https://www.gate.com/docs/developer/agent/zh_CN`（`developer` 单数）实际是 404 页（标题「404」，正文「Sorry, page not found」），首次访问返回 MCP -500。可用地址是 `https://www.gate.com/docs/developers/agent/zh_CN`（`developers` 复数），且加 `no_cache=true` 后才返回正文。
2. 本机 `curl https://www.gate.com/...` 复现「Connection reset by peer」，文档站只能经网页阅读器抓取；`raw.githubusercontent.com` 与 `api.gateio.ws` 本机 `curl` 可达。
3. `https://www.gate.com/docs/developers/apiv4/zh_CN/#futures` 未单独抓取。它与 ZH-ROOT 是同一文档，ZH-ROOT 的返回在接口章节之前已结束，接口内容改由 ZH-FUT 取得。
4. SDK 与文档站的版本对应关系，EN-SDK `README.md` 原文：「**the MAJOR version will be incremented, but the MINOR and PATCH version are still following REST API's」。
5. 本机缓存中的 `README.md`（SDK 根 README）只列出模型链接（如 `- [Contract](docs/Contract.md)`），不含 Contract 字段说明，也未出现 `X-Gate-Size-Decimal`。

## A. Contract 模型字段

### A.1 【文档站·中文】ZH-FUT「模型 › Contract」（合约详情）

同一份字段表出现在 `GET /futures/{settle}/contracts`、`GET /futures/{settle}/contracts_all`、`GET /futures/{settle}/contracts/{contract}` 三处。

任务点名的 11 个字段原文：

| 字段 | 类型 | 描述原文 |
|---|---|---|
| quanto_multiplier | string | 合约乘数，表示一张合约的面值为多少个标的物币种 |
| order_size_min | string | 最小下单数量 |
| order_size_max | string | 最大下单数量 |
| enable_decimal | boolean | 是否支持小数字符串类型合约张数。当该字段为 `true` 时，表示该合约支持小数张数（即 `size` 字段可以使用小数字符串类型）；当为 `false` 时，表示该合约不支持小数张数（即 `size` 字段只能使用整数类型） |
| order_price_round | string | 委托价格最小单位 |
| mark_price_round | string | 标记价格的最小单位 |
| order_price_deviate | string | 下单价与当前标记价格允许的正负偏移量， 即下单价 `order_price` 需满足如下条件:   abs(order_price - mark_price) <= mark_price * order_price_deviate |
| market_order_size_max | string | 合约市价下单支持的最大张数，默认值为0，为默认值时取`order_size_max`字段作为最大张数限制 |
| market_order_slip_ratio | string | 合约市价下单支持的最大滑点比率，比率计算以市场最新价格为基准 |
| leverage_min | string | 最小杠杆 |
| leverage_max | string | 最大杠杆 |

Contract 模型完整字段列表，共 48 个，按页面顺序。「必选」列 48 个字段均为 `false`，「限制」列均为 `none`，下表省略这两列。

| 字段 | 类型 | 描述原文 |
|---|---|---|
| name | string | 合约标识 |
| type | string | 合约类型, inverse - 反向合约, direct - 正向合约 |
| quanto_multiplier | string | 合约乘数，表示一张合约的面值为多少个标的物币种 |
| leverage_min | string | 最小杠杆 |
| leverage_max | string | 最大杠杆 |
| maintenance_rate | string | 风险限额的第一档维持保证金率要求 |
| mark_type | string | 已废弃 |
| mark_price | string | 当前标记价格 |
| index_price | string | 当前指数价格 |
| last_price | string | 上一次成交价格 |
| maker_fee_rate | string | 挂单成交的手续费率，负数代表返还后续费 |
| taker_fee_rate | string | 吃单成交的手续费率 |
| order_price_round | string | 委托价格最小单位 |
| mark_price_round | string | 标记价格的最小单位 |
| funding_rate | string | 当前资金费率 |
| funding_interval | integer | 资金费率应用间隔，以秒为单位 |
| funding_next_apply | number(double) | 下次资金费率应用时间 |
| risk_limit_base | string | 基础风险限额,已废弃 |
| interest_rate | string | 永续合约资金费率及溢价相关计算中使用的利率参数。以字符串表示的小数比率（如 `0.0003`），与 `funding_rate` 等同为比率而非百分数。 |
| risk_limit_step | string | 风险限额调整步长,已废弃 |
| risk_limit_max | string | 合约允许的最大风险限额,已废弃,建议使用/futures/{settle}/risk_limit_tiers来查询风险限额 |
| order_size_min | string | 最小下单数量 |
| enable_decimal | boolean | 是否支持小数字符串类型合约张数。当该字段为 `true` 时，表示该合约支持小数张数（即 `size` 字段可以使用小数字符串类型）；当为 `false` 时，表示该合约不支持小数张数（即 `size` 字段只能使用整数类型） |
| order_size_max | string | 最大下单数量 |
| order_price_deviate | string | 下单价与当前标记价格允许的正负偏移量， 即下单价 `order_price` 需满足如下条件:   abs(order_price - mark_price) <= mark_price * order_price_deviate |
| ref_discount_rate | string | 被推荐人享受交易费率折扣 |
| ref_rebate_rate | string | 推荐人享受交易费率返佣比例 |
| orderbook_id | integer(int64) | orderbook更新ID |
| trade_id | integer(int64) | 当前成交ID |
| trade_size | string | 历史累计成交 |
| position_size | string | 当前做多用户持有仓位总和 |
| config_change_time | number(double) | 配置最后更新时间 |
| in_delisting | boolean | `in_delisting=true` 并且position_size>0时候 表示该合约处于下线过渡期 `in_delisting=true`` 并且position_size=0时候 表示该合约处于下线状态 |
| orders_limit | integer | 最多挂单数量 |
| enable_bonus | boolean | 是否支持体验金 |
| enable_credit | boolean | 是否支持统一账户 |
| create_time | number(double) | 表示合约的创建时间 |
| funding_cap_ratio | string | 已废弃 |
| status | string | 合约状态 类型包含：prelaunch（预上线）, trading（交易中）,delisting（下架中）, delisted（已下架）, circuit_breaker（熔断) |
| launch_time | integer(int64) | 合约开盘时间 |
| delisting_time | integer(int64) | 合约进入只减仓状态时间 |
| delisted_time | integer(int64) | 合约下架时间 |
| market_order_slip_ratio | string | 合约市价下单支持的最大滑点比率，比率计算以市场最新价格为基准 |
| market_order_size_max | string | 合约市价下单支持的最大张数，默认值为0，为默认值时取`order_size_max`字段作为最大张数限制 |
| funding_rate_limit | string | 资金费率上限值 |
| contract_type | string | 合约分类类型，如 stocks-股票, metals-金属, indices-指数, forex-外汇, commodities-大宗商品等 |
| funding_impact_value | string | 资金费用深度影响额 |
| enable_circuit_breaker | boolean | 新开盘的合约是否启动标记价格熔断（如果平台要对某个新开盘的合约市场启动该机制以避免开盘后价格发生大幅波动导致过多爆仓，会提前发公告告知） |

枚举值：`type` 取 `inverse`、`direct`；`mark_type` 取 `internal`、`index`。

ZH-FUT 的返回示例（BTC_USDT）中与张数有关的示例值：`"quanto_multiplier": "0.0001"`、`"order_size_min": "1"`、`"enable_decimal": false`、`"order_size_max": "1000000"`、`"market_order_size_max": "0"`。这是文档里的静态示例，不是实时值。

### A.2 【SDK 文档·英文】EN-SDK `docs/Contract.md` 与 `model_contract.go`，同样 48 个字段

任务点名字段的英文原文（取自 `model_contract.go` 字段注释，与 `docs/Contract.md` 描述列一致）：

| 字段 | 英文原文 |
|---|---|
| QuantoMultiplier | The contract multiplier indicates how many units of the underlying asset the face value of one contract represents. |
| OrderSizeMin | Minimum order size in contracts. With `X-Gate-Size-Decimal: 1`, this field is a string that preserves the fractional part. With the header set to `0` or omitted, it is an integer with the fractional part discarded. A minimum below 1 is returned as 0 in this mode. |
| EnableDecimal | Whether the contract supports fractional contract sizes. If `true`, use `X-Gate-Size-Decimal: 1` to submit a fractional size. If `false`, the contract supports whole contract sizes only. |
| OrderSizeMax | Maximum order size in contracts. With `X-Gate-Size-Decimal: 1`, this field is a string that preserves the fractional part. With the header set to `0` or omitted, it is an integer with the fractional part discarded. |
| OrderPriceRound | Minimum order price increment |
| MarkPriceRound | Minimum mark price increment |
| OrderPriceDeviate | Maximum allowed deviation between order price and current mark price. The order price `order_price` must satisfy the following condition:      abs(order_price - mark_price) <= mark_price * order_price_deviate |
| MarketOrderSizeMax | The maximum number of contracts supported for market orders, with a default value of 0. When the default value is used, the maximum number of contracts is limited by the `order_size_max` field |
| MarketOrderSlipRatio | The maximum slippage allowed for market orders, with the slippage rate calculated based on the latest market price |
| LeverageMin | Minimum leverage |
| LeverageMax | Maximum leverage |

48 个字段名（Go 结构体 JSON tag，与 A.1 的 48 个一一对应）：`name type quanto_multiplier leverage_min leverage_max maintenance_rate mark_type mark_price index_price last_price maker_fee_rate taker_fee_rate order_price_round mark_price_round funding_rate funding_interval funding_next_apply risk_limit_base interest_rate risk_limit_step risk_limit_max order_size_min enable_decimal order_size_max order_price_deviate ref_discount_rate ref_rebate_rate orderbook_id trade_id trade_size position_size config_change_time in_delisting orders_limit enable_bonus enable_credit create_time funding_cap_ratio status launch_time delisting_time delisted_time market_order_slip_ratio market_order_size_max funding_rate_limit contract_type funding_impact_value enable_circuit_breaker`。SDK 仓库 `master` 分支的 `model_contract.go` 也是同样 48 个字段。

中英文差异（同一字段，中文文档站描述比 SDK 英文短）：

- `order_size_min`：中文只有「最小下单数量」；英文有「in contracts」以及不带请求头时「A minimum below 1 is returned as 0 in this mode」。
- `enable_decimal`：中文只说明 `size` 能否用小数；英文多出「If `true`, use `X-Gate-Size-Decimal: 1` to submit a fractional size.」

### A.3 是否存在 `order_size_min` 之外的「张数步长 / 张数精度」字段

不存在，依据有三条：

1. A.1 的 48 个字段里，名称带 step 或 round 的只有三个，都不是张数步长：`order_price_round`「委托价格最小单位」、`mark_price_round`「标记价格的最小单位」、`risk_limit_step`「风险限额调整步长,已废弃」。没有 `order_size_round`、`size_precision`、`order_size_step` 之类的字段。
2. EN-SDK `docs/Contract.md` 对 multiple、increment、step、precision、notional、minimum order 做关键词检索，共命中 4 行：`OrderPriceRound`「Minimum order price increment」、`MarkPriceRound`「Minimum mark price increment」、`RiskLimitStep`「Risk limit adjustment step (deprecated)」，以及 `OrderSizeMin`（因「Minimum order size in contracts」命中，全文见 A.2）。没有张数步长字段。
3. 【实测】LIVE 两次响应各有 53 个键，与 48 字段比对：多出 7 个未在模型中的键 `cross_leverage_default`、`funding_offset`、`funding_rate_indicative`、`is_pre_market`、`long_users`、`short_users`、`voucher_leverage`；少 2 个 `delisted_time`、`delisting_time`。多出的 7 个里没有张数步长或精度字段。其中 `funding_rate_indicative`、`funding_offset`、`short_users`、`long_users` 出现在 ZH-FUT 的示例响应里，但不在字段表中。

ZH-FUT 响应表里的「展开剩余 40 个字段」不是隐藏的 schema 字段数的证据：同一页 Position 模型显示「展开剩余 34 个字段」，而 SDK 的 `model_position.go` 只有 39 个 JSON 字段、页面已列出 37 个。

### A.4 【文档站·中文】ZH-ROOT changelog 中涉及 Contract 的条目原文

- v4.106.19（2026-01-27）：「`Contract` 模型，新增 `enable_decimal` 字段，用于标识是否支持小数字符串类型合约张数。当该字段为 `true` 时，表示该合约支持小数张数（即 `size` 字段可以使用小数字符串类型）；当为 `false` 时，表示该合约不支持小数张数（即 `size` 字段只能使用整数类型）」
- v4.106.8 与 v4.106.7（两条内容相同，均无日期）：「`Contract` 模型，新增 `market_order_slip_ratio` 字段，表示合约市价下单支持的最大滑点比率，比率计算以市场最新价格为基准」「`Contract` 模型，新增 `market_order_size_max` 字段，表示合约市价下单支持的最大张数，默认值为0，为默认值时取`order_size_max`字段作为最大张数限制」
- v4.106.3：「`Contract` 和 `DeliveryContract` 模型更新：`quanto_multiplier` 概念更新为"合约乘数"，`maintenance_rate` 明确为"第一档维持保证金率要求"，`mark_type` 和 `funding_cap_ratio` 标记为已废弃，`mark_price_round` 简化为"标记价格的最小单位"」
- v4.19.2（2021-03-01）：「增加合约价值的计算方法说明，具体参考 `Contract` 模型的描述」

最后一条所说的「计算方法说明」，在当前 ZH-FUT 的 Contract 模型描述里未找到公式，`quanto_multiplier` 的描述只有 A.1 那一句。

## B. 合约查询接口与 `X-Gate-Size-Decimal`

### B.1 【文档站·中文】ZH-FUT 两个接口的说明

`GET /futures/{settle}/contracts`，说明文字只有「查询所有的合约信息」：

| 名称 | 位置 | 类型 | 必选 | 描述 |
|---|---|---|---|---|
| settle | URL | string | 是 | 永续合约结算货币 |
| limit | 请求参数 | integer | 否 | 列表返回的最大数量 |
| offset | 请求参数 | integer | 否 | 列表返回的偏移量，从 0 开始 |

`GET /futures/{settle}/contracts/{contract}`，说明文字只有「查询单个合约信息」：

| 名称 | 位置 | 类型 | 必选 | 描述 |
|---|---|---|---|---|
| settle | URL | string | 是 | 永续合约结算货币 |
| contract | URL | string | 是 | 合约标识 |

两个接口均标注「该请求不需要认证」，`settle` 枚举值为 `btc`、`usdt`、`usd1`。

`X-Gate-Size-Decimal` 在 ZH-FUT 两个接口的说明与参数表里：未找到。ZH-FUT 整页通读也未出现该请求头。

### B.2 【文档站·中文】ZH-ROOT changelog v4.106.0 原文

> 为支持小数张数，Futures 相关字段在 OpenAPI 和 SDK 模型中使用 `string` 类型。REST 请求携带 `X-Gate-Size-Decimal: 1` 时，响应以 JSON 字符串返回这些字段。直接发送 HTTP 请求时，将请求头设为 `0` 或省略该头，响应返回截去小数部分的整数。受影响的模型包括：`FuturesOrder`（`size`、`iceberg`、`left`）、`BatchFuturesOrder`（`size`、`iceberg`、`left`）、`Position`（`size`、`trade_long_size`、`trade_short_size`）、`PositionTimerange`（`size`）、`MyFuturesTrade`（`size`、`close_size`）、`MyFuturesTradeTimeRange`（`size`、`close_size`）、`FuturesTrade`（`size`）、`FuturesOrderBook`（`asks` 和 `bids` 中的 `size`）、`FuturesCandlestick`（`v`）、`FuturesLiquidate`（`size`、`order_size`）、`FuturesLiqOrder`（`position_size`、`size`、`order_size`）、`FuturesAutoDeleverage`（`size`、`entry_size`）、`FuturesCollusionOrder`（`size`、`left`）、`FuturesBatchAmendOrderRequest`（`size`）、`Contract`（`order_size_min`、`order_size_max`、`trade_size`、`position_size`）、`ContractStat`（`long_liq_size`、`short_liq_size`、`open_interest`）
>
> Go、C#、Java、Python、PHP 和 TypeScript SDK 默认发送 `X-Gate-Size-Decimal: 1` 请求头。

### B.3 【SDK 文档·英文】EN-SDK `docs/FuturesApi.md`

`ListFuturesContracts`（对应 `GET /futures/{settle}/contracts`）：

> Send `X-Gate-Size-Decimal: 1` to read the actual order size limits for contracts that support fractional sizes. Check `enable_decimal` for each contract.  For example, `ETH_USDT` returns its `order_size_min` as the string `"0.1"` with `X-Gate-Size-Decimal: 1`. Omitting the header or setting it to `0` returns the integer `0` after discarding the fractional part. The actual minimum order size remains `0.1` contracts.

`GetFuturesContract`（对应 `GET /futures/{settle}/contracts/{contract}`）：

> Send `X-Gate-Size-Decimal: 1` to read the actual order size limits. For `ETH_USDT`, `order_size_min` is the string `"0.1"`. Omitting the header or setting it to `0` returns the integer `0`. The actual minimum order size remains `0.1` contracts.  Check the target contract's `enable_decimal` value for fractional size support. See the [API contract size compatibility rules](https://www.gate.com/announcements/article/48788) and the [market size precision adjustment announcement](https://www.gate.com/announcements/article/50325).

`ListFuturesContractsAll`（对应 `GET /futures/{settle}/contracts_all`，说明为「Query all contract information (including delisted)」）：没有 `X-Gate-Size-Decimal` 的说明。

### B.4 【公告】EN-ANN，https://www.gate.com/announcements/article/48788

请求头规则表（原文单元格内的换行被阅读器压平）：

| Header Parameter | Description | Remarks |
|---|---|---|
| X-Gate-Size-Decimal = 1 | size-related field types change from Integer to String | 1. Supports receiving decimal data. |
| X-Gate-Size-Decimal = 0 Or, not using this header field By default, this field is unused. | size-related field types remain Integer | 1. Does not support receiving decimal data. 2. Precision handling - Truncates towards zero (downwards). |

> If the X-Gate-Size-Decimal header is not set, the size field will continue to accept and return the Integer type by default.

1.3 节 REST 字段表中合约接口那一行：

| Interface Type | Endpoint | Field | Remarks |
|---|---|---|---|
| Public | 1. Query All Futures Info GET /futures/settle/contracts 2. Query Single Future Info GET /futures/settle/contracts/contract | Minimum order size allowed by the contract 'order_size_min' Maximum order size allowed by the contract 'order_size_max' Historical cumulative trading volume 'trade_size' Current total long position size 'position_size' | 1. Using header field X-Gate-Size-Decimal=1 , the type of the above fields will change from Integer to String . 2. Clients need to adjust the acceptance type for these fields. |

1.4 节「Querying Future Information Supporting Decimal Order Placement」：

> 1.Set Header X-Gate-Size-Decimal=1
> 2.Request the Contract Information endpoint
>
> ```
> curl --location 'https://api.gateio.ws/api/v4/futures/usdt/contracts' \
> --header 'X-Gate-Size-Decimal: 1'
> ```
>
> 3.Return Field order_size_min: If the value is a decimal, it indicates that the corresponding futures support decimal order placement.

REST 设置请求头的示例：

```
# Set request headers
headers =
{
'KEY': API_KEY,
'Timestamp': timestamp,
'SIGN': signature,
# Enable decimal size
'X-Gate-Size-Decimal': '1',
'Content-Type': 'application/json',
'Accept': 'application/json'
}
```

同一公告里与这个请求头后续走向有关的原文（Phase 3）：

> 1. All contract trading pairs will fully support fractional lot order placement.
> 2. At a specified time, the system will default to returning String type and adjust the default value of the header (X-Gate-Size-Decimal), switching the default parsing behavior from 0 to 1 to provide a short transition period for users who have not yet completed adaptation.
> 3. After the transition period, the X-Gate-Size-Decimal header field will be officially deprecated, and its configuration will no longer take effect.
> 4. All order quantity-related fields will uniformly use the String type to ensure precision and compatibility. The legacy Integer type will no longer be supported.
> 5. The specific effective date will be announced in advance via official notice. Please ensure all adaptations are completed beforehand.

Phase 3 是否已生效，在已抓取的资料里无法确认。ZH-ROOT changelog 截至 2026-11-18 没有相关条目，EN-SDK 的描述仍以「省略请求头返回整数 0」为前提，LIVE 实测也是这一行为（见「实测补充」）。

## C. 创建合约订单 `POST /futures/{settle}/orders`

### C.1 【文档站·中文】ZH-FUT「合约交易下单」

接口说明原文：

> - 下单时指定的是合约张数 `size` ，而非币的数量，每一张合约对应的币的数量是合约详情接口里返回的 `quanto_multiplier`
> - 0 成交的订单在撤单 10 分钟之后无法再获取到，会提到订单不存在
> - 设置 `reduce_only` 为 `true` 可以防止在减仓的时候穿仓
> - 单仓模式下，如果需要平仓，需要设置 `size` 为 0 ，`close` 为 `true`
> - 双仓模式下，
> - 减仓：reduce_only=true，size 为正数表示减空仓，负数表示减多仓
> - 加仓：reduce_only=false，size 为正数表示加多仓，负数表示加空仓
> - 平仓：size=0，根据 auto_size 设置平仓的方向，并同时设置 `reduce_only` 为 true
> - reduce_only：确保只执行减仓操作，防止增加仓位
> - 设置 `stp_act` 决定使用限制用户自成交的策略，详细用法参考body参数`stp_act`

参数表（请求体模型为 `FuturesOrder`；`text`、`stp_act`、`action_mode`、四个 `tpsl_*` 字段与张数无关，未摘录）：

| 名称 | 位置 | 类型 | 必选 | 描述 |
|---|---|---|---|---|
| x-gate-exptime | 请求头部 | string | 否 | 指定过期时间(毫秒); 如果 Gate 收到请求的时间大于过期时间, 请求将被拒绝 |
| » contract | body | string | 是 | 合约标识 |
| » size | body | string | 是 | 必选。交易数量，正数为买入，负数为卖出。平仓委托则设置为0。 |
| » iceberg | body | string | 否 | 冰山委托显示数量。0为完全不隐藏。注意，隐藏部分成交按照taker收取手续费。 |
| » price | body | string | 是 | 必选。委托价，价格为0并且`tif`为`ioc`，代表市价委托。 |
| » close | body | boolean | 否 | 设置为 true 的时候执行平仓操作，并且`size`应设置为0 |
| » reduce_only | body | boolean | 否 | 设置为 true 的时候，为只减仓委托 |
| » tif | body | string | 否 | Time in force 策略，市价单当前只支持 ioc 模式  - gtc: GoodTillCancelled - ioc: ImmediateOrCancelled，立即成交或者取消，只吃单不挂单 - poc: PendingOrCancelled，被动委托，只挂单不吃单 - fok: FillOrKill, 完全成交，或者完全取消 |
| » auto_size | body | string | 否 | 双仓模式下用于设置平仓的方向，`close_long` 平多头， `close_short` 平空头，需要同时设置 `size` 为 0 |
| » pid | body | integer(int64) | 否 | 仓位ID |
| » market_order_slip_ratio | body | string | 否 | 市价下单自定义最大滑点比率，不传和未返回表示使用合约默认配置 |
| » pos_margin_mode | body | string | 否 | 仓位保证金模式 isolated - 逐仓, cross - 全仓 只在简易分仓模式下传递 |
| settle | URL | string | 是 | 永续合约结算货币 |

枚举值：`tif` 为 `gtc`、`ioc`、`poc`、`fok`；`auto_size` 为 `close_long`、`close_short`。接口标注「该请求需要 API key 和 secret 认证」。

返回模型 `FuturesOrder` 中与方向、只减仓有关的原文：

- `size`：「必选。交易数量，正数为买入，负数为卖出。平仓委托则设置为0。」
- `is_close`：「是否为平仓委托。对应请求中的`close`。」
- `is_reduce_only`：「是否为只减仓委托。对应请求中的`reduce_only`。」
- `finish_as` 的枚举之一：「reduce_only: 增持仓位撤销，因为设置reduce_only或平仓」

请求体示例中的 `size` 是字符串：`"size":"6024"`。

`X-Gate-Size-Decimal` 在 ZH-FUT 这个接口的说明与参数表里：未找到，请求头只列了 `x-gate-exptime`。「小数张合约如何提交」在 ZH-FUT 这个接口的说明里：未找到。

### C.2 【SDK 文档·英文】EN-SDK `docs/FuturesApi.md` `CreateFuturesOrder`

说明里中文文档站没有的一条（原文在文件里是压平的一行，这里按条拆开，文字未改）：

> To place an order with a fractional contract size, query the contract with `X-Gate-Size-Decimal: 1`, check that `enable_decimal` is `true`, and read `order_size_min` and `order_size_max`. Send the same header when placing the order and pass fractional `size` or fractional displayed iceberg size `iceberg` as a decimal string.

`AmendFuturesOrder`（对应 `PUT /futures/{settle}/orders/{order_id}`）：

> To amend an order to a fractional size, query the original order contract with `X-Gate-Size-Decimal: 1`, check that `enable_decimal` is `true`, and read `order_size_min` and `order_size_max`. Send the same header when amending the order and pass the new `size` as a decimal string.

ZH-FUT 的「修改单个订单」没有这句。

EN-SDK `docs/FuturesOrder.md` 字段原文：

| 字段 | 英文原文 |
|---|---|
| Size | Required. Order size in contracts: positive to buy and negative to sell. Set to 0 for a closing order. |
| Iceberg | Display size for iceberg orders. 0 for non-iceberg orders. Note that hidden portions are charged taker fees. |
| Close | Set as `true` to close the position, with `size` set to 0 |
| ReduceOnly | Set as `true` to be reduce-only order |
| Tif | Time in force  - gtc: GoodTillCancelled - ioc: ImmediateOrCancelled, taker only - poc: PendingOrCancelled, makes a post-only order that always enjoys a maker fee - fok: FillOrKill, fill either completely or none |
| AutoSize | Set side to close dual-mode position. `close_long` closes the long side; while `close_short` the short one. Note `size` also needs to be set to 0 |
| Left | Unfilled size in contracts. With `X-Gate-Size-Decimal: 1`, this field is a string that preserves the fractional part. With the header set to `0` or omitted, it is an integer with the fractional part discarded. |

### C.3 【文档站·中文】ZH-ROOT 异常 label 中与数量、只减仓有关的行

合约相关：

| `label` | 含义 |
|---|---|
| SIZE_TOO_LARGE | 下单数量超过上限 |
| SIZE_TOO_SMALL | 下单数量不足下限 |
| INCREASE_POSITION | 只减仓委托会增加仓位 |

现货和杠杆相关（不属于合约分类）：

| `label` | 含义 |
|---|---|
| INVALID_PRECISION | 无效的精度 |
| AMOUNT_TOO_LITTLE | 数额小于最低值 |

### C.4 【文档站·中文】ZH-FUT 其他下单接口里 `size` / `amount` 的原文

| 接口 | 字段 | 类型 | 描述原文 |
|---|---|---|---|
| `PUT /futures/{settle}/orders/{order_id}`（修改单个订单） | » size | body, string, 否 | 新的委托大小。包括已成交委托的部分。  - 如果小于等于已成交数量，则撤销委托。 - 新的委托买卖方向必须跟原有的一致。 - 不能修改平仓单的size。 - 对于只减仓委托，如果调大size，则可能踢出其他只减仓委托。 - 如果不修改价格，则调小size不会影响深度排队，调大size会排到当前价位最后。 |
| `POST /futures/{settle}/bbo_orders`（档位bbo合约下单） | » size | body, integer(int64), 是 | 必选。交易数量，正数为买入，负数为卖出。平仓委托则设置为0。 |
| `POST /futures/{settle}/autoorder/v1/trail/create`（创建追踪委托） | » amount | body, string, 是 | 交易数量，单位是张，正数表示买，负数表示卖 |
| `POST /futures/{settle}/autoorder/v1/chase/create`（创建追逐限价单） | » amount | body, string, 是 | 委托总张数，字符串十进制，正数买、负数卖。不可为 0 |

BBO 下单的 `size` 类型是 `integer(int64)`，与普通下单的 `string` 不同。

## D. 「张数必须是最小下单量的整数倍 / 张数精度 / 步长」的说明

结论：**未找到**。已抓取的全部来源里，没有一句话写明下单张数必须是 `order_size_min` 的整数倍，也没有给出张数步长的定义。

### D.1 关键词命中（均非张数步长）

| 来源 | 命中原文 | 说明 |
|---|---|---|
| ZH-FUT | `order_price_round`：委托价格最小单位 | 价格 |
| ZH-FUT | `mark_price_round`：标记价格的最小单位 | 价格 |
| ZH-FUT | `risk_limit_step`：风险限额调整步长,已废弃 | 风险限额 |
| ZH-ROOT 异常 label | RISK_LIMIT_NOT_MULTIPLE：风险限额未按照步长调整 | 风险限额 |
| ZH-ROOT 异常 label | SIZE_TOO_SMALL：下单数量不足下限 | 只有上下限，没有倍数 |
| ZH-ROOT changelog v4.106.0 | 响应返回截去小数部分的整数 | 字段类型的解析行为 |
| EN-SDK `docs/Contract.md` | OrderPriceRound：Minimum order price increment | 价格 |
| EN-SDK `docs/Contract.md` | MarkPriceRound：Minimum mark price increment | 价格 |
| EN-SDK `docs/Contract.md` | RiskLimitStep：Risk limit adjustment step (deprecated) | 风险限额 |
| EN-ANN | Precision handling - Truncates towards zero (downwards). | 请求头为 0 或缺省时的字段解析行为 |

EN-SDK 侧的检索范围：本机缓存中全部 `.go` 与 `.md` 文件（根目录 15 个，`docs/` 下 5 个：`FuturesApi.md`、`FuturesInitialOrder.md`、`FuturesOrder.md`、`FuturesPriceTriggeredOrder.md`、`Position.md`），另加上游的 `docs/Contract.md`。关键词：integer multiple、multiple of、must be a multiple、lot size、size step、step size、increment、notional、minimum order value、min order value、minimum order amount、order_value、trade_value、contract value。除上表所列的价格与风险限额条目外，其余命中只有 `with_id` 与 `Position.update_id` 的「increments by 1」、`README.md` 的「incremented」，以及 `model_my_futures_trade.go` 里没有注释的 `TradeValue` 字段。integer multiple、lot size、notional、minimum order value 没有任何命中。缓存里没有 `FuturesOrderAmendment.md` 等其他模型文档。

### D.2 最接近的原文

【公告·中文】ZH-ANN，https://www.gate.com/zh/announcements/article/50325：

> 为进一步降低合约交易门槛，提升用户交易体验，Gate 将对部分永续合约市场的下单张数精度进行调整，具体信息如下：

调整时间：「2026 年 3 月 24 日 10:00 (UTC+8)」

调整内容：

| 交易对 | 当前最小下单张数 | 调整后最小下单张数 |
|---|---|---|
| ARIAUSDT | 1 | 0.1 |
| ESPORTSUSDT | 1 | 0.1 |
| ETHUSDT | 1 | 0.1 |
| LABUSDT | 1 | 0.1 |
| MUSDT | 1 | 0.1 |
| PEPEUSDT | 1 | 0.1 |
| RIVERUSDT | 1 | 0.1 |
| SIRENUSDT | 1 | 0.1 |
| SOLUSDT | 1 | 0.1 |
| TRXUSDT | 1 | 0.1 |
| UBUSDT | 1 | 0.1 |
| XRPUSDT | 1 | 0.1 |
| 我踏马来了USDT | 1 | 0.1 |

影响说明：

> 1. 本次调整不影响用户当前持仓及已成交订单；
> 2. 调整后的市场新建委托时适用最新的下单张数精度规则。

【公告·英文】EN-ANN 1.4 节：

> Return Field order_size_min: If the value is a decimal, it indicates that the corresponding futures support decimal order placement.

EN-ANN Phase 2 第 2 点：

> Starting from late January 2026, the fractional lot ordering functionality will be gradually rolled out to contracts. Specific contracts and timelines for the rollout will be announced by the platform in subsequent notices.

### D.3 这些原文能支持与不能支持的内容

- 能支持：官方公告把「下单张数精度」的调整表述为「最小下单张数」从 1 变为 0.1，并且 `order_size_min` 为小数即表示该合约支持小数下单。
- 不能支持：没有任何原文写出「张数是 `order_size_min` 的整数倍」。把 `order_size_min` 当作步长，在已抓取的资料里没有明文。
- 「所有小数张合约的步长都是 0.1」：未找到。ZH-ANN 只列了 13 个交易对，EN-ANN 称后续会逐步铺开。
- 「不在步长上时交易所如何处理（拒绝还是截断）」：未找到。只有请求头为 0 或缺省时的「Truncates towards zero (downwards)」，那是字段解析行为，不是撮合规则。

## E. 合约最小下单金额 / 最小名义价值

结论：**未找到**。已抓取的全部来源里，没有针对合约的最小下单金额、最小名义价值、1 USD 门槛之类的说明。

与 `order_size_min` 的区别：`order_size_min` 在各来源里都是按张数计的数量下限。

| 来源 | `order_size_min` 的表述 |
|---|---|
| ZH-FUT | 最小下单数量 |
| EN-SDK `docs/Contract.md` | Minimum order size in contracts |
| EN-ANN 1.3 | Minimum order size allowed by the contract 'order_size_min' |
| ZH-ANN | 最小下单张数 |

含「最小下单金额」「最小名义」字样但不属于 Gate 合约下单规则的原文：

| 来源 | 原文 | 归属 |
|---|---|---|
| ZH-ROOT 异常 label，理财相关 | ERR_QUOTA_LOWER_LIMIT：不满足最小下单金额 | 理财 |
| ZH-ROOT changelog v4.106.112（2026-08-03） | 更新 `POST /crossex/position`（`closeCrossexPosition`）说明，明确仓位严格小于最小名义金额或最小下单数量时才触发自动平仓 | CrossEx |
| ZH-ROOT changelog v4.106.175（2026-11-02） | 新增 `GET /crossex/rule/open_capacity`（`listCrossexRuleOpenCapacity`），响应包含必填字段 `symbol`、`position_side`（`LONG`/`SHORT`）和 `available_to_open_notional`。仅返回剩余可开仓价值低于阈值的交易对及仓位方向；`0` 表示该交易对在对应方向当前不能开仓。 | CrossEx |
| ZH-AGENT 用例 4 | 调用 cex_fx_get_fx_contract 确认合约参数（最小下单量、面值、维持保证金率） | Agent |

合约仓位模型里有名义价值字段，但没有最小值约束：ZH-FUT `Position.value` 原文为「按结算币种标记价格计算的合约价值」。

## F. 条件单 `POST /futures/{settle}/price_orders` 的 `size` 与 `amount`

### F.1 【文档站·中文】ZH-FUT「创建价格触发订单」参数表

请求体模型为 `FuturesPriceTriggeredOrder`，`initial` 为内嵌对象。

| 名称 | 位置 | 类型 | 必选 | 描述 |
|---|---|---|---|---|
| » initial | body | object | 是 |  |
| »» contract | body | string | 是 | 合约标识 |
| »» size | body | integer(int64) | 否 | 代表需要平仓的合约张数，全平仓:size=0 部分平仓:plan-close-short-position size>0  部分平仓:plan-close-long-position size<0 |
| »» amount | body | string | 否 | 同size参数，用于兼容小数,同时存在时以amount为准 |
| »» price | body | string | 是 | 交易价，当价格为 0 时，表示通过市价方式来下单 |
| »» close | body | boolean | 否 | 单仓模式全部平仓时,必须设置为true执行平仓操作 单仓模式部分平仓时/双仓模式下，可以不设置close，或close=false |
| »» tif | body | string | 否 | Time in force 策略,默认为gtc，市价单当前只支持 ioc 模式市价单当前只支持 ioc 模式  - gtc: GoodTillCancelled - ioc: ImmediateOrCancelled |
| »» text | body | string | 否 | 订单的来源，包括：  - web: 网页 - api: API 调用 - app: 移动端 |
| »» reduce_only | body | boolean | 否 | 设置为 true 的时候执行自动减仓操作，设为 true可确保订单不会开新仓，只用于平仓或减仓 |
| »» auto_size | body | string | 否 | 单仓模式不需设置auto_size 双仓模式全部平仓（size=0）时，必须设置auto_size，close_long 平多头， close_short 平空头 双仓模式部分平仓（size≠0）时，不需设置auto_size |

模型表里对应两行（「限制」列）：`size` 为 `integer(int64)`、`none`；`amount` 为 `string`、`none`；`close` 为 `只写`。

ZH-FUT「修改单个自动订单」（`PUT /futures/{settle}/price_orders/amend`，模型 `FuturesUpdatePriceTriggeredOrder`）：

| 名称 | 位置 | 类型 | 必选 | 描述 |
|---|---|---|---|---|
| » size | body | integer(int64) | 否 | 修改后的合约张数，全平：0；部分平仓：正负代表方向（同创建接口逻辑） |
| » amount | body | string | 否 | 同size参数，用于兼容小数,同时存在时以amount为准 |

### F.2 【文档站·中文】ZH-ROOT changelog

v4.106.52（2026-03-29）：

> `FuturesInitialOrder` 模型：新增可选 `amount` 字段（字符串），用于小数合约张数；与 `size` 同时存在时以 `amount` 为准
>
> `FuturesUpdatePriceTriggeredOrder` 模型：新增可选 `amount` 字段（字符串），语义与 `size` 相同

v4.106.160（2026-10-16）：

> **不兼容变更：**合约条件单改单接口由 `PUT /futures/{settle}/price_orders/amend` 改为 `PUT /futures/{settle}/price_orders/amend/{order_id}`。新增必填的字符串路径参数 `order_id`，JSON 请求体中的 `order_id` 仍需提供。调用方需在路径和请求体中传入相同的订单 ID。

ZH-FUT 的接口标题与请求示例仍是 `PUT /futures/{settle}/price_orders/amend`，与这条 changelog 不一致。

### F.3 【SDK 文档·英文】EN-SDK `docs/FuturesInitialOrder.md`

| 字段 | 类型 | 英文原文 |
|---|---|---|
| Size | int64 | Represents the number of contracts that need to be closed, full closing: size=0 Partial closing: plan-close-short-position size>0  Partial closing: plan-close-long-position size<0 |
| Amount | string | Same as `size`; used for decimal contract size. When both `size` and `amount` are provided, `amount` takes precedence. |
| Price | string | Order price. Set to 0 to use market price |
| Close | bool | When fully closing a position in single-position mode, close must be set to true to execute the close operation. When partially closing a position in single-position mode or in dual-position mode, close can be left unset or set to false. |
| Tif | string | Time in force strategy, default is gtc, market orders currently only support ioc mode  - gtc: GoodTillCancelled - ioc: ImmediateOrCancelled |
| ReduceOnly | bool | When set to true, perform automatic position reduction operation. Set to true to ensure that the order will not open a new position, and is only used to close or reduce positions |
| AutoSize | string | One-way Mode: auto_size is not required Hedge Mode full closing (size=0): auto_size must be set, close_long for closing long positions, close_short for closing short positions Hedge Mode partial closing (size≠0): auto_size is not required |

`docs/FuturesPriceTriggeredOrder.md` 中 `Initial` 行类型为 `FuturesInitialOrder`，未再单独定义 `size`、`amount`。

### F.4 【公告】EN-ANN 1.3 节条件单那一行

| 接口 | Field | Remarks |
|---|---|---|
| 1. Query auto order list GET /futures/settle/price_orders 2. Cancel all auto orders DELETE /futures/settle/price_orders 3. Query single auto order details GET /futures/settle/price_orders/order_id 4. Cancel single auto order DELETE /futures/settle/price_orders/order_id 5. Modify a Single Auto Order PUT /futures/settle/price_orders/order_id | New field: 'amount' (string type). | 1. New field: amount (string type). 2. The original field size (int ) and the new field amount (string) will be compatible. |

汇总：条件单的 `size` 是 `int64`，小数张走字符串字段 `amount`，两者同时给出时以 `amount` 为准。这一点中英文来源一致。

## G. Agent 文档

来源：【文档站·中文】ZH-AGENT，https://www.gate.com/docs/developers/agent/zh_CN ，页面标题「Gate for AI 开发者指南」。

### G.1 接入方式

| 接入方式 | 说明 | 鉴权 |
|---|---|---|
| MCP（Remote） | 云端托管（api.gatemcp.ai），无需自行部署，在 MCP 兼容客户端中添加 Server URL 即可连接 | OAuth |
| MCP（Local） | 本地运行 MCP Server，连接至本地 AI 客户端 | API Key |
| CLI | 命令行工具，适合本地调试与脚本自动化 | API Key |

### G.2 MCP 端点

Base URL：`https://api.gatemcp.ai`；传输协议：Streamable HTTP + SSE

| 端点 | 工具数 | 认证方式 | 说明 |
|---|---|---|---|
| `/mcp` | 58 | 无需认证 | 全品类行情数据（现货/合约/期权/交割/理财/Alpha） |
| `/mcp/exchange` | 400+ | OAuth2 | CEX 全功能交易（现货/合约/期权/闪兑/跟单/子账户等） |
| `/mcp/dex` | 33 | Google / Gate OAuth | DEX Swap、钱包管理、DApp 交互 |
| `/mcp/info` | 10 | 无需认证 | 币种信息、技术分析、链上数据、安全审计 |
| `/mcp/news` | 3 | 无需认证 | 新闻搜索、交易所公告、社交情绪 |

页面注明：「注意：所有类别包括但不局限于以上 Tools，详见 Gate MCP」。

### G.3 工具清单

各分类标题后的「需要 OAuth2 认证」「无需认证」等字样是页面小节标题原文；括号里的端点不是小节标题的内容，是按 G.2 端点表的「认证方式」与「说明」两列归类的。页面另有一句：「`/mcp` 端点的 58 个行情工具全部免认证。」

Market，`/mcp`，无需认证。永续合约行情：

| 工具名 | 说明 |
|---|---|
| cex_fx_list_fx_contracts | 获取所有合约列表 |
| cex_fx_get_fx_contract | 获取单个合约详情 |
| cex_fx_get_fx_tickers | 获取合约行情 |
| cex_fx_get_fx_order_book | 获取合约深度 |
| cex_fx_get_fx_trades | 获取最新成交 |
| cex_fx_get_fx_candlesticks | 获取 K 线 |
| cex_fx_get_fx_funding_rate | 获取当前资金费率 |
| cex_fx_list_fx_batch_funding_rates | 批量获取资金费率 |
| cex_fx_get_fx_premium_index | 获取溢价指数 |
| cex_fx_list_fx_insurance_ledger | 保险基金记录 |
| cex_fx_list_fx_liq_orders | 强平历史 |
| cex_fx_get_index_constituents | 指数成分 |
| cex_fx_list_contract_stats | 合约统计数据 |
| cex_fx_list_fx_risk_limit_tiers | 风险限额档位 |

Market，`/mcp`，无需认证。现货行情：

| 工具名 | 说明 |
|---|---|
| cex_spot_list_currencies | 获取所有币种信息 |
| cex_spot_get_currency | 获取单个币种详情 |
| cex_spot_list_currency_pairs | 获取所有现货交易对信息 |
| cex_spot_get_currency_pair | 获取单个交易对详情 |
| cex_spot_get_spot_tickers | 获取所有/单个现货行情 |
| cex_spot_get_spot_order_book | 获取现货深度/订单簿 |
| cex_spot_get_spot_trades | 获取最新成交记录 |
| cex_spot_get_spot_candlesticks | 获取现货 K 线数据 |

Market，`/mcp`，无需认证。期权行情：

| 工具名 | 说明 |
|---|---|
| cex_options_list_options_underlyings | 标的资产列表 |
| cex_options_list_options_expirations | 到期日列表 |
| cex_options_list_options_contracts | 所有期权合约 |
| cex_options_get_options_contract | 单个合约详情 |
| cex_options_list_options_order_book | 深度/订单簿 |
| cex_options_list_options_tickers | 所有期权行情 |
| cex_options_list_options_underlying_tickers | 标的行情 |
| cex_options_list_options_candlesticks | K 线 |
| cex_options_list_options_underlying_candlesticks | 标的 K 线 |

Market，`/mcp`，无需认证。交割合约行情：

| 工具名 | 说明 |
|---|---|
| cex_dc_list_dc_contracts | 获取所有交割合约 |
| cex_dc_get_dc_contract | 获取单个合约详情 |
| cex_dc_list_dc_tickers | 获取行情 |
| cex_dc_list_dc_order_book | 获取深度 |
| cex_dc_list_dc_trades | 获取成交记录 |
| cex_dc_list_dc_candlesticks | 获取 K 线 |
| cex_dc_list_dc_insurance_ledger | 保险基金记录 |
| cex_dc_list_dc_risk_limit_tiers | 风险限额档位 |

Market，`/mcp`，无需认证。理财产品：

| 工具名 | 说明 |
|---|---|
| cex_earn_list_uni_currencies | 获取活期理财产品列表 |
| cex_earn_list_dual_investment_plans | 获取双币投资产品列表 |
| cex_earn_list_structured_products | 获取结构化产品列表 |
| cex_earn_list_earn_fixed_term_products | 获取定期理财产品列表 |
| cex_earn_list_earn_fixed_term_products_by_asset | 按币种获取定期产品 |

Market，`/mcp`，无需认证。Alpha 市场：

| 工具名 | 说明 |
|---|---|
| cex_alpha_list_alpha_currencies | 获取 Alpha 支持币种 |
| cex_alpha_list_alpha_tickers | 获取 Alpha 代币行情 |
| cex_alpha_list_alpha_tokens | 获取 Alpha 代币列表 |

Futures Trading，「需要 OAuth2 认证」（`/mcp/exchange`）：

| 工具名 | 说明 |
|---|---|
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

Spot Trading，「需要 OAuth2 认证」（`/mcp/exchange`）：

| 工具名 | 说明 |
|---|---|
| cex_spot_get_spot_accounts | 查询现货账户余额 |
| cex_spot_create_spot_order | 创建现货订单 |
| cex_spot_cancel_spot_order | 取消订单 |
| cex_spot_amend_spot_order | 修改订单 |
| cex_spot_create_spot_batch_orders | 批量创建订单 |
| cex_spot_get_spot_order | 查询单个订单 |
| cex_spot_list_spot_orders | 订单列表 |
| cex_spot_list_spot_my_trades | 成交记录 |
| cex_spot_create_spot_price_triggered_order | 条件单 |
| cex_spot_list_all_open_orders | 所有挂单 |

Account，「需要 OAuth2 认证」（`/mcp/exchange`）：

| 工具名 | 说明 |
|---|---|
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

DEX，「需要 Google / Gate OAuth 认证」（`/mcp/dex`）：

| 工具名 | 说明 |
|---|---|
| dex_tx_swap | 执行 DEX 代币兑换 |
| dex_tx_quote | 获取兑换报价 |
| dex_token_list_swap_tokens | 获取支持的代币列表 |
| dex_wallet_get_addresses | 获取钱包地址列表 |
| dex_tx_transfer_preview + dex_tx_send_raw_transaction | 链上转账 |
| dex_wallet_get_token_list | 查询钱包余额 |
| dex_token_get_coin_info | 获取链上代币信息 |
| dex_token_get_risk_info | 代币安全审计 |
| dex_tx_list / dex_tx_history_list | 查询交易历史 |
| dex_dapp_connect | 钱包连接 |

Info，「无需认证」（`/mcp/info`）：

| 工具名 | 说明 |
|---|---|
| info_coin_get_coin_info | 获取币种基本信息（简介/市值/供应量） |
| info_markettrend_get_technical_analysis | 获取技术分析指标汇总 |
| info_markettrend_get_kline | 获取指定币种 K 线数据 |
| info_markettrend_get_indicator_history | 获取技术指标历史数据 |
| info_marketsnapshot_get_market_snapshot | 获取市场快照 |
| info_marketsnapshot_get_market_overview | 获取市场总览 |
| info_coin_get_coin_rankings | 获取币种排行榜 |
| info_onchain_get_address_info | 查询地址信息 |
| info_onchain_get_token_onchain | 查询代币链上数据 |
| info_compliance_check_token_security | 代币安全检测 |

News，「无需认证」（`/mcp/news`）：

| 工具名 | 说明 |
|---|---|
| news_feed_search_news | 搜索加密货币新闻 |
| news_feed_get_exchange_announcements | 获取交易所公告（上币/下架/维护） |
| news_feed_get_social_sentiment | 获取社交媒体情绪分析 |

与合约有关的 Skill（「全部 Skills 一览」）：

| Skill 名称 | 说明 | 认证 |
|---|---|---|
| gate-exchange-futures | USDT 永续合约：开平仓、条件单、止盈止损 | OAuth2 |

CLI：页面给出的子命令示例有 `gate-cli cex futures market funding-rate --contract BTC_USDT`（「行情查询（无需认证）」）与 `gate-cli cex futures position list --settle usdt`（「交易操作（需要授权）」），并写明「可用 `gate-cli cex -h` 查看完整子命令树」。页面没有列出 CLI 的合约下单子命令。

仓库链接：MCP `https://github.com/gate/gate-mcp`，Skills `https://github.com/gate/gate-skills`。

### G.4 与合约元数据、张数换算的关系

- 合约元数据工具：有。`cex_fx_get_fx_contract`（获取单个合约详情）与 `cex_fx_list_fx_contracts`（获取所有合约列表），均在 `/mcp`，无需认证。页面没有给出这两个工具的参数列表和返回字段。
- 张数换算相关的工具或说明：未找到。ZH-AGENT 整页未出现 `X-Gate-Size-Decimal`、`quanto_multiplier`、`enable_decimal`、`order_size_min`、`张数换算`。
- 页面唯一涉及下单张数的内容是「用例 4：合约开仓」，原文：

> | __用户提示词__ | BTC 永续做多 1 张 10x |
>
> 1️⃣ 解析意图：合约 = BTC_USDT，方向 = 做多（buy），数量 = 1 张，杠杆 = 10x
> 2️⃣ 调用 cex_fx_get_fx_contract 确认合约参数（最小下单量、面值、维持保证金率）
> 3️⃣ 调用 cex_fx_update_fx_position_leverage 设置杠杆倍数为 10x
> 4️⃣ 调用 cex_fx_get_fx_tickers 获取当前标记价格，计算所需保证金
> 5️⃣ 向用户展示订单预览（预估开仓价、保证金、强平价），等待确认
> 6️⃣ 用户确认后调用 cex_fx_create_fx_order 执行开多
> 7️⃣ 查询持仓确认开仓成功，输出持仓详情

这个用例以「张」为输入，没有金额到张数的换算步骤。

## 实测补充（非文档原文，仅用于核对文档说法）

时间：2026-11-24T04:57:53Z 与 2026-11-24T04:59:47Z（UTC）。请求：`GET https://api.gateio.ws/api/v4/futures/usdt/contracts/{contract}`，分别不带请求头、带 `X-Gate-Size-Decimal: 0`、带 `X-Gate-Size-Decimal: 1`。

| 合约 | 请求头 | order_size_min | order_size_max | enable_decimal | quanto_multiplier | market_order_size_max |
|---|---|---|---|---|---|---|
| ETH_USDT | 不带 | `0`（整数） | `10000000`（整数） | `true` | `"0.01"` | `"8000000"`（字符串） |
| ETH_USDT | `0` | `0`（整数） | `10000000`（整数） | `true` | `"0.01"` | `"8000000"`（字符串） |
| ETH_USDT | `1` | `"0.1"`（字符串） | `"10000000"`（字符串） | `true` | `"0.01"` | `"8000000"`（字符串） |
| BTC_USDT | 不带 | `1`（整数） | `12000000`（整数） | `false` | `"0.0001"` | `"10000000"`（字符串） |
| BTC_USDT | `0` | `1`（整数） | `12000000`（整数） | `false` | `"0.0001"` | `"10000000"`（字符串） |
| BTC_USDT | `1` | `"1"`（字符串） | `"12000000"`（字符串） | `false` | `"0.0001"` | `"10000000"`（字符串） |

与文档的对应关系：

- ETH_USDT 不带请求头时 `order_size_min` 返回整数 `0`，带请求头 `1` 时返回字符串 `"0.1"`，与 EN-SDK `GetFuturesContract`、EN-ANN 1.4 的说法一致。
- 不带请求头与带 `0` 的结果相同，与 EN-ANN 规则表一致。
- `enable_decimal`、`quanto_multiplier`、`market_order_size_max` 不随请求头变化；`market_order_size_max` 三种情况下都是字符串，它不在 ZH-ROOT changelog v4.106.0 列出的受影响字段里。
- 只测了这两个合约、这三个字段，没有下单，没有验证步长或最小金额规则。

## 未找到的项

1. D：「下单张数必须是 `order_size_min` 的整数倍」或张数步长的定义。全部来源未找到。最接近的是 ZH-ANN 把「下单张数精度调整」表述为「最小下单张数 1 变 0.1」。
2. D：「所有小数张合约步长都是 0.1」。未找到；ZH-ANN 只列 13 个交易对。
3. D：张数不在步长上时交易所的处理方式（拒绝还是截断）。未找到。
4. E：合约最小下单金额、最小名义价值。全部来源未找到。
5. B：中文文档站两个合约查询接口说明里的 `X-Gate-Size-Decimal`。未找到；该请求头在中文文档站只出现在 ZH-ROOT changelog v4.106.0。英文出处为 EN-SDK 与 EN-ANN。
6. C：中文文档站 `POST /futures/{settle}/orders` 说明里的 `X-Gate-Size-Decimal` 与「小数张如何提交」。未找到；英文出处为 EN-SDK。
7. A：`order_size_min` 之外的张数步长或精度字段。不存在（48 个模型字段与实测 53 个响应键里都没有）。
8. A：合约价值的计算公式。未找到。ZH-ROOT changelog v4.19.2 称已加到 `Contract` 模型描述，但当前描述里没有公式。
9. G：Agent 文档里的张数换算工具或说明。未找到。
10. EN-ANN 的中文版（`https://www.gate.com/zh/announcements/article/48788`）与 ZH-ANN 的英文版（`https://www.gate.com/announcements/article/50325`）：访问被重定向到「仅限于特定地区的服务」提示页，正文未取得。
11. EN-ANN Phase 3（默认改为返回字符串、请求头废弃）是否已生效。无法确认。

## 抓取过程中遇到的限制

1. **根页 ZH-ROOT 的范围**：返回内容止于「常见问题」，没有任何接口章节。changelog v4.106.72 写明文档站已「按 Tag 拆分展示」，所以接口内容需要从 Tag 页取。D、E 两项对 ZH-ROOT 的检索只覆盖 changelog、通用章节和异常 label。
2. **Tag 页 ZH-FUT 的范围**：只抓了 Futures 这一个 Tag。交割合约、统一账户、WebSocket 文档未抓取。
3. **折叠字段**：ZH-FUT 各接口的响应字段表有「展开剩余 N 个字段」的折叠行，阅读器无法展开。Contract 的完整字段改由页尾模型表（48 个）、EN-SDK（48 个）与 LIVE 实测（53 个键）三方交叉确认。ZH-FUT 页尾模型列表止于 `StopChaseOrderReq`，未见截断标记，但无法确认其后是否还有模型。
4. **ZH-FUT 是否完整**：页面从第一个接口到最后一个可见模型连续返回，未见截断标记；无法证明没有尾部内容丢失。
5. **ZH-FUT、ZH-ROOT 的关键词结论是通读结论**：页面内容只存在于阅读器返回结果里，没有落盘，未做程序化检索。其中「ZH-FUT 未出现 `X-Gate-Size-Decimal`」对 B、C 两项结论影响最大。
6. **Agent 地址**：任务给出的 `developer` 单数地址是 404，正文来自 `developers` 复数地址。
7. **公告的地区限制**：两篇公告各有一个语言版本被重定向到地区提示页。
8. **英文文档站未抓取**：`https://www.gate.com/docs/developers/apiv4/en/...` 没有访问，英文出处以 SDK 仓库文档为准。
9. **阅读器压平换行**：EN-ANN 表格单元格、EN-SDK 说明里的多行内容被压成一行；ZH-FUT 的列表项顺序与原页一致，但子项缩进丢失。
10. **版本对应**：EN-SDK 为 tag `v7.2.184`，文档站为 `v4.106.184`。ZH-FUT 有而 EN-SDK `docs/FuturesApi.md` 没有的内容，本文只按各自原文记录，未判断谁新谁旧。
