package market

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 末根 K 线的新鲜度：最后一根的开盘时间距今不超过两个周期
func TestKlinesFresh(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	previous := marketNow
	marketNow = func() time.Time { return now }
	t.Cleanup(func() { marketNow = previous })

	bars := func(openTime int64) []Kline {
		return []Kline{{OpenTime: openTime, Close: 100}}
	}
	// 当前未收线的 1m 根：新鲜
	require.NoError(t, klinesFresh(bars((now.Unix()-30)*1000), "1m"))
	// 上一根（行情源略滞后）：仍在两个周期内
	require.NoError(t, klinesFresh(bars((now.Unix()-90)*1000), "1m"))
	// 更早：不新鲜
	err := klinesFresh(bars((now.Unix()-200)*1000), "1m")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "last bar opened")
	// 10s 周期的容差更小
	assert.Error(t, klinesFresh(bars((now.Unix()-60)*1000), "10s"))
	assert.Error(t, klinesFresh(nil, "1m"), "no klines")
	assert.Error(t, klinesFresh(bars(now.Unix()*1000), "3m"), "non-retained interval")
}
