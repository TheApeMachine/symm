package compiler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/iceberg-go"
	sqlcat "github.com/apache/iceberg-go/catalog/sql"
	icetable "github.com/apache/iceberg-go/table"
	gorillaws "github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/network/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/store/tables"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

type failingAddition struct{ arithmetic.AddServer }

func (server *failingAddition) Done(ctx context.Context, call arithmetic.Add_done) error {
	return errnie.Error(errnie.Err(errnie.IO, "fixture: terminal result failed", nil))
}

func TestProgramExecute(t *testing.T) {
	Convey("Given a terminal capability whose done call fails", t, func() {
		registry := NewRegistry()
		registry.Register("test.FailingAddition", Factory{
			InterfaceID: arithmetic.Add_TypeID,
			New: func(ctx context.Context, config []byte) (capnp.Client, error) {
				return capnp.Client(arithmetic.Add_ServerToClient(&failingAddition{})), nil
			},
		})
		program, err := CompileJSON([]byte(`{"nodes":{"terminal":{"id":"terminal","type":"test.FailingAddition"}}}`), registry, nil)
		So(err, ShouldBeNil)
		defer program.Release()

		Convey("Then execution preserves the terminal failure and node identity", func() {
			err := program.Execute(context.Background(), nil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "terminal result failed")
			So(err.Error(), ShouldContainSubstring, `node "terminal"`)
		})
	})
}

func TestProgramCarriedPayload(t *testing.T) {
	Convey("Given a continuously circulating loop and an external source", t, func() {
		program, err := CompileJSON([]byte(`{"nodes":{
   "loop":{"id":"loop","type":"controlflow.Loop","inputData":{"data":{"value":"ping"},"active":{"value":true}}},
   "socket":{"id":"socket","type":"websocket.WebSocketClient"}
  }}`), nil, nil)
		So(err, ShouldBeNil)
		defer program.Release()
		So(program.Execute(context.Background(), nil), ShouldBeNil)

		Convey("Then circulating bytes do not count as incoming activity", func() {
			So(program.carriedPayload(), ShouldBeFalse)
		})

		Convey("When the external source has received bytes", func() {
			source := program.Nodes[program.NodeMap["socket"]]
			So(source.Source, ShouldBeTrue)
			_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
			So(err, ShouldBeNil)
			result, err := capnp.NewRootStruct(segment, source.Done.ResultSize)
			So(err, ShouldBeNil)
			field := source.Outputs["frame.read"]
			result.SetUint16(capnp.DataOffset(field.DiscriminantOffset*2), field.DiscriminantValue)
			So(result.SetData(uint16(source.Outputs["frame.read"].Offset), []byte("received frame")), ShouldBeNil)
			program.results[source.ID] = result
			So(program.carriedPayload(), ShouldBeTrue)

			Convey("Then consuming the bytes restores idle detection", func() {
				So(program.Execute(context.Background(), nil), ShouldBeNil)
				So(program.carriedPayload(), ShouldBeFalse)
			})
		})
	})
}

func BenchmarkProgramExecute(b *testing.B) {
	program, err := CompileJSON([]byte(`{"nodes":{"add":{"id":"add","type":"arithmetic.Add","inputData":{"a":{"value":2},"b":{"value":3}}}}}`), nil, nil)

	if err != nil {
		b.Fatal(err)
	}
	defer program.Release()
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := program.Execute(context.Background(), nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProgramCarriedPayload(b *testing.B) {
	// Exercise a circulating control value beside an idle source, without network I/O.
	program, err := CompileJSON([]byte(`{"nodes":{
  "loop":{"id":"loop","type":"controlflow.Loop","inputData":{"data":{"value":"ping"},"active":{"value":true}}},
  "socket":{"id":"socket","type":"websocket.WebSocketClient"}
 }}`), nil, nil)

	if err != nil {
		b.Fatal(err)
	}
	defer program.Release()
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := program.Execute(context.Background(), nil); err != nil {
			b.Fatal(err)
		}

		if program.carriedPayload() {
			b.Fatal("idle graph reported external activity")
		}
	}
}

func TestProgramExecuteResource(t *testing.T) {
	Convey("Given a capability-only Transform bound into a consumer", t, func() {
		registry := NewRegistry()
		registry.Register("test.Transform", Factory{InterfaceID: data.Transform_TypeID, New: func(ctx context.Context, config []byte) (capnp.Client, error) {
			return capnp.Client(data.Transform_ServerToClient(data.NewScale(ctx))), nil
		}})
		registry.Register("data.Map", DefaultRegistry().ResolveMust("data.Map"))
		program, err := CompileJSON([]byte(`{"nodes":{
  "transform":{"id":"transform","type":"test.Transform","connections":{"outputs":{"self":[{"nodeId":"map","portName":"body"}]}}},
  "map":{"id":"map","type":"data.Map","inputData":{"data":{"value":"{\"values\":[1,2,3]}"},"path":{"value":"values"}},"connections":{"inputs":{"body":[{"nodeId":"transform","portName":"self"}]}}}
  }}`), registry, nil)
		So(err, ShouldBeNil)
		defer program.Release()
		resource := program.Nodes[program.NodeMap["transform"]]
		So(resource.Resource, ShouldBeTrue)
		So(program.Roots, ShouldNotContain, resource.Index)
		for range 3 {
			So(program.Execute(context.Background(), nil), ShouldBeNil)
			_, found := program.Result("transform")
			So(found, ShouldBeFalse)
			value, found := program.Result("map")
			So(found, ShouldBeTrue)
			result := data.Map_done_Results(value)
			So(result.Count(), ShouldEqual, 3)
		}
	})
}

func TestProgramExecuteExcursion(t *testing.T) {
	Convey("Given a consumer of completed excursion values", t, func() {
		program, err := CompileJSON([]byte(`{"nodes":{
   "excursion":{"id":"excursion","type":"temporal.Excursion","connections":{"outputs":{"move.anchor":[{"nodeId":"consumer","portName":"a"}],"move.ignition":[{"nodeId":"consumer","portName":"b"}]}}},
   "consumer":{"id":"consumer","type":"arithmetic.Add","connections":{"inputs":{"a":[{"nodeId":"excursion","portName":"move.anchor"}],"b":[{"nodeId":"excursion","portName":"move.ignition"}]}}}
  }}`), nil, nil)
		So(err, ShouldBeNil)
		defer program.Release()
		delivered := 0
		price := 100.0

		for step := range 100 {
			rate := 0.005

			if step >= 60 {
				rate = -0.005
			}

			price *= 1 + rate
			_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
			So(err, ShouldBeNil)
			params, err := temporal.NewExcursion_write_Params(segment)
			So(err, ShouldBeNil)
			params.SetValue(price)
			So(program.Execute(context.Background(), map[NodeID]capnp.Struct{program.NodeMap["excursion"]: capnp.Struct(params)}), ShouldBeNil)
			source, found := program.Result("excursion")
			So(found, ShouldBeTrue)
			_, ran := program.Result("consumer")
			move := temporal.ExcursionResult(source).Which() == temporal.ExcursionResult_Which_move
			So(ran, ShouldEqual, move)
			_, err = program.Float64Result("excursion", "move.anchor")

			if !move {
				So(err, ShouldNotBeNil)
			}

			if move {
				So(err, ShouldBeNil)
			}

			if ran {
				delivered++
			}
		}

		So(delivered, ShouldBeGreaterThan, 0)
		So(delivered, ShouldBeLessThan, 100)
	})
}

type failingDurable struct {
	tables.IcebergTableServer
	calls int
}

func (owner *failingDurable) Flush(ctx context.Context, call runtime.Durable_flush) error {
	owner.calls++
	return errnie.Error(errnie.Err(errnie.IO, "fixture: durability unavailable", nil))
}

func TestProgramFlush(t *testing.T) {
	Convey("Given a durable owner whose flush fails", t, func() {
		owner := &failingDurable{}
		registry := NewRegistry()
		registry.Register("test.Durable", Factory{InterfaceID: tables.IcebergTable_TypeID,
			New: func(ctx context.Context, config []byte) (capnp.Client, error) {
				return capnp.Client(tables.IcebergTable_ServerToClient(owner)), nil
			},
		})
		program, err := CompileJSON([]byte(`{"nodes":{"archive":{"id":"archive","type":"test.Durable"}}}`), registry, nil)
		So(err, ShouldBeNil)
		defer program.Release()
		err = program.Flush(context.Background())
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "archive")
		So(err.Error(), ShouldContainSubstring, "durability unavailable")
		So(owner.calls, ShouldEqual, 1)
	})
}

