package compiler

import (
	"context"
	"fmt"
	"os"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
CompiledNode stores the metadata, local capability client, and invocation assembler for one node.
*/
type CompiledNode struct {
	ID          string
	Type        string
	Server      any
	Client      capnp.Client
	Assembler   *InvocationAssembler
	Inputs      map[string]PortType
	Outputs     map[string]PortType
	Descriptor  PrimitiveDescriptor
	Downstreams map[string][]capnp.Client
}

/*
Pipeline represents an executable, in-memory composition of Cap'n Proto primitives.
Zero any payloads or graph traversals exist on the runtime execution path.
*/
type Pipeline struct {
	Nodes      map[string]*CompiledNode
	ExecOrder  []string
	SourceNode *CompiledNode
	SinkNode   *CompiledNode
}

/*
InputSink returns the typed Float64Sink capability for an input port on a node.
*/
func (p *Pipeline) InputSink(nodeID, portName string) (types.Float64Sink, error) {
	node, exists := p.Nodes[nodeID]
	if !exists {
		return types.Float64Sink{}, errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("pipeline: node %q not found", nodeID),
			nil,
		))
	}
	client, err := node.Assembler.InputSink(portName)
	if err != nil {
		return types.Float64Sink{}, err
	}
	return types.Float64Sink(client), nil
}

/*
ConnectOutput binds a downstream typed Float64Sink capability to a node's output port.
*/
func (p *Pipeline) ConnectOutput(nodeID, portName string, sink types.Float64Sink) error {
	node, exists := p.Nodes[nodeID]
	if !exists {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("pipeline: node %q not found", nodeID),
			nil,
		))
	}
	node.Downstreams[portName] = append(node.Downstreams[portName], capnp.Client(sink))
	return node.Descriptor.BindDownstream(node.Server, portName, node.Downstreams[portName])
}

/*
WaitStreaming waits for all streaming calls across all pipeline nodes to flush.
*/
func (p *Pipeline) WaitStreaming() error {
	for _, id := range p.ExecOrder {
		if n, ok := p.Nodes[id]; ok {
			if err := n.Assembler.WaitStreaming(); err != nil {
				return err
			}
		}
	}
	return nil
}

/*
WriteFloat64 sends a float64 observation into the pipeline's ingress.
*/
func (p *Pipeline) WriteFloat64(ctx context.Context, val float64) error {
	if p.SourceNode == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"pipeline: no source node configured",
			nil,
		))
	}
	sink, err := p.InputSink(p.SourceNode.ID, "in")
	if err != nil {
		return err
	}
	if err := sink.Write(ctx, func(params types.Float64Sink_write_Params) error {
		params.SetValue(val)
		return nil
	}); err != nil {
		return err
	}
	return p.WaitStreaming()
}

