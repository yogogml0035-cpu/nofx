package market

import "testing"

// 取数根数换算：配置根数低于指标回看下限时多取，超过 Gate 单次查询上限时夹紧。
// 上限取自接口契约（文档：Maximum of 2000 points per query；实测 limit=2001 被拒）。
func TestKlineFetchBars(t *testing.T) {
	cases := []struct {
		name  string
		count int
		want  int
	}{
		{name: "极小配置取回看下限", count: 1, want: klineLookbackFloorBars},
		{name: "下限减一仍取下限", count: klineLookbackFloorBars - 1, want: klineLookbackFloorBars},
		{name: "下限本身保持", count: klineLookbackFloorBars, want: klineLookbackFloorBars},
		{name: "区间内按配置取数", count: 1000, want: 1000},
		{name: "接口上限保持", count: gateKlineMaxLimit, want: gateKlineMaxLimit},
		{name: "超上限夹到接口上限", count: gateKlineMaxLimit + 1, want: gateKlineMaxLimit},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := klineFetchBars(tc.count); got != tc.want {
				t.Fatalf("klineFetchBars(%d) = %d, want %d", tc.count, got, tc.want)
			}
		})
	}
}

func TestGateKlineMaxLimitMatchesDocumentedCap(t *testing.T) {
	if gateKlineMaxLimit != 2000 {
		t.Fatalf("gateKlineMaxLimit = %d, want 2000 (Gate candlesticks per-query cap)", gateKlineMaxLimit)
	}
}
