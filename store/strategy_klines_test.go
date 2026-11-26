package store

import "testing"

// K 线根数上限必须等于 Gate candlesticks 单次查询上限（2000 点）：用户填 2000 能原样
// 保存，超出才夹紧；保存后回读不再被旧上限 30/200 静默改写。
func TestClampLimitsKlineCount(t *testing.T) {
	cases := []struct {
		name  string
		input int
		want  int
	}{
		{name: "2000 保持", input: 2000, want: 2000},
		{name: "超上限夹到 2000", input: 2001, want: 2000},
		{name: "低于下限夹到 10", input: 5, want: 10},
		{name: "缺省值 0 夹到 10", input: 0, want: 10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := GetDefaultStrategyConfig("zh")
			cfg.Indicators.Klines.PrimaryCount = tc.input

			cfg.ClampLimits()

			if cfg.Indicators.Klines.PrimaryCount != tc.want {
				t.Fatalf("PrimaryCount = %d, want %d", cfg.Indicators.Klines.PrimaryCount, tc.want)
			}
		})
	}
}
