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
				consumer, exists := graph.Nodes[target.NodeID]

				if !exists {
					continue
				}

				// Handing a node to another is not a dependency on it. The
				// consumer holds a reference and calls it when it decides to,
				// so the two do not have to be ordered against each other and
				// a node may be handed to something it also reads from.
				if carriesCapability(registry, consumer, target.PortName) {
					continue
				}

				adjacency[id] = append(adjacency[id], target.NodeID)
				inDegree[target.NodeID]++
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

		schemasMap[i] = ifaceSchema

		compiledNode := CompiledNode{
			ID:           id,
			Index:        NodeID(i),
			Inputs:       make(map[string]CompiledField),
			Outputs:      make(map[string]CompiledField),
			InputIndices: make(map[string]FieldID),
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
	var fanInEdges []fanInEdge

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
				isSink := vSchema == nil || vSchema.InterfaceID == 0
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
				if !isSink && (toField.Which == schema.Type_Which_interface || toField.CapabilityList) {
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
						listed:   toField.CapabilityList,
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

				// A boundary port carries whatever the definition routes through
				// it, so it adopts the type of the field it feeds rather than
				// forcing every metric input to be opaque data.
				if fromField.ValueList && !toField.ValueList {
					slot, numbered := outputSlot(outPort)

					if !numbered {
						return nil, errnie.Error(errnie.Err(
							errnie.Validation,
							fmt.Sprintf(
								"compiler: port %q of node %q hands back several values, so it is read as %s_<slot>",
								outPort, uID, outPort,
							),
							nil,
						))
					}

					copier, err := CompileFanOutCopier(fromField, toField, slot)

					if err != nil {
						return nil, errnie.Error(errnie.Err(
							errnie.Validation,
							fmt.Sprintf(
								"compiler: type mismatch reading slot %d of %q", slot, outPort,
							),
							err,
						))
					}

					routes = append(routes, Route{
						FromNode:  NodeID(uIdx),
						FromField: fromFieldID,
						ToNode:    vIdx,
						ToField:   toFieldID,
						Copy:      copier,
					})

					compiledNodes[vIdx].RequiredMask |= (1 << toFieldID)
					continue
				}

				// A gathering port holds every producer that lands on it, so
				// its slots are handed out once they are all known.
				if toField.ValueList {
					fanInEdges = append(fanInEdges, fanInEdge{
						fromNode:  NodeID(uIdx),
						fromField: fromFieldID,
						fromInfo:  fromField,
						toNode:    vIdx,
						toField:   toFieldID,
						toInfo:    toField,
						port:      target.PortName,
					})

					continue
				}

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
	gathered, err := compileFanIn(fanInEdges)

	if err != nil {
		return nil, err
	}

	routes = append(routes, gathered...)

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

	if ok {
		return fi, true
	}

	// A port handing back several values is read one numbered slot at a time.
	base, _, numbered := strings.Cut(port, "_")

	if !numbered {
		return FieldInfo{}, false
	}

	fi, ok = ifaceSchema.Outputs[base]

	if !ok || !fi.ValueList {
		return FieldInfo{}, false
	}

	return fi, true
}

/*
outputSlot reads the slot a numbered output port names.
*/
func outputSlot(port string) (int, bool) {
	_, suffix, numbered := strings.Cut(port, "_")

	if !numbered {
		return 0, false
	}

	slot, err := strconv.Atoi(suffix)

	if err != nil {
		return 0, false
	}

	return slot, true
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

		prefix := defID + "__"

		childIngressTargets := make(map[string][]ConnectionTarget)
		childEgressSources := make(map[string][]ConnectionTarget)

		// 1. Add all namespaced child nodes
		for cid, cnode := range childGraph.Nodes {
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
					namespacedNode.Connections.Inputs[inPort] = append(
						namespacedNode.Connections.Inputs[inPort],
						ConnectionTarget{
							NodeID:   prefix + target.NodeID,
							PortName: target.PortName,
						},
					)
				}
			}

			for outPort, targets := range cnode.Connections.Outputs {
				for _, target := range targets {
					namespacedNode.Connections.Outputs[outPort] = append(
						namespacedNode.Connections.Outputs[outPort],
						ConnectionTarget{
							NodeID:   prefix + target.NodeID,
							PortName: target.PortName,
						},
					)
				}
			}

			graph.Nodes[namespacedNode.ID] = namespacedNode
		}

		// 1b. Wire the parent through the definition's own ports. A boundary
		// node is not needed to name them: a port is "<node>.<field>", which is
		// the field of a child node the enclosing graph reaches directly.
		if err := wireDefinitionPorts(
			graph, defID, defNode, prefix, childIngressTargets, childEgressSources,
		); err != nil {
			return Graph{}, err
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
	// listed marks an edge into a port that carries several capabilities, so
	// the providers accumulate instead of replacing one another.
	listed bool
}