func TestProgramExecuteIdleCapture(t *testing.T) {
	Convey("Given a source wired to capture through the frame arm", t, func() {
		program, err := CompileJSON([]byte(`{"nodes":{
 "feed":{"id":"feed","type":"websocket.WebSocketClient","connections":{"outputs":{
  "frame.read":[{"nodeId":"capture","portName":"payload"}],
  "frame.endpoint":[{"nodeId":"capture","portName":"endpoint"}],
  "frame.receivedAt":[{"nodeId":"capture","portName":"receivedAt"}]
 }}},
 "capture":{"id":"capture","type":"store.Capture","connections":{"inputs":{
  "payload":[{"nodeId":"feed","portName":"frame.read"}],
  "endpoint":[{"nodeId":"feed","portName":"frame.endpoint"}],
  "receivedAt":[{"nodeId":"feed","portName":"frame.receivedAt"}]
 }}}
}}`), nil)
		So(err, ShouldBeNil)
		defer program.Release()

		Convey("Then repeated idle polls do not execute capture with empty data", func() {
			for observation := 0; observation < 3; observation++ {
				So(program.Execute(context.Background(), nil), ShouldBeNil)
				_, captured := program.results["capture"]
				So(captured, ShouldBeFalse)
				So(program.carriedPayload(), ShouldBeFalse)
			}
		})
	})
}

/* The archive and live programs use the existing signal definitions. */
func TestProgramExecuteSignals(t *testing.T) {
	Convey("Given the training signal JSON and actual channel-tagged records", t, func() {
		ctx := context.Background()
		replay := func(repository DefinitionRepository) []float64 {
			program, err := Compile(trainingSignals(t, repository), nil, repository)
			So(err, ShouldBeNil)
			defer program.Release()
			records := program.NodeMap["records"]
			spreads := make([]float64, 0, 3)
			for _, payload := range []string{
				`{"channel":"ticker","data":[{"symbol":"BTC/USD","last":101,"bid":100,"ask":102},{"symbol":"ETH/USD","last":201,"bid":200,"ask":203}]}`,
				`{"channel":"ticker","data":[{"symbol":"BTC/USD","last":106,"bid":104,"ask":108}]}`,
				"",
			} {
				_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
				So(err, ShouldBeNil)
				args, err := data.NewIterate_write_Params(segment)
				So(err, ShouldBeNil)
				So(args.SetPath("data"), ShouldBeNil)
				args.SetEnvelope(true)
				if payload != "" {
					input, err := args.NewData(1)
					So(err, ShouldBeNil)
					So(input.Set(0, []byte(payload)), ShouldBeNil)
				}
				So(program.Execute(ctx, map[NodeID]capnp.Struct{records: capnp.Struct(args)}), ShouldBeNil)
				value, err := program.Float64Result("definition-liquidity_ticker__spread", "out")
				So(err, ShouldBeNil)
				spreads = append(spreads, value)
			}
			return spreads
		}
		Convey("Then every record reaches the existing liquidity graph", func() {
			So(replay(DefaultRepository()), ShouldResemble, []float64{2, 3, 4})
		})
		Convey("When one training connection is changed from ask to last in JSON", func() {
			repository := NewRepository()
			graph, err := repository.Load("training")
			So(err, ShouldBeNil)
			signal := graph.Nodes["definition-liquidity_ticker"]
			signal.Connections.Inputs["spread.a"] = []ConnectionTarget{{NodeID: "grid", PortName: "values_1"}}
			grid := graph.Nodes["grid"]
			for port, targets := range grid.Connections.Outputs {
				retained := make([]ConnectionTarget, 0, len(targets))
				for _, target := range targets {
					if target.NodeID == signal.ID && target.PortName == "spread.a" {
						continue
					}
					retained = append(retained, target)
				}
				grid.Connections.Outputs[port] = retained
			}
			grid.Connections.Outputs["values_1"] = append(grid.Connections.Outputs["values_1"], ConnectionTarget{NodeID: signal.ID, PortName: "spread.a"})
			encoded, err := json.Marshal(graph)
			So(err, ShouldBeNil)
			So(repository.Save("training", encoded), ShouldBeNil)
			So(replay(repository), ShouldResemble, []float64{1, 1, 2})
		})
	})
}

func BenchmarkProgramExecuteSignals(b *testing.B) {
	program, err := Compile(trainingSignals(b, DefaultRepository()), nil, DefaultRepository())
	if err != nil {
		b.Fatal(err)
	}
	defer program.Release()
	_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		b.Fatal(err)
	}
	args, err := data.NewIterate_write_Params(segment)
	if err != nil {
		b.Fatal(err)
	}
	args.SetEnvelope(true)
	if err := args.SetPath("data"); err != nil {
		b.Fatal(err)
	}
	payload, err := args.NewData(1)
	if err != nil {
		b.Fatal(err)
	}
	if err := payload.Set(0, []byte(`{"channel":"ticker","data":[{"symbol":"BTC/USD","last":100,"bid":99,"ask":101,"bid_qty":2,"ask_qty":3}]}`)); err != nil {
		b.Fatal(err)
	}
	inputs := map[NodeID]capnp.Struct{program.NodeMap["records"]: capnp.Struct(args)}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := program.Execute(context.Background(), inputs); err != nil {
			b.Fatal(err)
		}
	}
}

/* trainingSignals keeps the real training computation and injects its I/O boundary. */
func trainingSignals(t testing.TB, repository DefinitionRepository) Graph {
	t.Helper()
	graph, err := repository.Load("training")
	if err != nil {
		t.Fatal(err)
	}
	for id := range graph.Nodes {
		if id != "grid" && !strings.HasPrefix(id, "definition-") {
			delete(graph.Nodes, id)
		}
	}
	graph.Nodes["records"] = Node{ID: "records", Type: "data.Iterate", Connections: Connections{
		Outputs: map[string][]ConnectionTarget{"out": {{NodeID: "grid", PortName: "data"}}},
	}}
	grid := graph.Nodes["grid"]
	grid.Connections.Inputs["data"] = []ConnectionTarget{{NodeID: "records", PortName: "out"}}
	graph.Nodes["grid"] = grid
	return graph
}

/* TestProgramExecuteArchive proves the shipped graph performs each archive operation. */
func TestProgramExecuteArchive(t *testing.T) {
	Convey("Given a real Iceberg snapshot served through a local catalog HTTP boundary", t, func() {
		ctx := context.Background()
		directory := t.TempDir()
		database, err := sql.Open("sqlite3", filepath.Join(directory, "catalog.db"))
		So(err, ShouldBeNil)
		defer func() {
			if err := database.Close(); err != nil {
				t.Error(err)
			}
		}()
		catalog, err := sqlcat.NewCatalog("fixture", database, sqlcat.SQLite, iceberg.Properties{"warehouse": "file://" + directory})
		So(err, ShouldBeNil)
		So(catalog.CreateNamespace(ctx, []string{"fixture"}, nil), ShouldBeNil)
		table, err := catalog.CreateTable(ctx, []string{"fixture", "capture"}, iceberg.NewSchema(0,
			iceberg.NestedField{ID: 1, Name: "sequence", Type: iceberg.PrimitiveTypes.Int64, Required: true},
			iceberg.NestedField{ID: 2, Name: "payload", Type: iceberg.PrimitiveTypes.Binary, Required: true},
		))
		So(err, ShouldBeNil)
		arrowSchema, err := icetable.SchemaToArrowSchema(table.Schema(), nil, true, false)
		So(err, ShouldBeNil)
		builder := array.NewRecordBuilder(memory.DefaultAllocator, arrowSchema)
		defer builder.Release()
		for index := int64(0); index < 3; index++ {
			builder.Field(0).(*array.Int64Builder).Append(9007199254740993 + index)
			builder.Field(1).(*array.BinaryBuilder).Append([]byte{byte(index), 0, 255})
		}
		batch := builder.NewRecordBatch()
		defer batch.Release()
		reader, err := array.NewRecordReader(arrowSchema, []arrow.RecordBatch{batch})
		So(err, ShouldBeNil)
		defer reader.Release()
		table, err = table.Append(ctx, reader, nil)
		So(err, ShouldBeNil)
		var configCalls, tableCalls atomic.Int64
		endpoint := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			response.Header().Set("Content-Type", "application/json")
			if request.URL.Path == "/config" {
				configCalls.Add(1)
				if err := json.NewEncoder(response).Encode(map[string]any{"defaults": map[string]string{}, "overrides": map[string]string{}}); err != nil {
					t.Error(err)
				}
				return
			}
			tableCalls.Add(1)
			if err := json.NewEncoder(response).Encode(map[string]any{"metadata": table.Metadata(), "config": map[string]string{}}); err != nil {
				t.Error(err)
			}
		}))
		defer endpoint.Close()
		repository := DefaultRepository()
		graph, err := repository.Load("archive_scan")
		So(err, ShouldBeNil)
		declaration, err := json.Marshal(map[string]any{"catalogUrl": endpoint.URL + "/config", "tableUrl": endpoint.URL + "/table", "properties": map[string]string{}})
		So(err, ShouldBeNil)
		value, err := json.Marshal(map[string]string{"value": string(declaration)})
		So(err, ShouldBeNil)
		entry := graph.Nodes["input"]
		entry.InputData["through"] = value
		graph.Nodes["input"] = entry
		program, err := Compile(graph, nil, repository)
		So(err, ShouldBeNil)
		defer program.Release()
		for index := int64(0); index < 3; index++ {
			So(program.Execute(ctx, nil), ShouldBeNil)
			result, found := program.Result("project")
			So(found, ShouldBeTrue)
			raw, err := data.Arrow_done_Results(result).Out()
			So(err, ShouldBeNil)
			var row struct {
				Sequence int64
				Payload  []byte
			}
			So(json.Unmarshal(raw, &row), ShouldBeNil)
			So(row.Sequence, ShouldEqual, 9007199254740993+index)
			So(row.Payload, ShouldResemble, []byte{byte(index), 0, 255})
		}
		So(program.Execute(ctx, nil), ShouldBeNil)
		result, found := program.Result("scan")
		So(found, ShouldBeTrue)
		So(tables.Scanned(result).Exhausted(), ShouldBeTrue)
		So(configCalls.Load(), ShouldEqual, 1)
		So(tableCalls.Load(), ShouldEqual, 1)
	})
}

