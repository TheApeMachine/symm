package compiler

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/transport"
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
	isBranch := make(map[string]bool)
	branchNodes := make([]string, 0)
	trunkNodes := make([]string, 0)

	for _, id := range opOrder {
		nonSinkChildren := 0
		for _, child := range adjacency[id] {
			childNode := graph.Nodes[child]
			if !isSink(child, childNode) {
				nonSinkChildren++
			}
		}

		if nonSinkChildren == 0 && len(adjacency[id]) > 0 {
			isBranch[id] = true
			branchNodes = append(branchNodes, id)
		} else {
			trunkNodes = append(trunkNodes, id)
		}
	}

	// Case A: Pure Linear Pipeline (e.g. system.json, logic.json, execution.json)
	if len(branchNodes) <= 1 && len(trunkNodes) == len(opOrder) {
		stages := make([]types.Value[any, any], len(opOrder))
		for i, id := range opOrder {
			stages[i] = instances[id]
		}
		num := nomagique.NewNumber[any](stages...)
		return types.Value[any, any](num)
	}

	// Case B: Linear Trunk with Multi-Sink Fan-Out (e.g. signals like cvd_trade, derivatives_trade)
	// All branch nodes receive input from the last trunk node (or source)
	branchesFromSameParent := true
	if len(branchNodes) > 1 {
		var commonParent string
		for _, bid := range branchNodes {
			parents := incoming[bid]
			operationalParent := ""
			for _, p := range parents {
				pNode := graph.Nodes[p]
				if !isSource(p, pNode) {
					operationalParent = p
					break
				}
			}

			if commonParent == "" {
				commonParent = operationalParent
			} else if operationalParent != commonParent {
				branchesFromSameParent = false
				break
			}
		}
	}

	if len(branchNodes) > 1 && branchesFromSameParent {
		branchClosures := make([]types.Value[any, any], len(branchNodes))
		for i, id := range branchNodes {
			branchClosures[i] = instances[id]
		}
		fan := transport.NewFan[any, any](branchClosures...)
		fanVal := func(in any) any { return fan(in) }

		if len(trunkNodes) > 0 {
			trunkStages := make([]types.Value[any, any], len(trunkNodes))
			for i, id := range trunkNodes {
				trunkStages[i] = instances[id]
			}
			trunkNum := nomagique.NewNumber[any](trunkStages...)
			return types.Value[any, any](nomagique.NewNumber[any](
				types.Value[any, any](trunkNum),
				fanVal,
			))
		}

		return fanVal
	}

	// Case C: General DAG Composition (indexed flat slot array, zero map allocations)
	slotIndex := make(map[string]int)
	for i, id := range opOrder {
		slotIndex[id] = i
	}

	n := len(opOrder)
	inputIndices := make([]int, n)
	closures := make([]types.Value[any, any], n)

	for i, id := range opOrder {
		closures[i] = instances[id]
		inputIndices[i] = -1 // -1 indicates read from graph input

		for _, upID := range incoming[id] {
			upNode := graph.Nodes[upID]
			if !isSource(upID, upNode) {
				if idx, found := slotIndex[upID]; found {
					inputIndices[i] = idx
					break
				}
			}
		}
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
			if inputIndices[i] == -1 {
				inVal = in
			} else {
				inVal = slots[inputIndices[i]]
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
