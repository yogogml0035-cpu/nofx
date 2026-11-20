package coinank_api

import (
	"strings"

	"nofx/provider/coinank/coinank_enum"
)

// normalizePairSymbol 将常见交易对形态转换为 Coinank 各交易所合约格式：
// Gate 用下划线形态（BTC_USDT）、OKX 用 SWAP 形态（BTC-USDT-SWAP）、其余交易所用紧凑形态（BTCUSDT）。
// 输入接受紧凑、下划线与 OKX 三种形态；识别不出常见报价后缀时原样返回，由上游报错 fast-fail。
func normalizePairSymbol(symbol string, exchange coinank_enum.Exchange) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	switch exchange {
	case coinank_enum.Gate:
		return withQuoteSuffix(s, "_")
	case coinank_enum.Okex:
		base, quote := splitBaseQuote(s)
		if base == "" {
			return s
		}
		return base + "-" + quote + "-SWAP"
	default:
		return compactSymbol(s)
	}
}

// compactSymbol 去掉分隔符与 SWAP 后缀，返回紧凑形态
func compactSymbol(s string) string {
	s = strings.NewReplacer("_", "", "-", "").Replace(s)
	return strings.TrimSuffix(s, "SWAP")
}

// splitBaseQuote 拆出基础币与报价币（识别常见稳定币报价后缀）
func splitBaseQuote(s string) (base, quote string) {
	s = compactSymbol(s)
	switch {
	case strings.HasSuffix(s, "USDT"):
		return strings.TrimSuffix(s, "USDT"), "USDT"
	case strings.HasSuffix(s, "USDC"):
		return strings.TrimSuffix(s, "USDC"), "USDC"
	case strings.HasSuffix(s, "USD"):
		return strings.TrimSuffix(s, "USD"), "USD"
	default:
		return "", ""
	}
}

// withQuoteSuffix 输出 base<sep>quote 形态
func withQuoteSuffix(s, sep string) string {
	base, quote := splitBaseQuote(s)
	if base == "" {
		return s
	}
	return base + sep + quote
}