/* TestProgramExecuteGrading exercises the actual JSON truth branches at tape boundaries. */
func TestProgramExecuteGrading(t *testing.T) {
	Convey("Given the shipped grading graph and a resolved rising excursion", t, func() {
		repository := DefaultRepository()
		graph, err := repository.Load("training_grade")
		So(err, ShouldBeNil)
		program, err := Compile(graph, nil, repository)
		So(err, ShouldBeNil)
		defer program.Release()
		for _, fixture := range []struct {
			name              string
			cursor, direction int
			missingA          bool
			action            string
			holding           bool
		}{
			{"before sampled A", 0, 1, false, "", false},
			{"precursor", 1, 1, false, "WAIT", false},
			{"ignition", 2, 1, false, "ENTER", false},
			{"holding leg", 3, 1, false, "WAIT", true},
			{"extremum", 4, 1, false, "EXIT", true},
			{"after extremum", 5, 1, false, "", false},
			{"missing A is not replaced", 2, 1, true, "", false},
			{"down leg exits inventory", 2, -1, false, "EXIT", true},
			{"down leg stays flat", 3, -1, false, "WAIT", false},
		} {
			Convey(fixture.name, func() {
				// All record offsets share one exact sequence above the float64 integer range.
				cursor := func(record int) map[string]any {
					return map[string]any{"sequence": int64(9007199254740993), "record": record}
				}
				event := map[string]any{"precursor": cursor(0), "b": cursor(2), "c": cursor(4), "d": cursor(5), "excursion": fixture.direction}
				if !fixture.missingA {
					event["a"] = cursor(1)
				}
				payload, err := json.Marshal(map[string]any{"cursor": cursor(fixture.cursor), "event": event})
				So(err, ShouldBeNil)
				_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
				So(err, ShouldBeNil)
				args, err := transport.NewFan_write_Params(segment)
				So(err, ShouldBeNil)
				So(args.SetData(payload), ShouldBeNil)
				So(program.Execute(context.Background(), map[NodeID]capnp.Struct{program.NodeMap["input"]: capnp.Struct(args)}), ShouldBeNil)
				result, found := program.Result("labels")
				So(found, ShouldBeTrue)
				labels := data.Iterate_done_Results(result)
				if fixture.action == "" {
					So(labels.Found(), ShouldBeFalse)
					return
				}
				So(labels.Found(), ShouldBeTrue)
				raw, err := labels.Out()
				So(err, ShouldBeNil)
				var label struct {
					Truth struct {
						Action  string
						Holding bool
					}
					Cursor struct{ Sequence int64 }
				}
				So(json.Unmarshal(raw, &label), ShouldBeNil)
				So(label.Truth.Action, ShouldEqual, fixture.action)
				So(label.Truth.Holding, ShouldEqual, fixture.holding)
				So(label.Cursor.Sequence, ShouldEqual, 9007199254740993)
				So(labels.Pending(), ShouldEqual, 0)
			})
		}
	})
}

/* BenchmarkProgramExecuteGrading measures the complete compiled JSON classifier. */
func BenchmarkProgramExecuteGrading(b *testing.B) {
	repository := DefaultRepository()
	graph, err := repository.Load("training_grade")
	if err != nil {
		b.Fatal(err)
	}
	program, err := Compile(graph, nil, repository)
	if err != nil {
		b.Fatal(err)
	}
	defer program.Release()
	_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		b.Fatal(err)
	}
	args, err := transport.NewFan_write_Params(segment)
	if err != nil {
		b.Fatal(err)
	}
	if err := args.SetData([]byte(`{"cursor":{"sequence":2,"record":0},"event":{"a":{"sequence":1,"record":0},"b":{"sequence":2,"record":0},"c":{"sequence":4,"record":0},"d":{"sequence":5,"record":0},"excursion":1}}`)); err != nil {
		b.Fatal(err)
	}
	input := map[NodeID]capnp.Struct{program.NodeMap["input"]: capnp.Struct(args)}
	b.ReportAllocs()
	for b.Loop() {
		if err := program.Execute(context.Background(), input); err != nil {
			b.Fatal(err)
		}
	}
}

func TestProgramExecuteRetained(t *testing.T) {
	Convey("Given a keyed store in an explicit feedback graph", t, func() {
		program, err := CompileJSON([]byte(`{"nodes":{
"input":{"id":"input","type":"transport.Fan","connections":{"outputs":{"out":[{"nodeId":"key","portName":"data"},{"nodeId":"value","portName":"data"}]}}},
"key":{"id":"key","type":"data.Extract","inputData":{"path":{"value":"key"},"encoding":{"value":"text"}},"connections":{"inputs":{"data":[{"nodeId":"input","portName":"out"}]},"outputs":{"text":[{"nodeId":"memory","portName":"key"}]}}},
"value":{"id":"value","type":"data.Extract","inputData":{"path":{"value":"value"}},"connections":{"inputs":{"data":[{"nodeId":"input","portName":"out"}]},"outputs":{"out":[{"nodeId":"replace","portName":"value"}]}}},
"memory":{"id":"memory","type":"store.Radix","connections":{"inputs":{"key":[{"nodeId":"key","portName":"text"}],"value":[{"nodeId":"replace","portName":"out"}]},"outputs":{"out_0":[{"nodeId":"replace","portName":"data"}]}}},
"replace":{"id":"replace","type":"data.Insert","inputData":{"path":{"value":"count"}},"connections":{"inputs":{"data":[{"nodeId":"memory","portName":"out_0"}],"value":[{"nodeId":"value","portName":"out"}]},"outputs":{"out":[{"nodeId":"memory","portName":"value"}]}}}
}}`), nil, nil)
		So(err, ShouldBeNil)
		defer program.Release()

		Convey("Then each key reads its own prior state and commits the feedback exactly once", func() {
			observations := []struct{ input, previous string }{
				{`{"key":"BTC","value":7}`, ""},
				{`{"key":"ETH","value":3}`, ""},
				{`{"key":"BTC","value":11}`, `{"count":7}`},
				{`{"key":"ETH","value":5}`, `{"count":3}`},
				{`{"key":"BTC","value":13}`, `{"count":11}`},
			}
			for _, observation := range observations {
				_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
				So(err, ShouldBeNil)
				params, err := transport.NewFan_write_Params(segment)
				So(err, ShouldBeNil)
				So(params.SetData([]byte(observation.input)), ShouldBeNil)
				So(program.Execute(context.Background(), map[NodeID]capnp.Struct{program.NodeMap["input"]: capnp.Struct(params)}), ShouldBeNil)
				result, found := program.Result("memory")
				So(found, ShouldBeTrue)
				previous, err := result.Ptr(0)
				So(err, ShouldBeNil)
				slot, err := capnp.PointerList(previous.List()).At(0)
				So(err, ShouldBeNil)
				So(string(slot.Data()), ShouldEqual, observation.previous)
				_, updated := program.Result("replace")
				So(updated, ShouldBeTrue)
			}
		})
	})
}

