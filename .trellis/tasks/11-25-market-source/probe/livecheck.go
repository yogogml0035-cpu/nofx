//go:build ignore

// 子任务 12 真实取数探针：7 个标的 × 12 个保留周期全部走 Gate 公开行情；
// 校验时间间距、OHLC、成交量（sum）、ticker 与最新 K 线一致性，并拒绝非保留周期。
// 公开行情无需 API key。输出逐步 JSONL 与完整日志。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"nofx/market"
)

var (
	symbols = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "ZECUSDT", "HYPEUSDT", "DOGEUSDT", "XAUUSDT"}
	out     *os.File
	step    int
	failed  int
)

func fail(format string, args ...interface{}) {
	failed++
	fmt.Printf("FAIL: "+format+"\n", args...)
}

func logf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(msg)
	if out != nil {
		step++
		line, _ := json.Marshal(map[string]interface{}{"step": step, "ts": time.Now().Format(time.RFC3339), "msg": msg})
		out.Write(append(line, '\n'))
		out.Sync()
	}
}

// rawCandle 独立解析一次原始响应，用于核对 Volume 等于 sum（不经交付代码的模型）
type rawCandle struct {
	T   json.Number `json:"t"`
	C   json.Number `json:"c"`
	Sum json.Number `json:"sum"`
}

