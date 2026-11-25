//go:build probe

// 提示词预览（子任务 9 sizing-engine）：用固定的模拟本金与百分比渲染中英文系统提示词，
// 留档证明开仓金额按「本金 × position_percent」展示、决策契约不含金额字段。
//
// 运行（宿主机无 Go，走 Docker）：
//
//	docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
//	  golang:1.25-alpine go run -tags probe ./.trellis/tasks/11-25-sizing-engine/probe/prompt_preview.go \
//	  -out-dir .trellis/tasks/11-25-sizing-engine/probe
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"nofx/kernel"
	"nofx/store"
)

func main() {
	outDir := flag.String("out-dir", ".", "directory for the rendered prompts")
	flag.Parse()

	// 当前账户本金的真实量级（子任务 7 的带仓读数），百分比取默认 10
	principal := 107.49769421
	for _, lang := range []string{"zh", "en"} {
		cfg := store.GetDefaultStrategyConfig(lang)
		cfg.RiskControl.PositionPercent = 10
		engine := kernel.NewStrategyEngine(&cfg)
		prompt := engine.BuildSystemPrompt(principal, "balanced")

		path := filepath.Join(*outDir, fmt.Sprintf("sizing-prompt-%s.txt", lang))
		if err := os.WriteFile(path, []byte(prompt), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d bytes)\n", path, len(prompt))
	}
}