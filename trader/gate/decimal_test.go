package gate

import (
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustRat(t *testing.T, raw string) *big.Rat {
	t.Helper()
	value, err := parseDecimal("test value", raw)
	require.NoError(t, err)
	return value
}

func TestParseDecimal(t *testing.T) {
	cases := []struct {
		raw     string
		want    string // 有理数的最简分数形式
		wantErr bool
	}{
		{raw: "0.0001", want: "1/10000"},
		{raw: "0.1", want: "1/10"},
		{raw: "1", want: "1"},
		{raw: "12000000", want: "12000000"},
		{raw: "-0.6", want: "-3/5"},
		{raw: "0.00030000000000000003", want: "30000000000000003/100000000000000000000"},
		{raw: "", wantErr: true},
		{raw: "1e-4", wantErr: true},
		{raw: "1E-4", wantErr: true},
		{raw: ".5", wantErr: true},
		{raw: "1.", wantErr: true},
		{raw: "0.1.2", wantErr: true},
		{raw: "abc", wantErr: true},
		{raw: " 0.1", wantErr: true},
		{raw: "0.1 ", wantErr: true},
		{raw: "NaN", wantErr: true},
		{raw: "+Inf", wantErr: true},
		{raw: "1_000", wantErr: true},
		{raw: "0x10", wantErr: true},
		{raw: "1/3", wantErr: true},
	}
	for _, c := range cases {
		value, err := parseDecimal("field", c.raw)
		if c.wantErr {
			assert.Error(t, err, "raw=%q", c.raw)
			continue
		}
		require.NoError(t, err, "raw=%q", c.raw)
		assert.Equal(t, c.want, value.RatString(), "raw=%q", c.raw)
	}
}

func TestDecimalFromFloat(t *testing.T) {
	value, err := decimalFromFloat("price", 0.3)
	require.NoError(t, err)
	assert.Equal(t, "3/10", value.RatString(), "按最短十进制表示取值，而不是二进制近似值")

	// 3 × 0.0001 的浮点乘法结果略大于 0.0003，最短十进制表示如实保留这一点。
	// 必须在运行时相乘：写成常量表达式会被编译器精确折叠，不产生误差
	var three = 3.0
	noisy, err := decimalFromFloat("quantity", three*0.0001)
	require.NoError(t, err)
	assert.Equal(t, 1, noisy.Cmp(mustRat(t, "0.0003")))

	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err := decimalFromFloat("price", v)
		assert.Error(t, err, "v=%v", v)
	}
}

func TestDecimalScale(t *testing.T) {
	cases := map[string]int{"0.10": 1, "1.0": 0, "1": 0, "0.0001": 4, "100": 0, "0.5": 1, "12.3400": 2}
	for raw, want := range cases {
		assert.Equal(t, want, decimalScale(raw), "raw=%q", raw)
	}
}

func TestFormatDecimal(t *testing.T) {
	cases := []struct {
		value   string
		scale   int
		want    string
		wantErr bool
	}{
		{value: "3", scale: 0, want: "3"},
		{value: "0.6", scale: 1, want: "0.6"},
		{value: "0.01", scale: 2, want: "0.01"},
		{value: "0.1", scale: 3, want: "0.100"},
		{value: "1", scale: 3, want: "1.000"},
		{value: "0", scale: 2, want: "0.00"},
		{value: "0.05", scale: 2, want: "0.05"},
		{value: "-1.5", scale: 1, want: "-1.5"},
		{value: "0.6", scale: 0, wantErr: true},
		{value: "123.45", scale: 1, wantErr: true},
	}
	for _, c := range cases {
		got, err := formatDecimal(mustRat(t, c.value), c.scale)
		if c.wantErr {
			assert.Error(t, err, "value=%s scale=%d", c.value, c.scale)
			continue
		}
		require.NoError(t, err, "value=%s scale=%d", c.value, c.scale)
		assert.Equal(t, c.want, got, "value=%s scale=%d", c.value, c.scale)
	}
}

func TestFloorAndCeilDiv(t *testing.T) {
	assert.Equal(t, "3", floorDiv(mustRat(t, "3.5"), mustRat(t, "1")).String())
	assert.Equal(t, "4", floorDiv(mustRat(t, "4"), mustRat(t, "1")).String())
	assert.Equal(t, "20", floorDiv(mustRat(t, "10"), mustRat(t, "0.5")).String())
	assert.Equal(t, "0", floorDiv(mustRat(t, "0.99"), mustRat(t, "1")).String())

	assert.Equal(t, "4", ceilDiv(mustRat(t, "3.5"), mustRat(t, "1")).String())
	assert.Equal(t, "4", ceilDiv(mustRat(t, "4"), mustRat(t, "1")).String())
	assert.Equal(t, "20", ceilDiv(mustRat(t, "10"), mustRat(t, "0.5")).String())
	assert.Equal(t, "1", ceilDiv(mustRat(t, "0.99"), mustRat(t, "1")).String())
	assert.Equal(t, "0", ceilDiv(mustRat(t, "0"), mustRat(t, "1")).String())
}
