package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/std/capnp/schema"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

/*
Compile lowers a declarative JSON Graph into an immutable in-memory executable Program.
Zero any payloads or graph traversals exist on the runtime execution path.
*/
func Compile(
	graph Graph,
	registry *Registry,
	repos ...DefinitionRepository,
) (*Program, error) {
	return CompileWithPrevious(graph, registry, nil, repos...)
}

/*
CompileWithPrevious compiles a graph candidate with capability reuse from a previous Program.
*/
func CompileWithPrevious(
	graph Graph,
	registry *Registry,
	previous *Program,
	repos ...DefinitionRepository,
) (*Program, error) {
	if registry == nil {
		registry = DefaultRegistry()
	}

	var repo DefinitionRepository
	if len(repos) > 0 {
		repo = repos[0]
	}

	// 1. Phase 2: Recursively expand nested definitions
	expandedGraph, err := expandDefinitions(graph, repo)
	if err != nil {
		return nil, err
	}
	graph = expandedGraph

	if len(graph.Nodes) == 0 {
		return &Program{
			Version: graph.ID,
			Nodes:   nil,
			Routes:  nil,
			Roots:   nil,
			NodeMap: make(map[string]NodeID),
		}, nil
	}

	// 2. Phase 8: Topological ordering and cycle detection
	inDegree := make(map[string]int, len(graph.Nodes))
	adjacency := make(map[string][]string, len(graph.Nodes))

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

	var queue []string
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)

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
			"compiler: graph contains an unsupported cycle",
			nil,
		))
	}

	// 3. Phase 10: Assign numeric NodeIDs
	nodeCount := len(execOrder)
	nodeMap := make(map[string]NodeID, nodeCount)
	for i, id := range execOrder {
		nodeMap[id] = NodeID(i)
	}

	// 4. Phase 3 & 4: Resolve factories and reflect schemas
	compiledNodes := make([]CompiledNode, nodeCount)
	schemasMap := make([]*InterfaceSchema, nodeCount)
	factoriesMap := make([]Factory, nodeCount)

	for i, id := range execOrder {
		node := graph.Nodes[id]
		factory, err := registry.Resolve(node.Type)
		if err != nil {
			return nil, err
		}
		factoriesMap[i] = factory

		var ifaceSchema *InterfaceSchema
		if factory.InterfaceID != 0 {
			ifaceSchema, err = ReflectInterface(factory.InterfaceID)
			if err != nil {
				return nil, err
			}
		}

		if factory.InterfaceID == 0 {
			ifaceSchema = makeBoundarySchema()
		}
		schemasMap[i] = ifaceSchema

		compiledNode := CompiledNode{
			ID:           id,
			Index:        NodeID(i),
			Inputs:       make(map[string]CompiledField),
			Outputs:      make(map[string]CompiledField),
			InputIndices: make(map[string]FieldID),
			IsSource:     isBoundarySource(id, node),
			IsSink:       isBoundarySink(id, node),
		}

		if ifaceSchema != nil {
			compiledNode.Write = CompiledMethod{
				InterfaceID: ifaceSchema.InterfaceID,
				MethodID:    ifaceSchema.WriteMethod,
				ParamsSize:  ifaceSchema.WriteParams,
				ResultSize:  ifaceSchema.WriteResult,
			}
			compiledNode.Done = CompiledMethod{
				InterfaceID: ifaceSchema.InterfaceID,
				MethodID:    ifaceSchema.DoneMethod,
				ParamsSize:  ifaceSchema.DoneParams,
				ResultSize:  ifaceSchema.DoneResult,
			}

			// Sort inputs for stable field index assignment
			var inNames []string
			for name := range ifaceSchema.Inputs {
				inNames = append(inNames, name)
			}
			sort.Strings(inNames)
			for idx, name := range inNames {
				fi := ifaceSchema.Inputs[name]
				compiledNode.Inputs[name] = CompiledField{
					Name:               name,
					Which:              fi.Which,
					Offset:             fi.Offset,
					Index:              FieldID(idx),
					InUnion:            fi.InUnion,
					DiscriminantValue:  fi.DiscriminantValue,
					DiscriminantOffset: fi.DiscriminantOffset,
				}
				compiledNode.InputIndices[name] = FieldID(idx)
			}

			// Sort outputs for stable field index assignment
			var outNames []string
			for name := range ifaceSchema.Outputs {
				outNames = append(outNames, name)
			}
			sort.Strings(outNames)
			for idx, name := range outNames {
				fi := ifaceSchema.Outputs[name]
				compiledNode.Outputs[name] = CompiledField{
					Name:               name,
					Which:              fi.Which,
					Offset:             fi.Offset,
					Index:              FieldID(idx),
					InUnion:            fi.InUnion,
					DiscriminantValue:  fi.DiscriminantValue,
					DiscriminantOffset: fi.DiscriminantOffset,
				}
			}

			// Phase 6: Compile static inputs into ArgsTemplate
			if ifaceSchema.WriteParams.DataSize > 0 || ifaceSchema.WriteParams.PointerCount > 0 {
				_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
				if err == nil {
					tmpl, err := capnp.NewRootStruct(seg, ifaceSchema.WriteParams)
					if err == nil {
						for portName, rawBytes := range node.InputData {
							fi, exists := resolveInputField(ifaceSchema, portName)
							if exists {
								rawStr := parseRawInputString(rawBytes)
								_ = SetStaticField(tmpl, fi, rawStr)
							}
						}
						compiledNode.ArgsTemplate = tmpl
					}
				}
			}
		}

		compiledNodes[i] = compiledNode
	}

	// 5. Phase 7, 9 & 10: Validate edges, compile routes and readiness masks
	var routes []Route
	var capabilityEdges []capabilityEdge

	for uIdx, uID := range execOrder {
		uNode := graph.Nodes[uID]
		uSchema := schemasMap[uIdx]

		for outPort, targets := range uNode.Connections.Outputs {
			for _, target := range targets {
				vID := target.NodeID
				vIdx, targetExists := nodeMap[vID]
				if !targetExists {
					return nil, errnie.Error(errnie.Err(
						errnie.Validation,
						fmt.Sprintf("compiler: target node %q not found", vID),
						nil,
					))
				}

				vNode := graph.Nodes[vID]
				vSchema := schemasMap[vIdx]

				toField, inExists := resolveInputField(vSchema, target.PortName)
				isSink := isBoundarySink(vID, vNode) || vSchema.InterfaceID == 0
				if !inExists && !isSink {
					return nil, errnie.Error(errnie.Err(
						errnie.Validation,
						fmt.Sprintf("compiler: node %q (%s) has no input port %q", vID, vNode.Type, target.PortName),
						nil,
					))
				}

				// A port typed as an interface carries a capability, not a
				// value. Wiring a node into one binds the node itself, so the
				// consumer calls it back as a function; there is no output to
				// read, and the source port only names the connection.
				if !isSink && toField.Which == schema.Type_Which_interface {
					if !Implements(uSchema.InterfaceID, toField.InterfaceID) {
						return nil, errnie.Error(errnie.Err(
							errnie.Validation,
							fmt.Sprintf(
								"compiler: node %q (%s) does not implement the interface port %q of node %q requires",
								uID, uNode.Type, target.PortName, vID,
							),
							nil,
						))
					}

					capabilityEdges = append(capabilityEdges, capabilityEdge{
						provider: NodeID(uIdx),
						consumer: NodeID(vIdx),
						field:    uint16(toField.Offset),
						port:     target.PortName,
					})

					continue
				}

				fromField, exists := resolveOutputField(uSchema, outPort)
				if !exists {
					return nil, errnie.Error(errnie.Err(
						errnie.Validation,
						fmt.Sprintf("compiler: node %q (%s) has no output port %q", uID, uNode.Type, outPort),
						nil,
					))
				}

				fromFieldID := compiledNodes[uIdx].Outputs[fromField.Name].Index

				if isSink {
					toFieldID, hasInput := compiledNodes[vIdx].InputIndices[target.PortName]
					if !hasInput {
						toFieldID = FieldID(len(compiledNodes[vIdx].Inputs))
						compiledNodes[vIdx].InputIndices[target.PortName] = toFieldID
					}
					toField = FieldInfo{
						Name:   target.PortName,
						Offset: uint32(toFieldID),
						Which:  fromField.Which,
					}
					compiledNodes[vIdx].Inputs[target.PortName] = CompiledField{
						Name:   target.PortName,
						Which:  fromField.Which,
						Offset: uint32(toFieldID),
						Index:  toFieldID,
					}
				}

				toFieldID := compiledNodes[vIdx].Inputs[toField.Name].Index

				// Compile typed Copier with type compatibility validation
				copier, err := CompileCopier(fromField, toField)
				if err != nil {
					return nil, errnie.Error(errnie.Err(
						errnie.Validation,
						fmt.Sprintf("compiler: type mismatch on edge %s.%s (%s) -> %s.%s (%s)",
							uID, outPort, formatWhich(fromField.Which),
							vID, target.PortName, formatWhich(toField.Which)),
						err,
					))
				}

				routes = append(routes, Route{
					FromNode:       NodeID(uIdx),
					FromField:      fromFieldID,
					ToNode:         vIdx,
					ToField:        toFieldID,
					Copy:           copier,
					FromInUnion:    fromField.InUnion,
					FromDiscVal:    fromField.DiscriminantValue,
					FromDiscOffset: fromField.DiscriminantOffset,
				})

				// Mark destination field as required dynamic input
				compiledNodes[vIdx].RequiredMask |= (1 << toFieldID)
			}
		}
	}

	// 6. Phase 11: Reuse or construct capabilities
	for i, id := range execOrder {
		node := graph.Nodes[id]
		factory := factoriesMap[i]

		var configBytes []byte
		if len(node.InputData) > 0 {
			configBytes, _ = sonic.Marshal(node.InputData)
		}
		configDigest := sha256.Sum256(configBytes)

		identity := NodeIdentity{
			ID:           id,
			Type:         node.Type,
			InterfaceID:  factory.InterfaceID,
			ConfigDigest: configDigest,
		}
		compiledNodes[i].Identity = identity

		var client capnp.Client

		// Check capability reuse from previous Program
		if previous != nil {
			for _, prevNode := range previous.Nodes {
				if prevNode.Identity == identity && prevNode.Client.IsValid() {
					client = prevNode.Client.AddRef()
					break
				}
			}
		}

		// Construct new capability if not reused
		if !client.IsValid() {
			var err error
			client, err = factory.New(context.Background(), configBytes)
			if err != nil {
				// Clean up any acquired references on candidate failure
				for _, cn := range compiledNodes {
					if cn.Client.IsValid() {
						cn.Client.Release()
					}
				}
				return nil, errnie.Error(errnie.Err(
					errnie.Internal,
					fmt.Sprintf("compiler: failed to construct capability for node %q (%s)", id, node.Type),
					err,
				))
			}
		}

		compiledNodes[i].Client = client
	}

	// 6b. Bind capability edges now that every node owns a client. A
	// capability is bound into the consumer's argument template, so it is
	// present on every call rather than arriving with one observation.
	if err := bindCapabilities(compiledNodes, capabilityEdges); err != nil {
		return nil, err
	}

	// 7. Find root nodes (in-degree 0)
	var roots []NodeID
	for i := 0; i < nodeCount; i++ {
		if inDegree[execOrder[i]] == 0 {
			roots = append(roots, NodeID(i))
		}
	}

	return &Program{
		Version: graph.ID,
		Nodes:   compiledNodes,
		Routes:  routes,
		Roots:   roots,
		NodeMap: nodeMap,
	}, nil
}

