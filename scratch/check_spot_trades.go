package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/compiler"
	"github.com/theapemachine/symm/nomagique/data"
)

func main() {
	errnie.Apply(&errnie.Config{
		Level: "warn",
	})

	repo := compiler.DefaultRepository()
	graphDef, err := repo.Load("live_spot")
	if err != nil {
		fmt.Printf("Load failed: %v\n", err)
		os.Exit(1)
	}

	graph, err := compiler.Compile(graphDef, nil, repo)
	if err != nil {
		fmt.Printf("Compile failed: %v\n", err)
		os.Exit(1)
	}
	defer graph.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	channelCounts := make(map[string]int)
	symbolCounts := make(map[string]int)
	tradeSymbols := make(map[string]int)

	for ctx.Err() == nil {
		if err := graph.Execute(ctx, nil); err != nil {
			fmt.Printf("Execute failed: %v\n", err)
			break
		}

		if res, ok := graph.Result("records"); ok && res.IsValid() {
			iterRes := data.Iterate_done_Results(res)
			if out, _ := iterRes.Out(); len(out) > 0 {
				var doc map[string]any
				if err := sonic.Unmarshal(out, &doc); err == nil {
					ch, _ := doc["channel"].(string)
					channelCounts[ch]++
					if d, ok := doc["data"].(map[string]any); ok {
						sym, _ := d["symbol"].(string)
						symbolCounts[sym]++
						if ch == "trade" {
							tradeSymbols[sym]++
							fmt.Printf("[TRADE MATCH]: symbol=%s side=%v price=%v qty=%v\n",
								sym, d["side"], d["price"], d["qty"])
						}
					}
				}
			}
		}

		time.Sleep(2 * time.Millisecond)
	}

	fmt.Printf("\nChannels received: %+v\n", channelCounts)
	fmt.Printf("Trade symbols received: %+v\n", tradeSymbols)
	fmt.Printf("Total unique symbols: %d\n", len(symbolCounts))
}
