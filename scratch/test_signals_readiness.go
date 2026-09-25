package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/compiler"
	"github.com/theapemachine/symm/nomagique/data"
)

func withoutNodes(graph *compiler.Graph, ids ...string) {
	removed := make(map[string]bool, len(ids))
	for _, id := range ids {
		removed[id] = true
		delete(graph.Nodes, id)
	}

	for id, node := range graph.Nodes {
		for _, ports := range []map[string][]compiler.ConnectionTarget{node.Connections.Inputs, node.Connections.Outputs} {
			for port, targets := range ports {
				kept := targets[:0]
				for _, target := range targets {
					if !removed[target.NodeID] {
						kept = append(kept, target)
					}
				}
				ports[port] = kept
				if len(kept) == 0 {
					delete(ports, port)
				}
			}
		}
		graph.Nodes[id] = node
	}
}

func main() {
	errnie.Apply(&errnie.Config{
		Level: "info",
	})

	fmt.Println("Loading system.json...")
	repo := compiler.DefaultRepository()
	graphDef, err := repo.Load("system")
	if err != nil {
		fmt.Printf("Load failed: %v\n", err)
		os.Exit(1)
	}

	withoutNodes(&graphDef, "training", "learning")

	fmt.Println("Compiling system graph...")
	graph, err := compiler.Compile(graphDef, nil, repo)
	if err != nil {
		fmt.Printf("Compile failed: %v\n", err)
		os.Exit(1)
	}
	defer graph.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	l3Records := 0
	spotRecords := 0
	btcTickerCount := 0
	btcTradeCount := 0

	for ctx.Err() == nil {
		if err := graph.Execute(ctx, nil); err != nil {
			fmt.Printf("Execute failed: %v\n", err)
			break
		}

		if res, ok := graph.Result("level3__records"); ok && res.IsValid() {
			iterRes := data.Iterate_done_Results(res)
			if out, _ := iterRes.Out(); len(out) > 0 {
				l3Records++
			}
		}

		if res, ok := graph.Result("spot__records"); ok && res.IsValid() {
			iterRes := data.Iterate_done_Results(res)
			if out, _ := iterRes.Out(); len(out) > 0 {
				spotRecords++
			}
		}

		if res, ok := graph.Result("focus_spot"); ok && res.IsValid() {
			filtRes := data.Filtered(res)
			if filtRes.Which() == data.Filtered_Which_out {
				out, _ := filtRes.Out()
				if len(out) > 0 {
					var doc map[string]any
					if err := sonic.Unmarshal(out, &doc); err == nil {
						ch, _ := doc["channel"].(string)
						if ch == "ticker" {
							btcTickerCount++
						} else if ch == "trade" {
							btcTradeCount++
							fmt.Printf("\n>>> [FOCUS_SPOT BTC/USD TRADE #%d]: %s <<<\n\n",
								btcTradeCount, string(out))
						}
					}
				}
			}
		}

		select {
		case <-ticker.C:
			fmt.Printf("\n=== Tick at %s | spot_recs=%d, l3_recs=%d (BTC: %d tickers, %d trades) ===\n",
				time.Now().Format("15:04:05"), spotRecords, l3Records, btcTickerCount, btcTradeCount)

			// Inspect signals__gather
			if res, ok := graph.Result("signals__gather"); ok && res.IsValid() {
				gRes := data.Gathered(res)
				readinessBytes, _ := gRes.Readiness()
				phase, _ := gRes.Phase()
				which := gRes.Which()

				var readiness map[string]any
				if len(readinessBytes) > 0 {
					_ = sonic.Unmarshal(readinessBytes, &readiness)
				}

				fmt.Printf("[signals__gather]: which=%v, phase=%s, readiness=%+v\n",
					which, phase, readiness)
			} else {
				fmt.Println("[signals__gather]: no valid result yet")
			}

			// Check which definition-* nodes in signals have results
			activeDefs := make(map[string]int)
			for nodeID := range graph.NodeMap {
				idStr := string(nodeID)
				if strings.HasPrefix(idStr, "signals__definition-") {
					parts := strings.Split(idStr, "__")
					if len(parts) >= 2 {
						defName := parts[1]
						if res, ok := graph.Result(idStr); ok && res.IsValid() {
							activeDefs[defName]++
						}
					}
				}
			}
			fmt.Printf("Active signal definition nodes: %d / 15\n", len(activeDefs))
			for name, count := range activeDefs {
				fmt.Printf("  %s (%d active subnodes)\n", name, count)
			}

		default:
			time.Sleep(2 * time.Millisecond)
		}
	}
}