/*
CompileFile reads a JSON graph from disk and compiles it into an immutable Program.
*/
func CompileFile(
	jsonPath string,
	reg *Registry,
	repos ...DefinitionRepository,
) (*Program, error) {
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

/*
ParseGraph parses a raw JSON byte slice into a Graph AST.
*/
func ParseGraph(data []byte) (Graph, error) {
	var graph Graph
	if err := sonic.Unmarshal(data, &graph); err != nil {
		return Graph{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"compiler: failed to unmarshal graph JSON",
			err,
		))
	}
	return graph, nil
}

/*
CompileJSON compiles raw JSON graph bytes directly into an immutable Program.
*/
func CompileJSON(
	data []byte,
	reg *Registry,
	repos ...DefinitionRepository,
) (*Program, error) {
	graph, err := ParseGraph(data)
	if err != nil {
		return nil, err
	}
	return Compile(graph, reg, repos...)
}

func resolveInputField(ifaceSchema *InterfaceSchema, port string) (FieldInfo, bool) {
	if ifaceSchema == nil {
		return FieldInfo{}, false
	}

	fieldInfo, exists := ifaceSchema.Inputs[port]
	if exists {
		return fieldInfo, true
	}

	index := strings.LastIndex(port, "_")
	if index != -1 {
		prefix := port[:index]
		fieldInfo, exists := ifaceSchema.Inputs[prefix]
		if exists {
			return fieldInfo, true
		}
	}

	return FieldInfo{}, false
}

