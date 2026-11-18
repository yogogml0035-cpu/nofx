<p align="center"><strong>由 <a href="https://vergex.trade">vergex.trade</a> 支持</strong></p>

<h1 align="center">NOFX</h1>

<p align="center">
  <strong>面向加密永续合约市场的 AI 交易终端。</strong><br/>
  <strong>覆盖加密货币永续合约的研究、策略生成、执行与监控。</strong>
</p>

<p align="center">
  <a href="https://github.com/NoFxAiOS/nofx/stargazers"><img src="https://img.shields.io/github/stars/NoFxAiOS/nofx?style=for-the-badge" alt="Stars"></a>
  <a href="https://github.com/NoFxAiOS/nofx/releases"><img src="https://img.shields.io/github/v/release/NoFxAiOS/nofx?style=for-the-badge" alt="Release"></a>
  <a href="https://github.com/NoFxAiOS/nofx/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-AGPL--3.0-blue.svg?style=for-the-badge" alt="License"></a>
  <a href="https://t.me/nofx_dev_community"><img src="https://img.shields.io/badge/Telegram-Community-blue?style=for-the-badge&logo=telegram" alt="Telegram"></a>
</p>

<p align="center">
  <a href="https://golang.org/"><img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go" alt="Go"></a>
  <a href="https://reactjs.org/"><img src="https://img.shields.io/badge/React-18+-61DAFB?style=flat&logo=react" alt="React"></a>
</p>

<p align="center">
  <a href="../../../README.md">English</a> ·
  <a href="README.md">中文</a> ·
  <a href="../ja/README.md">日本語</a> ·
  <a href="../ko/README.md">한국어</a> ·
  <a href="../ru/README.md">Русский</a> ·
  <a href="../uk/README.md">Українська</a> ·
  <a href="../vi/README.md">Tiếng Việt</a>
</p>

> **语言声明：** 本中文版本文档仅为方便海外华人社区阅读而提供，不代表本软件面向中国大陆、香港、澳门或台湾地区用户开放。如您位于上述地区，请勿使用本软件。

---

NOFX 是一个开源 AI 交易终端，面向需要统一工作区完成市场研究、策略开发、交易执行与组合监控的活跃交易者。

产品围绕加密货币永续合约市场设计。AI 层将交易意图转化为观察列表、信号、策略逻辑、风控约束与执行工作流。

```bash
curl -fsSL https://raw.githubusercontent.com/NoFxAiOS/nofx/main/install.sh | bash
```

打开 **http://127.0.0.1:3000**。

---

## 注册交易所

通过以下链接开通加密永续合约交易账户。该链接来自 NOFX 合作伙伴计划，可能包含手续费折扣或推荐权益。

