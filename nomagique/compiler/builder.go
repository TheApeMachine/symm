package compiler

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Builder reads a JSON graph definition and dynamically composes it 
into a single, executable `Value` pipeline at runtime. 
*/
type Builder struct {
	graph Graph
}

func NewBuilder(jsonPath string) (*Builder, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}

	var graph Graph
	if err := json.Unmarshal(data, &graph); err != nil {
		return nil, err
	}

	return &Builder{graph: graph}, nil
}

/*
Interests extracts the required data keys from the graph's source nodes.
*/
func (b *Builder) Interests() [][]string {
	var interests [][]string
	for _, node := range b.graph.Nodes {
		if node.Type == "source" || node.Type == "data.Source" || node.ID == "source" || node.ID == "src" {
			if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if interestStr, ok := cfg["interests"].(string); ok {
					// Temporary fallback mapping for legacy single-string configs
					switch interestStr {
					case "trade":
						interests = append(interests, []string{"trade", "price"})
					case "ticker":
						interests = append(interests, []string{"ticker", "data", "last"})
					case "level3":
						interests = append(interests, []string{"level3", "price"})
					default:
						interests = append(interests, []string{interestStr})
					}
				} else if interestArr, ok := cfg["interests"].([]any); ok {
					// Handle the new structure where interests is an array of arrays
					for _, arr := range interestArr {
						if path, ok := arr.([]any); ok {
							var strPath []string
							for _, p := range path {
								if s, ok := p.(string); ok {
									strPath = append(strPath, s)
								}
							}
							if len(strPath) > 0 {
								interests = append(interests, strPath)
							}
						}
					}
				}
			}
		}
	}
	return interests
}

/*
Compose dynamically wires the graph at runtime.
It returns a single execution closure that runs the entire graph topologically.
*/
func (b *Builder) Compose() (types.Value[any, any], error) {
	instances := make(map[string]types.Value[any, any])
	inDegree := make(map[string]int)
	adjacency := make(map[string][]string)

	// 1. Instantiate the nodes and build dependency graph
	for id, node := range b.graph.Nodes {
		inDegree[id] = 0 // Initialize

		if factory, exists := Registry[node.Type]; exists {
			instances[id] = factory()
		}

		// Map outgoing edges for topological sort
		for _, targets := range node.Connections.Outputs {
			for _, target := range targets {
				adjacency[id] = append(adjacency[id], target.NodeID)
				inDegree[target.NodeID]++
			}
		}
	}

	// 2. Topological Sort (Kahn's Algorithm)
	var queue []string
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	var execOrder []string
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		execOrder = append(execOrder, current)

		for _, neighbor := range adjacency[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(execOrder) != len(b.graph.Nodes) {
		return nil, fmt.Errorf("cycle detected in signal graph: %s", b.graph.Name)
	}

	// 3. Return the dynamic execution closure
	return func(input any) any {
		state := make(map[string]any)

		for id, node := range b.graph.Nodes {
			if node.Type == "source" || node.Type == "data.Source" || id == "source" || id == "src" {
				state[id] = input
			}
		}

		for _, nodeID := range execOrder {
			node := b.graph.Nodes[nodeID]
			closure, ok := instances[nodeID]

			if !ok {
				continue
			}

			var inputData any

			for _, targets := range node.Connections.Inputs {
				if len(targets) > 0 {
					upstreamID := targets[0].NodeID
					inputData = state[upstreamID]
					break
				}
			}

			if inputData == nil {
				state[nodeID] = nil
				continue
			}

			state[nodeID] = closure(inputData)
		}

		for id, node := range b.graph.Nodes {
			if id != "sink" && node.Type != "sink" && node.Type != "data.Sink" {
				continue
			}

			for _, targets := range node.Connections.Inputs {
				if len(targets) == 0 {
					continue
				}

				if value := state[targets[0].NodeID]; value != nil {
					return value
				}
			}
		}

		return nil
	}, nil
}
