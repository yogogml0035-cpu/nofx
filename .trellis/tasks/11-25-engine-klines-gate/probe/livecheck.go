//go:build ignore

// 子任务 13 真实探针：引擎取数全部走 Gate。
//  1. 12 个保留周期逐一经 GetWithTimeframes（含 10s/7d/30d，F41）
//  2. GetBoxData 走 Gate 1h
//  3. 用真实行情构建 User/System 提示词（不调用 AI），核对新周期与 USDT 成交量口径
//
// 公开行情无需 API key。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"nofx/kernel"
	"nofx/market"
	"nofx/store"
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

	intervals := []string{"10s", "1m", "5m", "15m", "30m", "1h", "4h", "8h", "1d", "7d", "1w", "30d"}
	for _, tf := range intervals {
		data, err := market.GetWithTimeframes("DOGEUSDT", []string{tf}, tf, 30)
		if err != nil {
			fail("GetWithTimeframes DOGEUSDT %s: %v", tf, err)
			continue
		}
		series, ok := data.TimeframeData[tf]
		if !ok || series == nil || len(series.Klines) == 0 {
			fail("DOGEUSDT %s: timeframe data missing", tf)
			continue
		}
		last := series.Klines[len(series.Klines)-1]
		if data.CurrentPrice <= 0 || last.Close <= 0 || last.Volume < 0 {
			fail("DOGEUSDT %s: invalid price/volume current=%v close=%v volume=%v", tf, data.CurrentPrice, last.Close, last.Volume)
		}
		if last.Volume == 0 {
			logf("note: DOGEUSDT %s last bar quote volume is 0 (no trades in that period)", tf)
		}
		logf("DOGEUSDT %s: klines=%d current=%.8f lastClose=%.8f lastVolumeUSDT=%.2f", tf, len(series.Klines), data.CurrentPrice, last.Close, last.Volume)
	}

	// 非保留周期必须失败（不会被引擎静默当成有效周期）
	if _, err := market.GetWithTimeframes("DOGEUSDT", []string{"3m"}, "3m", 5); err == nil {
		fail("3m must be rejected by GetWithTimeframes")
	} else {
		logf("3m rejected: %v", err)
	}

	// 箱体（Gate 1h × 500）
	box, err := market.GetBoxData("DOGEUSDT")
	if err != nil {
		fail("GetBoxData: %v", err)
	} else if box.ShortUpper <= 0 || box.LongUpper <= 0 || box.CurrentPrice <= 0 {
		fail("box data invalid: %+v", box)
	} else {
		logf("box: short=[%.6f,%.6f] mid=[%.6f,%.6f] long=[%.6f,%.6f] current=%.6f",
			box.ShortLower, box.ShortUpper, box.MidLower, box.MidUpper, box.LongLower, box.LongUpper, box.CurrentPrice)
	}

	// 多周期行情 + 提示词（不调用 AI）
	selected := []string{"10s", "5m", "1h", "1d", "7d", "30d"}
	data, err := market.GetWithTimeframes("DOGEUSDT", selected, "5m", 30)
	if err != nil {
		fail("multi-timeframe data: %v", err)
	} else {
		cfg := store.GetDefaultStrategyConfig("en")
		cfg.Indicators.Klines.SelectedTimeframes = selected
		cfg.Indicators.Klines.PrimaryTimeframe = "5m"
		cfg.Indicators.Klines.PrimaryCount = 30
		engine := kernel.NewStrategyEngine(&cfg)
		ctx := &kernel.Context{
			CurrentTime: time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
			Account: kernel.AccountInfo{
				Principal: 1000, TotalEquity: 1000, AvailableBalance: 1000,
			},
			CandidateCoins: []kernel.CandidateCoin{{Symbol: "DOGEUSDT"}},
			MarketDataMap:  map[string]*market.Data{"DOGEUSDT": data},
		}
		userPrompt := engine.BuildUserPrompt(ctx)
		systemPrompt := engine.BuildSystemPrompt(1000, "balanced")

		for _, marker := range []string{"=== 10S Timeframe", "=== 7D Timeframe", "=== 30D Timeframe", "Volume(USDT)"} {
			if !strings.Contains(userPrompt, marker) {
				fail("user prompt is missing %q", marker)
			}
		}
		if !strings.Contains(systemPrompt, "USDT quote volume") {
			fail("system prompt does not declare USDT quote volume")
		}
		// 摘录提示词里的行情段落（只保留包含 Timeframe/Volume 的关键行）作为证据
		for _, line := range strings.Split(userPrompt, "\n") {
			if strings.Contains(line, "Timeframe") || strings.Contains(line, "Volume(USDT)") {
				logf("prompt line: %s", line)
			}
		}
		logf("prompt: gate prices present current=%.8f; new periods rendered", data.CurrentPrice)
	}

	if failed == 0 {
		logf("PROBE_PASS")
	} else {
		logf("PROBE_FAIL failures=%d", failed)
		os.Exit(1)
	}
}
