package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/std/capnp/schema"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
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

	if err := agree(graph); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "compiler: graph must agree", err,
		))
	}

	// 1. Phase 2: Recursively expand nested definitions
	expandedGraph, err := expandDefinitions(graph, repo)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "compiler: failed to expand definitions", err,
		))
	}

	graph = expandedGraph
	registry, err = compileFactories(graph, registry, repo)

	if err != nil {
		return nil, err
	}
	registry, err = compileDefinitionCapabilities(graph, registry, repo)

	if err != nil {
		return nil, err
	}

	if len(graph.Nodes) == 0 {
		return &Program{
			Version: graph.ID,
			Nodes:   nil,
			Routes:  nil,
			Roots:   nil,
			NodeMap: make(map[string]NodeID),
		}, nil
	}

	// 1b. Domain Partitioning: Lower UI hierarchy and cross-domain bindings
	var uiPlan *UIPlan
	var bindingPlan *BindingPlan

	hasUINodes := false

	for _, node := range graph.Nodes {
		if strings.HasPrefix(node.Type, "ui.") {
			hasUINodes = true
			break
		}
	}

	if hasUINodes {
		uiPlan, bindingPlan = lowerUIAndBindings(graph)
	}

	backendNodes := make(map[string]Node)

	for id, node := range graph.Nodes {
		if !strings.HasPrefix(node.Type, "ui.") {
			backendNodes[id] = node
		}
	}

	if len(backendNodes) == 0 {
		return &Program{
			Version:  graph.ID,
			Nodes:    nil,
			Routes:   nil,
			Roots:    nil,
			NodeMap:  make(map[string]NodeID),
			UI:       uiPlan,
			Bindings: bindingPlan,
		}, nil
	}

	// A feedback edge writes a retained node from one of its descendants.
	// Cut that write dependency, not the store's read dependency: a dynamic
	// key must arrive before its state is read. Feedback commits after the
	// observation has finished, so no truth update can alter its prediction.
	feedback := make(map[string]map[string]bool)

	for retainedID, retained := range backendNodes {
		if !holdsRetained(registry, retained) {
			continue
		}

		reachable := make(map[string]bool)
		pending := []string{retainedID}

		for len(pending) > 0 {
			current := pending[0]
			pending = pending[1:]

			if reachable[current] {
				continue
			}

			reachable[current] = true

			// Another retained owner ends this evaluation dependency. Walking
			// through it would incorrectly turn upstream lookup keys into writes.
			if current != retainedID && holdsRetained(registry, backendNodes[current]) {
				continue
			}

			for _, targets := range backendNodes[current].Connections.Outputs {
				for _, target := range targets {
					consumer, exists := backendNodes[target.NodeID]

					if exists && !carriesCapability(registry, consumer, target.PortName) {
						pending = append(pending, target.NodeID)
					}
				}
			}
		}

		feedback[retainedID] = reachable
	}

	// 2. Phase 8: Topological ordering and cycle detection
	inDegree := make(map[string]int, len(backendNodes))
	adjacency := make(map[string][]string, len(backendNodes))

	for id := range backendNodes {
		inDegree[id] = 0
	}

	for id, node := range backendNodes {
		for _, targets := range node.Connections.Outputs {
			for _, target := range targets {
				_, exists := backendNodes[target.NodeID]

				if !exists {
					continue
				}

				// A retained owner's capability can read the prior revision
				// before this observation supplies its lookup keys. Requiring
				// those keys first would turn a valid read into a value cycle.
				if holdsRetained(registry, node) && carriesCapability(registry, backendNodes[target.NodeID], target.PortName) {
					continue
				}
				// Other capability providers configure before invocation.

				if feedback[target.NodeID][id] {
					continue
				}

				adjacency[id] = append(adjacency[id], target.NodeID)
				inDegree[target.NodeID]++
			}
		}
	}

	var queue []string
	// What a node's indegree was before the sort consumed it. Kahn's
	// algorithm decrements every entry to zero on its way through, so the
	// map it leaves behind reports every node as an origin.
	origins := make(map[string]int, len(inDegree))

	maps.Copy(origins, inDegree)

	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	sort.Strings(queue)

	execOrder := make([]string, 0, len(backendNodes))

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

	if len(execOrder) != len(backendNodes) {
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
			return nil, errnie.Error(errnie.Err(
				errnie.Validation, "compiler: failed to resolve factory", err,
			))
		}

		factoriesMap[i] = factory

		var ifaceSchema *InterfaceSchema

		if factory.InterfaceID != 0 {
			ifaceSchema, err = ReflectInterface(factory.InterfaceID)

			if err != nil {
				return nil, errnie.Error(errnie.Err(
					errnie.Validation, "compiler: failed to reflect interface", err,
				))
			}
		}

		if Implements(factory.InterfaceID, runtime.Queued_TypeID) {
			pending, found := ifaceSchema.Outputs["pending"]

			if !found || pending.Which != schema.Type_Which_uint64 {
				return nil, errnie.Error(errnie.Err(
					errnie.Validation, "compiler: queued node "+id+" must report pending as UInt64", nil,
				))
			}
		}

		schemasMap[i] = ifaceSchema

		compiledNode := CompiledNode{
			Configured:   Implements(factory.InterfaceID, runtime.Configured_TypeID),
			ID:           id,
			Index:        NodeID(i),
			Source:       Implements(factory.InterfaceID, runtime.Source_TypeID),
			Queued:       Implements(factory.InterfaceID, runtime.Queued_TypeID),
			Standing:     Implements(factory.InterfaceID, runtime.Standing_TypeID),
			Inputs:       make(map[string]CompiledField),
			Outputs:      make(map[string]CompiledField),
			InputIndices: make(map[string]FieldID),
		}

		if ifaceSchema != nil {
			compiledNode.Resource = !ifaceSchema.HasWrite || !ifaceSchema.HasDone

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
					SchemaField:        fi.SchemaField,
					Name:               name,
					Which:              fi.Which,
					Offset:             fi.Offset,
					Index:              FieldID(idx),
					InUnion:            fi.InUnion,
					DiscriminantValue:  fi.DiscriminantValue,
					DiscriminantOffset: fi.DiscriminantOffset,
				}
			}

			// Phase 6: Compile authored inputs into the argument template.
			_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

			if err != nil {
				return nil, errnie.Error(errnie.Err(
					errnie.Internal, "compiler: allocate argument message", err,
				))
			}

			template, err := capnp.NewRootStruct(segment, ifaceSchema.WriteParams)

			if err != nil {
				return nil, errnie.Error(errnie.Err(
					errnie.Internal, "compiler: allocate argument template", err,
				))
			}

			for portName, rawBytes := range node.InputData {
				field, exists := resolveInputField(ifaceSchema, portName)

				if !exists {
					return nil, errnie.Error(errnie.Err(
						errnie.Validation,
						fmt.Sprintf("compiler: node %q (%s) has no input port %q", id, node.Type, portName),
						nil,
					))
				}

				// A wired port is supplied by its producer, not its editor control.
				if len(node.Connections.Inputs[portName]) > 0 {
					continue
				}

				literal, err := parseRawInputString(rawBytes)

				if err == nil {
					err = SetStaticField(template, field, literal)
				}

				if err != nil {
					return nil, errnie.Error(errnie.Err(
						errnie.Validation,
						fmt.Sprintf("compiler: invalid static input %q on node %q (%s)", portName, id, node.Type),
						err,
					))
				}
			}

			// The template is the program's own data, copied into every
			// evaluation for as long as the program runs. The read budget
			// guards against amplification in untrusted messages; spent on a
			// template, it would stop a long-running graph mid-stream.
			template.Message().ResetReadLimit(math.MaxUint64)
			compiledNode.ArgsTemplate = template
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
					if targetNode, ok := graph.Nodes[vID]; ok && strings.HasPrefix(targetNode.Type, "ui.") {
						continue
					}

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
						list:     toField.CapabilityList,
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

						var offset uint32

						switch fromField.Which {
						case schema.Type_Which_text, schema.Type_Which_data,
							schema.Type_Which_structType, schema.Type_Which_list,
							schema.Type_Which_anyPointer, schema.Type_Which_interface:
							offset = uint32(compiledNodes[vIdx].Write.ParamsSize.PointerCount)
							compiledNodes[vIdx].Write.ParamsSize.PointerCount++
						default:
							offset = uint32(compiledNodes[vIdx].Write.ParamsSize.DataSize / 8)
							compiledNodes[vIdx].Write.ParamsSize.DataSize += 8
						}

						toField = FieldInfo{
							Name:   target.PortName,
							Offset: offset,
							Which:  fromField.Which,
						}

						compiledNodes[vIdx].Inputs[target.PortName] = CompiledField{
							Name:   target.PortName,
							Which:  fromField.Which,
							Offset: offset,
							Index:  toFieldID,
						}
					}

					if hasInput {
						existing := compiledNodes[vIdx].Inputs[target.PortName]
						toField = FieldInfo{
							Name:   target.PortName,
							Offset: existing.Offset,
							Which:  existing.Which,
						}
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

					// A producer that fills only some of its slots says so
					// alongside them, so a consumer reading a slot that stayed
					// empty keeps waiting instead of reading a zero.
					presence, carried := resolveOutputField(uSchema, presencePort)

					if carried && presence.ElementWhich != schema.Type_Which_bool {
						return nil, errnie.Error(errnie.Err(
							errnie.Validation,
							fmt.Sprintf(
								"compiler: node %q (%s) reports slot presence on %q, which must be a list of Bool",
								uID, uNode.Type, presencePort,
							),
							nil,
						))
					}

					if !carried {
						presence = FieldInfo{}
					}

					copier, err := CompileFanOutCopier(
						fromField, toField, slot, presence, carried,
					)

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
						Delivered: CompileFanOutDelivery(fromField, slot, presence, carried),
					})

					compiledNodes[vIdx].RequiredMask |= (1 << toFieldID)
					continue
				}

				// A gathering port holds every producer that lands on it, so
				// its slots are handed out once they are all known.
				if toField.ValueList {
					presence, carried := resolveOutputField(uSchema, presencePort)
					var targetPresence *FieldInfo

					// present flags a numeric gathering port slot by slot. A port
					// gathering documents or names has slots of its own count,
					// which the flags do not describe.
					if presentField, ok := vSchema.Inputs["present"]; ok && presentField.Which == schema.Type_Which_list &&
						presentField.ElementWhich == schema.Type_Which_bool && toField.ElementWhich == schema.Type_Which_float64 {
						targetPresence = &presentField
					}

					fanInEdges = append(fanInEdges, fanInEdge{
						fromPort:       outPort,
						presence:       presence,
						carried:        carried,
						fromNode:       NodeID(uIdx),
						fromField:      fromFieldID,
						fromInfo:       fromField,
						toNode:         vIdx,
						toField:        toFieldID,
						toInfo:         toField,
						port:           target.PortName,
						targetPresence: targetPresence,
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
			var err error
			configBytes, err = json.Marshal(node.InputData)

			if err != nil {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "compiler: node configuration", err))
			}
		}

		// A capability owner must be replaced when any capability it owns changes.
		// Providers precede their consumers in execOrder, so this also covers
		// definition -> Consumer -> Group -> Workspace transitively.
		dependencies := make([]string, 0)

		for _, edge := range capabilityEdges {
			if edge.consumer == NodeID(i) {
				provider := compiledNodes[edge.provider].Identity
				dependencies = append(dependencies, fmt.Sprintf("%d:%s:%s:%s:%x:%x", edge.field, edge.port,
					provider.ID, provider.Type, provider.InterfaceID, provider.ConfigDigest))
			}
		}
		sort.Strings(dependencies)
		fingerprint, err := json.Marshal(struct {
			Configuration json.RawMessage
			Capabilities  []string
		}{configBytes, dependencies})

		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.Internal, "compiler: capability identity", err))
		}
		configDigest := sha256.Sum256(fingerprint)

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

	for index := range routes {
		route := &routes[index]
		destination := &compiledNodes[route.ToNode]

		if feedback[destination.ID][compiledNodes[route.FromNode].ID] {
			route.Deferred = true
			destination.RequiredMask &^= 1 << route.ToField
		}
	}

	markOrigins(compiledNodes, routes)

	if err := bindCapabilities(compiledNodes, capabilityEdges); err != nil {
		return nil, err
	}

	// 7. The nodes nothing produces for: where an evaluation begins.
	var roots []NodeID

	for i := range nodeCount {
		if compiledNodes[i].Resource || origins[execOrder[i]] != 0 {
			continue
		}

		roots = append(roots, compiledNodes[i].Index)
	}

	return &Program{
		Version:  graph.ID,
		Nodes:    compiledNodes,
		Routes:   routes,
		Roots:    roots,
		NodeMap:  nodeMap,
		UI:       uiPlan,
		Bindings: bindingPlan,
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

	// A graph file is authored JSON; it passes the same edge agreement as
	// every other document parsed into a graph.
	graph, err := ParseGraph(data)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("compiler: parse %s", jsonPath),
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

	if err := agree(graph); err != nil {
		return Graph{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"compiler: edge agreement failed",
			err,
		))
	}

	return graph, nil
}

