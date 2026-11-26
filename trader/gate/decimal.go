package gate

import (
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// decimalPattern 只接受 Gate 返回的普通十进制写法：可带负号，最多一个小数点，不接受科学计数法
var decimalPattern = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

// parseDecimal 把十进制字符串精确解析为有理数；空串、科学计数法与其他写法都报错
func parseDecimal(field, raw string) (*big.Rat, error) {
	if !decimalPattern.MatchString(raw) {
		return nil, fmt.Errorf("invalid %s %q", field, raw)
	}
	value, ok := new(big.Rat).SetString(raw)
	if !ok {
		return nil, fmt.Errorf("invalid %s %q", field, raw)
	}
	return value, nil
}

// decimalFromFloat 按 float64 的最短十进制表示取值：0.3 就是 3/10，而不是它背后的二进制近似值。
// NaN 与无穷大的输出不是十进制写法，随 parseDecimal 报错。
func decimalFromFloat(field string, v float64) (*big.Rat, error) {
	return parseDecimal(field, strconv.FormatFloat(v, 'f', -1, 64))
}

// decimalScale 返回已校验的十进制字符串去掉尾随 0 后的小数位数（"0.10" 为 1，"1.0" 为 0）
func decimalScale(raw string) int {
	dot := strings.IndexByte(raw, '.')
	if dot < 0 {
		return 0
	}
	return len(strings.TrimRight(raw[dot+1:], "0"))
}

// formatDecimal 按固定小数位数输出有理数；不能在该精度下精确表示时报错
func formatDecimal(value *big.Rat, scale int) (string, error) {
	scaled := new(big.Rat).Mul(value, pow10(scale))
	if !scaled.IsInt() {
		return "", fmt.Errorf("value %s is not a multiple of 10^-%d", value.RatString(), scale)
	}
	number := scaled.Num()
	digits := new(big.Int).Abs(number).String()
	if scale > 0 {
		if len(digits) <= scale {
			digits = strings.Repeat("0", scale-len(digits)+1) + digits
		}
		digits = digits[:len(digits)-scale] + "." + digits[len(digits)-scale:]
	}
	if number.Sign() < 0 {
		digits = "-" + digits
	}
	return digits, nil
}

// pow10 返回 10^n
func pow10(n int) *big.Rat {
	return new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil))
}

// floorDiv 返回 a ÷ b 向下取整的整数（b > 0）。big.Int.Quo 向零截断，负的余数要再减 1
func floorDiv(a, b *big.Rat) *big.Int {
	quotient := new(big.Rat).Quo(a, b)
	numerator, denominator := quotient.Num(), quotient.Denom()
	rounded := new(big.Int).Quo(numerator, denominator)
	if numerator.Sign() < 0 && !quotient.IsInt() {
		rounded.Sub(rounded, big.NewInt(1))
	}
	return rounded
}

// ceilDiv 返回 a ÷ b 向上取整的整数（b > 0）。big.Int.Quo 向零截断，正的余数要再加 1
func ceilDiv(a, b *big.Rat) *big.Int {
	quotient := new(big.Rat).Quo(a, b)
	numerator, denominator := quotient.Num(), quotient.Denom()
	rounded := new(big.Int).Quo(numerator, denominator)
	if numerator.Sign() > 0 && !quotient.IsInt() {
		rounded.Add(rounded, big.NewInt(1))
	}
	return rounded
}

// ratFloat 把有理数转换为最近的 float64（精确十进制运算的结果在此处一次性舍入）
func ratFloat(value *big.Rat) float64 {
	f, _ := value.Float64()
	return f
}
