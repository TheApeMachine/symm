package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/compiler"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/network/websocket"
	"github.com/theapemachine/symm/nomagique/transport"
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

	fmt.Println("Compiled nodes matching level3:")
	for nodeID := range graph.NodeMap {
		if strings.Contains(string(nodeID), "level3") {
			fmt.Printf("  %s\n", nodeID)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	lastPrint := time.Now()
	spotCount := 0
	l3Count := 0

	for ctx.Err() == nil {
		if err := graph.Execute(ctx, nil); err != nil {
			fmt.Printf("Execute failed: %v\n", err)
			break
		}

		// Check raw records from spot and level3
		if res, ok := graph.Result("spot__records"); ok && res.IsValid() {
			iterRes := data.Iterate_done_Results(res)
			out, _ := iterRes.Out()
			if len(out) > 0 {
				spotCount++
				if spotCount <= 3 {
					fmt.Printf("[SPOT RAW #%d]: %s\n", spotCount, string(out))
				}
			}
		}

		// Inspect level3 pipeline
		if res, ok := graph.Result("level3__symbols"); ok && res.IsValid() {
			fanRes := transport.Fan_done_Results(res)
			out, _ := fanRes.Out()
			if len(out) > 0 && l3Count == 0 {
				fmt.Printf("[L3 symbols]: len=%d\n", len(out))
			}
		}

		if res, ok := graph.Result("level3__chunk"); ok && res.IsValid() {
			chunkRes := data.Chunk_done_Results(res)
			out0, _ := chunkRes.Out0()
			ep0, _ := chunkRes.Endpoint0()
			if len(out0) > 0 || len(ep0) > 0 {
				fmt.Printf("[L3 chunk]: ep0=%s, out0 len=%d\n", ep0, len(out0))
			}
		}

		if res, ok := graph.Result("level3__shard_0__socket"); ok && res.IsValid() {
			sockRes := websocket.Received(res)
			status := sockRes.Status()
			which := sockRes.Which()
			if which != websocket.Received_Which_idle {
				fmt.Printf("[L3 shard_0 socket]: status=%v, which=%v\n", status, which)
			}
		}

		if res, ok := graph.Result("level3__shard_0__token"); ok && res.IsValid() {
			fmt.Printf("[L3 shard_0 token]: has result\n")
		}

		if res, ok := graph.Result("level3__shard_0__subscription"); ok && res.IsValid() {
			fmt.Printf("[L3 shard_0 subscription]: has result\n")
		}

		if res, ok := graph.Result("level3__shard_0__books"); ok && res.IsValid() {
			filtRes := data.Filtered(res)
			if filtRes.Which() == data.Filtered_Which_out {
				out, _ := filtRes.Out()
				fmt.Printf("[L3 shard_0 books MATCH!]: %s\n", string(out))
			}
		}

		if res, ok := graph.Result("level3__records"); ok && res.IsValid() {
			iterRes := data.Iterate_done_Results(res)
			out, _ := iterRes.Out()
			if len(out) > 0 {
				l3Count++
				fmt.Printf("[L3 RECORD #%d]: %s\n", l3Count, string(out))
			}
		}

		if res, ok := graph.Result("focus_level3"); ok && res.IsValid() {
			filterRes := data.Filtered(res)
			if filterRes.Which() == data.Filtered_Which_out {
				out, _ := filterRes.Out()
				fmt.Printf("[FOCUS_L3 MATCH!]: %s\n", string(out))
			}
		}

		if time.Since(lastPrint) >= 2*time.Second {
			lastPrint = time.Now()
			fmt.Printf("\n--- Status at %s (l3_records=%d) ---\n", time.Now().Format("15:04:05"), l3Count)
			fmt.Println("Nodes under level3 with results:")
			for nodeID := range graph.NodeMap {
				if strings.HasPrefix(string(nodeID), "level3__") {
					if res, ok := graph.Result(string(nodeID)); ok && res.IsValid() {
						fmt.Printf("  %s\n", nodeID)
					}
				}
			}
		}

		time.Sleep(5 * time.Millisecond)
	}
}
