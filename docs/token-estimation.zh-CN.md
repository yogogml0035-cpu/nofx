# 📊 Token 预算与模型上下文

> 版本：v2.0 | 更新：2026-11-27
> 适用：策略配置 · 模型选择 · 运行时提示词预算

---

## 目录

- [两层预算口径](#-两层预算口径)
- [真实失败与实测校准](#-真实失败与实测校准)
- [配置级估算公式](#-配置级估算公式)
- [运行时预算公式](#-运行时预算公式)
- [模型能力参考](#-模型能力参考)
- [典型配置的 token 开销](#-典型配置的-token-开销)
- [校验点与失败语义](#-校验点与失败语义)

---

## 🧭 两层预算口径

| 口径 | 入口 | 能看到什么 | 用途 |
| --- | --- | --- | --- |
| 配置级估算 | `store.StrategyConfig.EstimateTokens()` | 只看到策略配置形状，没有行情与历史数据 | 保存、启动预检、UI 提示 |
| 运行时预算 | `kernel.CheckPromptBudget()` | 已经取数并渲染好的 system + user prompt，加上客户端 `max_tokens` | 真正发起请求前的硬校验 |

运行时以**渲染后的真实文本**为准。配置级估算只在还没有行情数据时使用，不能替代真实请求口径。

---

## 🔬 真实失败与实测校准

2026-11-27 的线上失败配置：2 个币（BTC/ETH）× 4 个周期（5m/15m/1h/4h）× 300 根 K 线，开启 EMA/MACD/ATR/成交量、Open Interest 与资金费率，中文策略。

同一份真实提示词直接发给配置中的 `DeepSeek-V4.1-Flash-1017`，provider 返回：

- `prompt_tokens` = **139,981**
- `completion_tokens` = 8,060（其中 reasoning 7,018）
- `total_tokens` = 148,041
- `finish_reason` = `stop`，HTTP 200

这说明两件事：

1. 该模型的真实上下文远大于旧代码假定的 131,072；官方文档标注 **1M 上下文 / 384K 最大输出**。
2. 旧配置级估算只报 114,333，比真实输入低约 18%。数字表格的 token 密度接近 **1.8 ASCII 字节/token**，不是旧公式假设的 4 字节/token。

---

## 🧮 配置级估算公式

入口：`store/strategy.go` → `EstimateTokens()`。纯计算，不取行情。

```
system  = (base 3000(zh)/4000(en) + Σ prompt section bytes + custom prompt bytes)
          / 2 (zh) 或 / 4 (en)
fixed   = 200

每币每周期 = K × (35 + I)
  I = EMA 10（EMA20 + EMA50 两个数组）
    + MACD 5
    + RSI 10（RSI7 + RSI14 两个数组）
    + BOLL 15（上/中/下三个数组）
  ATR 是每个周期一个标量，成交量已包含在 OHLCV 行内，二者不另加每行成本，
  由下方 15% 安全边际覆盖。

market  = 币数 × 周期数 × 每币每周期
        + 币数 × 25（OI + 资金费率行）
        + 600（仅勾选市场环境上下文且非网格策略时）
total   = (system + market + fixed) × 1.15
```

`35`（每根 K 线行）和 `5`（每个指标值）来自上述真实提示词的实测校准；`1.15` 为安全边际，覆盖当前标记、小数位长度等格式漂移。

### 配置级示例

上述真实配置的配置级拆解：

| 组成 | tokens |
| --- | ---: |
| System prompt | ~5,570 |
| Market data（2 × 4 × 300 × 50 = 120,000，+ OI/资金费率 50 + 市场环境 600） | 120,650 |
| Fixed overhead | 200 |
| 小计 | 126,420 |
| × 1.15 | **145,383** |

与 provider 实测 139,981 相比高约 3.9%，属于有意保留的保守区间。

---

## 🎯 运行时预算公式

入口：`kernel/prompt_budget.go` → `CheckPromptBudget()`。

```
input    = ASCII 字节数 / 1.7 + 非 ASCII rune 数 + 64（消息框架开销）
required = input + client.MaxTokens
```

- 只使用 `client.MaxTokens`（即请求真正会发送的 `max_tokens`），不会偷偷调低或省略。
- ASCII 按 1.7 字节/token 计，低于实测的 1.83 字节/token，因此是保守上界。
- 中文等非 ASCII 字符按 1 个 rune 1 token 计。
- 如果 `required > ModelCapability.ContextWindow`，在调用点明确失败，不做静默截断。
- 如果 `client.MaxTokens > ModelCapability.MaxOutputTokens`（已知最大输出时），同样明确失败。

运行时日志会打印本次输入估算、输出预算、上下文上限和使用率，便于直接核对三者关系。

---

## 🤖 模型能力参考

来源：`store/strategy.go` → `ModelContextLimits` / `GetModelCapabilities()`；DeepSeek 的 1M / 384K 来自 <https://api-docs.deepseek.com/quick_start/pricing>（2026-11-27 读取）。

| Provider / 模型匹配 | Context | Max output | 说明 |
| --- | ---: | ---: | --- |
| `deepseek` / `deepseek-v4.1-flash-*` | 1,000,000 | 384,000 | DeepSeek V4.1-Flash，官方文档 2026-09 |
| `deepseek` / `deepseek-v4-pro-*` | 1,000,000 | 384,000 | DeepSeek V4-Pro，官方文档 2026-09 |
| `deepseek` provider fallback | 1,000,000 | 0（未知） | 当前代模型；旧模型 ID 由 V4.1-Flash 承载 |
| `openai` | 128,000 | 0（未知） | provider 默认 |
| `claude` | 200,000 | 0（未知） | provider 默认 |
| `qwen` | 131,072 | 0（未知） | provider 默认 |
| `gemini` / `minimax` | 1,000,000 | 0（未知） | provider 默认 |
| `grok` / `kimi` | 131,072 | 0（未知） | provider 默认 |
| 未知 provider / 未知模型 | 131,072 | 0（未知） | 保守兜底 |

模型 ID 优先于 provider：自定义 `BaseURL` 可能把某个 provider 名称路由到别的模型，只有模型 ID 未知时才退回 provider 默认值。未知模型不会被“抬”到 1M。

---

## 📐 典型配置的 token 开销

按配置级公式，每枚币的开销：

| 配置 | 每币 tokens |
| --- | ---: |
| 3 周期 × 20 根，仅成交量 | 2,100 |
| 3 周期 × 200 根，仅成交量 | 21,000 |
| 3 周期 × 2000 根，仅成交量 | 210,000 |
| 4 周期 × 2000 根，全部指标 | 600,000 |

按 80% 上下文警戒线估算最大安全币数（另加 ~5K 静态开销）：

| 模型上下文 | 默认 3TF/K=20 | 3TF/K=200 | 3TF/K=2000 | 4TF/K=2000/全指标 |
| --- | ---: | ---: | ---: | ---: |
| 128K | 10（封顶） | 4 | 0 | 0 |
| 200K | 10（封顶） | 7 | 0 | 0 |
| 1M | 10（封顶） | 10（封顶） | 3 | 1 |

> `MaxKlineCount=2000` 是 Gate candlesticks 单次查询上限，不是提示词安全上限。真正的安全边界以绑定模型的运行时预算为准。

---

## 🔒 校验点与失败语义

1. **保存策略**：`ExceedsAllModelLimits()` 只在配置估算超过**所有**已知模型时返回 400；否则允许保存并给出警告。警告中的 provider 上限是保守参考，不代表已绑定模型。
2. **启动 trader**：`strategyTokenBlocker()` 按绑定模型的 `CustomModelName` 解析真实上下文与最大输出，并把客户端 `max_tokens`（由思考强度决定）一起算进预算；未知模型回退到 provider 默认。
3. **运行时决策**：`GetFullDecisionWithStrategy()` 取数、渲染 system/user prompt 后调用 `kernel.CheckPromptBudget()`；超限在调用点失败，错误中包含真实估算、输出预算和模型上下文。
4. **AI 试运行**：只提前拒绝所有已知模型都放不下的配置；真正调用前同样经过 `CheckPromptBudget()`。
5. **不静默截断**：任何一层都不会删除币种、周期、K 线或指标来“塞进”上下文；超限就是超限，由用户调整配置或换模型。