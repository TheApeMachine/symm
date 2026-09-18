package signal

import (
	"context"
	"fmt"
	"iter"
	"sync/atomic"
	"unsafe"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/runtime/catalog"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/signal/shared"
)

/*
Definition is the Flume-compatible JSON schema describing one signal's
pipeline topology, declared interests, and metric sinks.
*/
type Definition struct {
	ID        string               `json:"id"`
	Name      string               `json:"name"`
	Source    string               `json:"source"`
	Symbol    string               `json:"symbol,omitempty"`
	Interests [][]string           `json:"interests,omitempty"`
	Nodes     map[string]FlumeNode `json:"nodes"`
}

type FlumeNode struct {
	ID          string                    `json:"id"`
	Type        string                    `json:"type"`
	X           float64                   `json:"x"`
	Y           float64                   `json:"y"`
	InputData   map[string]map[string]any `json:"inputData"`
	Connections FlumeConnections          `json:"connections"`
}

type FlumeConnections struct {
	Inputs  map[string][]FlumeConnection `json:"inputs"`
	Outputs map[string][]FlumeConnection `json:"outputs"`
}

type FlumeConnection struct {
	NodeID   string `json:"nodeId"`
	PortName string `json:"portName"`
}

/*
Compile parses raw Flume JSON into a running Signal registered to grid.
*/
func Compile(
	ctx context.Context,
	grid *store.Grid[*geometry.Coordinate],
	rawJSON []byte,
	symbolOverride string,
) (*Signal, error) {
	var def Definition

	if err := sonic.Unmarshal(rawJSON, &def); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"signal: decode flume json definition",
			err,
		))
	}

	symbol := def.Symbol

	if symbolOverride != "" {
		symbol = symbolOverride
	}

	name := def.Name

	if name == "" {
		name = def.ID
	}

	source := def.Source

	if source == "" {
		source = name
	}

	signal := NewSignal(ctx, name, source, symbol)
	compiler := &graphCompiler{
		def:       &def,
		signal:    signal,
		symbol:    symbol,
		instances: make(map[string]core.Primitive),
		visited:   make(map[string]bool),
	}

	if err := compiler.compile(grid); err != nil {
		return nil, errnie.Error(err)
	}

	return signal, nil
}

var metricCoordCounter atomic.Int64

type metricSink struct {
	*core.PrimitiveError
	coord    *geometry.Coordinate
	retained *store.Retained[float64]
}

func newMetricSink(coord *geometry.Coordinate, retained *store.Retained[float64]) *metricSink {
	return &metricSink{
		PrimitiveError: core.NewPrimitiveError(),
		coord:          coord,
		retained:       retained,
	}
}

func (sink *metricSink) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			for ptr := range sink.retained.Next(nil) {
				if !yield(ptr) {
					return
				}
			}

			return
		}

		for arriving := range in {
			if arriving == nil {
				continue
			}

			val := *(*float64)(arriving)
			one := func(yieldRet func(unsafe.Pointer) bool) {
				yieldRet(arriving)
			}

			for range sink.retained.Next(one) {
			}

			obs := statistic.NewObservation[*geometry.Coordinate](sink.coord, val, 1.0, 1.0)

			if !yield(unsafe.Pointer(obs)) {
				return
			}
		}
	}
}

type graphCompiler struct {
	def       *Definition
	signal    *Signal
	symbol    string
	instances map[string]core.Primitive
	visited   map[string]bool
}

func (c *graphCompiler) compile(grid *store.Grid[*geometry.Coordinate]) error {
	sources := make([]FlumeNode, 0)

	for _, node := range c.def.Nodes {
		if node.Type == "source" || node.Type == "builtin.source" {
			sources = append(sources, node)
		}
	}

	if len(sources) == 0 {
		return errnie.Err(
			errnie.Validation,
			fmt.Sprintf("signal: definition %s contains no source nodes", c.def.Name),
			nil,
		)
	}

	for _, sourceNode := range sources {
		targets := c.collectStreamTargets(sourceNode)

		if len(targets) == 0 {
			continue
		}

		branches := make([]core.Primitive, 0, len(targets))

		for _, target := range targets {
			branch, err := c.compileStreamBranch(target.NodeID)

			if err != nil {
				return err
			}

			if branch != nil {
				branches = append(branches, branch)
			}
		}

		if len(branches) == 0 {
			continue
		}

		var pipeline core.Primitive

		if len(branches) == 1 {
			pipeline = branches[0]
		}

		if len(branches) > 1 {
			pipeline = transport.NewFan(branches...)
		}

		interests := c.resolveInterests(sourceNode)
		conn := transport.NewConn[*geometry.Coordinate](pipeline)
		shared.Register(grid, conn, interests)
		c.signal.conns = append(c.signal.conns, conn)
	}

	return nil
}

