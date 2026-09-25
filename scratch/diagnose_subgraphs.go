package main

import (
	"fmt"
	"strings"

	"github.com/theapemachine/symm/nomagique/compiler"
)

func main() {
	repo := compiler.DefaultRepository()
	graphDef, err := repo.Load("signals")
	if err != nil {
		fmt.Printf("Load signals failed: %v\n", err)
		return
	}

	p, err := compiler.Compile(graphDef, nil, repo)
	if err != nil {
		fmt.Printf("Compile signals failed: %v\n", err)
		return
	}
	defer p.Release()

	subgraphs := []string{
		"definition-cvd_trade",
		"definition-pumpdump_trade",
		"definition-toxicity_trade",
		"definition-hawkes_trade",
	}

	for _, sub := range subgraphs {
		fmt.Printf("\n=================== %s ===================\n", sub)
		var subNodes []*compiler.CompiledNode
		for i := range p.Nodes {
			node := &p.Nodes[i]
			if strings.HasPrefix(node.ID, sub+"__") {
				subNodes = append(subNodes, node)
			}
		}

		fmt.Printf("Total compiled nodes in %s: %d\n", sub, len(subNodes))

		origins := 0
		blockedByRequired := 0
		for _, node := range subNodes {
			if node.Origin {
				origins++
			}

			// Find incoming routes to this node
			routesCount := 0
			var wiredFields []string
			for _, r := range p.Routes {
				if r.ToNode == node.Index {
					routesCount++
					for name, f := range node.Inputs {
						if f.Index == r.ToField {
							wiredFields = append(wiredFields, name)
						}
					}
				}
			}

			if node.RequiredMask != 0 && routesCount == 0 && !node.Origin {
				blockedByRequired++
				fmt.Printf("  BLOCKED ENTRY NODE: %s (%s): RequiredMask=%016b, WiredInputs=%v\n",
					node.ID, node.Identity.Type, node.RequiredMask, wiredFields)
			}
		}

		fmt.Printf("Origins: %d, Blocked entry nodes: %d\n", origins, blockedByRequired)

		// Print entry nodes (nodes receiving routes from outside this subgraph)
		fmt.Printf("Entry nodes (receiving from grid or outside):\n")
		for _, node := range subNodes {
			for _, r := range p.Routes {
				if r.ToNode == node.Index {
					fromNode := &p.Nodes[r.FromNode]
					if !strings.HasPrefix(fromNode.ID, sub+"__") {
						toFieldName := ""
						for name, f := range node.Inputs {
							if f.Index == r.ToField {
								toFieldName = name
								break
							}
						}
						fmt.Printf("  %s <- %s.%s (to field %s, RequiredMask=%016b, Origin=%v)\n",
							node.ID, fromNode.ID, fromNode.Outputs, toFieldName, node.RequiredMask, node.Origin)
					}
				}
			}
		}
	}
}