/*
agree refuses a graph whose two halves disagree about an edge.

An edge is written down twice: once under the producer that sends it and once
under the consumer that receives it. Routing is built from the producer's
half alone, so a consumer declaring an edge its producer does not is not a
wiring mistake that shows up as a compile error. It shows up as a node with
nothing routed into it, running on whatever its arguments held — zero — while
the graph compiles, executes, and reports success.

That is the one failure mode this whole system cannot survive, because it
looks exactly like working. So the two halves have to say the same thing.
*/
func agree(graph Graph) error {
	type edge struct {
		from, fromPort, to, toPort string
	}

	declared := make(map[edge]bool)
	received := make(map[edge]bool)

	for id, node := range graph.Nodes {
		for port, targets := range node.Connections.Outputs {
			for _, target := range targets {
				declared[edge{id, port, target.NodeID, target.PortName}] = true
			}
		}

		for port, sources := range node.Connections.Inputs {
			for _, source := range sources {
				received[edge{source.NodeID, source.PortName, id, port}] = true
			}
		}
	}

	for held := range received {
		if declared[held] {
			continue
		}

		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"compiler: %q reads %q.%q on %q, but %q does not send it: "+
					"nothing would be routed and the node would run on zero",
				held.to, held.from, held.fromPort, held.toPort, held.from,
			),
			nil,
		))
	}

	for held := range declared {
		if received[held] {
			continue
		}

		// A wire the consumer does not acknowledge still routes, so this
		// one runs. It is refused anyway: an edge only one end knows about
		// is an edge nobody can reason about.
		if _, exists := graph.Nodes[held.to]; !exists {
			continue
		}

		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"compiler: %q sends %q to %q.%q, but %q does not read it",
				held.from, held.fromPort, held.to, held.toPort, held.to,
			),
			nil,
		))
	}

	return nil
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
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"compiler: failed to parse graph",
			err,
		))
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
presencePort is the port a producer uses to say which of its slots it filled.
A producer without one filled every slot it handed back.
*/
const presencePort = "present"

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