func resolveOutputField(ifaceSchema *InterfaceSchema, port string) (FieldInfo, bool) {
	if ifaceSchema == nil {
		return FieldInfo{}, false
	}
	fi, ok := ifaceSchema.Outputs[port]
	return fi, ok
}

func makeBoundarySchema() *InterfaceSchema {
	return &InterfaceSchema{
		InterfaceID: 0,
		WriteParams: capnp.ObjectSize{DataSize: 64, PointerCount: 8},
		DoneResult:  capnp.ObjectSize{DataSize: 64, PointerCount: 8},
		Inputs: map[string]FieldInfo{
			"value": {Name: "value", Offset: 0, Which: schema.Type_Which_data},
			"data":  {Name: "data", Offset: 0, Which: schema.Type_Which_data},
			"in":    {Name: "in", Offset: 0, Which: schema.Type_Which_data},
		},
		Outputs: map[string]FieldInfo{
			"out":   {Name: "out", Offset: 0, Which: schema.Type_Which_data},
			"value": {Name: "value", Offset: 0, Which: schema.Type_Which_data},
			"data":  {Name: "data", Offset: 0, Which: schema.Type_Which_data},
		},
		HasDone: true,
	}
}

func parseRawInputString(raw json.RawMessage) string {
	var strVal string
	if err := sonic.Unmarshal(raw, &strVal); err == nil {
		return strVal
	}
	var numVal float64
	if err := sonic.Unmarshal(raw, &numVal); err == nil {
		return strconv.FormatFloat(numVal, 'f', -1, 64)
	}
	var boolVal bool
	if err := sonic.Unmarshal(raw, &boolVal); err == nil {
		return strconv.FormatBool(boolVal)
	}
	var m map[string]json.RawMessage
	if err := sonic.Unmarshal(raw, &m); err == nil {
		for _, k := range []string{"string", "float", "number", "int", "bool", "value"} {
			if sub, ok := m[k]; ok {
				return parseRawInputString(sub)
			}
		}
	}
	return string(raw)
}