| 交易所 | 状态 | 享手续费折扣注册 |
| :--- | :---: | :--- |
| <img src="../../../web/public/exchange-icons/gate.svg" width="20" height="20" style="vertical-align: middle;"/> **Gate** | ✅ | [注册](https://www.gatenode.xyz/share/VQBGUAxY) |

---

## 快速演示

<p align="center">
  <a href="https://drive.google.com/file/d/1frzw-HDZ3viQvLOQKsAJGc9bT0dXs68D/view">
    <img src="../../../screenshots/demo-cover.png" alt="NOFX quick demo video" width="900"/>
  </a>
</p>

<p align="center">
  点击封面图观看演示视频。
</p>

---

## 市场

**加密货币永续合约**

NOFX 按加密永续合约工作流组织研究、策略构建、执行与监控，而不是停留在单一交易所界面。

---

## AI 模型接入

NOFX 通过标准 API Key 供应商接入 AI 模型。在模型设置中配置您自己的供应商密钥（DeepSeek、OpenAI、Claude、Gemini、Qwen、Kimi、Grok、MiniMax），终端即可按需路由推理请求。

---

## 能力

| 能力 | 描述 |
| :--- | :--- |
| **AI 交易终端** | 面向加密永续合约工作流的一体化工作区 |
| **AI 模型接入** | 自带 API Key，支持 8 家供应商（DeepSeek、OpenAI、Claude 等） |
| **交易所连接** | Binance、Bybit、OKX、Bitget、KuCoin、Gate、Indodax、Aster、Lighter |
| **策略工作室** | 市场范围、指标、风控与策略逻辑 |
| **模型竞赛** | 比较 AI 交易员的实时表现与排行榜 |
| **Telegram Agent** | 通过聊天控制和监控交易助手 |
| **组合仪表板** | 持仓、盈亏、执行历史与模型决策日志 |

---

## 截图

<details>
<summary><b>配置页</b></summary>

|                         配置                         |                         交易员列表                         |
| :----------------------------------------------------: | :----------------------------------------------------------: |
| <img src="../../../screenshots/config-ai-exchanges.png" width="400"/> | <img src="../../../screenshots/config-traders-list.png" width="400"/> |

</details>

<details>
<summary><b>仪表板</b></summary>

|                        概览                         |                          行情图表                           |
| :-----------------------------------------------------: | :-------------------------------------------------------------: |
| <img src="../../../screenshots/dashboard-page.png" width="400"/> | <img src="../../../screenshots/dashboard-market-chart.png" width="400"/> |

|                          交易统计                           |                          持仓历史                           |
| :--------------------------------------------------------------: | :-----------------------------------------------------------------: |
| <img src="../../../screenshots/dashboard-trading-stats.png" width="400"/> | <img src="../../../screenshots/dashboard-position-history.png" width="400"/> |

|                          持仓                           |                    交易员详情                     |
| :----------------------------------------------------------: | :---------------------------------------------------: |
| <img src="../../../screenshots/dashboard-positions.png" width="400"/> | <img src="../../../screenshots/details-page.png" width="400"/> |

</details>

<details>
<summary><b>策略工作室</b></summary>

|                     策略编辑器                      |                      指标配置                       |
| :------------------------------------------------------: | :----------------------------------------------------------: |
| <img src="../../../screenshots/strategy-studio.png" width="400"/> | <img src="../../../screenshots/strategy-indicators.png" width="400"/> |

</details>

<details>
<summary><b>竞赛</b></summary>

|                     竞赛模式                      |
| :-------------------------------------------------------: |
| <img src="../../../screenshots/competition-page.png" width="400"/> |

</details>

---

## 安装

### Linux / macOS

```bash
curl -fsSL https://raw.githubusercontent.com/NoFxAiOS/nofx/main/install.sh | bash
```

### Railway（云部署）

[![Deploy on Railway](https://railway.com/button.svg)](https://railway.com/deploy/nofx?referralCode=nofx)

### Docker

```bash
curl -O https://raw.githubusercontent.com/NoFxAiOS/nofx/main/docker-compose.prod.yml
docker compose -f docker-compose.prod.yml up -d
```

### Windows

安装 [Docker Desktop](https://www.docker.com/products/docker-desktop/)，然后：

```powershell
curl -o docker-compose.prod.yml https://raw.githubusercontent.com/NoFxAiOS/nofx/main/docker-compose.prod.yml
docker compose -f docker-compose.prod.yml up -d
```

### 从源码构建

```bash
# Prerequisites: Go 1.21+, Node.js 18+, TA-Lib
# macOS: brew install ta-lib
# Ubuntu: sudo apt-get install libta-lib0-dev

git clone https://github.com/NoFxAiOS/nofx.git && cd nofx
go build -o nofx && ./nofx
cd web && npm install && npm run dev
```

### 更新

```bash
curl -fsSL https://raw.githubusercontent.com/NoFxAiOS/nofx/main/install.sh | bash
```

---

## 配置

1. 配置 AI 模型访问
2. 连接交易所凭证
3. 构建或导入策略
4. 创建 AI 交易员配置
5. 在仪表板启动、监控并迭代

所有配置均可在 Web UI **http://127.0.0.1:3000** 完成。

---

## 部署到服务器

**HTTP 部署：**

```bash
curl -fsSL https://raw.githubusercontent.com/NoFxAiOS/nofx/main/install.sh | bash
# 通过 http://YOUR_IP:3000 访问
```

**通过 Cloudflare 启用 HTTPS：**

1. 在 [Cloudflare](https://dash.cloudflare.com)（免费套餐）添加域名
2. A 记录指向你的服务器 IP（开启代理）
3. SSL/TLS 选择 Flexible
4. 在 `.env` 中设置 `TRANSPORT_ENCRYPTION=true`

---

## 架构

```
                              NOFX
    ┌─────────────────────────────────────────────────┐
    │                 Trading Terminal                 │
    │        React + TypeScript + TradingView          │
    │            Crypto Perpetual Futures              │
    ├─────────────────────────────────────────────────┤
    │                  API Server (Go)                  │
    ├──────────────┬──────────────┬───────────────────┤
    │   Strategy    │   Telegram   │   Trader Runtime  │
    │    Engine     │    Agent     │   Risk Controls   │
    ├──────────────┴──────────────┴───────────────────┤
    │                 AI Model Layer                    │
    │      API-key providers (BYO key, 8 vendors)       │
    │    Model routing · execution support              │
    ├─────────────────────────────────────────────────┤
    │              Exchange Connectivity                │
    │ Binance · Bybit · OKX · Bitget · KuCoin          │
    │ Gate · Indodax · Aster · Lighter                 │
    └─────────────────────────────────────────────────┘
```

---

## 文档

| | |
| :--- | :--- |
| [架构概览](../../architecture/README.md) | 系统设计和模块索引 |
| [策略模块](../../architecture/STRATEGY_MODULE.md) | 币种选择、AI 提示词、执行 |
| [常见问题](../../faq/README.md) | FAQ |
| [快速开始](../../getting-started/README.md) | 部署指南 |

---

## 贡献

查看 [贡献指南](../../../CONTRIBUTING.md)、[行为准则](../../../CODE_OF_CONDUCT.md) 与 [安全政策](../../../SECURITY.md)。

### 贡献者计划

NOFX 会记录有价值的贡献，并计划在生态增长后回馈贡献者。优先级 Issue 拥有更高奖励权重。

| Contribution | Weight |
| :--- | :---: |
| Pinned Issue PRs | ★★★★★★ |
| Code (Merged PRs) | ★★★★★ |
| Bug Fixes | ★★★★ |
| Feature Ideas | ★★★ |
| Bug Reports | ★★ |
| Documentation | ★★ |

---

## 链接

| | |
| :--- | :--- |
| 官网 | [vergex.trade](https://vergex.trade) |
| Dashboard | [vergex.trade/explore](https://vergex.trade/explore) |
| Telegram | [nofx_dev_community](https://t.me/nofx_dev_community) |
| Twitter | [@vergex_ai](https://x.com/vergex_ai) |

> **风险提示**：自动化交易存在重大风险。请控制仓位，理解每个交易场所的机制，不要投入无法承受损失的资金。

---

## 赞助者

<a href="https://github.com/pjl914335852-ux"><img src="https://github.com/pjl914335852-ux.png" width="50" height="50" style="border-radius:50%"/></a>
<a href="https://github.com/cat9999aaa"><img src="https://github.com/cat9999aaa.png" width="50" height="50" style="border-radius:50%"/></a>
<a href="https://github.com/1733055465"><img src="https://github.com/1733055465.png" width="50" height="50" style="border-radius:50%"/></a>
<a href="https://github.com/kolal2020"><img src="https://github.com/kolal2020.png" width="50" height="50" style="border-radius:50%"/></a>
<a href="https://github.com/CyberFFarm"><img src="https://github.com/CyberFFarm.png" width="50" height="50" style="border-radius:50%"/></a>
<a href="https://github.com/vip3001003"><img src="https://github.com/vip3001003.png" width="50" height="50" style="border-radius:50%"/></a>
<a href="https://github.com/mrtluh"><img src="https://github.com/mrtluh.png" width="50" height="50" style="border-radius:50%"/></a>
<a href="https://github.com/cpcp1117-source"><img src="https://github.com/cpcp1117-source.png" width="50" height="50" style="border-radius:50%"/></a>
<a href="https://github.com/match-007"><img src="https://github.com/match-007.png" width="50" height="50" style="border-radius:50%"/></a>
<a href="https://github.com/leiwuhen1715"><img src="https://github.com/leiwuhen1715.png" width="50" height="50" style="border-radius:50%"/></a>
<a href="https://github.com/SHAOXIA1991"><img src="https://github.com/SHAOXIA1991.png" width="50" height="50" style="border-radius:50%"/></a>

[成为赞助者](https://github.com/sponsors/NoFxAiOS)

## License

[AGPL-3.0](../../../LICENSE)

[![Star History Chart](https://api.star-history.com/svg?repos=NoFxAiOS/nofx&type=Date)](https://star-history.com/#NoFxAiOS/nofx&Date)