func parseRawInputString(raw json.RawMessage) (string, error) {
	literal := strings.TrimSpace(string(raw))

	if literal == "" || literal == "null" {
		return "", errnie.Error(errnie.Err(errnie.Validation, "static input has no value", nil))
	}

	if literal[0] == '"' {
		var value string
		err := sonic.Unmarshal(raw, &value)
		return value, err
	}

	if literal[0] != '{' {
		// Preserve integer digits; parsing through float64 loses large Int64 values.
		return literal, nil
	}

	var control map[string]json.RawMessage

	if err := sonic.Unmarshal(raw, &control); err != nil {
		return "", errnie.Error(errnie.Err(errnie.Validation, "invalid static control", err))
	}

	for _, key := range []string{"string", "float", "number", "int", "bool", "value"} {
		if value, exists := control[key]; exists {
			return parseRawInputString(value)
		}
	}

	// A JSON object can itself be a text/data literal. Its destination validates it.
	return literal, nil
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
			if strings.HasPrefix(node.Type, "definition:") && len(node.Connections.Outputs["self"]) == 0 {
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

		if graph.Origins == nil {
			graph.Origins = make(map[string]Origin)
		}

		for cid := range childGraph.Nodes {
			graph.Origins[prefix+cid] = Origin{Definition: defName, Node: cid}
		}

		childIngressTargets := make(map[string][]ConnectionTarget)
		childEgressSources := make(map[string][]ConnectionTarget)

		// 1. Add all namespaced child nodes
		for cid, cnode := range childGraph.Nodes {
			namespacedNode := Node{
				ID:        prefix + cid,
				Type:      cnode.Type,
				InputData: maps.Clone(cnode.InputData),
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
			return Graph{}, errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("compiler: failed to wire definition ports for %q", defID),
				err,
			))
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
	list     bool
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
	grouped := make(map[capabilitySlot][]capabilityEdge)

	for _, edge := range edges {
		slot := capabilitySlot{consumer: edge.consumer, field: edge.field}
		grouped[slot] = append(grouped[slot], edge)
	}

	for _, group := range grouped {
		if group[0].list {
			if err := bindCapabilityList(nodes, group); err != nil {
				return err
			}
			continue
		}

		if len(group) != 1 {
			return errnie.Error(errnie.Err(errnie.Validation, "compiler: scalar capability port has multiple providers", nil))
		}

		if err := bindCapability(nodes, group[0]); err != nil {
			return err
		}
	}
	return nil
}

/* bindCapabilityList preserves every wired provider in numbered-port order. */
func bindCapabilityList(nodes []CompiledNode, edges []capabilityEdge) error {
	sort.SliceStable(edges, func(left, right int) bool {
		first, _ := outputSlot(edges[left].port)
		second, _ := outputSlot(edges[right].port)

		if first != second {
			return first < second
		}
		return nodes[edges[left].provider].ID < nodes[edges[right].provider].ID
	})
	consumer := &nodes[edges[0].consumer]
	list, err := capnp.NewPointerList(consumer.ArgsTemplate.Segment(), int32(len(edges)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "compiler: allocate capability list", err))
	}

	for index, edge := range edges {
		provider, _, err := capabilityEnds(nodes, edge)

		if err != nil {
			return err
		}
		interfaceID := list.Segment().Message().CapTable().Add(provider.Client.AddRef())

		if err := list.Set(index, capnp.NewInterface(list.Segment(), interfaceID).ToPtr()); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "compiler: bind capability list member", err))
		}
	}

	if err := consumer.ArgsTemplate.SetPtr(edges[0].field, list.ToPtr()); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "compiler: bind capability list", err))
	}
	return nil
}

