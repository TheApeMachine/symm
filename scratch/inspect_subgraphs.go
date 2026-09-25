package main

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/compiler"
)

func inspectGraph(name string) {
	fmt.Printf("\n================ %s ================\n", name)
	repo := compiler.DefaultRepository()
	graphDef, err := repo.Load(name)
	if err != nil {
		fmt.Printf("Load %s failed: %v\n", name, err)
		return
	}

	p, err := compiler.Compile(graphDef, nil, repo)
	if err != nil {
		fmt.Printf("Compile %s failed: %v\n", name, err)
		return
	}
	defer p.Release()

	fmt.Printf("Total nodes: %d, routes: %d\n", len(p.Nodes), len(p.Routes))

	// Find nodes with no incoming routes (origins or required inputs)
	for i := range p.Nodes {
		node := &p.Nodes[i]
		hasIncoming := false
		for _, r := range p.Routes {
			if r.ToNode == node.Index {
				hasIncoming = true
				break
			}
		}

		if !hasIncoming {
			fmt.Printf("Node %s (type %s): Origin=%v, RequiredMask=%016b\n",
				node.ID, node.Identity.Type, node.Origin, node.RequiredMask)
		}
	}
}

func main() {
	for _, name := range []string{"cvd_trade", "pumpdump_trade", "toxicity_trade", "hawkes_trade"} {
		inspectGraph(name)
	}
}
