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
	return func(in any) any {
		// Wire state holds the output of every executed node
		state := make(map[string]any)
		state["source"] = in // Inject the tick/stream data

		for _, nodeID := range execOrder {
			node := b.graph.Nodes[nodeID]
			closure, ok := instances[nodeID]
			if !ok {
				// If it's a boundary node (source/sink) or unmapped, skip execution
				continue
			}

			// Gather inputs
			var inputData any
			for _, targets := range node.Connections.Inputs {
				if len(targets) > 0 {
					upstreamID := targets[0].NodeID
					inputData = state[upstreamID]
					break // For simplicity, we assume single main input per port in this demo
				}
			}

			// Execute the atom and save its output to the wire
			state[nodeID] = closure(inputData)
		}

		// Retrieve data from the node connected to the sink
		for _, targets := range b.graph.Nodes["sink"].Connections.Inputs {
			if len(targets) > 0 {
				return state[targets[0].NodeID]
			}
		}
		
		return nil
	}, nil
}