/*
capabilitySlot names one consumer port that producers are wired into.
*/
type capabilitySlot struct {
	consumer NodeID
	field    uint16
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
	// Definition controls address the same child fields as edges.
	for port, value := range defNode.InputData {
		childID, field, addressed := strings.Cut(port, ".")

		if !addressed || field == "" {
			return errnie.Error(errnie.Err(errnie.Validation, fmt.Sprintf("compiler: definition %q static input %q must name <node>.<field>", defID, port), nil))
		}

		child, known := graph.Nodes[prefix+childID]

		if !known {
			return errnie.Error(errnie.Err(errnie.Validation, fmt.Sprintf("compiler: definition %q static input %q names unknown child %q", defID, port, childID), nil))
		}

		if child.InputData == nil {
			child.InputData = make(map[string]json.RawMessage)
		}

		child.InputData[field] = value
		graph.Nodes[prefix+childID] = child
	}

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
	fromPort       string
	presence       FieldInfo
	carried        bool
	fromNode       NodeID
	fromField      FieldID
	fromInfo       FieldInfo
	toNode         NodeID
	toField        FieldID
	toInfo         FieldInfo
	port           string
	targetPresence *FieldInfo
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

		// Numbered ports represent numeric positions, not lexical labels.
		sort.SliceStable(group, func(left, right int) bool {
			first, _ := outputSlot(group[left].port)
			second, _ := outputSlot(group[right].port)
			return first < second
		})

		// A gathering port gathers producers, but one producer may already
		// carry the whole list: a grid handing over every value it delivered
		// is one wire, not one wire per value. Gathering that into a list of
		// lists would bury it a level deeper than the consumer reads.
		_, indexed := outputSlot(group[0].fromPort)

		if len(group) == 1 && !indexed && group[0].fromInfo.ValueList &&
			group[0].fromInfo.ElementWhich == group[0].toInfo.ElementWhich {
			edge := group[0]

			copier, err := CompileCopier(
				FieldInfo{
					Name:   edge.fromInfo.Name,
					Offset: edge.fromInfo.Offset,
					Which:  schema.Type_Which_list,
				},
				FieldInfo{
					Name:   edge.toInfo.Name,
					Offset: edge.toInfo.Offset,
					Which:  schema.Type_Which_list,
				},
			)

			if err != nil {
				return nil, errnie.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf(
						"compiler: %q hands a whole list to gathering port %q but their elements differ",
						edge.fromInfo.Name, edge.port,
					),
					err,
				))
			}

			// A list inside a union branch is only there while that branch is
			// active; handing over an inactive branch would read another
			// member's slot.
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

			continue
		}

		for index, edge := range group {
			var copier Copier
			var err error
			var delivered func(capnp.Struct) bool
			sourceIndex, indexed := outputSlot(edge.fromPort)

			if edge.fromInfo.ValueList && indexed {
				copier, err = CompileFanInSlotCopier(edge.fromInfo, edge.toInfo, edge.targetPresence, sourceIndex, index, len(group))
				delivered = CompileFanOutDelivery(edge.fromInfo, sourceIndex, edge.presence, edge.carried)
			}

			if !edge.fromInfo.ValueList || !indexed {
				copier, err = CompileFanInCopier(edge.fromInfo, edge.toInfo, edge.targetPresence, index, len(group))
			}

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
				Delivered:      delivered,
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

func lowerUIAndBindings(graph Graph) (*UIPlan, *BindingPlan) {
	inputs := make(map[string]Origin)
	for id, node := range graph.Nodes {
		if !strings.HasPrefix(node.Type, "ui.") {
			continue
		}
		origin, known := graph.Origins[id]
		if !known {
			origin = Origin{Definition: graph.ID, Node: id}
		}
		inputs[id] = origin
	}
	bindings := make([]BindingEntry, 0)
	routes := make([]UIRoutePlan, 0)

	for id, node := range graph.Nodes {
		if strings.HasPrefix(node.Type, "ui.") {
			continue
		}

		for outPort, targets := range node.Connections.Outputs {
			for _, target := range targets {
				consumer, exists := graph.Nodes[target.NodeID]

				if !exists {
					continue
				}

				if strings.HasPrefix(consumer.Type, "ui.") {
					origin, expanded := graph.Origins[target.NodeID]

					if !expanded {
						origin = Origin{Definition: graph.ID, Node: target.NodeID}
					}

					bindings = append(bindings, BindingEntry{
						SourceNode:      id,
						SourcePort:      outPort,
						TargetNode:      target.NodeID,
						TargetProp:      target.PortName,
						TargetGraph:     origin.Definition,
						TargetComponent: origin.Node,
					})
				}
			}
		}
	}

	for id, node := range graph.Nodes {
		if node.Type != "ui.UIRoute" {
			continue
		}

		pathVal := "/"

		if val, ok := node.InputData["path"]; ok {
			var p struct {
				Value string `json:"value"`
			}

			err := sonic.Unmarshal(val, &p)

			if err == nil && p.Value != "" {
				pathVal = p.Value
			}

			if err != nil || p.Value == "" {
				var direct string
				err = sonic.Unmarshal(val, &direct)

				if err == nil && direct != "" {
					pathVal = direct
				}
			}
		}

		titleVal := ""

		if val, ok := node.InputData["title"]; ok {
			var t struct {
				Value string `json:"value"`
			}

			err := sonic.Unmarshal(val, &t)

			if err == nil {
				titleVal = t.Value
			}

			if err != nil {
				var direct string
				err = sonic.Unmarshal(val, &direct)

				if err == nil {
					titleVal = direct
				}
			}
		}

		components := lowerUIChildren(graph, id)

		routes = append(routes, UIRoutePlan{
			Path:       pathVal,
			Title:      titleVal,
			Components: components,
		})
	}

	if len(routes) == 0 {
		childSet := make(map[string]bool)

		for _, node := range graph.Nodes {
			if !strings.HasPrefix(node.Type, "ui.") {
				continue
			}

			for inPort, targets := range node.Connections.Inputs {
				if strings.HasPrefix(inPort, "components") {
					for _, target := range targets {
						childSet[target.NodeID] = true
					}
				}
			}
		}

		var rootComponents []UINodePlan

		for id, node := range graph.Nodes {
			if !strings.HasPrefix(node.Type, "ui.") || node.Type == "ui.UIRoute" {
				continue
			}

			if !childSet[id] {
				rootComponents = append(rootComponents, lowerUINode(graph, id, make(map[string]bool)))
			}
		}

		if len(rootComponents) > 0 {
			routes = append(routes, UIRoutePlan{
				Path:       "/",
				Title:      "Root",
				Components: rootComponents,
			})
		}
	}

	return &UIPlan{Routes: routes}, &BindingPlan{Bindings: bindings, Inputs: inputs}
}

func lowerUIChildren(graph Graph, parentID string) []UINodePlan {
	parent, ok := graph.Nodes[parentID]

	if !ok {
		return nil
	}

	ports := make([]string, 0)

	for portName := range parent.Connections.Inputs {
		if strings.HasPrefix(portName, "components") {
			ports = append(ports, portName)
		}
	}

	sort.Slice(ports, func(i, j int) bool {
		return parsePortIndex(ports[i]) < parsePortIndex(ports[j])
	})

	var children []UINodePlan
	visited := map[string]bool{parentID: true}

	for _, port := range ports {
		for _, target := range parent.Connections.Inputs[port] {
			child, exists := graph.Nodes[target.NodeID]

			if exists && strings.HasPrefix(child.Type, "ui.") {
				children = append(children, lowerUINode(graph, target.NodeID, copyVisited(visited)))
			}
		}
	}

	return children
}

func parsePortIndex(port string) int {
	if port == "components" {
		return 0
	}

	if after, ok := strings.CutPrefix(port, "components_"); ok {
		num, err := strconv.Atoi(after)

		if err == nil {
			return num
		}
	}

	return int(^uint(0) >> 1)
}

func copyVisited(visited map[string]bool) map[string]bool {
	cp := make(map[string]bool, len(visited)+1)
	maps.Copy(cp, visited)

	return cp
}

func lowerUINode(graph Graph, id string, visited map[string]bool) UINodePlan {
	node := graph.Nodes[id]
	visited[id] = true

	compName := strings.TrimPrefix(node.Type, "ui.")
	props := make(map[string]any)
	var className string

	for propName, rawVal := range node.InputData {
		if propName == "components" || strings.HasPrefix(propName, "components_") {
			continue
		}

		var entry struct {
			Value any `json:"value"`
		}
		err := sonic.Unmarshal(rawVal, &entry)

		if err == nil && entry.Value != nil {
			if propName == "className" {
				if s, ok := entry.Value.(string); ok {
					className = s
				}
			}

			if propName != "className" {
				props[propName] = entry.Value
			}

			continue
		}

		var direct any
		err = sonic.Unmarshal(rawVal, &direct)

		if err == nil && direct != nil {
			if propName == "className" {
				if s, ok := direct.(string); ok {
					className = s
				}
			}

			if propName != "className" {
				props[propName] = direct
			}
		}
	}

	children := lowerUIChildren(graph, id)

	return UINodePlan{
		ID:        id,
		Name:      compName,
		ClassName: className,
		Props:     props,
		Children:  children,
	}
}

/*
holdsRetained reports a producer that hands back what it held before this
evaluation, which a consumer reads without waiting for it.
*/
func holdsRetained(registry *Registry, producer Node) bool {
	factory, err := registry.Resolve(producer.Type)

	if err != nil || factory.InterfaceID == 0 {
		return false
	}

	return Implements(factory.InterfaceID, store.Retained_TypeID)
}

/*
markOrigins finds the nodes an evaluation starts from. A node with nothing
wired into it owns its own timing, a source or a queue has something of its
own to hand out, a standing node reports even when told nothing, and a store
written back by feedback carries a loop that steps once per evaluation, so
each of these runs every evaluation. A node
fed only through gathering ports is not an origin: it runs when something
lands on one of them, and an evaluation that brings it nothing leaves it
alone.
*/
func markOrigins(nodes []CompiledNode, routes []Route) {
	wired := make([]bool, len(nodes))
	fed := make([]bool, len(nodes))

	for _, route := range routes {
		if route.Deferred {
			fed[route.ToNode] = true
			continue
		}

		wired[route.ToNode] = true
	}

	for index := range nodes {
		node := &nodes[index]
		node.Origin = node.Source || node.Standing || (node.RequiredMask == 0 && (!wired[index] || fed[index] || node.Queued))
	}
}