func main() {
	flag.Parse()
	if path := flag.Arg(0); path != "" {
		f, err := os.Create(path)
		if err != nil {
			fmt.Printf("FAIL: create out: %v\n", err)
			os.Exit(1)
		}
		out = f
		defer out.Close()
	}

	intervals := []string{"10s", "1m", "5m", "15m", "30m", "1h", "4h", "8h", "1d", "7d", "1w", "30d"}
	const base = "https://api.gateio.ws/api/v4"

	for _, symbol := range symbols {
		contract := symbol[:len(symbol)-4] + "_USDT"
		ticker, err := market.GetGateTicker(symbol)
		if err != nil {
			fail("%s ticker: %v", symbol, err)
			continue
		}
		if ticker.Last <= 0 {
			fail("%s ticker last = %v", symbol, ticker.Last)
		}
		logf("%s ticker: last=%.8f change24h=%.4f%% quote24h=%.2f", symbol, ticker.Last, ticker.ChangePercent24h, ticker.QuoteVolume24h)

		for _, interval := range intervals {
			duration, err := market.GateIntervalDuration(interval)
			if err != nil {
				fail("%s %s duration: %v", symbol, interval, err)
				continue
			}
			klines, err := market.GetGateKlines(symbol, interval, 200)
			if err != nil {
				fail("%s %s klines: %v", symbol, interval, err)
				continue
			}
			if len(klines) < 2 {
				fail("%s %s returned %d bars", symbol, interval, len(klines))
				continue
			}
			spacing := duration.Seconds()
			badSpacing := 0
			for i := 1; i < len(klines); i++ {
				stepSeconds := float64(klines[i].OpenTime-klines[i-1].OpenTime) / 1000
				if interval == "30d" {
					// 30d 是自然月：28–31 天
					if stepSeconds < 28*24*3600 || stepSeconds > 31*24*3600 {
						badSpacing++
					}
					continue
				}
				if stepSeconds != spacing {
					badSpacing++
				}
			}
			if interval == "1w" {
				for _, bar := range klines {
					if time.UnixMilli(bar.OpenTime).UTC().Weekday() != time.Monday {
						fail("%s 1w bar %s is not Monday-aligned", symbol, time.UnixMilli(bar.OpenTime).UTC())
						break
					}
				}
			}
			if interval == "30d" {
				for _, bar := range klines {
					if time.UnixMilli(bar.OpenTime).UTC().Day() != 1 {
						fail("%s 30d bar %s is not month-start aligned", symbol, time.UnixMilli(bar.OpenTime).UTC())
						break
					}
				}
			}
			if badSpacing > 0 {
				fail("%s %s has %d/%d bars with spacing != %v", symbol, interval, badSpacing, len(klines)-1, duration)
			}
			last := klines[len(klines)-1]
			drift := 0.0
			if ticker.Last > 0 {
				drift = (last.Close - ticker.Last) / ticker.Last * 100
				if drift < 0 {
					drift = -drift
				}
			}
			if drift > 5 {
				fail("%s %s last close %.8f drifts %.2f%% from ticker %.8f", symbol, interval, last.Close, drift, ticker.Last)
			}
			failIfNegative(fmt.Sprintf("%s %s last", symbol, interval), last)
			logf("%s %s: bars=%d first=%s last=%s close=%.8f volume=%.2f spacingOK=%v tickerDrift=%.4f%%",
				symbol, interval, len(klines),
				time.UnixMilli(klines[0].OpenTime).UTC().Format(time.RFC3339),
				time.UnixMilli(last.OpenTime).UTC().Format(time.RFC3339),
				last.Close, last.Volume, badSpacing == 0, drift)
		}

		// 独立核对：最后一根的 Volume 等于原始响应的 sum
		url := fmt.Sprintf("%s/futures/usdt/candlesticks?contract=%s&interval=1m&limit=3", base, contract)
		resp, err := http.Get(url)
		if err != nil {
			fail("%s raw klines: %v", symbol, err)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			fail("%s raw klines status %d err %v", symbol, resp.StatusCode, err)
			continue
		}
		var raw []rawCandle
		if err := json.Unmarshal(body, &raw); err != nil || len(raw) < 2 {
			fail("%s raw klines decode: %v (bars=%d)", symbol, err, len(raw))
			continue
		}
		mapped, err := market.GetGateKlines(symbol, "1m", 5)
		if err != nil || len(mapped) == 0 {
			fail("%s mapped klines: %v", symbol, err)
			continue
		}
		// 取原始响应里倒数第二根（已收线，避免两次请求之间新开一分钟造成边界错位），
		// 按开盘时间在映射结果里找同一根，核对成交量等于 sum。
		rawBar := raw[len(raw)-2]
		rawT, err := strconv.ParseFloat(string(rawBar.T), 64)
		if err != nil {
			fail("%s raw t: %v", symbol, err)
			continue
		}
		wantSum, err := strconv.ParseFloat(string(rawBar.Sum), 64)
		if err != nil {
			fail("%s raw sum: %v", symbol, err)
			continue
		}
		openMillis := int64(rawT) * 1000
		found := false
		for _, bar := range mapped {
			if bar.OpenTime != openMillis {
				continue
			}
			found = true
			if bar.Volume != wantSum {
				fail("%s volume %.8f != raw sum %.8f at %d", symbol, bar.Volume, wantSum, openMillis)
			} else {
				logf("%s volume check: mapped=%.8f rawSum=%.8f at %d", symbol, bar.Volume, wantSum, openMillis)
			}
		}
		if !found {
			fail("%s mapped klines do not contain the sampled raw bar %d", symbol, openMillis)
		}
	}

	// 全合约 24h 列表（⑮ 的输入）
	tickers, err := market.ListGateTickers()
	if err != nil {
		fail("list tickers: %v", err)
	} else {
		logf("list tickers: %d contracts, first=%s last=%.8f", len(tickers), tickers[0].Contract, tickers[0].Last)
	}

	// 非保留周期必须被拒（不发请求）
	for _, interval := range []string{"3m", "2h", "6h", "12h", "3d"} {
		if _, err := market.GetGateKlines("BTCUSDT", interval, 200); err == nil {
			fail("interval %s must be rejected", interval)
		}
	}
	logf("non-retained intervals rejected")

	if failed == 0 {
		logf("PROBE_PASS")
	} else {
		logf("PROBE_FAIL failures=%d", failed)
		os.Exit(1)
	}
}

func failIfNegative(label string, bar market.Kline) {
	if bar.Open <= 0 || bar.High <= 0 || bar.Low <= 0 || bar.Close <= 0 || bar.Volume < 0 {
		fail("%s has invalid OHLCV: %+v", label, bar)
	}
	if bar.Low > bar.Open || bar.Open > bar.High || bar.Low > bar.Close || bar.Close > bar.High {
		fail("%s has inconsistent OHLC: %+v", label, bar)
	}
	if bar.CloseTime <= bar.OpenTime {
		fail("%s close time not after open time: %+v", label, bar)
	}
}
