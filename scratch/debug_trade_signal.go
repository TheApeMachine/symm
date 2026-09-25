package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/compiler"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

func main() {
	errnie.Apply(&errnie.Config{
		Level: "info",
	})

	repo := compiler.DefaultRepository()
	graphDef, err := repo.Load("signals")
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

	ctx := context.Background()

	tradeRecord := map[string]any{
		"channel":     "trade",
		"type":        "update",
		"_connection": 1,
		"_focus":      "BTC/USD",
		"data": map[string]any{
			"symbol":    "BTC/USD",
			"side":      "buy",
			"price":     84500.5,
			"qty":       0.1234,
			"timestamp": "2026-09-25T13:00:00.123456789Z",
		},
	}
	payload, _ := sonic.Marshal(tradeRecord)

	gridIdx := graph.NodeMap["grid"]

	fmt.Println("Executing 5 steps with trade payload directly to grid...")
	for step := 1; step <= 5; step++ {
		_, seg, _ := capnp.NewMessage(capnp.SingleSegment(nil))
		params, _ := store.NewRootGrid_write_Params(seg)
		_ = capnp.Struct(params).CopyFrom(graph.Nodes[gridIdx].ArgsTemplate)
		dataList, _ := params.NewData(1)
		_ = dataList.Set(0, payload)

		initialInputs := map[compiler.NodeID]capnp.Struct{
			gridIdx: capnp.Struct(params),
		}

		if err := graph.Execute(ctx, initialInputs); err != nil {
			fmt.Printf("Step %d execute failed: %v\n", step, err)
			break
		}

		// Check grid results
		if res, ok := graph.Result("grid"); ok && res.IsValid() {
			gridRes := store.Grid_done_Results(res)
			vals, _ := gridRes.Values()
			pres, _ := gridRes.Present()
			fmt.Printf("Step %d [grid]: delivered=%d, %d values, %d present\n",
				step, gridRes.Delivered(), vals.Len(), pres.Len())
		}

		// Check gather readiness
		if res, ok := graph.Result("gather"); ok && res.IsValid() {
			gRes := data.Gathered(res)
			rBytes, _ := gRes.Readiness()
			phase, _ := gRes.Phase()
			which := gRes.Which()
			var rMap map[string]any
			_ = sonic.Unmarshal(rBytes, &rMap)
			fmt.Printf("Step %d [gather]: which=%v phase=%s readiness=%+v\n", step, which, phase, rMap)
		}

		// Check subnodes of cvd_trade, pumpdump_trade, toxicity_trade
		checkSubs := []string{"cvd_trade", "pumpdump_trade", "toxicity_trade", "hawkes_trade"}
		for _, sub := range checkSubs {
			activeCount := 0
			var activeNames []string
			for nodeID := range graph.NodeMap {
				idStr := string(nodeID)
				if strings.HasPrefix(idStr, "definition-"+sub+"__") {
					if res, ok := graph.Result(idStr); ok && res.IsValid() {
						activeCount++
						activeNames = append(activeNames, strings.TrimPrefix(idStr, "definition-"+sub+"__"))
					}
				}
			}
			fmt.Printf("  %s: %d active (%v)\n", sub, activeCount, activeNames)
		}
	}
}
