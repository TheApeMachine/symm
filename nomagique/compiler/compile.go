package compiler

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Compile lowers a declarative JSON Graph into an in-memory executable nomagique composition.
It instantiates primitives via the provided Registry, reduces topologies (sequences,
forks/fans, joins) into nomagique composition primitives, and returns a types.Value[T, T] closure.
Zero map allocations or graph edge traversals occur on the execution hot path.
*/
func Compile[T any](graph Graph, reg *Registry) (types.Value[T, T], error) {
	if reg == nil {
		reg = DefaultRegistry()
	}

	errnie.Debug(fmt.Sprintf("[compiler.Compile] compiling graph %s (%d nodes)...", graph.Name, len(graph.Nodes)))

	// 1. Build adjacency and in-degree maps
	inDegree := make(map[string]int)
	adjacency := make(map[string][]string)
	incoming := make(map[string][]string)

	for id := range graph.Nodes {
		inDegree[id] = 0
	}

	for id, node := range graph.Nodes {
		for _, targets := range node.Connections.Outputs {
			for _, target := range targets {
				if _, exists := graph.Nodes[target.NodeID]; exists {
					adjacency[id] = append(adjacency[id], target.NodeID)
					incoming[target.NodeID] = append(incoming[target.NodeID], id)
					inDegree[target.NodeID]++
				}
			}
		}
	}

	// 2. Topological Sort (Kahn's Algorithm)
	queue := make([]string, 0)
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	execOrder := make([]string, 0, len(graph.Nodes))
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		execOrder = append(execOrder, curr)

		for _, neighbor := range adjacency[curr] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(execOrder) != len(graph.Nodes) {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("compiler: cycle detected in graph %s", graph.Name),
			nil,
		))
	}

	// 3. Instantiate operational nodes via registry
	instances := make(map[string]types.Value[any, any])
	for id, node := range graph.Nodes {
		if isSource(id, node) || isSink(id, node) {
			continue
		}

		closure, err := reg.Resolve(node)
		if err != nil {
			return nil, errnie.Error(err)
		}

		errnie.Debug(fmt.Sprintf("[compiler.Compile] resolved node %s (%s)", id, node.Type))
		instances[id] = closure
	}

	// Filter operational nodes in topological order
	opOrder := make([]string, 0)
	for _, id := range execOrder {
		if _, ok := instances[id]; ok {
			opOrder = append(opOrder, id)
		}
	}

	if len(opOrder) == 0 {
		return func(in T) T { return in }, nil
	}

	// 4. Analyze topology for nomagique composition primitives
	compiled := composeTopology(graph, opOrder, incoming, adjacency, instances)
	errnie.Debug(fmt.Sprintf("[compiler.Compile] graph %s successfully compiled (%d op nodes)", graph.Name, len(opOrder)))

	return func(in T) T {
		res := compiled(in)
		if typed, ok := res.(T); ok {
			return typed
		}

		var zero T
		return zero
	}, nil
}

/*
CompileFile reads a JSON graph from disk and compiles it into an executable closure.
*/
func CompileFile[T any](jsonPath string, reg *Registry) (types.Value[T, T], error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("compiler: read %s", jsonPath),
			err,
		))
	}

	var graph Graph
	if err := json.Unmarshal(data, &graph); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("compiler: unmarshal %s", jsonPath),
			err,
		))
	}

	return Compile[T](graph, reg)
}

func composeTopology(
	graph Graph,
	opOrder []string,
	incoming map[string][]string,
	adjacency map[string][]string,
	instances map[string]types.Value[any, any],
) types.Value[any, any] {
	// Identify branch nodes (nodes whose only outgoing edges go to sinks)
	// Case A: Pure Linear Pipeline (e.g. system.json, logic.json, execution.json)
	// Every operational node must have at most 1 input and at most 1 operational child.
	isLinear := true
	for _, id := range opOrder {
		if len(incoming[id]) > 1 {
			isLinear = false
			break
		}
		opsChildren := 0
		for _, child := range adjacency[id] {
			if _, ok := instances[child]; ok {
				opsChildren++
			}
		}
		if opsChildren > 1 {
			isLinear = false
			break
		}
	}

	if isLinear {
		stages := make([]types.Value[any, any], len(opOrder))
		for i, id := range opOrder {
			stages[i] = instances[id]
		}
		num := nomagique.NewNumber[any](stages...)
		return types.Value[any, any](num)
	}


	// Case C: General DAG Composition (indexed flat slot array, zero map allocations)
	slotIndex := make(map[string]int)
	for i, id := range opOrder {
		slotIndex[id] = i
	}

	n := len(opOrder)
	inputSlots := make([][]int, n)
	closures := make([]types.Value[any, any], n)

	for i, id := range opOrder {
		closures[i] = instances[id]
		var upIndices []int

		for _, upID := range incoming[id] {
			upNode := graph.Nodes[upID]
			if !isSource(upID, upNode) {
				if idx, found := slotIndex[upID]; found {
					upIndices = append(upIndices, idx)
				}
			}
		}
		inputSlots[i] = upIndices
	}

	// Identify terminal output slot indices
	terminalSlots := make([]int, 0)
	for _, id := range opOrder {
		hasNonSinkChild := false
		for _, child := range adjacency[id] {
			childNode := graph.Nodes[child]
			if !isSink(child, childNode) {
				hasNonSinkChild = true
				break
			}
		}

		if !hasNonSinkChild {
			terminalSlots = append(terminalSlots, slotIndex[id])
		}
	}

	if len(terminalSlots) == 0 && n > 0 {
		terminalSlots = append(terminalSlots, n-1)
	}

	return func(in any) any {
		slots := make([]any, n)

		for i := 0; i < n; i++ {
			var inVal any
			ups := inputSlots[i]
			if len(ups) == 0 {
				inVal = in
			} else if len(ups) == 1 {
				inVal = slots[ups[0]]
			} else if len(ups) == 2 {
				f1, ok1 := slots[ups[0]].(float64)
				f2, ok2 := slots[ups[1]].(float64)
				if ok1 && ok2 {
					inVal = [2]float64{f1, f2}
				} else {
					inVal = []any{slots[ups[0]], slots[ups[1]]}
				}
			} else {
				vals := make([]any, len(ups))
				for j, u := range ups {
					vals[j] = slots[u]
				}
				inVal = vals
			}

			if closures[i] != nil {
				slots[i] = closures[i](inVal)
			}
		}

		if len(terminalSlots) == 1 {
			return slots[terminalSlots[0]]
		}

		results := make([]any, len(terminalSlots))
		for i, slot := range terminalSlots {
			results[i] = slots[slot]
		}
		return results
	}
}

func isSource(id string, node Node) bool {
	return node.Type == "source" || node.Type == "data.Source" || id == "source" || id == "src"
}

func isSink(id string, node Node) bool {
	return node.Type == "sink" || node.Type == "data.Sink" || strings.HasPrefix(id, "sink")
}