func formatWhich(w schema.Type_Which) string {
	switch w {
	case schema.Type_Which_float64:
		return "Float64"
	case schema.Type_Which_int64:
		return "Int64"
	case schema.Type_Which_uint64:
		return "UInt64"
	case schema.Type_Which_text:
		return "Text"
	case schema.Type_Which_data:
		return "Data"
	case schema.Type_Which_bool:
		return "Bool"
	case schema.Type_Which_enum:
		return "Enum"
	default:
		return fmt.Sprint(w)
	}
}

func isBoundarySource(id string, node Node) bool {
	return node.Type == "source" || node.Type == "data.Source" || node.Type == "test.Float64Source" || id == "source" || id == "src" || strings.HasPrefix(id, "source")
}

func isBoundarySink(id string, node Node) bool {
	return node.Type == "sink" || node.Type == "data.Sink" || id == "sink" || strings.HasPrefix(id, "sink")
}

func expandDefinitions(
	graph Graph,
	repo DefinitionRepository,
) (Graph, error) {
	if repo == nil {
		return graph, nil
	}

	for depth := 0; depth < 64; depth++ {
		var defID string
		var defNode Node
		found := false

		for id, node := range graph.Nodes {
			if strings.HasPrefix(node.Type, "definition:") {
				defID = id
				defNode = node
				found = true
				break
			}
		}

		if !found {
			break
		}

		defName := strings.TrimPrefix(defNode.Type, "definition:")
		childGraph, err := repo.Load(defName)
		if err != nil {
			return Graph{}, errnie.Error(errnie.Err(
				errnie.NotFound,
				fmt.Sprintf("compiler: failed to load definition %q for node %q", defName, defID),
				err,
			))
		}

		var childSourceID string
		var childSourceNode Node
		var childSinkID string
		var childSinkNode Node

		for cid, cnode := range childGraph.Nodes {
			if isBoundarySource(cid, cnode) {
				childSourceID = cid
				childSourceNode = cnode
			}

			if isBoundarySink(cid, cnode) {
				childSinkID = cid
				childSinkNode = cnode
			}
		}

		prefix := defID + "__"

		childIngressTargets := make(map[string][]ConnectionTarget)
		if childSourceID != "" {
			for outPort, targets := range childSourceNode.Connections.Outputs {
				for _, t := range targets {
					childIngressTargets[outPort] = append(childIngressTargets[outPort], ConnectionTarget{
						NodeID:   prefix + t.NodeID,
						PortName: t.PortName,
					})
				}
			}
		}

		childEgressSources := make(map[string][]ConnectionTarget)
		if childSinkID != "" {
			for inPort, targets := range childSinkNode.Connections.Inputs {
				for _, t := range targets {
					childEgressSources[inPort] = append(childEgressSources[inPort], ConnectionTarget{
						NodeID:   prefix + t.NodeID,
						PortName: t.PortName,
					})
				}
			}
		}

		// 1. Add all namespaced child nodes (except boundary source/sink)
		for cid, cnode := range childGraph.Nodes {
			if cid == childSourceID || cid == childSinkID {
				continue
			}

			namespacedNode := Node{
				ID:        prefix + cid,
				Type:      cnode.Type,
				InputData: cnode.InputData,
				Connections: Connections{
					Inputs:  make(map[string][]ConnectionTarget),
					Outputs: make(map[string][]ConnectionTarget),
				},
			}

			for inPort, targets := range cnode.Connections.Inputs {
				for _, target := range targets {
					if target.NodeID == childSourceID {
						parentWires := defNode.Connections.Inputs[inPort]
						if len(parentWires) == 0 {
							parentWires = defNode.Connections.Inputs["in"]
						}

						for _, pw := range parentWires {
							namespacedNode.Connections.Inputs[inPort] = append(
								namespacedNode.Connections.Inputs[inPort],
								ConnectionTarget{
									NodeID:   pw.NodeID,
									PortName: pw.PortName,
								},
							)
						}
					}

					if target.NodeID != childSourceID {
						namespacedNode.Connections.Inputs[inPort] = append(
							namespacedNode.Connections.Inputs[inPort],
							ConnectionTarget{
								NodeID:   prefix + target.NodeID,
								PortName: target.PortName,
							},
						)
					}
				}
			}

			for outPort, targets := range cnode.Connections.Outputs {
				for _, target := range targets {
					if target.NodeID == childSinkID {
						parentTargets := defNode.Connections.Outputs[outPort]
						if len(parentTargets) == 0 {
							parentTargets = defNode.Connections.Outputs["out"]
						}

						for _, pt := range parentTargets {
							namespacedNode.Connections.Outputs[outPort] = append(
								namespacedNode.Connections.Outputs[outPort],
								ConnectionTarget{
									NodeID:   pt.NodeID,
									PortName: pt.PortName,
								},
							)
						}
					}

					if target.NodeID != childSinkID {
						namespacedNode.Connections.Outputs[outPort] = append(
							namespacedNode.Connections.Outputs[outPort],
							ConnectionTarget{
								NodeID:   prefix + target.NodeID,
								PortName: target.PortName,
							},
						)
					}
				}
			}

			graph.Nodes[namespacedNode.ID] = namespacedNode
		}

		// 2. Remove definition node from graph
		delete(graph.Nodes, defID)

		// 3. Update all existing nodes that referenced defID
		for nid, n := range graph.Nodes {
			if strings.HasPrefix(nid, prefix) {
				continue
			}

			for outPort, targets := range n.Connections.Outputs {
				var remapped []ConnectionTarget
				for _, t := range targets {
					if t.NodeID == defID {
						remapTargets := childIngressTargets[t.PortName]
						if len(remapTargets) == 0 {
							remapTargets = childIngressTargets["out"]
						}

						remapped = append(remapped, remapTargets...)
					}

					if t.NodeID != defID {
						remapped = append(remapped, t)
					}
				}

				n.Connections.Outputs[outPort] = remapped
			}

			for inPort, targets := range n.Connections.Inputs {
				var remapped []ConnectionTarget
				for _, t := range targets {
					if t.NodeID == defID {
						remapSources := childEgressSources[t.PortName]
						if len(remapSources) == 0 {
							remapSources = childEgressSources["in"]
						}

						remapped = append(remapped, remapSources...)
					}

					if t.NodeID != defID {
						remapped = append(remapped, t)
					}
				}

				n.Connections.Inputs[inPort] = remapped
			}

			graph.Nodes[nid] = n
		}
	}

	return graph, nil
}

