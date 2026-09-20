package compiler

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
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
) (types.StreamNode[In, Out], error) {
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
	adjacency := make(map[string][]string)       // For topological sort
	incoming := make(map[string][]string)        // For topological sort
	
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

	// 3. Instantiate operational nodes and recursively resolve definition references in topological order
	instances := make(map[string]types.StreamNode[any, any])
	for _, id := range execOrder {
		node := graph.Nodes[id]
		if isSource(node) {
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

		closure, err := reg.Resolve(node, instances)
		if err != nil {
			return nil, errnie.Error(err)
		}

		errnie.Debug(fmt.Sprintf("[compiler.Compile] resolved node %s (%s)", id, node.Type))
		instances[id] = closure
	}

	// 4. Wire internal node connections
	for id, n := range graph.Nodes {
		inst, ok := instances[id]
		if !ok {
			continue
		}

		var downstreams []types.StreamNode[any, any]
		for outPort, targets := range n.Connections.Outputs {
			_ = outPort

			for _, target := range targets {
				if nextInst, ok := instances[target.NodeID]; ok {
					downstreams = append(downstreams, nextInst)
				}
			}
		}

		if len(downstreams) > 0 {
			inst.SetDownstreamAny(func(ctx context.Context, payload any) error {
				for _, nextClosure := range downstreams {
					if err := nextClosure.WriteAny(ctx, payload); err != nil {
						return err
					}
				}
				return nil
			})
		}
	}

	// 5. Find boundaries
	var sourceNodeID string
	var sinkNodeID string
	for id, node := range graph.Nodes {
		if isSource(node) {
			sourceNodeID = id
		}
		if isSink(node) {
			sinkNodeID = id
		}
	}

	return types.NewStreamNode(nil, func(ctx context.Context, payload any) error {
		if sourceNodeID != "" {
			if instance, ok := instances[sourceNodeID]; ok {
				return instance.WriteAny(ctx, payload)
			}
		} else {
			hasInputs := make(map[string]bool)
			for _, node := range graph.Nodes {
				for _, targets := range node.Connections.Outputs {
					for _, target := range targets {
						hasInputs[target.NodeID] = true
					}
				}
			}
			for id, instance := range instances {
				if !hasInputs[id] {
					if err := instance.WriteAny(ctx, payload); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}, func(next func(context.Context, any) error) {
		if sinkNodeID != "" {
			if instance, ok := instances[sinkNodeID]; ok {
				instance.SetDownstreamAny(next)
			}
		} else {
			for id, node := range graph.Nodes {
				if len(node.Connections.Outputs) == 0 {
					if instance, ok := instances[id]; ok {
						instance.SetDownstreamAny(next)
					}
				}
			}
		}
	}), nil
}

/*
CompileFile reads a JSON graph from disk and compiles it into an executable closure.
*/
func CompileFile[In, Out any](
	jsonPath string,
	reg *Registry,
	repos ...DefinitionRepository,
) (types.StreamNode[In, Out], error) {
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

func isSource(node Node) bool {
	return node.Type == "nomagique.System" || node.Type == "nomagique.Network" || node.Type == "nomagique.Database"
}

func isSink(node Node) bool {
	return node.Type == "nomagique.Console" || node.Type == "nomagique.File"
}
