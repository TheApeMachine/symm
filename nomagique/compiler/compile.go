package compiler

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Compile lowers a declarative JSON Graph into an in-memory executable nomagique composition.
Topologies are resolved once during compilation into nested nomagique primitives
(NewNumber, NewFan, NewFork, NewJoin, NewTee).
Zero map allocations or graph edge traversals occur on the execution hot path.
*/
func Compile[In, Out any](
	graph Graph,
	reg *Registry,
	repos ...DefinitionRepository,
) (types.Value[In, Out], error) {
	if reg == nil {
		reg = DefaultRegistry()
	}

	var repo DefinitionRepository
	if len(repos) > 0 {
		repo = repos[0]
		if reg.Repository() == nil {
			reg.SetRepository(repo)
		}
	} else if reg.Repository() != nil {
		repo = reg.Repository()
	}

	if len(graph.Nodes) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("compiler: graph %q contains no nodes", graph.Name),
			nil,
		))
	}

	errnie.Debug(fmt.Sprintf("[compiler.Compile] compiling graph %s (%d nodes)...", graph.Name, len(graph.Nodes)))

	// 1. Build adjacency and in-degree maps for operational and source/sink nodes
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

	// 2. Topological Sort (Kahn's Algorithm) to guarantee acyclicity
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

	// 3. Instantiate operational nodes and recursively resolve definition references
	instances := make(map[string]types.Value[any, any])
	for id, node := range graph.Nodes {
		if isSource(node) || isSink(node) {
			continue
		}

		if strings.HasPrefix(node.Type, "definition:") {
			defName := strings.TrimPrefix(node.Type, "definition:")
			if repo == nil {
				return nil, errnie.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf("compiler: node %q references definition %q but no repository provided", id, defName),
					nil,
				))
			}

			childGraph, err := repo.Load(defName)
			if err != nil {
				return nil, errnie.Error(errnie.Err(
					errnie.NotFound,
					fmt.Sprintf("compiler: child definition %q not found for node %q", defName, id),
					err,
				))
			}

			childClosure, err := Compile[any, any](childGraph, reg, repo)
			if err != nil {
				return nil, errnie.Error(err)
			}

			instances[id] = childClosure
			errnie.Debug(fmt.Sprintf("[compiler.Compile] recursively compiled definition %s for node %s", defName, id))
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

	// If no operational nodes exist, verify if source connects directly to sink (explicit identity)
	if len(opOrder) == 0 {
		hasDirectPassThrough := false
		for _, node := range graph.Nodes {
			if isSource(node) {
				for _, targets := range node.Connections.Outputs {
					for _, target := range targets {
						if sinkNode, exists := graph.Nodes[target.NodeID]; exists && isSink(sinkNode) {
							hasDirectPassThrough = true
							break
						}
					}
				}
			}
		}

		if !hasDirectPassThrough {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("compiler: graph %s has no operational nodes and is not a direct pass-through", graph.Name),
				nil,
			))
		}

		return func(in In) Out {
			return any(in).(Out)
		}, nil
	}

	// 4. Lower graph topology directly into nested nomagique composition closures
	composed, err := lowerTopology(graph, opOrder, incoming, adjacency, instances)
	if err != nil {
		return nil, errnie.Error(err)
	}

	errnie.Debug(fmt.Sprintf("[compiler.Compile] graph %s successfully compiled (%d op nodes)", graph.Name, len(opOrder)))

	return func(in In) Out {
		res := composed(in)
		if out, ok := res.(Out); ok {
			return out
		}
		// If Out is any, interface conversion succeeds
		if anyOut, ok := any(res).(Out); ok {
			return anyOut
		}
		panic(fmt.Sprintf("compiler: boundary type mismatch for graph %s: expected %T, got %T", graph.Name, *new(Out), res))
	}, nil
}

/*
CompileFile reads a JSON graph from disk and compiles it into an executable closure.
*/
func CompileFile[In, Out any](
	jsonPath string,
	reg *Registry,
	repos ...DefinitionRepository,
) (types.Value[In, Out], error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("compiler: read %s", jsonPath),
			err,
		))
	}

	var graph Graph
	if err := sonic.Unmarshal(data, &graph); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("compiler: unmarshal %s", jsonPath),
			err,
		))
	}

	return Compile[In, Out](graph, reg, repos...)
}