func TestProgramExecuteReinforcement(t *testing.T) {
	Convey("Given the shipping prediction, grading and reinforcement graph", t, func() {
		program, err := CompileFile("../../manifest/training_reinforce.json", nil, NewRepository())
		So(err, ShouldBeNil)
		defer program.Release()
		ignition := 3
		offset := 0
		holding := false
		settled := true
		execute := func(identity string, cursor int, vocabulary string) error {
			input := map[string]any{
				"capture": map[string]string{"session": identity, "endpoint": "spot"},
				"holding": holding,
				"settled": map[string]any{"vocabulary": vocabulary, "tokens": []string{"A", "B"}},
				"cursor":  map[string]int{"sequence": offset + cursor, "record": 0},
				"event": map[string]any{
					"a": map[string]int{"sequence": offset + 1, "record": 0},
					"b": map[string]int{"sequence": offset + ignition, "record": 0},
					"c": map[string]int{"sequence": offset + 5, "record": 0},
					"d": map[string]int{"sequence": offset + 7, "record": 0}, "excursion": 1,
				},
			}
			if !settled {
				delete(input, "settled")
			}

			payload, err := json.Marshal(input)
			So(err, ShouldBeNil)
			_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
			So(err, ShouldBeNil)
			params, err := transport.NewFan_write_Params(segment)
			So(err, ShouldBeNil)
			So(params.SetData(payload), ShouldBeNil)
			return program.Execute(context.Background(), map[NodeID]capnp.Struct{program.NodeMap["input"]: capnp.Struct(params)})
		}
		prediction := func(expected string, total uint64) {
			result, found := program.Result("prediction")
			So(found, ShouldBeTrue)
			class, err := cognition.Attractor_done_Results(result).Class()
			So(err, ShouldBeNil)
			So(string(class), ShouldEqual, expected)
			counts, found := program.Result("observed")
			So(found, ShouldBeTrue)
			So(statistic.Tally_done_Results(counts).Total(), ShouldEqual, total)
		}

		Convey("Then truth bootstraps an empty trie and incorrect predictions still learn", func() {
			So(execute("entry-1", 3, "v1"), ShouldBeNil)
			prediction("", 0)
			So(execute("wait-1", 2, "v1"), ShouldBeNil)
			prediction("ENTER", 1)
			So(execute("wait-1", 2, "v1"), ShouldBeNil)
			prediction("", 2)
			_, updated := program.Result("reinforce")
			So(updated, ShouldBeFalse)
			So(execute("wait-2", 2, "v1"), ShouldBeNil)
			prediction("", 2)
			So(execute("wait-2", 2, "v1"), ShouldBeNil)
			prediction("WAIT", 3)

			Convey("And another vocabulary has no borrowed evidence", func() {
				So(execute("entry-2", 3, "v2"), ShouldBeNil)
				prediction("", 0)
			})

			Convey("And adjacent capture identities above Float64 precision remain distinct", func() {
				offset = 1 << 53
				So(execute("large", 3, "exact"), ShouldBeNil)
				prediction("", 0)
				offset++
				So(execute("large", 3, "exact"), ShouldBeNil)
				prediction("ENTER", 1)
				offset--
				So(execute("large", 3, "exact"), ShouldBeNil)
				prediction("ENTER", 2)
				_, updated := program.Result("reinforce")
				So(updated, ShouldBeFalse)
			})

			Convey("And held inventory has separate evidence", func() {
				holding = true
				So(execute("held", 4, "v1"), ShouldBeNil)
				prediction("", 0)
				So(execute("held", 4, "v1"), ShouldBeNil)
				prediction("WAIT", 1)
			})

			Convey("And an unsettled observation cannot reach reinforcement", func() {
				settled = false
				So(execute("unsettled", 3, "v1"), ShouldBeNil)
				_, updated := program.Result("reinforce")
				So(updated, ShouldBeFalse)
				result, read := program.Result("memory")
				So(read, ShouldBeTrue)
				pointer, err := result.Ptr(0)
				So(err, ShouldBeNil)
				So(pointer.List().Len(), ShouldEqual, 0)
			})

			Convey("And a contradictory label for the same example is rejected", func() {
				ignition = 2
				err := execute("wait-2", 2, "v1")
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "conflicting value at unique path")
			})
		})
	})
}

func BenchmarkProgramExecuteReinforcement(b *testing.B) {
	errnie.Apply(&errnie.Config{Level: "error"})
	defer errnie.Apply(&errnie.Config{Level: "info"})
	program, err := CompileFile("../../manifest/training_reinforce.json", nil, NewRepository())

	if err != nil {
		b.Fatal(err)
	}

	defer program.Release()
	b.ReportAllocs()
	b.ResetTimer()
	observation := 0

	for b.Loop() {
		observation++
		payload, err := json.Marshal(map[string]any{
			"capture": map[string]string{"session": "benchmark", "endpoint": "spot"},
			"holding": false,
			"settled": map[string]any{"vocabulary": "v1", "tokens": []string{"region-A", "region-B"}},
			"cursor":  map[string]int{"sequence": observation*4 + 1, "record": 0},
			"event": map[string]any{
				"a": map[string]int{"sequence": observation * 4, "record": 0},
				"b": map[string]int{"sequence": observation*4 + 1, "record": 0},
				"c": map[string]int{"sequence": observation*4 + 2, "record": 0},
				"d": map[string]int{"sequence": observation*4 + 3, "record": 0}, "excursion": 1,
			},
		})

		if err != nil {
			b.Fatal(err)
		}

		_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

		if err != nil {
			b.Fatal(err)
		}

		params, err := transport.NewFan_write_Params(segment)

		if err != nil {
			b.Fatal(err)
		}

		if err := params.SetData(payload); err != nil {
			b.Fatal(err)
		}

		if err := program.Execute(context.Background(), map[NodeID]capnp.Struct{program.NodeMap["input"]: capnp.Struct(params)}); err != nil {
			b.Fatal(err)
		}

		if _, updated := program.Result("reinforce"); !updated {
			b.Fatal("resolved example was not reinforced")
		}
	}
}

/* TestProgramExecuteRecords keeps ingress identity attached while collections queue. */
func TestProgramExecuteRecords(t *testing.T) {
	Convey("Given the shipping archive-record projection", t, func() {
		program, err := CompileFile("../../manifest/archive_records.json", nil, NewRepository())
		So(err, ShouldBeNil)
		defer program.Release()
		for step := range 4 {
			inputs := make(map[NodeID]capnp.Struct)
			if step < 2 {
				row, err := json.Marshal(map[string]any{"capture_session": []string{"first", "second"}[step], "capture_sequence": int64(9007199254740993) + int64(step), "endpoint": []string{"spot", "futures"}[step], "received_time": "2026-09-23T00:00:00.123456789Z"})
				So(err, ShouldBeNil)
				frame, err := json.Marshal(map[string]any{"channel": "ticker", "data": []map[string]int{{"last": step*2 + 1}, {"last": step*2 + 2}}})
				So(err, ShouldBeNil)
				for name, payload := range map[string][]byte{"row": row, "frame": frame} {
					_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
					So(err, ShouldBeNil)
					params, err := transport.NewFan_write_Params(segment)
					So(err, ShouldBeNil)
					So(params.SetData(payload), ShouldBeNil)
					inputs[program.NodeMap[name]] = capnp.Struct(params)
				}
			}
			So(program.Execute(context.Background(), inputs), ShouldBeNil)
			result, found := program.Result("records")
			So(found, ShouldBeTrue)
			raw, err := data.Iterate_done_Results(result).Out()
			So(err, ShouldBeNil)
			var record struct {
				Capture struct{ Session, Endpoint, ReceivedAt string }
				Cursor  struct {
					Sequence int64
					Record   int
				}
				Market struct {
					Channel string
					Data    struct{ Last int }
				}
			}
			So(json.Unmarshal(raw, &record), ShouldBeNil)
			So(record.Cursor.Sequence == int64(9007199254740993)+int64(step/2), ShouldBeTrue)
			So(record.Cursor.Record, ShouldEqual, step%2)
			So(record.Capture.Session, ShouldEqual, []string{"first", "second"}[step/2])
			So(record.Capture.Endpoint, ShouldEqual, []string{"spot", "futures"}[step/2])
			So(record.Capture.ReceivedAt, ShouldEqual, "2026-09-23T00:00:00.123456789Z")
			So(record.Market.Data.Last, ShouldEqual, step+1)
		}
	})
}