/*
bindCapabilities places each provider's client into its consumer's argument
template. A capability is a reference, so the consumer holds its own
reference for as long as the program does, and releasing the program releases
it.
*/
func bindCapabilities(nodes []CompiledNode, edges []capabilityEdge) error {
	single, listed := partitionCapabilities(edges)

	for _, edge := range single {
		if err := bindCapability(nodes, edge); err != nil {
			return err
		}
	}

	for slot, group := range listed {
		if err := bindCapabilityList(nodes, slot, group); err != nil {
			return err
		}
	}

	return nil
}

/*
capabilitySlot names one consumer port that capabilities are wired into.
*/
type capabilitySlot struct {
	consumer NodeID
	field    uint16
}

/*
partitionCapabilities separates the wires that carry one capability from those
that accumulate into a list, grouping the latter by the port they feed.
*/
func partitionCapabilities(
	edges []capabilityEdge,
) ([]capabilityEdge, map[capabilitySlot][]capabilityEdge) {
	single := make([]capabilityEdge, 0, len(edges))
	listed := make(map[capabilitySlot][]capabilityEdge)

	for _, edge := range edges {
		if !edge.listed {
			single = append(single, edge)
			continue
		}

		slot := capabilitySlot{consumer: edge.consumer, field: edge.field}
		listed[slot] = append(listed[slot], edge)
	}

	return single, listed
}

/*
bindCapability places one provider's client into its consumer's arguments.
*/
func bindCapability(nodes []CompiledNode, edge capabilityEdge) error {
	provider, consumer, err := capabilityEnds(nodes, edge)

	if err != nil {
		return err
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
				"compiler: failed to bind capability onto port %q of node %q",
				edge.port, consumer.ID,
			),
			err,
		))
	}

	return nil
}

/*
bindCapabilityList places every provider wired into one port as a capability
list, which is how a node that serves many others holds all of them.

The providers are ordered by the port name the graph used, so the list a
consumer reads is the order the graph shows rather than the order the compiler
happened to walk the nodes in.
*/
func bindCapabilityList(
	nodes []CompiledNode,
	slot capabilitySlot,
	group []capabilityEdge,
) error {
	sort.Slice(group, func(left, right int) bool {
		return group[left].port < group[right].port
	})

	consumer := &nodes[slot.consumer]

	if !consumer.ArgsTemplate.IsValid() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"compiler: node %q has no arguments to carry the capabilities on port %q",
				consumer.ID, group[0].port,
			),
			nil,
		))
	}

	segment := consumer.ArgsTemplate.Segment()
	message := consumer.ArgsTemplate.Message()

	// A capability list is a pointer list whose entries are interfaces.
	list, err := capnp.NewPointerList(segment, int32(len(group)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"compiler: failed to allocate capability list for port %q of node %q",
				group[0].port, consumer.ID,
			),
			err,
		))
	}

	for index, edge := range group {
		provider, _, err := capabilityEnds(nodes, edge)

		if err != nil {
			return err
		}

		interfaceID := message.CapTable().Add(provider.Client.AddRef())

		if err := list.Set(index, capnp.NewInterface(segment, interfaceID).ToPtr()); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				fmt.Sprintf(
					"compiler: failed to place capability %q into port %q of node %q",
					provider.ID, edge.port, consumer.ID,
				),
				err,
			))
		}
	}

	if err := consumer.ArgsTemplate.SetPtr(slot.field, list.ToPtr()); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"compiler: failed to bind capability list onto port %q of node %q",
				group[0].port, consumer.ID,
			),
			err,
		))
	}

	return nil
}

/*
capabilityEnds resolves the provider and consumer of a capability wire,
refusing a provider that owns no capability to hand over.
*/
func capabilityEnds(
	nodes []CompiledNode,
	edge capabilityEdge,
) (*CompiledNode, *CompiledNode, error) {
	provider := &nodes[edge.provider]
	consumer := &nodes[edge.consumer]

	if !provider.Client.IsValid() {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"compiler: node %q cannot be wired to port %q because it owns no capability",
				provider.ID, edge.port,
			),
			nil,
		))
	}

	if !consumer.ArgsTemplate.IsValid() {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"compiler: node %q has no arguments to carry the capability on port %q",
				consumer.ID, edge.port,
			),
			nil,
		))
	}

	return provider, consumer, nil
}

