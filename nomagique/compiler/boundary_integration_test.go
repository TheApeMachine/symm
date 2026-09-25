package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
)

// boundaryGraph is an actual signal -> dependent logic graph. Both owners
// publish measurements; the map does not know which category an owner is in.
func boundaryGraph(t *testing.T, replay bool) Graph {
	t.Helper()
	graph := Graph{ID: "boundary-fixture", Nodes: make(map[string]Node)}
	add := func(id, kind string, static map[string]any) {
		node := Node{ID: id, Type: kind, InputData: make(map[string]json.RawMessage), Connections: Connections{Inputs: make(map[string][]ConnectionTarget), Outputs: make(map[string][]ConnectionTarget)}}
		for port, value := range static {
			encoded, err := json.Marshal(map[string]any{"value": value})
			if err != nil { t.Fatal(err) }
			node.InputData[port] = encoded
		}
		graph.Nodes[id] = node
	}
	wire := func(from, out, to, input string) {
		source, target := graph.Nodes[from], graph.Nodes[to]
		source.Connections.Outputs[out] = append(source.Connections.Outputs[out], ConnectionTarget{NodeID: to, PortName: input})
		target.Connections.Inputs[input] = append(target.Connections.Inputs[input], ConnectionTarget{NodeID: from, PortName: out})
	}
	add("boundary", "store.Boundary", map[string]any{"layout": `[{"producer":"signal","coordinates":[0]},{"producer":"logic","coordinates":[1]}]`})
	add("map", "definition:impulse_map", nil)
	wire("boundary", "ready.values", "map", "change.value")
	wire("boundary", "ready.present", "map", "change.present")

	if replay { return graph }

	add("input", "store.Grid", map[string]any{"interests": "value"})
	add("signal", "arithmetic.Add", map[string]any{"b": 0})
	add("logic", "arithmetic.Multiply", map[string]any{"b": -2})
	wire("input", "values_0", "signal", "a")
	wire("signal", "out", "logic", "a")

	for index, producer := range []string{"signal", "logic"} {
		publication := "publication_" + producer
		add(publication, "data.MeasurementService", map[string]any{"producer": producer, "coordinates": fmt.Sprintf("[%d]", index)})
		wire(producer, "out", publication, "values_0")
		wire("input", "run", publication, "run")
		wire("input", "sequence", publication, "tick")
		wire(publication, "read", "boundary", fmt.Sprintf("publications_%d", index))
	}

	wire("input", "run", "boundary", "run")
	wire("input", "sequence", "boundary", "sequence")
	wire("input", "receipt", "boundary", "receipt")
	return graph
}

func boundaryArguments(t *testing.T, program *Program, id string) capnp.Struct {
	t.Helper()
	_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil { t.Fatal(err) }
	arguments, err := capnp.NewRootStruct(segment, program.Nodes[program.NodeMap[id]].Write.ParamsSize)
	if err != nil { t.Fatal(err) }
	return arguments
}

func boundaryOutput(t *testing.T, program *Program) store.BoundaryResult {
	t.Helper()
	result, found := program.Result("boundary")
	if !found { t.Fatal("the compiled boundary did not run") }
	return store.BoundaryResult(result)
}

// mapReading compares only real numeric geometry, not RPC message allocation.
func boundaryMapReading(t *testing.T, program *Program) []float64 {
	t.Helper()
	result, found := program.Result("map.relaxation")
	if !found { return nil }
	node := program.Nodes[program.NodeMap["map.relaxation"]]
	field := node.Outputs["positions"]
	pointer, err := result.Ptr(uint16(field.Offset))
	if err != nil { t.Fatal(err) }
	values := capnp.Float64List(pointer.List())
	out := make([]float64, values.Len())
	for index := range out { out[index] = values.At(index) }
	return out
}

func TestBoundaryCompiledReplay(t *testing.T) {
	Convey("Signal and dependent logic measurements are one replayable graph boundary", t, func() {
		live, err := Compile(boundaryGraph(t, false), nil, DefaultRepository())
		So(err, ShouldBeNil)
		if err != nil { return }
		defer live.Release()
		replay, err := Compile(boundaryGraph(t, true), nil, DefaultRepository())
		So(err, ShouldBeNil)
		if err != nil { return }
		defer replay.Release()

		var rows [][]byte
		var positions [][]float64

		// Multiple alternating developments, not a single favourable spike.
		for index := range 64 {
			sequence := int64(index + 1)
			value := float64((index%16)-8)
			arguments := boundaryArguments(t, live, "input")
			input := store.Grid_write_Params(arguments)
			So(input.SetInterests("value"), ShouldBeNil)
			So(input.SetRun("captured-run"), ShouldBeNil)
			input.SetSequence(sequence)
			So(input.SetReceipt([]byte(fmt.Sprintf(`{"sourceSequence":%d}`, sequence))), ShouldBeNil)
			payloads, err := input.NewData(1)
			So(err, ShouldBeNil)
			So(payloads.Set(0, []byte(fmt.Sprintf(`{"value":%g}`, value))), ShouldBeNil)
			So(live.Execute(t.Context(), map[NodeID]capnp.Struct{live.NodeMap["input"]: arguments}), ShouldBeNil)
			result := boundaryOutput(t, live)
			So(result.Which(), ShouldEqual, store.BoundaryResult_Which_ready)
			values, err := result.Ready().Values()
			So(err, ShouldBeNil)
			So(values.At(0), ShouldEqual, value)
			So(values.At(1), ShouldEqual, -2*value)
			row, err := result.Row()
			So(err, ShouldBeNil)
			rows = append(rows, bytes.Clone(row))
			positions = append(positions, boundaryMapReading(t, live))
		}

		So(len(rows), ShouldEqual, 64)
		So(len(positions[len(positions)-1]), ShouldEqual, 4)

		for index, row := range rows {
			arguments := boundaryArguments(t, replay, "boundary")
			So(store.Boundary_write_Params(arguments).SetReplay(row), ShouldBeNil)
			So(replay.Execute(t.Context(), map[NodeID]capnp.Struct{replay.NodeMap["boundary"]: arguments}), ShouldBeNil)
			result := boundaryOutput(t, replay)
			replayedRow, err := result.Row()
			So(err, ShouldBeNil)
			So(replayedRow, ShouldResemble, row)
			So(boundaryMapReading(t, replay), ShouldResemble, positions[index])
		}

		Convey("An idle evaluation cannot manufacture another measurement cut", func() {
			So(live.Execute(t.Context(), nil), ShouldBeNil)
			result := boundaryOutput(t, live)
			So(result.Which(), ShouldEqual, store.BoundaryResult_Which_idle)
			row, err := result.Row()
			So(err, ShouldBeNil)
			So(row, ShouldBeEmpty)
		})
	})
}