/*
capabilityEdge records a wire whose consumer port is typed as an interface.
The provider is bound into the consumer's arguments as a live reference, so
the consumer invokes it as a function instead of reading a copied value.
*/
type capabilityEdge struct {
	provider NodeID
	consumer NodeID
	field    uint16
	port     string
}

/*
bindCapabilities places each provider's client into its consumer's argument
template. A capability is a reference, so the consumer holds its own
reference for as long as the program does, and releasing the program releases
it.
*/
func bindCapabilities(nodes []CompiledNode, edges []capabilityEdge) error {
	for _, edge := range edges {
		provider := &nodes[edge.provider]
		consumer := &nodes[edge.consumer]

		if !provider.Client.IsValid() {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"compiler: node %q cannot be wired to port %q because it owns no capability",
					provider.ID, edge.port,
				),
				nil,
			))
		}

		if !consumer.ArgsTemplate.IsValid() {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"compiler: node %q has no arguments to carry the capability on port %q",
					consumer.ID, edge.port,
				),
				nil,
			))
		}

		message := consumer.ArgsTemplate.Message()
		interfaceID := message.CapTable().Add(provider.Client.AddRef())
		segment := consumer.ArgsTemplate.Segment()

		if err := consumer.ArgsTemplate.SetPtr(
			edge.field, capnp.NewInterface(segment, interfaceID).ToPtr(),
		); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				fmt.Sprintf(
					"compiler: failed to bind capability from %q to %q port %q",
					provider.ID, consumer.ID, edge.port,
				),
				err,
			))
		}
	}

	return nil
}