/*
wireDefinitionPorts connects a parent to the fields a definition exposes.

A sub-graph declares no boundary: what it needs are the inputs nothing inside
it feeds, and what it publishes are the outputs nothing inside it consumes.
Both are addressed as "<node>.<field>", so the enclosing graph reaches straight
into the child rather than through a pseudo-node that only forwards.
*/
func wireDefinitionPorts(
	graph Graph,
	defID string,
	defNode Node,
	prefix string,
	ingress map[string][]ConnectionTarget,
	egress map[string][]ConnectionTarget,
) error {
	for port, targets := range defNode.Connections.Inputs {
		childID, field, addressed := strings.Cut(port, ".")

		if !addressed {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"compiler: definition node %q port %q must name the field it stands for, as <node>.<field>",
					defID, port,
				),
				nil,
			))
		}

		child, known := graph.Nodes[prefix+childID]

		if !known {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"compiler: definition node %q has no node %q to carry input port %q",
					defID, childID, port,
				),
				nil,
			))
		}

		for _, target := range targets {
			child.Connections.Inputs[field] = append(
				child.Connections.Inputs[field], target,
			)
		}

		// The enclosing graph still points at the definition, so the port has
		// to resolve to the field it stood for when those references are
		// rewritten.
		ingress[port] = append(ingress[port], ConnectionTarget{
			NodeID:   prefix + childID,
			PortName: field,
		})

		graph.Nodes[prefix+childID] = child
	}

	for port, targets := range defNode.Connections.Outputs {
		childID, field, addressed := strings.Cut(port, ".")

		if !addressed {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"compiler: definition node %q port %q must name the field it stands for, as <node>.<field>",
					defID, port,
				),
				nil,
			))
		}

		child, known := graph.Nodes[prefix+childID]

		if !known {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"compiler: definition node %q has no node %q to carry output port %q",
					defID, childID, port,
				),
				nil,
			))
		}

		for _, target := range targets {
			child.Connections.Outputs[field] = append(
				child.Connections.Outputs[field], target,
			)
		}

		egress[port] = append(egress[port], ConnectionTarget{
			NodeID:   prefix + childID,
			PortName: field,
		})

		graph.Nodes[prefix+childID] = child
	}

	return nil
}

/*
fanInEdge records a wire landing on a port that gathers several producers.
*/
type fanInEdge struct {
	fromNode  NodeID
	fromField FieldID
	fromInfo  FieldInfo
	toNode    NodeID
	toField   FieldID
	toInfo    FieldInfo
	port      string
}

/*
compileFanIn turns the wires landing on gathering ports into routes.

Every producer on one port shares its list and is given a slot of its own, so
several feeds land on a single input without overwriting one another. Slots are
handed out in port-name order, which is the order the graph shows.
*/
func compileFanIn(edges []fanInEdge) ([]Route, error) {
	grouped := make(map[capabilitySlot][]fanInEdge)

	for _, edge := range edges {
		slot := capabilitySlot{consumer: edge.toNode, field: uint16(edge.toField)}
		grouped[slot] = append(grouped[slot], edge)
	}

	slots := make([]capabilitySlot, 0, len(grouped))

	for slot := range grouped {
		slots = append(slots, slot)
	}

	sort.Slice(slots, func(left, right int) bool {
		if slots[left].consumer != slots[right].consumer {
			return slots[left].consumer < slots[right].consumer
		}

		return slots[left].field < slots[right].field
	})

	routes := make([]Route, 0, len(edges))

	for _, slot := range slots {
		group := grouped[slot]

		sort.Slice(group, func(left, right int) bool {
			return group[left].port < group[right].port
		})

		for index, edge := range group {
			copier, err := CompileFanInCopier(edge.fromInfo, edge.toInfo, index, len(group))

			if err != nil {
				return nil, errnie.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf(
						"compiler: type mismatch landing on gathering port %q",
						edge.port,
					),
					err,
				))
			}

			routes = append(routes, Route{
				FromNode:       edge.fromNode,
				FromField:      edge.fromField,
				ToNode:         edge.toNode,
				ToField:        edge.toField,
				Copy:           copier,
				FromInUnion:    edge.fromInfo.InUnion,
				FromDiscVal:    edge.fromInfo.DiscriminantValue,
				FromDiscOffset: edge.fromInfo.DiscriminantOffset,
			})
		}
	}

	return routes, nil
}

/*
carriesCapability reports a port that receives a node rather than a value.
*/
func carriesCapability(registry *Registry, consumer Node, port string) bool {
	factory, err := registry.Resolve(consumer.Type)

	if err != nil || factory.InterfaceID == 0 {
		return false
	}

	reflected, err := ReflectInterface(factory.InterfaceID)

	if err != nil {
		return false
	}

	field, resolved := resolveInputField(reflected, port)

	if !resolved {
		return false
	}

	return field.CapabilityList || field.Which == schema.Type_Which_interface
}