/*
Compile lowers a declarative JSON Graph into an in-memory executable Cap'n Proto composition.
Nodes are instantiated once, capabilities are wired directly, and port types are strictly validated.
*/
func Compile(
	graph Graph,
	reg *Registry,
	repos ...DefinitionRepository,
) (*Pipeline, error) {
	if reg == nil {
		reg = DefaultRegistry()
	}

	if len(graph.Nodes) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("compiler: graph %q contains no nodes", graph.Name),
			nil,
		))
	}

	// 1. Build adjacency and in-degree maps for topological sort
	inDegree := make(map[string]int)
	adjacency := make(map[string][]string)

	for id := range graph.Nodes {
		inDegree[id] = 0
	}

	for id, node := range graph.Nodes {
		for _, targets := range node.Connections.Outputs {
			for _, target := range targets {
				if _, exists := graph.Nodes[target.NodeID]; exists {
					adjacency[id] = append(adjacency[id], target.NodeID)
					inDegree[target.NodeID]++
				}
			}
		}
	}

	// 2. Kahn's Algorithm for topological ordering and cycle detection
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

	// 3. Instantiate nodes and setup InvocationAssemblers
	nodes := make(map[string]*CompiledNode)
	for _, id := range execOrder {
		node := graph.Nodes[id]
		desc, err := reg.Resolve(node.Type)
		if err != nil {
			return nil, errnie.Error(err)
		}

		server, client, err := desc.Construct(node)
		if err != nil {
			return nil, errnie.Error(err)
		}

		assembler := NewInvocationAssembler(
			node.ID,
			client,
			desc.InputPorts,
			desc.CreateSetter,
			func(ctx context.Context, setters map[string]func(capnp.Struct)) error {
				return desc.Invoke(ctx, client, setters)
			},
			func(ctx context.Context) error {
				return desc.Done(ctx, client)
			},
		)

		// Populate static inputs from inputData if unwired
		if node.InputData != nil {
			for portName, valData := range node.InputData {
				if _, isInput := desc.InputPorts[portName]; isInput {
					// Check if port is not wired
					isWired := false
					if wires, ok := node.Connections.Inputs[portName]; ok && len(wires) > 0 {
						isWired = true
					}
					if !isWired {
						if setterGen, ok := desc.CreateSetter[portName]; ok {
							var val float64
							switch v := valData.(type) {
							case float64:
								val = v
							case map[string]any:
								if f, ok := v["float"].(float64); ok {
									val = f
								}
								if f, ok := v["number"].(float64); ok {
									val = f
								}
							}
							assembler.SetStaticInput(portName, setterGen(val))
						}
					}
				}
			}
		}

		compiled := &CompiledNode{
			ID:          id,
			Type:        node.Type,
			Server:      server,
			Client:      client,
			Assembler:   assembler,
			Inputs:      desc.InputPorts,
			Outputs:     desc.OutputPorts,
			Descriptor:  desc,
			Downstreams: make(map[string][]capnp.Client),
		}

		nodes[id] = compiled
	}

	// 4. Validate port types on edges and bind capabilities
	for id, fromNode := range nodes {
		origNode := graph.Nodes[id]
		for outPort, targets := range origNode.Connections.Outputs {
			fromPortType, hasOut := fromNode.Outputs[outPort]
			if !hasOut {
				return nil, errnie.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf("compiler: node %q (%s) has no output port %q", id, fromNode.Type, outPort),
					nil,
				))
			}

			for _, target := range targets {
				toNode, exists := nodes[target.NodeID]
				if !exists {
					return nil, errnie.Error(errnie.Err(
						errnie.Validation,
						fmt.Sprintf("compiler: target node %q not found", target.NodeID),
						nil,
					))
				}

				toPortType, hasIn := toNode.Inputs[target.PortName]
				if !hasIn {
					return nil, errnie.Error(errnie.Err(
						errnie.Validation,
						fmt.Sprintf("compiler: node %q (%s) has no input port %q", target.NodeID, toNode.Type, target.PortName),
						nil,
					))
				}

				// Type check: output and input port types must match exactly
				if fromPortType != toPortType {
					return nil, errnie.Error(errnie.Err(
						errnie.Validation,
						fmt.Sprintf("compiler: type mismatch on edge %s.%s (%s) -> %s.%s (%s)",
							fromNode.ID, outPort, fromPortType, toNode.ID, target.PortName, toPortType),
						nil,
					))
				}

				// Obtain typed input sink capability from target assembler
				targetSink, err := toNode.Assembler.InputSink(target.PortName)
				if err != nil {
					return nil, err
				}

				fromNode.Downstreams[outPort] = append(fromNode.Downstreams[outPort], targetSink)
			}
		}
	}

	// 5. Connect downstream capabilities to each primitive server
	for _, node := range nodes {
		for outPort, sinks := range node.Downstreams {
			if err := node.Descriptor.BindDownstream(node.Server, outPort, sinks); err != nil {
				return nil, errnie.Error(err)
			}
		}
	}

	pipeline := &Pipeline{
		Nodes:     nodes,
		ExecOrder: execOrder,
	}

	// Identify Source and Sink nodes
	for _, node := range nodes {
		if node.Type == "source" || node.Type == "data.Source" {
			pipeline.SourceNode = node
		}
		if node.Type == "sink" || node.Type == "data.Sink" {
			pipeline.SinkNode = node
		}
	}

	// If no explicit SourceNode, pick the first node in topological order with 0 in-degree
	if pipeline.SourceNode == nil && len(execOrder) > 0 {
		pipeline.SourceNode = nodes[execOrder[0]]
	}
	// If no explicit SinkNode, pick the last node in topological order
	if pipeline.SinkNode == nil && len(execOrder) > 0 {
		pipeline.SinkNode = nodes[execOrder[len(execOrder)-1]]
	}

	return pipeline, nil
}

/*
CompileFile reads a JSON graph from disk and compiles it into an executable Pipeline.
*/
func CompileFile(
	jsonPath string,
	reg *Registry,
	repos ...DefinitionRepository,
) (*Pipeline, error) {
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

	return Compile(graph, reg, repos...)
}
