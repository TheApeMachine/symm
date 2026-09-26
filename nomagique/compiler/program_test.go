package compiler

import (
	"bytes"
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
	"github.com/theapemachine/symm/nomagique/financial/kraken"
	"github.com/theapemachine/symm/nomagique/financial/paper"
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
  "frame.provenance":[{"nodeId":"capture","portName":"provenance"}]
 }}},
 "capture":{"id":"capture","type":"store.Capture","connections":{"inputs":{
  "payload":[{"nodeId":"feed","portName":"frame.read"}],
  "provenance":[{"nodeId":"feed","portName":"frame.provenance"}]
 }}}
}}`), nil)
		So(err, ShouldBeNil)
		defer program.Release()

		Convey("Then repeated idle polls capture no row from empty data", func() {
			for observation := 0; observation < 3; observation++ {
				So(program.Execute(context.Background(), nil), ShouldBeNil)
				result, captured := program.results["capture"]
				So(captured && store.Captured(result).Which() == store.Captured_Which_row, ShouldBeFalse)
				So(program.carriedPayload(), ShouldBeFalse)
			}
		})
	})
}

/* Signal fixtures execute the production Consumer and Workspace ownership path. */
func TestProgramExecuteSignals(t *testing.T) {
	Convey("Given the production signal stage and actual channel-tagged records", t, func() {
		ctx := context.Background()
		replay := func(repository DefinitionRepository) []float64 {
			program, err := Compile(signalFixture(t, repository), nil, repository)
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
				So(program.Flush(ctx), ShouldBeNil)
				consumer := runtime.Consumer(program.Nodes[program.NodeMap["definition-liquidity_ticker_consumer"]].Client)
				future, release := consumer.Done(ctx, nil)
				done, err := future.Struct()
				So(err, ShouldBeNil)
				outputs, err := done.Outputs()
				So(err, ShouldBeNil)
				for index := range outputs.Len() {
					name, err := outputs.At(index).Node()
					So(err, ShouldBeNil)
					if name != "spread" {
						continue
					}
					pointer, err := outputs.At(index).Value()
					So(err, ShouldBeNil)
					spreads = append(spreads, arithmetic.Subtract_done_Results(pointer.Struct()).Out())
				}
				release()
			}
			return spreads
		}
		Convey("Then every record reaches the existing liquidity graph", func() {
			So(replay(DefaultRepository()), ShouldResemble, []float64{2, 3, 4})
		})
		Convey("When one signal connection is changed from ask to last in JSON", func() {
			repository := NewRepository()
			graph, err := repository.Load("system")
			So(err, ShouldBeNil)
			consumer := graph.Nodes["definition-liquidity_ticker_consumer"]
			var configured struct{ Value string }
			So(json.Unmarshal(consumer.InputData["bindings"], &configured), ShouldBeNil)
			var bindings []map[string]any
			So(json.Unmarshal([]byte(configured.Value), &bindings), ShouldBeNil)
			for _, binding := range bindings {
				if binding["target"] == "spread.a" {
					binding["field"] = "values_1"
				}
			}
			encoded, err := json.Marshal(bindings)
			So(err, ShouldBeNil)
			consumer.InputData["bindings"], err = json.Marshal(string(encoded))
			So(err, ShouldBeNil)
			graph.Nodes[consumer.ID] = consumer
			encoded, err = json.Marshal(graph)
			So(err, ShouldBeNil)
			So(repository.Save("system", encoded), ShouldBeNil)
			So(replay(repository), ShouldResemble, []float64{1, 1, 2})
		})
	})
}

func BenchmarkProgramExecuteSignals(b *testing.B) {
	program, err := Compile(signalFixture(b, DefaultRepository()), nil, DefaultRepository())
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
		if err := program.Flush(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

/* signalFixture supplies records to the real production projection and liquidity groups. */
func signalFixture(t testing.TB, repository DefinitionRepository) Graph {
	t.Helper()
	graph, err := repository.Load("system")
	if err != nil {
		t.Fatal(err)
	}
	keep := map[string]bool{"workspace": true, "projection_graph": true, "projection_consumer": true, "projection_group": true, "metrics_group": true, "definition-liquidity_ticker_consumer": true, "definition-liquidity_ticker_graph": true}
	for name := range graph.Nodes {
		if !keep[name] {
			withoutNodes(graph, name)
		}
	}
	graph.Nodes["records"] = Node{ID: "records", Type: "data.Iterate", Connections: Connections{Outputs: map[string][]ConnectionTarget{"out": {{NodeID: "workspace", PortName: "data"}}}}}
	workspace := graph.Nodes["workspace"]
	workspace.Connections.Inputs["data"] = []ConnectionTarget{{NodeID: "records", PortName: "out"}}
	graph.Nodes["workspace"] = workspace
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
		// Batches arrive whole, one per evaluation, until the snapshot is read.
		var rows [][]byte

		for pass := 0; pass < 10; pass++ {
			So(program.Execute(ctx, nil), ShouldBeNil)
			scanned, found := program.Result("scan")
			So(found, ShouldBeTrue)

			if tables.Scanned(scanned).Exhausted() {
				break
			}

			result, found := program.Result("project")
			So(found, ShouldBeTrue)
			list, err := data.Arrow_done_Results(result).Rows()
			So(err, ShouldBeNil)

			for index := range list.Len() {
				row, err := list.At(index)
				So(err, ShouldBeNil)
				rows = append(rows, bytes.Clone(row))
			}
		}

		So(rows, ShouldHaveLength, 3)

		for index, raw := range rows {
			var row struct {
				Sequence int64
				Payload  []byte
			}
			So(json.Unmarshal(raw, &row), ShouldBeNil)
			So(row.Sequence, ShouldEqual, 9007199254740993+int64(index))
			So(row.Payload, ShouldResemble, []byte{byte(index), 0, 255})
		}

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
		vocabulary := "v1"
		live := false
		liveToken := ""
		execute := func(identity string, cursor int, history string) error {
			input := map[string]any{
				"capture": map[string]string{"session": identity, "endpoint": "spot"},
				"holding": holding,
				"settled": map[string]any{"vocabulary": vocabulary, "tokens": []string{"A", "B"}, "sequence": history},
				"cursor":  map[string]int{"sequence": offset + cursor, "record": 0},
				"event": map[string]any{
					"a": map[string]int{"sequence": offset + 1, "record": 0},
					"b": map[string]int{"sequence": offset + ignition, "record": 0},
					"c": map[string]int{"sequence": offset + 5, "record": 0},
					"d": map[string]int{"sequence": offset + 7, "record": 0}, "excursion": 1, "seed": 1,
				},
			}
			if live {
				delete(input, "event")
				context := input["settled"].(map[string]any)
				delete(context, "sequence")
				context["tokens"] = []string{liveToken}
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
			inputs := map[NodeID]capnp.Struct{program.NodeMap["input"]: capnp.Struct(params)}
			if settled {
				tokens, steps := []string{"A", "B"}, strings.Split(history, "/")
				if live {
					tokens, steps = []string{liveToken}, nil
				}
				native, err := modelContextFixture(segment, vocabulary, holding, tokens, steps)
				So(err, ShouldBeNil)
				native.SetSequence(int64(offset + cursor))
				causal, err := cognition.NewPrecursor_write_Params(segment)
				So(err, ShouldBeNil)
				So(causal.SetContext(native), ShouldBeNil)
				So(causal.SetModel(store.Radix(program.Nodes[program.NodeMap["memory"]].Client).AddRef()), ShouldBeNil)
				inputs[program.NodeMap["context"]] = capnp.Struct(causal)
			}
			return program.Execute(context.Background(), inputs)
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

		Convey("Live prediction recognizes a learned precursor after unrelated earlier tokens", func() {
			So(execute("entry", 3, "A/B"), ShouldBeNil)
			live = true
			for index, token := range []string{"C", "A", "B"} {
				liveToken = token
				So(execute("live", 8+index, ""), ShouldBeNil)
			}
			prediction("ENTER", 1)
			_, learned := program.Result("reinforce")
			So(learned, ShouldBeFalse)
		})

		Convey("Then truth bootstraps an empty trie and incorrect predictions still learn", func() {
			So(execute("entry-1", 3, "A/B"), ShouldBeNil)
			prediction("", 0)
			So(execute("wait-1", 2, "A/B"), ShouldBeNil)
			prediction("ENTER", 1)
			So(execute("wait-1", 2, "A/B"), ShouldBeNil)
			prediction("", 2)
			_, updated := program.Result("reinforce")
			So(updated, ShouldBeFalse)
			So(execute("wait-2", 2, "A/B"), ShouldBeNil)
			prediction("", 2)
			So(execute("wait-2", 2, "A/B"), ShouldBeNil)
			prediction("WAIT", 3)

			Convey("And the same history under another region vocabulary has no borrowed evidence", func() {
				vocabulary = "v2"
				So(execute("entry-3", 3, "A/B"), ShouldBeNil)
				prediction("", 0)
			})

			Convey("And another precursor history has no borrowed evidence", func() {
				So(execute("entry-2", 3, "C/A/B"), ShouldBeNil)
				prediction("", 0)
			})

			Convey("And adjacent capture identities above Float64 precision remain distinct", func() {
				offset = 1 << 53
				So(execute("large", 3, "L/A/B"), ShouldBeNil)
				prediction("", 0)
				offset++
				So(execute("large", 3, "L/A/B"), ShouldBeNil)
				prediction("ENTER", 1)
				offset--
				So(execute("large", 3, "L/A/B"), ShouldBeNil)
				prediction("ENTER", 2)
				_, updated := program.Result("reinforce")
				So(updated, ShouldBeFalse)
			})

			Convey("And held inventory has separate evidence", func() {
				holding = true
				So(execute("held", 4, "A/B"), ShouldBeNil)
				prediction("", 0)
				So(execute("held", 4, "A/B"), ShouldBeNil)
				prediction("WAIT", 1)
			})

			Convey("And an unsettled observation cannot reach reinforcement", func() {
				settled = false
				So(execute("unsettled", 3, "A/B"), ShouldBeNil)
				_, updated := program.Result("reinforce")
				So(updated, ShouldBeFalse)
				_, predicted := program.Result("prediction")
				So(predicted, ShouldBeFalse)
			})

			Convey("And a contradictory label for the same example is rejected", func() {
				ignition = 2
				err := execute("wait-2", 2, "A/B")
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "conflicting value at unique path")
			})

			Convey("And two different precursor histories ending at the same current token remain separate", func() {
				executeWithSeq := func(identity string, cursor int, seq string, tokens []string) error {
					input := map[string]any{
						"capture": map[string]string{"session": identity, "endpoint": "spot"},
						"holding": holding,
						"settled": map[string]any{"vocabulary": vocabulary, "tokens": tokens, "sequence": seq},
						"cursor":  map[string]int{"sequence": offset + cursor, "record": 0},
						"event": map[string]any{
							"a": map[string]int{"sequence": offset + 1, "record": 0},
							"b": map[string]int{"sequence": offset + ignition, "record": 0},
							"c": map[string]int{"sequence": offset + 5, "record": 0},
							"d": map[string]int{"sequence": offset + 7, "record": 0}, "excursion": 1, "seed": 1,
						},
					}

					payload, err := json.Marshal(input)
					So(err, ShouldBeNil)
					_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
					So(err, ShouldBeNil)
					params, err := transport.NewFan_write_Params(segment)
					So(err, ShouldBeNil)
					So(params.SetData(payload), ShouldBeNil)
					native, err := modelContextFixture(segment, vocabulary, holding, tokens, strings.Split(seq, "/"))
					So(err, ShouldBeNil)
					causal, err := cognition.NewPrecursor_write_Params(segment)
					So(err, ShouldBeNil)
					So(causal.SetContext(native), ShouldBeNil)
					So(causal.SetModel(store.Radix(program.Nodes[program.NodeMap["memory"]].Client).AddRef()), ShouldBeNil)
					return program.Execute(context.Background(), map[NodeID]capnp.Struct{program.NodeMap["input"]: capnp.Struct(params), program.NodeMap["context"]: capnp.Struct(causal)})
				}

				// History 1: R1 -> R4 -> [R7,R9], ending at [R7,R9], trained to ENTER (cursor == ignition == 3)
				So(executeWithSeq("hist-1", 3, "R1/R4/[R7,R9]", []string{"R7", "R9"}), ShouldBeNil)

				// History 2: R3 -> R2 -> [R7,R9], ending at [R7,R9], trained to WAIT (cursor == 2 != ignition)
				So(executeWithSeq("hist-2", 2, "R3/R2/[R7,R9]", []string{"R7", "R9"}), ShouldBeNil)

				// Query History 1 (cursor == 2, so it doesn't train a conflicting ENTER on the same session)
				So(executeWithSeq("query-1", 2, "R1/R4/[R7,R9]", []string{"R7", "R9"}), ShouldBeNil)
				prediction("ENTER", 1)

				// Query History 2 (cursor == 2)
				So(executeWithSeq("query-2", 2, "R3/R2/[R7,R9]", []string{"R7", "R9"}), ShouldBeNil)
				prediction("WAIT", 1)
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
			"settled": map[string]any{"vocabulary": "v1", "tokens": []string{"region-A", "region-B"}, "sequence": "region-A/region-B"},
			"cursor":  map[string]int{"sequence": observation*4 + 1, "record": 0},
			"event": map[string]any{
				"a": map[string]int{"sequence": observation * 4, "record": 0},
				"b": map[string]int{"sequence": observation*4 + 1, "record": 0},
				"c": map[string]int{"sequence": observation*4 + 2, "record": 0},
				"d": map[string]int{"sequence": observation*4 + 3, "record": 0}, "excursion": 1, "seed": 1,
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

		native, err := modelContextFixture(segment, "v1", false, []string{"region-A", "region-B"}, []string{"region-A", "region-B"})
		if err != nil {
			b.Fatal(err)
		}
		causal, err := cognition.NewPrecursor_write_Params(segment)
		if err != nil {
			b.Fatal(err)
		}
		if err := causal.SetContext(native); err != nil {
			b.Fatal(err)
		}
		if err := causal.SetModel(store.Radix(program.Nodes[program.NodeMap["memory"]].Client).AddRef()); err != nil {
			b.Fatal(err)
		}
		if err := program.Execute(context.Background(), map[NodeID]capnp.Struct{program.NodeMap["input"]: capnp.Struct(params), program.NodeMap["context"]: capnp.Struct(causal)}); err != nil {
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

		// documents are one tape evaluation: two frames of a session, each in
		// the envelope the tape hands out, and an instrument catalogue.
		documents := [][]byte{}

		for step := range 2 {
			document, err := json.Marshal(map[string]any{
				"capture": map[string]string{"session": "first", "endpoint": "spot", "receivedAt": "2026-09-23T00:00:00.123456789Z"},
				"cursor":  map[string]int64{"sequence": int64(9007199254740993) + int64(step)},
				"market":  map[string]any{"channel": "ticker", "data": []map[string]int{{"last": step*2 + 1}, {"last": step*2 + 2}}},
			})
			So(err, ShouldBeNil)
			documents = append(documents, document)
		}

		catalogue, err := json.Marshal(map[string]any{
			"capture": map[string]string{"session": "first", "endpoint": "spot", "receivedAt": "2026-09-23T00:00:00.123456789Z"},
			"cursor":  map[string]int64{"sequence": int64(9007199254740995)},
			"market":  map[string]any{"channel": "instrument", "data": map[string]any{"pairs": []any{}}},
		})
		So(err, ShouldBeNil)
		documents = append(documents, catalogue)

		seed := func(id string, list func(capnp.Struct, int32) (capnp.DataList, error)) capnp.Struct {
			node := program.Nodes[program.NodeMap[id]]
			_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
			So(err, ShouldBeNil)
			args, err := capnp.NewRootStruct(segment, node.Write.ParamsSize)
			So(err, ShouldBeNil)
			So(args.CopyFrom(node.ArgsTemplate), ShouldBeNil)
			data, err := list(args, int32(len(documents)))
			So(err, ShouldBeNil)

			for index, document := range documents {
				So(data.Set(index, document), ShouldBeNil)
			}

			return args
		}

		inputs := map[NodeID]capnp.Struct{
			program.NodeMap["records"]: seed("records", func(args capnp.Struct, count int32) (capnp.DataList, error) {
				return data.Iterate_write_Params(args).NewData(count)
			}),
			program.NodeMap["instruments"]: seed("instruments", func(args capnp.Struct, count int32) (capnp.DataList, error) {
				return data.Where_write_Params(args).NewData(count)
			}),
		}
		So(program.Execute(context.Background(), inputs), ShouldBeNil)

		Convey("Every record of every frame is handed over on the evaluation the frames arrived on", func() {
			result, found := program.Result("records")
			So(found, ShouldBeTrue)
			all, err := data.Iterate_done_Results(result).All()
			So(err, ShouldBeNil)
			So(all.Len(), ShouldEqual, 5)

			for position := range 4 {
				raw, err := all.At(position)
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
				So(record.Cursor.Sequence == int64(9007199254740993)+int64(position/2), ShouldBeTrue)
				So(record.Cursor.Record, ShouldEqual, position%2)
				So(record.Capture.Session, ShouldEqual, "first")
				So(record.Capture.Endpoint, ShouldEqual, "spot")
				So(record.Capture.ReceivedAt, ShouldEqual, "2026-09-23T00:00:00.123456789Z")
				So(record.Market.Data.Last, ShouldEqual, position+1)
			}
		})

		Convey("The instrument catalogue alone is handed on as a frame of its own", func() {
			result, found := program.Result("instrument")
			So(found, ShouldBeTrue)
			out, err := data.Iterate_done_Results(result).Out()
			So(err, ShouldBeNil)
			So(string(out), ShouldContainSubstring, `"instrument"`)
			So(string(out), ShouldContainSubstring, `"pairs"`)
		})
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
					err = connection.WriteMessage(gorillaws.TextMessage, []byte(`{"channel":"instrument","type":"snapshot","data":{"pairs":[{"symbol":"BTC/USD","base":"BTC","quote":"USD","status":"online"},{"symbol":"ETH/USD","base":"ETH","quote":"USD","status":"online"},{"symbol":"BTC/EUR","base":"BTC","quote":"EUR","status":"online"},{"symbol":"OLD/USD","base":"OLD","quote":"USD","status":"delisted"},{"symbol":"GBP/USD","base":"GBP","quote":"USD","status":"online"},{"symbol":"EUR/USD","base":"EUR","quote":"USD","status":"online"},{"symbol":"USDT/USD","base":"USDT","quote":"USD","status":"online"},{"symbol":"USDC/USD","base":"USDC","quote":"USD","status":"online"}]}}`))
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
				withoutNodes(graph, id)
			}
		}
		// Only the spot venue is faked; every other outside source leaves.
		withoutNodes(graph, "level3", "futures", "raw_capture_graph")
		withoutLearningStages(graph)
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
		var completed uint64
		workspace := runtime.Workspace(program.Nodes[program.NodeMap["workspace"]].Client)
		consumer := runtime.Consumer(program.Nodes[program.NodeMap["definition-liquidity_ticker_consumer"]].Client)
		for len(spreads) < 2 && ctx.Err() == nil {
			So(program.Execute(ctx, nil), ShouldBeNil)
			future, release := workspace.Flush(ctx, nil)
			_, err := future.Struct()
			release()
			So(err, ShouldBeNil)
			result, release := consumer.Done(ctx, nil)
			done, err := result.Struct()
			So(err, ShouldBeNil)
			if done.Completed() > completed {
				completed = done.Completed()
				outputs, err := done.Outputs()
				So(err, ShouldBeNil)
				for index := range outputs.Len() {
					name, err := outputs.At(index).Node()
					So(err, ShouldBeNil)
					if name == "spread" {
						pointer, err := outputs.At(index).Value()
						So(err, ShouldBeNil)
						spreads = append(spreads, arithmetic.Subtract_done_Results(pointer.Struct()).Out())
					}
				}
			}
			release()
			time.Sleep(time.Millisecond)
		}
		So(spreads, ShouldResemble, []float64{2, 5})
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
		t.Skip("set SYMM_LIVE_VERIFY=1 to verify the live Kraken source group")
	}
	Convey("The shipping source group joins spot, Level 3 and futures before projection", t, func() {
		errnie.Apply(&errnie.Config{Level: "error"})
		defer errnie.Apply(&errnie.Config{Level: "info"})
		graph, err := DefaultRepository().Load("system")
		So(err, ShouldBeNil)
		keep := map[string]bool{"workspace": true, "feeds_group": true, "spot": true, "level3": true, "futures": true, "spot_consumer": true, "level3_consumer": true, "futures_consumer": true, "projection_graph": true, "projection_consumer": true, "projection_group": true}
		for name := range graph.Nodes {
			if !keep[name] {
				withoutNodes(graph, name)
			}
		}
		program, err := Compile(graph, nil, DefaultRepository())
		So(err, ShouldBeNil)
		defer program.Release()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		expected, subscriptions := map[string]bool{}, map[string]bool{}
		records := map[string]int{}
		symbols := map[string]bool{}
		projected := 0
		reconciled := uint64(0)
		for ctx.Err() == nil {
			if err := program.Execute(ctx, nil); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"spot", "level3", "futures", "projection"} {
				client := runtime.Consumer(program.Nodes[program.NodeMap[name+"_consumer"]].Client)
				future, release := client.Done(ctx, nil)
				result, err := future.Struct()
				if err != nil {
					release()
					t.Fatal(err)
				}
				payload, err := result.Data()
				if err != nil {
					release()
					t.Fatal(err)
				}
				if len(payload) > 0 {
					var record struct {
						Channel string
						Data    struct{ Symbol string }
					}
					if err := json.Unmarshal(payload, &record); err != nil {
						release()
						t.Fatal(err)
					}
					records[name+":"+record.Channel]++
					symbols[record.Data.Symbol] = true
				}
				outputs, err := result.Outputs()
				if err != nil {
					release()
					t.Fatal(err)
				}
				for index := range outputs.Len() {
					output := outputs.At(index)
					pointer, err := output.Value()
					if err != nil {
						release()
						t.Fatal(err)
					}
					if name == "projection" {
						projected++
					}
					if output.InterfaceId() == paper.Book_TypeID {
						reconciled = paper.Book_done_Results(pointer.Struct()).Reconciled()
					}
					if output.InterfaceId() == kraken.Universe_TypeID {
						universe := kraken.UniverseResult(pointer.Struct())
						if universe.Which() == kraken.UniverseResult_Which_ready {
							pairs, err := universe.Ready().Symbols()
							if err != nil {
								release()
								t.Fatal(err)
							}
							for position := range pairs.Len() {
								symbol, err := pairs.At(position)
								if err != nil {
									release()
									t.Fatal(err)
								}
								expected["ticker:"+symbol], expected["trade:"+symbol] = true, true
							}
						}
					}
					if name != "spot" || output.InterfaceId() != websocket.WebSocketClient_TypeID {
						continue
					}
					received := websocket.Received(pointer.Struct())
					if received.Which() != websocket.Received_Which_frame {
						continue
					}
					raw, err := received.Frame().Read()
					if err != nil {
						release()
						t.Fatal(err)
					}
					var ack struct {
						Success bool
						Result  struct{ Channel, Symbol string }
					}
					if err := json.Unmarshal(raw, &ack); err != nil {
						release()
						t.Fatal(err)
					}
					if ack.Success && ack.Result.Symbol != "" {
						subscriptions[ack.Result.Channel+":"+ack.Result.Symbol] = true
					}
				}
				release()
			}
			complete := len(expected) > 0 && len(expected) == len(subscriptions) && reconciled > 0
			for _, channel := range []string{"spot:ticker", "spot:trade", "level3:level3", "futures:futures_ticker", "futures:futures_trade"} {
				complete = complete && records[channel] > 0
			}
			if complete {
				break
			}
			if !program.carriedPayload() {
				time.Sleep(time.Millisecond)
			}
		}
		So(subscriptions, ShouldResemble, expected)
		So(len(expected), ShouldBeGreaterThan, 0)
		for _, channel := range []string{"spot:ticker", "spot:trade", "level3:level3", "futures:futures_ticker", "futures:futures_trade"} {
			So(records[channel], ShouldBeGreaterThan, 0)
		}
		So(projected, ShouldBeGreaterThan, 0)
		So(reconciled, ShouldBeGreaterThan, 0)
		t.Logf("native Book reconciled %d live Level 3 messages", reconciled)
		t.Logf("native source group: %d eligible USD pairs, %d subscription acknowledgements, %d markets, %d projections, records=%v", len(expected)/2, len(subscriptions), len(symbols), projected, records)
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

/*
withoutNodes removes nodes from a graph together with every connection that
names them, so a test can keep only the sources it fakes.
*/
func withoutNodes(graph Graph, ids ...string) {
	removed := make(map[string]bool, len(ids))

	for _, id := range ids {
		removed[id] = true
	}
	for changed := true; changed; {
		changed = false
		for id, node := range graph.Nodes {
			if removed[id] || (node.Type != "runtime.Consumer" && node.Type != "runtime.Group") {
				continue
			}
			dependencies, missing := 0, 0
			for port, targets := range node.Connections.Inputs {
				if port != "target" && !strings.HasPrefix(port, "consumers") {
					continue
				}
				for _, target := range targets {
					dependencies++
					if removed[target.NodeID] {
						missing++
					}
				}
			}
			if dependencies > 0 && dependencies == missing {
				removed[id], changed = true, true
			}
		}
	}
	for id := range removed {
		delete(graph.Nodes, id)
	}
	if workspace, exists := graph.Nodes["workspace"]; removed["feeds_group"] && exists {
		workspace.InputData["advance"] = json.RawMessage(`false`)
		graph.Nodes["workspace"] = workspace
	}

	for id, node := range graph.Nodes {
		for _, ports := range []map[string][]ConnectionTarget{node.Connections.Inputs, node.Connections.Outputs} {
			for port, targets := range ports {
				kept := targets[:0]

				for _, target := range targets {
					if !removed[target.NodeID] {
						kept = append(kept, target)
					}
				}

				ports[port] = kept

				if len(kept) == 0 {
					delete(ports, port)
				}
			}
		}

		graph.Nodes[id] = node
	}
}

/* withoutLearningStages isolates the production feed-to-cut ownership chain. */
func withoutLearningStages(graph Graph) {
	withoutNodes(graph, "forward_graph", "forward_consumer", "forward_group",
		"training_graph", "training_consumer", "training_group", "model_graph",
		"model_live_consumer", "model_live_held_consumer", "model_train_consumer", "model_held_consumer", "model_group",
		"paper_graph", "paper_consumer", "paper_group", "paper_checkpoint",
		"logic_graph", "logic_consumer", "logic_group",
		"resonance_graph", "resonance_consumer", "resonance_group", "resonance_checkpoint",
		"manifold_graph", "manifold_consumer", "manifold_group",
		"physical_cut_graph", "physical_cut_consumer", "physical_cut_group",
		"combined_cut_graph", "combined_cut_consumer", "combined_cut_group",
		"view_graph", "view_consumer", "view_group")
}

/* modelContextFixture supplies causal data through native Cap'n Proto fields. */
func modelContextFixture(segment *capnp.Segment, vocabulary string, holding bool, tokens, history []string) (cognition.Context, error) {
	input, err := cognition.NewContext(segment)
	if err != nil {
		return input, err
	}
	if err := input.SetSymbol("BTC/USD"); err != nil {
		return input, err
	}
	if err := input.SetVocabulary(vocabulary); err != nil {
		return input, err
	}
	input.SetEpoch(1)
	input.SetHolding(holding)
	values, err := input.NewTokens(int32(len(tokens)))
	if err != nil {
		return input, err
	}
	for index, value := range tokens {
		if err := values.Set(index, value); err != nil {
			return input, err
		}
	}
	input.SetLive()
	if history == nil {
		return input, nil
	}
	steps, err := input.NewHistory(int32(len(history)))
	if err != nil {
		return input, err
	}
	for index, value := range history {
		if err := steps.Set(index, value); err != nil {
			return input, err
		}
	}
	return input, nil
}

/* tradeSignalObserver inspects every stamped observation after the metric group. */
type tradeSignalObserver struct {
	trades, initialized, bidMatches, askMatches int
	defined                                     map[string]int
}

func (observer *tradeSignalObserver) Step(ctx context.Context, call runtime.StageNode_step) error {
	payload, err := call.Args().Data()
	if err != nil {
		return err
	}
	if bytes.Contains(payload, []byte(`"channel":"trade"`)) {
		observer.trades++
	}
	outputs, err := call.Args().Upstream()
	if err != nil {
		return err
	}
	for index := range outputs.Len() {
		output := outputs.At(index)
		pointer, err := output.Value()
		if err != nil {
			return err
		}
		if output.InterfaceId() == paper.Book_TypeID {
			bookResult := paper.Book_done_Results(pointer.Struct())
			values, err := bookResult.Values()
			if err != nil {
				return err
			}
			present, err := bookResult.Present()
			if err != nil {
				return err
			}
			if present.Len() > 14 && present.At(10) {
				observer.initialized++
				if values.At(13) == 1 {
					observer.bidMatches++
				}
				if values.At(14) == 1 {
					observer.askMatches++
				}
			}
		}
		producer, err := output.Producer()
		if err != nil {
			return err
		}
		node, err := output.Node()
		if err != nil {
			return err
		}
		if producer == "definition-toxicity_trade" && strings.HasPrefix(node, "fill_fraction_zscore:") {
			quotient := arithmetic.Quotient(pointer.Struct())
			if quotient.Which() == arithmetic.Quotient_Which_out {
				observer.defined[node]++
			}
		}
	}
	return nil
}

func (observer *tradeSignalObserver) Fence(ctx context.Context, call runtime.StageNode_fence) error {
	return nil
}

func TestProgramExecuteTradeSignals(t *testing.T) {
	if os.Getenv("SYMM_LIVE_VERIFY") != "1" {
		t.Skip("set SYMM_LIVE_VERIFY=1 to verify live trade signals")
	}
	Convey("Live source records reach the authored trade toxicity calculation", t, func() {
		errnie.Apply(&errnie.Config{Level: "error"})
		defer errnie.Apply(&errnie.Config{Level: "info"})
		graph, err := DefaultRepository().Load("system")
		So(err, ShouldBeNil)
		keep := map[string]bool{"workspace": true, "feeds_group": true, "spot": true, "level3": true, "futures": true, "spot_consumer": true, "level3_consumer": true, "futures_consumer": true, "projection_graph": true, "projection_consumer": true, "projection_group": true, "metrics_group": true, "definition-toxicity_trade_graph": true, "definition-toxicity_trade_consumer": true}
		for name := range graph.Nodes {
			if !keep[name] {
				withoutNodes(graph, name)
			}
		}
		observer := &tradeSignalObserver{defined: map[string]int{}}
		registry := DefaultRegistry()
		registry.Register("test.TradeSignals", Factory{InterfaceID: runtime.StageNode_TypeID, New: func(ctx context.Context, config []byte) (capnp.Client, error) {
			return capnp.Client(runtime.StageNode_ServerToClient(observer)), nil
		}})
		graph.Nodes["observer"] = Node{ID: "observer", Type: "test.TradeSignals"}
		graph.Nodes["observer_consumer"] = Node{ID: "observer_consumer", Type: "runtime.Consumer"}
		graph.Nodes["observer_group"] = Node{ID: "observer_group", Type: "runtime.Group"}
		for _, edge := range [][3]string{{"observer", "observer_consumer", "target"}, {"observer_consumer", "observer_group", "consumers_0"}, {"observer_group", "workspace", "groups_6"}} {
			source, destination := graph.Nodes[edge[0]], graph.Nodes[edge[1]]
			if source.Connections.Outputs == nil {
				source.Connections.Outputs = map[string][]ConnectionTarget{}
			}
			if destination.Connections.Inputs == nil {
				destination.Connections.Inputs = map[string][]ConnectionTarget{}
			}
			source.Connections.Outputs["self"] = append(source.Connections.Outputs["self"], ConnectionTarget{NodeID: edge[1], PortName: edge[2]})
			destination.Connections.Inputs[edge[2]] = []ConnectionTarget{{NodeID: edge[0], PortName: "self"}}
			graph.Nodes[edge[0]], graph.Nodes[edge[1]] = source, destination
		}
		program, err := Compile(graph, registry, DefaultRepository())
		So(err, ShouldBeNil)
		defer program.Release()
		ctx := context.Background()
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) && (observer.defined["fill_fraction_zscore:bid"] == 0 || observer.defined["fill_fraction_zscore:ask"] == 0) {
			if err := program.Execute(ctx, nil); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("live trades=%d initialized books=%d bid matches=%d ask matches=%d defined=%v", observer.trades, observer.initialized, observer.bidMatches, observer.askMatches, observer.defined)
		So(observer.defined["fill_fraction_zscore:bid"], ShouldBeGreaterThan, 0)
		So(observer.defined["fill_fraction_zscore:ask"], ShouldBeGreaterThan, 0)
	})
}