/*
lowerTopology transforms DAG node instances into directly nested nomagique primitives
without runtime slot allocation or interpretive loops.
*/
func lowerTopology(
	graph Graph,
	opOrder []string,
	incoming map[string][]string,
	adjacency map[string][]string,
	instances map[string]types.Value[any, any],
) (types.Value[any, any], error) {
	// Case 1: Pure linear pipeline (each node has <= 1 operational input and <= 1 operational child)
	isLinear := true
	for _, id := range opOrder {
		opsInputs := 0
		for _, parent := range incoming[id] {
			if _, ok := instances[parent]; ok {
				opsInputs++
			}
		}
		if opsInputs > 1 {
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
		ingressIdx := -1
		for i, id := range opOrder {
			if graph.Nodes[id].Type == "transport.WSStream" {
				ingressIdx = i
				break
			}
		}

		if ingressIdx >= 0 {
			upstreamStages := make([]types.Value[any, any], ingressIdx)
			for i := 0; i < ingressIdx; i++ {
				upstreamStages[i] = instances[opOrder[i]]
			}

			downstreamStages := make([]types.Value[any, any], len(opOrder)-(ingressIdx+1))
			for i := ingressIdx + 1; i < len(opOrder); i++ {
				downstreamStages[i-(ingressIdx+1)] = instances[opOrder[i]]
			}
			downstreamPipeline := nomagique.NewNumber[any](downstreamStages...)
			ingressClosure := instances[opOrder[ingressIdx]]

			return func(in any) any {
				if inCtx, ok := in.(context.Context); ok && inCtx != nil {
					var subPayload any
					if len(upstreamStages) > 0 {
						subPayload = nomagique.NewNumber[any](upstreamStages...)(nil)
					}
					streamVal := ingressClosure(subPayload)
					if stream, ok := streamVal.(transport.WSStream); ok {
						return stream(types.Value[any, any](downstreamPipeline))
					}
					return nil
				}
				return downstreamPipeline(in)
			}, nil
		}

		stages := make([]types.Value[any, any], len(opOrder))
		for i, id := range opOrder {
			stages[i] = instances[id]
		}
		return types.Value[any, any](nomagique.NewNumber[any](stages...)), nil
	}

	// Case 2: Fan-out from a common input/source across independent branches
	// Check if all roots receive from source and do not rejoin
	rootNodes := make([]string, 0)
	for _, id := range opOrder {
		opsInputs := 0
		for _, parent := range incoming[id] {
			if _, ok := instances[parent]; ok {
				opsInputs++
			}
		}
		if opsInputs == 0 {
			rootNodes = append(rootNodes, id)
		}
	}

	// Build direct node evaluator map for composed DAG branches
	// Each operational node computes its output from its immediate upstream dependencies
	nodeClosures := make(map[string]types.Value[any, any])

	for _, id := range opOrder {
		instance := instances[id]
		var upstreamOps []string

		for _, upID := range incoming[id] {
			if _, exists := instances[upID]; exists {
				upstreamOps = append(upstreamOps, upID)
			}
		}

		if len(upstreamOps) == 0 {
			// Receives directly from graph input
			nodeClosures[id] = instance
		} else if len(upstreamOps) == 1 {
			// Chained directly to upstream output closure
			upClosure := nodeClosures[upstreamOps[0]]
			nodeClosures[id] = func(in any) any {
				return instance(upClosure(in))
			}
		} else {
			// Explicit collection/join of upstream outputs
			upClosures := make([]types.Value[any, any], len(upstreamOps))
			for i, upID := range upstreamOps {
				upClosures[i] = nodeClosures[upID]
			}

			fork := transport.NewFork[any, any](upClosures...)
			nodeClosures[id] = func(in any) any {
				branchOutputs := fork(in)
				return instance(branchOutputs)
			}
		}
	}

	// Find terminal operational nodes (nodes with no operational children)
	terminalNodes := make([]string, 0)
	for _, id := range opOrder {
		hasOpChild := false
		for _, child := range adjacency[id] {
			if _, exists := instances[child]; exists {
				hasOpChild = true
				break
			}
		}
		if !hasOpChild {
			terminalNodes = append(terminalNodes, id)
		}
	}

	if len(terminalNodes) == 0 {
		terminalNodes = append(terminalNodes, opOrder[len(opOrder)-1])
	}

	if len(terminalNodes) == 1 {
		return nodeClosures[terminalNodes[0]], nil
	}

	// Multiple terminal nodes lower into transport.NewFork
	terminalClosures := make([]types.Value[any, any], len(terminalNodes))
	for i, termID := range terminalNodes {
		terminalClosures[i] = nodeClosures[termID]
	}

	fork := transport.NewFork[any, any](terminalClosures...)
	return func(in any) any {
		return fork(in)
	}, nil
}

func isSource(node Node) bool {
	return node.Type == "data.Source" || node.Type == "source"
}

func isSink(node Node) bool {
	return node.Type == "data.Sink" || node.Type == "sink"
}