func (c *graphCompiler) instantiateNode(nodeID string) (core.Primitive, error) {
	if inst, ok := c.instances[nodeID]; ok {
		return inst, nil
	}

	node, exists := c.def.Nodes[nodeID]

	if !exists {
		return nil, errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("signal: node %s not found in definition %s", nodeID, c.def.Name),
			nil,
		)
	}

	if c.visited[nodeID] {
		return nil, errnie.Err(
			errnie.Validation,
			fmt.Sprintf("signal: cyclic connection detected at node %s", nodeID),
			nil,
		)
	}

	c.visited[nodeID] = true
	defer func() {
		delete(c.visited, nodeID)
	}()

	config := c.extractConfig(node)

	if node.Type == "sink" || node.Type == "builtin.sink" {
		metricName, _ := config["metric"].(string)

		if metricName == "" {
			metricName = nodeID
		}

		hold, exists := c.signal.holds[metricName]

		if !exists {
			hold = store.NewRetained[float64]()
			c.signal.holds[metricName] = hold
		}

		coord := geometry.NewCoordinate(int(metricCoordCounter.Add(1)), 0)
		sink := newMetricSink(coord, hold)
		c.instances[nodeID] = sink
		return sink, nil
	}

	// Resolve constructor connections (all incoming ports EXCEPT "in")
	childPrimitives := make(map[string]core.Primitive)

	for portName, conns := range node.Connections.Inputs {
		if portName == "in" {
			continue
		}

		if len(conns) == 1 {
			child, err := c.instantiateNode(conns[0].NodeID)

			if err != nil {
				return nil, err
			}

			childPrimitives[portName] = child
		}

		if len(conns) > 1 {
			for idx, conn := range conns {
				child, err := c.instantiateNode(conn.NodeID)

				if err != nil {
					return nil, err
				}

				childPrimitives[fmt.Sprintf("%s_%d", portName, idx)] = child

				if idx == 0 {
					childPrimitives[portName] = child
				}
			}
		}
	}

	prim, err := catalog.Build(node.Type, config, childPrimitives)

	if err != nil {
		return nil, err
	}

	c.instances[nodeID] = prim
	return prim, nil
}

func (c *graphCompiler) compileStreamBranch(nodeID string) (core.Primitive, error) {
	node, exists := c.def.Nodes[nodeID]

	if !exists {
		return nil, errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("signal: node %s not found in definition %s", nodeID, c.def.Name),
			nil,
		)
	}

	prim, err := c.instantiateNode(nodeID)

	if err != nil {
		return nil, err
	}

	targets := c.collectStreamTargets(node)

	if len(targets) == 0 {
		return prim, nil
	}

	if len(targets) == 1 {
		nextPrim, err := c.compileStreamBranch(targets[0].NodeID)

		if err != nil {
			return nil, err
		}

		return nomagique.NewNumber(prim, nextPrim), nil
	}

	downstreamBranches := make([]core.Primitive, 0, len(targets))

	for _, target := range targets {
		branch, err := c.compileStreamBranch(target.NodeID)

		if err != nil {
			return nil, err
		}

		if branch != nil {
			downstreamBranches = append(downstreamBranches, branch)
		}
	}

	return nomagique.NewNumber(prim, transport.NewFan(downstreamBranches...)), nil
}

func (c *graphCompiler) collectStreamTargets(node FlumeNode) []FlumeConnection {
	targets := make([]FlumeConnection, 0)

	for _, conns := range node.Connections.Outputs {
		for _, conn := range conns {
			targetNode, exists := c.def.Nodes[conn.NodeID]

			if !exists {
				continue
			}

			// A stream target receives data on port "in", or is a sink on port "value"
			if conn.PortName == "in" || ((targetNode.Type == "sink" || targetNode.Type == "builtin.sink") && conn.PortName == "value") {
				targets = append(targets, conn)
			}
		}
	}

	return targets
}

func (c *graphCompiler) extractConfig(node FlumeNode) map[string]any {
	result := make(map[string]any)

	if node.InputData == nil {
		return result
	}

	if cfg, ok := node.InputData["_config"]; ok {
		for key, val := range cfg {
			result[key] = val
		}
	}

	for portName, portControls := range node.InputData {
		if portName == "_config" {
			continue
		}

		for key, val := range portControls {
			result[key] = val
		}
	}

	if _, hasSym := result["symbol"]; !hasSym && c.symbol != "" {
		result["symbol"] = c.symbol
	}

	return result
}

func (c *graphCompiler) resolveInterests(sourceNode FlumeNode) [][]string {
	config := c.extractConfig(sourceNode)

	if rawInterests, ok := config["interests"]; ok {
		if arr, isArr := rawInterests.([]any); isArr {
			interests := make([][]string, 0, len(arr))

			for _, item := range arr {
				if segArr, isSegArr := item.([]any); isSegArr {
					segs := make([]string, 0, len(segArr))

					for _, seg := range segArr {
						if str, isStr := seg.(string); isStr {
							segs = append(segs, str)
						}
					}

					interests = append(interests, segs)
				}
			}

			if len(interests) > 0 {
				return interests
			}
		}

		if str, isStr := rawInterests.(string); isStr {
			switch str {
			case "ticker":
				return shared.Ticker
			case "trade":
				return shared.Trade
			case "level3":
				return shared.Level3
			case "symbol_last":
				return shared.SymbolLast
			case "symbol_last_time":
				return shared.SymbolLastTime
			}
		}
	}

	if len(c.def.Interests) > 0 {
		return c.def.Interests
	}

	return shared.Ticker
}
