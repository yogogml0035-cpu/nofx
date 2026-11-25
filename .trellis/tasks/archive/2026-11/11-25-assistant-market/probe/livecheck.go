//go:build ignore

// 子任务 15 真实探针：助手行情全部走 Gate。
//  1. ticker（24h 高/低/涨跌/成交额）
//  2. 合约指标（标记价/指数价/资金费率/下次结算/持仓量 USD）
//  3. K 线工具取数（10s/15m/1d/7d/30d，含 D25 新周期）
//
// 公开行情无需 API key。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"nofx/market"
)

var (
	out    *os.File
	step   int
	failed int
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

	symbols := []string{"BTCUSDT", "ETHUSDT", "DOGEUSDT"}
	var dogeTicker market.GateTicker
	for _, symbol := range symbols {
		ticker, err := market.GetGateTicker(symbol)
		if err != nil {
			fail("%s ticker: %v", symbol, err)
			continue
		}
		if ticker.Last <= 0 || ticker.QuoteVolume24h < 0 {
			fail("%s ticker invalid: %+v", symbol, ticker)
		}
		if symbol == "DOGEUSDT" {
			dogeTicker = ticker
		}
		logf("%s ticker: last=%.8f chg24h=%.4f%% chg=%.6f high=%.8f low=%.8f quoteVol24h=%.2f",
			symbol, ticker.Last, ticker.ChangePercent24h, ticker.Change24h, ticker.High24h, ticker.Low24h, ticker.QuoteVolume24h)

		metrics, err := market.GetGateContractMetrics(symbol)
		if err != nil {
			fail("%s metrics: %v", symbol, err)
			continue
		}
		if metrics.MarkPrice <= 0 || metrics.IndexPrice <= 0 || metrics.OpenInterestUSD <= 0 {
			fail("%s metrics invalid: %+v", symbol, metrics)
		}
		logf("%s metrics: mark=%.8f index=%.8f funding=%.8f nextFunding=%s oiUSD=%.2f",
			symbol, metrics.MarkPrice, metrics.IndexPrice, metrics.FundingRate,
			time.UnixMilli(metrics.NextFundingTime).UTC().Format(time.RFC3339), metrics.OpenInterestUSD)
	}

	// K 线工具（助手允许的周期里取 D25 新周期与常用周期）
	for _, interval := range []string{"10s", "15m", "1d", "7d", "30d"} {
		klines, err := market.GetGateKlines("DOGEUSDT", interval, 50)
		if err != nil {
			fail("kline DOGEUSDT %s: %v", interval, err)
			continue
		}
		if len(klines) == 0 {
			fail("kline DOGEUSDT %s empty", interval)
			continue
		}
		last := klines[len(klines)-1]
		if last.Close <= 0 || last.Volume < 0 || last.CloseTime <= last.OpenTime {
			fail("kline DOGEUSDT %s invalid last bar: %+v", interval, last)
		}
		if dogeTicker.Last > 0 {
			drift := (last.Close - dogeTicker.Last) / dogeTicker.Last * 100
			if drift < 0 {
				drift = -drift
			}
			if drift > 5 {
				fail("kline DOGEUSDT %s last close %.8f drifts %.2f%% from ticker %.8f", interval, last.Close, drift, dogeTicker.Last)
			}
		}
		if interval == "30d" {
			// 30d 小 limit 修复后：末根必须是月初开盘且为该月真实成交（Gate 小 limit 的占位根）
			lastOpen := time.UnixMilli(last.OpenTime).UTC()
			if lastOpen.Day() != 1 || last.Volume <= 0 {
				fail("kline DOGEUSDT 30d last bar not month-start/real: open=%s volume=%.2f", lastOpen, last.Volume)
			}
		}
		logf("kline DOGEUSDT %s: bars=%d lastClose=%.8f lastVolumeUSDT=%.2f lastOpen=%s",
			interval, len(klines), last.Close, last.Volume,
			time.UnixMilli(last.OpenTime).UTC().Format(time.RFC3339))
	}
	if _, err := market.GetGateKlines("DOGEUSDT", "3m", 10); err == nil {
		fail("3m must be rejected by the kline tool")
	} else {
		logf("3m rejected: %v", err)
	}

	// 全量 tickers（/api/agent/tickers 的数据源）
	tickers, err := market.ListGateTickers()
	if err != nil {
		fail("list tickers: %v", err)
	} else {
		logf("list tickers: %d contracts; BTC=%s", len(tickers), findTicker(tickers, "BTCUSDT"))
	}

	if failed == 0 {
		logf("PROBE_PASS")
	} else {
		logf("PROBE_FAIL failures=%d", failed)
		os.Exit(1)
	}
}

func findTicker(tickers []market.GateTicker, symbol string) string {
	for _, t := range tickers {
		if t.Symbol == symbol {
			return strconv.FormatFloat(t.Last, 'f', -1, 64)
		}
	}
	return "missing"
}