/* TestProgramExecuteFragment verifies the causal/truth separation on the actual JSON graph. */
func TestProgramExecuteFragment(t *testing.T) {
	Convey("Given an archived fragment with A before B before C and confirmation at D", t, func() {
		program, err := CompileFile("../../manifest/training_fragment.json", nil, NewRepository())
		So(err, ShouldBeNil)
		defer program.Release()
		sequence := int64(9007199254740993)
		confirmation := sequence + 3
		excursion := 1.0
		session, endpoint, symbol := "capture", "spot", "BTC/USD"
		execute := func(at int64, record int) (string, bool) {
			cursor := func(offset int64, position int) map[string]any {
				return map[string]any{"sequence": sequence + offset, "record": position}
			}
			observation := map[string]any{"capture": map[string]string{"session": session, "endpoint": endpoint}, "cursor": map[string]any{"sequence": at, "record": record}, "market": map[string]any{"channel": "ticker", "data": map[string]any{"symbol": symbol, "last": 100}}}
			fragment := map[string]any{"capture": map[string]string{"session": "capture", "endpoint": "spot"}, "event": map[string]any{"symbol": "BTC/USD", "a": cursor(0, 1), "b": cursor(1, 0), "c": cursor(2, 0), "d": map[string]any{"sequence": confirmation, "record": 0}, "excursion": excursion}}
			inputs := make(map[NodeID]capnp.Struct)
			for name, document := range map[string]any{"record": observation, "fragment": fragment} {
				payload, err := json.Marshal(document)
				So(err, ShouldBeNil)
				_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
				So(err, ShouldBeNil)
				params, err := transport.NewFan_write_Params(segment)
				So(err, ShouldBeNil)
				So(params.SetData(payload), ShouldBeNil)
				inputs[program.NodeMap[name]] = capnp.Struct(params)
			}
			So(program.Execute(context.Background(), inputs), ShouldBeNil)
			truth, graded := program.Result("truth")
			if graded {
				payload, err := data.Insert_done_Results(truth).Out()
				So(err, ShouldBeNil)
				So(string(payload), ShouldContainSubstring, `"event"`)
			}
			result, found := program.Result("observation")
			if !found {
				return "", graded
			}
			payload, err := transport.Fan_done_Results(result).Out()
			So(err, ShouldBeNil)
			So(string(payload), ShouldNotContainSubstring, `"event"`)
			So(string(payload), ShouldNotContainSubstring, `"fragment"`)
			return string(payload), graded
		}
		Convey("Only A through C enter the predictor, including record-index boundaries", func() {
			for _, test := range []struct {
				offset   int64
				record   int
				accepted bool
			}{{-1, 1, false}, {0, 0, false}, {0, 1, true}, {1, 0, true}, {2, 0, true}, {2, 1, false}, {3, 0, false}} {
				output, graded := execute(sequence+test.offset, test.record)
				So(output != "", ShouldEqual, test.accepted)
				So(graded, ShouldEqual, test.accepted)
			}
		})
		Convey("Changing future confirmation or outcome leaves precursor input unchanged", func() {
			original, graded := execute(sequence, 1)
			So(graded, ShouldBeTrue)
			confirmation += 10
			excursion = -2
			changed, graded := execute(sequence, 1)
			So(graded, ShouldBeTrue)
			var originalFields, changedFields map[string]any
			So(json.Unmarshal([]byte(original), &originalFields), ShouldBeNil)
			So(json.Unmarshal([]byte(changed), &changedFields), ShouldBeNil)
			So(changedFields, ShouldResemble, originalFields)
		})
		Convey("Other feeds contribute observations but cannot receive another instrument's truth", func() {
			endpoint = "futures"
			output, graded := execute(sequence, 1)
			So(output, ShouldNotBeEmpty)
			So(graded, ShouldBeFalse)
			endpoint = "spot"
			symbol = "ETH/USD"
			output, graded = execute(sequence, 1)
			So(output, ShouldNotBeEmpty)
			So(graded, ShouldBeFalse)
			session = "another capture"
			output, graded = execute(sequence, 1)
			So(output, ShouldBeEmpty)
			So(graded, ShouldBeFalse)
		})
		Convey("Unconfirmed event ordering cannot emit a training observation", func() {
			confirmation = sequence + 1
			output, graded := execute(sequence, 1)
			So(output, ShouldBeEmpty)
			So(graded, ShouldBeFalse)
		})
	})
}

/* TestProgramExecuteReplay traverses records once, with future events confined to truth. */
func TestProgramExecuteReplay(t *testing.T) {
	Convey("Given overlapping confirmed fragments and a captured market tape", t, func() {
		program, err := CompileFile("../../manifest/training_replay.json", nil, NewRepository())
		So(err, ShouldBeNil)
		defer program.Release()
		execute := func(record, event []byte, ready bool) {
			inputs := make(map[NodeID]capnp.Struct)
			arrivals := map[string][]byte{"record": record, "event": event}
			if ready {
				arrivals["ready"] = []byte(`{"ready":1}`)
			}
			for name, payload := range arrivals {
				if len(payload) == 0 {
					continue
				}
				_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
				So(err, ShouldBeNil)
				args, err := transport.NewFan_write_Params(segment)
				So(err, ShouldBeNil)
				So(args.SetData(payload), ShouldBeNil)
				inputs[program.NodeMap[name]] = capnp.Struct(args)
			}
			So(program.Execute(context.Background(), inputs), ShouldBeNil)
		}
		for sequence := 0; sequence < 6; sequence++ {
			record, err := json.Marshal(map[string]any{"capture": map[string]string{"session": "capture", "endpoint": "spot"}, "cursor": map[string]int{"sequence": sequence, "record": 0}, "market": map[string]any{"channel": "ticker", "data": map[string]any{"symbol": "BTC/USD", "last": 100 + sequence}}})
			So(err, ShouldBeNil)
			var event []byte
			if sequence < 2 {
				cursor := func(offset int) map[string]int { return map[string]int{"sequence": offset + sequence, "record": 0} }
				event, err = json.Marshal(map[string]any{"capture": map[string]string{"session": "capture", "endpoint": "spot"}, "event": map[string]any{"symbol": "BTC/USD", "a": cursor(0), "b": cursor(2), "c": cursor(4), "d": cursor(5), "excursion": 1}})
				So(err, ShouldBeNil)
			}
			execute(record, event, false)
			_, observed := program.Result("observation")
			So(observed, ShouldBeFalse)
			_, graded := program.Result("fragment__truth")
			So(graded, ShouldBeFalse)
		}
		observations := make([]int, 0, 6)
		grades := 0
		// Each of six records visits two fragment slots and one end-of-list slot.
		for iteration := range 6*3 + 1 {
			if iteration%3 == 1 {
				execute(nil, nil, false)
				_, observed := program.Result("observation")
				So(observed, ShouldBeFalse)
				_, graded := program.Result("fragment__truth")
				So(graded, ShouldBeFalse)
			}
			execute(nil, nil, true)
			if result, observed := program.Result("observation"); observed {
				payload, err := data.Extracted(result).Json()
				So(err, ShouldBeNil)
				var observation struct{ Cursor struct{ Sequence int } }
				So(json.Unmarshal(payload, &observation), ShouldBeNil)
				observations = append(observations, observation.Cursor.Sequence)
				So(string(payload), ShouldNotContainSubstring, `"event"`)
			}
			if _, graded := program.Result("fragment__truth"); graded {
				grades++
			}
		}
		So(observations, ShouldResemble, []int{0, 1, 2, 3, 4, 5})
		So(grades, ShouldEqual, 10)
		result, found := program.Result("frames")
		So(found, ShouldBeTrue)
		So(store.Item(result).Found(), ShouldBeFalse)
	})
}

