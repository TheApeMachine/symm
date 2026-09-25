package main

import (
	"context"
	"fmt"
	"os"
	"time"

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
		Level: "warn",
	})

	repo := compiler.DefaultRepository()
	graphDef, err := repo.Load("system")
	if err != nil {
		fmt.Printf("Load failed: %v\n", err)
		os.Exit(1)
	}

	withoutNodes(&graphDef, "training", "learning")

	graph, err := compiler.Compile(graphDef, nil, repo)
	if err != nil {
		fmt.Printf("Compile failed: %v\n", err)
		os.Exit(1)
	}
	defer graph.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	gatherHits := 0
	mapHits := 0

	for ctx.Err() == nil {
		if err := graph.Execute(ctx, nil); err != nil {
			fmt.Printf("Execute failed: %v\n", err)
			break
		}

		if res, ok := graph.Result("signals__gather"); ok && res.IsValid() {
			gRes := data.Gathered(res)
			if gRes.Which() == data.Gathered_Which_gathered {
				gatherHits++
				if gatherHits%50 == 1 {
					fmt.Printf("[signals__gather #%d GATHERED!]\n", gatherHits)
				}
			}
		}

		// Check live_map nodes (e.g. relaxation, authority)
		if res, ok := graph.Result("live_map__relaxation"); ok && res.IsValid() {
			mapHits++
			if mapHits%50 == 1 {
				fmt.Printf("[live_map__relaxation #%d ACTIVE!]\n", mapHits)
			}
		}

		if res, ok := graph.Result("live_map__authority"); ok && res.IsValid() {
			if mapHits%50 == 1 {
				fmt.Printf("[live_map__authority ACTIVE!]\n")
			}
		}

		time.Sleep(2 * time.Millisecond)
	}

	fmt.Printf("\nDone: gatherHits=%d, mapHits=%d\n", gatherHits, mapHits)
}