/* BenchmarkProgramExecuteReplay measures the compiled traversal, including fragment predicates. */
func BenchmarkProgramExecuteReplay(b *testing.B) {
	errnie.Apply(&errnie.Config{Level: "error"})
	defer errnie.Apply(&errnie.Config{Level: "info"})
	program, err := CompileFile("../../manifest/training_replay.json", nil, NewRepository())
	if err != nil {
		b.Fatal(err)
	}
	defer program.Release()
	makeInput := func(node, payload string) map[NodeID]capnp.Struct {
		_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		if err != nil {
			b.Fatal(err)
		}
		args, err := transport.NewFan_write_Params(segment)
		if err != nil {
			b.Fatal(err)
		}
		if err := args.SetData([]byte(payload)); err != nil {
			b.Fatal(err)
		}
		return map[NodeID]capnp.Struct{program.NodeMap[node]: capnp.Struct(args)}
	}
	// The fixed two fragments are fixture topology; timed record count grows with b.N.
	for _, payload := range []string{
		`{"capture":{"session":"capture","endpoint":"spot"},"event":{"symbol":"BTC/USD","a":{"sequence":0,"record":0},"b":{"sequence":1,"record":0},"c":{"sequence":2,"record":0},"d":{"sequence":3,"record":0},"excursion":1}}`,
		`{"capture":{"session":"capture","endpoint":"spot"},"event":{"symbol":"BTC/USD","a":{"sequence":1,"record":0},"b":{"sequence":2,"record":0},"c":{"sequence":3,"record":0},"d":{"sequence":4,"record":0},"excursion":-1}}`,
	} {
		if err := program.Execute(context.Background(), makeInput("event", payload)); err != nil {
			b.Fatal(err)
		}
	}
	for index := 0; index < b.N; index++ {
		payload, err := json.Marshal(map[string]any{"capture": map[string]string{"session": "capture", "endpoint": "spot"}, "cursor": map[string]int{"sequence": index, "record": 0}, "market": map[string]any{"channel": "ticker", "data": map[string]any{"symbol": "BTC/USD", "last": 100 + index}}})
		if err != nil {
			b.Fatal(err)
		}
		if err := program.Execute(context.Background(), makeInput("record", string(payload))); err != nil {
			b.Fatal(err)
		}
	}
	ready := makeInput("ready", `{"ready":1}`)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		for range 3 {
			if err := program.Execute(context.Background(), ready); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func TestProgramExecutePair(t *testing.T) {
	Convey("Given the graph for paired, resolved reaction intervals", t, func() {
		program, err := CompileFile("../../manifest/training_pair.json", nil, NewRepository())
		So(err, ShouldBeNil)
		defer program.Release()
		scope := "capture-A:MOVE/USD"
		levels := map[string]map[int][2]float64{}
		futureLabel := "ENTER"
		execute := func(index int, left, right any) error {
			if levels[scope] == nil {
				levels[scope] = map[int][2]float64{}
			}
			pair := [2]float64{}
			if index >= 0 {
				pair = levels[scope][index-1]
			}
			if value, valid := left.(float64); valid {
				pair[0] += value
			}
			if value, valid := left.(int); valid {
				pair[0] += float64(value)
			}
			if value, valid := right.(float64); valid {
				pair[1] += value
			}
			if value, valid := right.(int); valid {
				pair[1] += float64(value)
			}
			levels[scope][index] = pair
			var observedRight any = pair[1]
			if right == nil {
				observedRight = nil
			}
			payload, err := json.Marshal(map[string]any{
				"scope":  scope,
				"pair":   map[string]any{"left": []int{0, 0}, "right": []int{1, 0}},
				"cursor": map[string]int{"sequence": index + 1, "record": 0},
				"left":   pair[0], "right": observedRight,
				"future": futureLabel,
			})
			So(err, ShouldBeNil)
			_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
			So(err, ShouldBeNil)
			params, err := transport.NewFan_write_Params(segment)
			So(err, ShouldBeNil)
			So(params.SetData(payload), ShouldBeNil)
			return program.Execute(context.Background(), map[NodeID]capnp.Struct{program.NodeMap["input"]: capnp.Struct(params)})
		}
		So(execute(-1, 0, 0), ShouldBeNil)
		read := func(name string) map[string]float64 {
			result, found := program.Result("step__" + name)
			So(found, ShouldBeTrue)
			payload, err := data.Insert_done_Results(result).Out()
			So(err, ShouldBeNil)
			var document map[string]float64
			So(json.Unmarshal(payload, &document), ShouldBeNil)
			return document
		}

		Convey("Consistently inverted reactions attract and scale does not change their match", func() {
			for index, value := range []float64{1, -2, 4, -3} {
				So(execute(index, value, -10*value), ShouldBeNil)
				if index == 0 {
					_, produced := program.Result("step__evidence")
					So(produced, ShouldBeFalse)
				}
			}
			evidence := read("evidence")
			So(evidence["support"], ShouldEqual, 4)
			So(evidence["relation"], ShouldEqual, 1)
			So(evidence["magnitude"], ShouldAlmostEqual, 1)
			So(evidence["sympathy"], ShouldAlmostEqual, 2)

			Convey("Repeating the last interval does not change sufficient statistics", func() {
				futureLabel = "EXIT"
				So(execute(3, -3, 30), ShouldBeNil)
				_, produced := program.Result("step__evidence")
				So(produced, ShouldBeFalse)
				So(execute(4, 2, -20), ShouldBeNil)
				So(read("evidence")["support"], ShouldEqual, 5)
			})

			Convey("A conflicting replacement for an observed interval is rejected", func() {
				err := execute(3, -3, 29)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "conflicting value at unique path")
			})

			Convey("Another session or instrument starts with no borrowed pair evidence", func() {
				scope = "capture-B:OTHER/USD"
				So(execute(-1, 0, 0), ShouldBeNil)
				So(execute(0, 1, 1), ShouldBeNil)
				_, produced := program.Result("step__evidence")
				So(produced, ShouldBeFalse)
			})
		})

		Convey("Alternating relationships repel even when magnitudes match", func() {
			for index, right := range []float64{1, 1, -1, -1} {
				left := []float64{1, -1, 1, -1}[index]
				So(execute(index, left, right), ShouldBeNil)
			}
			evidence := read("evidence")
			So(evidence["relation"], ShouldAlmostEqual, -1.0/3)
			So(evidence["sympathy"], ShouldAlmostEqual, -2.0/3)
		})

		Convey("Observed nonresponse repels without inventing a magnitude scale", func() {
			So(execute(0, 2, 0), ShouldBeNil)
			So(execute(1, -3, 0), ShouldBeNil)
			So(read("co_movement")["relation"], ShouldEqual, -1)
			_, produced := program.Result("step__evidence")
			So(produced, ShouldBeFalse)
		})

		Convey("Joint quiet intervals add no evidence of a reaction relationship", func() {
			So(execute(0, 0, 0), ShouldBeNil)
			So(execute(1, 1, 1), ShouldBeNil)
			_, produced := program.Result("step__evidence")
			So(produced, ShouldBeFalse)
			So(execute(2, -1, -1), ShouldBeNil)
			So(read("evidence")["support"], ShouldEqual, 2)
		})

		Convey("Missing observations do not become observed nonresponses", func() {
			So(execute(0, 0, nil), ShouldBeNil)
			So(execute(1, 1, 1), ShouldBeNil)
			_, produced := program.Result("step__evidence")
			So(produced, ShouldBeFalse)
			So(execute(2, -1, -1), ShouldBeNil)
			So(read("evidence")["support"], ShouldEqual, 2)
		})

		Convey("New observations cannot move the pair anchor backward", func() {
			So(execute(3, 2, -2), ShouldBeNil)
			err := execute(1, 1, -1)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "observations must follow capture cursor order")
		})

		Convey("Mixed reactions agree with direct enumeration over distinct active intervals", func() {
			reactions := [][2]float64{{1, -2}, {-2, 1}, {0, 0}, {0, 1}, {3, -4}, {1, 0}, {-4, -3}}
			var signs []float64
			nonresponse := 0.0
			for index, reaction := range reactions {
				So(execute(index, reaction[0], reaction[1]), ShouldBeNil)
				if reaction[0] == 0 && reaction[1] == 0 {
					continue
				}
				product := reaction[0] * reaction[1]
				sign := 0.0
				if product > 0 {
					sign = 1
				}
				if product < 0 {
					sign = -1
				}
				if product == 0 {
					nonresponse++
				}
				signs = append(signs, sign)
				if len(signs) < 2 {
					continue
				}
				sum, pairs := 0.0, 0.0
				for first := range signs {
					for second := first + 1; second < len(signs); second++ {
						sum += signs[first] * signs[second]
						pairs++
					}
				}
				evidence := read("evidence")
				So(evidence["support"], ShouldEqual, len(signs))
				So(evidence["relation"], ShouldAlmostEqual, sum/pairs-nonresponse/float64(len(signs)))
			}
		})

		Convey("Different relative magnitudes weaken the additional attraction", func() {
			So(execute(0, 1, 4), ShouldBeNil)
			So(execute(1, -4, -1), ShouldBeNil)
			evidence := read("evidence")
			So(evidence["relation"], ShouldEqual, 1)
			So(evidence["magnitude"], ShouldAlmostEqual, 8.0/17)
			So(evidence["sympathy"], ShouldAlmostEqual, 1+8.0/17)
		})
	})
}

func BenchmarkProgramExecutePair(b *testing.B) {
	errnie.Apply(&errnie.Config{Level: "error"})
	defer errnie.Apply(&errnie.Config{Level: "info"})
	program, err := CompileFile("../../manifest/training_pair.json", nil, NewRepository())

	if err != nil {
		b.Fatal(err)
	}
	defer program.Release()
	b.ReportAllocs()
	b.ResetTimer()
	index := 0

	for b.Loop() {
		index++
		payload, err := json.Marshal(map[string]any{
			"scope":  "benchmark:MOVE/USD",
			"pair":   map[string]any{"left": []int{0, 0}, "right": []int{1, 0}},
			"cursor": map[string]int{"sequence": index + 1, "record": 0},
			"left":   index % 7, "right": -(index % 7),
		})

		if err != nil {
			b.Fatal(err)
		}
		_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

		if err != nil {
			b.Fatal(err)
		}
		params, err := transport.NewFan_write_Params(segment)

		if err != nil {
			b.Fatal(err)
		}

		if err := params.SetData(payload); err != nil {
			b.Fatal(err)
		}

		if err := program.Execute(context.Background(), map[NodeID]capnp.Struct{program.NodeMap["input"]: capnp.Struct(params)}); err != nil {
			b.Fatal(err)
		}
	}
}

func TestProgramExecuteSignalPair(t *testing.T) {
	Convey("Given existing signal outputs connected to the pair graph in one compiled program", t, func() {
		repository := NewRepository()
		graph := trainingSignals(t, repository)
		var connections Graph
		So(json.Unmarshal([]byte(`{"nodes":{
"context":{"id":"context","type":"data.Extract","inputData":{"path":{"value":"context"},"encoding":{"value":"json"}},"connections":{"inputs":{"data":[{"nodeId":"records","portName":"out"}]},"outputs":{"json":[{"nodeId":"pair_left","portName":"data"}]}}},
"pair_left":{"id":"pair_left","type":"data.Insert","inputData":{"path":{"value":"left"}},"connections":{"inputs":{"data":[{"nodeId":"context","portName":"json"}],"value":[{"nodeId":"definition-liquidity_ticker","portName":"spread.out"}]},"outputs":{"out":[{"nodeId":"pair_right","portName":"data"}]}}},
"pair_right":{"id":"pair_right","type":"data.Insert","inputData":{"path":{"value":"right"}},"connections":{"inputs":{"data":[{"nodeId":"pair_left","portName":"out"}],"value":[{"nodeId":"definition-liquidity_ticker","portName":"two_sided_touch_notional.out"}]},"outputs":{"out":[{"nodeId":"pair","portName":"input.data"}]}}},
"pair":{"id":"pair","type":"definition:training_pair","connections":{"inputs":{"input.data":[{"nodeId":"pair_right","portName":"out"}]}}}
}}`), &connections), ShouldBeNil)
		for id, node := range connections.Nodes {
			graph.Nodes[id] = node
		}
		// The fixture connects the existing outputs; it does not change any
		// signal formula or substitute a second calculation for those nodes.
		records := graph.Nodes["records"]
		records.Connections.Outputs["out"] = append(records.Connections.Outputs["out"], ConnectionTarget{NodeID: "context", PortName: "data"})
		signal := graph.Nodes["definition-liquidity_ticker"]
		signal.Connections.Outputs["spread.out"] = []ConnectionTarget{{NodeID: "pair_left", PortName: "value"}}
		signal.Connections.Outputs["two_sided_touch_notional.out"] = []ConnectionTarget{{NodeID: "pair_right", PortName: "value"}}
		program, err := Compile(graph, nil, repository)
		So(err, ShouldBeNil)
		defer program.Release()

		for index, reading := range []struct{ spread, quantity float64 }{{1, 10}, {2, 11}, {1, 10}, {4, 13}} {
			payload, err := json.Marshal(map[string]any{
				"channel": "ticker",
				"data":    []map[string]any{{"symbol": "MOVE/USD", "bid": 100, "ask": 100 + reading.spread, "bid_qty": reading.quantity, "ask_qty": 100}},
				"context": map[string]any{
					"scope":  map[string]string{"session": "signal-replay", "symbol": "MOVE/USD"},
					"pair":   map[string]any{"left": []int{0, 0}, "right": []int{0, 1}},
					"cursor": map[string]uint64{"sequence": 9007199254740993 + uint64(index), "record": 0},
				},
			})
			So(err, ShouldBeNil)
			_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
			So(err, ShouldBeNil)
			params, err := data.NewIterate_write_Params(segment)
			So(err, ShouldBeNil)
			So(params.SetPath("data"), ShouldBeNil)
			params.SetEnvelope(true)
			arrivals, err := params.NewData(1)
			So(err, ShouldBeNil)
			So(arrivals.Set(0, payload), ShouldBeNil)
			So(program.Execute(context.Background(), map[NodeID]capnp.Struct{program.NodeMap["records"]: capnp.Struct(params)}), ShouldBeNil)
		}
		result, found := program.Result("pair__evidence")
		So(found, ShouldBeTrue)
		payload, err := data.Insert_done_Results(result).Out()
		So(err, ShouldBeNil)
		var evidence struct {
			Scope    map[string]string `json:"scope"`
			Interval struct {
				Start struct{ Sequence uint64 } `json:"start"`
				End   struct{ Sequence uint64 } `json:"end"`
			} `json:"interval"`
			Evidence map[string]float64 `json:"evidence"`
		}
		So(json.Unmarshal(payload, &evidence), ShouldBeNil)
		So(evidence.Scope["session"], ShouldEqual, "signal-replay")
		So(evidence.Interval.Start.Sequence == uint64(9007199254740995), ShouldBeTrue)
		So(evidence.Interval.End.Sequence == uint64(9007199254740996), ShouldBeTrue)
		So(evidence.Evidence["support"], ShouldEqual, 3)
		So(evidence.Evidence["sympathy"], ShouldAlmostEqual, 2)
	})
}

func TestProgramExecuteLiveSpot(t *testing.T) {
	Convey("Given the shipping system graph and a real WebSocket venue", t, func() {
		failures := make(chan error, 16)
		subscribed := make(chan string, 16)
		upgrader := gorillaws.Upgrader{}
		venue := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			connection, err := upgrader.Upgrade(writer, request, nil)
			if err != nil {
				failures <- err
				return
			}
			defer func() {
				if err := connection.Close(); err != nil {
					failures <- err
				}
			}()
			for {
				_, payload, err := connection.ReadMessage()
				if err != nil {
					return
				}
				var command struct {
					Method string `json:"method"`
					Params struct {
						Channel string   `json:"channel"`
						Symbol  []string `json:"symbol"`
					} `json:"params"`
				}
				if err := json.Unmarshal(payload, &command); err != nil {
					failures <- err
					return
				}
				if command.Method != "subscribe" {
					failures <- fmt.Errorf("unexpected command: %s", payload)
					return
				}
				if command.Params.Channel == "instrument" {
					err = connection.WriteMessage(gorillaws.TextMessage, []byte(`{"channel":"instrument","type":"snapshot","data":{"pairs":[{"symbol":"BTC/USD","quote":"USD","status":"online"},{"symbol":"ETH/USD","quote":"USD","status":"online"},{"symbol":"BTC/EUR","quote":"EUR","status":"online"},{"symbol":"OLD/USD","quote":"USD","status":"delisted"}]}}`))
					if err != nil {
						failures <- err
						return
					}
					continue
				}
				if len(command.Params.Symbol) != 2 {
					failures <- fmt.Errorf("missing symbol: %s", payload)
					return
				}
				for _, symbol := range command.Params.Symbol {
					subscribed <- command.Params.Channel + ":" + symbol
					if err := connection.WriteMessage(gorillaws.TextMessage, []byte(`{"method":"subscribe","success":true}`)); err != nil {
						failures <- err
						return
					}
					if command.Params.Channel != "ticker" {
						continue
					}
					// A single venue frame deliberately contains two records. Both must run.
					payload, err = json.Marshal(map[string]any{"channel": "ticker", "type": "update", "data": []map[string]any{{"symbol": symbol, "bid": 100, "ask": 102, "bid_qty": 2, "ask_qty": 3, "last": 101}, {"symbol": symbol, "bid": 100, "ask": 105, "bid_qty": 2, "ask_qty": 3, "last": 103}}})
					if err != nil {
						failures <- err
						return
					}
					if err := connection.WriteMessage(gorillaws.TextMessage, payload); err != nil {
						failures <- err
						return
					}
				}
			}
		}))
		defer venue.Close()
		repository := NewRepository()
		graph, err := repository.Load("system")
		So(err, ShouldBeNil)
		for id, node := range graph.Nodes {
			if node.Type == "http.HTTPServer" {
				delete(graph.Nodes, id)
			}
		}
		spot := graph.Nodes["spot"]
		endpoint, err := json.Marshal(map[string]string{"value": "ws" + strings.TrimPrefix(venue.URL, "http")})
		So(err, ShouldBeNil)
		spot.InputData = map[string]json.RawMessage{"socket.endpoint": endpoint}
		graph.Nodes["spot"] = spot
		program, err := Compile(graph, nil, repository)
		So(err, ShouldBeNil)
		defer program.Release()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var spreads []float64
		for len(spreads) < 4 && ctx.Err() == nil {
			So(program.Execute(ctx, nil), ShouldBeNil)
			if _, found := program.Result("definition-liquidity_ticker__spread"); found {
				spread, err := program.Float64Result("definition-liquidity_ticker__spread", "out")
				So(err, ShouldBeNil)
				spreads = append(spreads, spread)
			}
			time.Sleep(time.Millisecond)
		}
		So(spreads, ShouldResemble, []float64{2, 5, 2, 5})
		received := map[string]bool{}
		for len(received) < 4 && ctx.Err() == nil {
			select {
			case err := <-failures:
				So(err, ShouldBeNil)
			case name := <-subscribed:
				received[name] = true
			case <-ctx.Done():
			}
		}
		So(received, ShouldResemble, map[string]bool{"ticker:BTC/USD": true, "trade:BTC/USD": true, "ticker:ETH/USD": true, "trade:ETH/USD": true})
		select {
		case err := <-failures:
			So(err, ShouldBeNil)
		default:
		}
	})
}

func TestProgramStart(t *testing.T) {
	Convey("Given a graph evaluation that cannot satisfy its precondition", t, func() {
		program, err := CompileJSON([]byte(`{"nodes":{"required":{"id":"required","type":"controlflow.Require","inputData":{"data":{"value":"observation"},"test":{"value":false},"reason":{"value":"fixture: subscription rejected"}}}}}`), nil, nil)
		So(err, ShouldBeNil)
		defer program.Release()
		err = program.Start(context.Background())
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "fixture: subscription rejected")
	})
}

func TestProgramCarriedPayloadQueued(t *testing.T) {
	Convey("Given a queued collection with no further external arrivals", t, func() {
		program, err := CompileJSON([]byte(`{"nodes":{"records":{"id":"records","type":"data.Iterate"}}}`), nil, nil)
		So(err, ShouldBeNil)
		defer program.Release()
		_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)
		params, err := data.NewIterate_write_Params(segment)
		So(err, ShouldBeNil)
		arrivals, err := params.NewData(1)
		So(err, ShouldBeNil)
		So(arrivals.Set(0, []byte(`[1,2,3]`)), ShouldBeNil)
		So(program.Execute(context.Background(), map[NodeID]capnp.Struct{program.NodeMap["records"]: capnp.Struct(params)}), ShouldBeNil)
		So(program.carriedPayload(), ShouldBeTrue)
		So(program.Execute(context.Background(), nil), ShouldBeNil)
		So(program.carriedPayload(), ShouldBeTrue)
		So(program.Execute(context.Background(), nil), ShouldBeNil)
		So(program.carriedPayload(), ShouldBeFalse)
	})
}

func TestProgramExecuteKraken(t *testing.T) {
	if os.Getenv("SYMM_LIVE_VERIFY") != "1" {
		t.Skip("set SYMM_LIVE_VERIFY=1 to verify the live Kraken graph")
	}
	Convey("Given the shipping graph connected to Kraken public spot", t, func() {
		errnie.Apply(&errnie.Config{Level: "error"})
		defer errnie.Apply(&errnie.Config{Level: "info"})
		graph, err := NewRepository().Load("system")
		if err != nil {
			t.Fatal(err)
		}
		for id, node := range graph.Nodes {
			if node.Type == "http.HTTPServer" {
				delete(graph.Nodes, id)
			}
		}
		program, err := Compile(graph, nil, NewRepository())
		if err != nil {
			t.Fatal(err)
		}
		defer program.Release()
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		subscriptions := map[string]bool{}
		symbols := map[string]bool{}
		records, signals := 0, 0
		discovered := false
		expected := map[string]bool{}
		for ctx.Err() == nil {
			if err := program.Execute(ctx, nil); err != nil {
				t.Fatal(err)
			}
			if result, found := program.Result("spot__socket"); found {
				received := websocket.Received(result)
				if received.Which() == websocket.Received_Which_frame {
					payload, err := received.Frame().Read()
					if err != nil {
						t.Fatal(err)
					}
					var frame struct {
						Channel string          `json:"channel"`
						Data    json.RawMessage `json:"data"`
						Success bool            `json:"success"`
						Result  struct {
							Channel string `json:"channel"`
							Symbol  string `json:"symbol"`
						} `json:"result"`
					}
					if err := json.Unmarshal(payload, &frame); err != nil {
						t.Fatal(err)
					}
					if frame.Channel == "instrument" {
						discovered = true
						var instruments struct {
							Pairs []struct {
								Symbol string `json:"symbol"`
								Quote  string `json:"quote"`
								Status string `json:"status"`
							} `json:"pairs"`
						}
						So(json.Unmarshal(frame.Data, &instruments), ShouldBeNil)
						for _, pair := range instruments.Pairs {
							if pair.Quote == "USD" && pair.Status == "online" {
								expected["ticker:"+pair.Symbol] = true
								expected["trade:"+pair.Symbol] = true
							}
						}
					}
					if frame.Success && frame.Result.Symbol != "" {
						subscriptions[frame.Result.Channel+":"+frame.Result.Symbol] = true
					}
				}
			}
			if result, found := program.Result("spot__records"); found && data.Iterate_done_Results(result).Found() {
				records++
				payload, err := data.Iterate_done_Results(result).Out()
				if err != nil {
					t.Fatal(err)
				}
				var record struct {
					Data struct {
						Symbol string `json:"symbol"`
					} `json:"data"`
				}
				if err := json.Unmarshal(payload, &record); err != nil {
					t.Fatal(err)
				}
				symbols[record.Data.Symbol] = true
			}
			if _, found := program.Result("definition-liquidity_ticker__spread"); found {
				signals++
			}
			if discovered && len(expected) > 0 && len(subscriptions) == len(expected) && len(symbols) >= 10 && signals >= 20 {
				break
			}
			if !program.carriedPayload() {
				time.Sleep(time.Millisecond)
			}
		}
		if !discovered || len(expected) == 0 || len(subscriptions) != len(expected) || len(symbols) < 10 || signals < 20 {
			t.Fatalf("live delivery incomplete: discovery=%v subscriptions=%d symbols=%d records=%d signals=%d", discovered, len(subscriptions), len(symbols), records, signals)
		}
		So(subscriptions, ShouldResemble, expected)
		t.Logf("live Kraken: all eligible USD spot pairs subscribed; %d acknowledged subscriptions; %d symbols; %d records; %d liquidity signal evaluations", len(subscriptions), len(symbols), records, signals)
	})
}

func TestProgramExecuteFanIn(t *testing.T) {
	Convey("Given twelve numbered numeric inputs", t, func() {
		nodes := map[string]any{}
		inputs := map[string]any{}
		for index := range 12 {
			id := fmt.Sprintf("number%d", index)
			port := fmt.Sprintf("values_%d", index)
			nodes[id] = map[string]any{"id": id, "type": "arithmetic.Add", "inputData": map[string]any{"a": map[string]any{"value": index}}, "connections": map[string]any{"outputs": map[string]any{"out": []any{map[string]any{"nodeId": "element", "portName": port}}}}}
			inputs[port] = []any{map[string]any{"nodeId": id, "portName": "out"}}
		}
		nodes["element"] = map[string]any{"id": "element", "type": "data.Element", "inputData": map[string]any{"index": map[string]any{"value": 10}}, "connections": map[string]any{"inputs": inputs}}
		raw, err := json.Marshal(map[string]any{"nodes": nodes})
		So(err, ShouldBeNil)
		program, err := CompileJSON(raw, nil, nil)
		So(err, ShouldBeNil)
		defer program.Release()
		So(program.Execute(context.Background(), nil), ShouldBeNil)
		actual, err := program.Float64Result("element", "out")
		So(err, ShouldBeNil)
		So(actual, ShouldEqual, 10)
	})
}
