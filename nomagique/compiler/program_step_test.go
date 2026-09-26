package compiler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/iceberg-go"
	sqlcat "github.com/apache/iceberg-go/catalog/sql"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/financial/execution"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/network/websocket"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/store/tables"
	"github.com/theapemachine/symm/nomagique/temporal"

	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/ui"
	marketfixture "github.com/theapemachine/symm/tests/market"
)

/* BenchmarkProgramStep traverses actual definition capabilities through LMAX. */
func BenchmarkProgramStep(b *testing.B) {
	repository := NewRepository()

	for name, graph := range map[string]string{
		"read": `{"id":"read","nodes":{"extract":{"id":"extract","type":"data.Extract","inputData":{"path":"value"}}}}`,
		"mean": `{"id":"mean","nodes":{"mean":{"id":"mean","type":"statistic.Mean"}}}`,
	} {
		if err := repository.Save(name, []byte(graph)); err != nil {
			b.Fatal(err)
		}
	}
	program, err := Compile(callableWorkspace(), nil, repository)

	if err != nil {
		b.Fatal(err)
	}
	defer program.Release()
	ctx := context.Background()

	if err := program.Execute(ctx, nil); err != nil {
		b.Fatal(err)
	}
	workspace := runtime.Workspace(program.Nodes[program.NodeMap["workspace"]].Client)
	b.ReportAllocs()

	for b.Loop() {
		if err := workspace.Write(ctx, func(params runtime.Workspace_write_Params) error {
			payloads, err := params.NewData(1)

			if err != nil {
				return err
			}
			return payloads.Set(0, []byte(`{"value":123.45}`))
		}); err != nil {
			b.Fatal(err)
		}

		if err := workspace.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := workspace.Flush(ctx, nil)
		_, err := future.Struct()
		release()

		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestProgramStep(t *testing.T) {
	Convey("Stage bindings preserve independent branches and real join requirements", t, func() {
		for _, slots := range []bool{false, true} {
			graph := Graph{ID: "branches", Nodes: map[string]Node{
				"left":  {ID: "left", Type: "statistic.Mean"},
				"right": {ID: "right", Type: "statistic.Mean"},
				"sum":   {ID: "sum", Type: "arithmetic.Add"},
			}}
			program, err := Compile(graph, nil)
			So(err, ShouldBeNil)
			client := runtime.StageNode_ServerToClient(program)
			defer client.Release()
			for _, observation := range []struct {
				left, right bool
				value       float64
			}{
				{true, false, 10}, {false, true, 20}, {true, true, 30},
			} {
				future, release := client.Step(context.Background(), func(params runtime.StageNode_step_Params) error {
					bindings, err := params.NewBindings(4)
					if err != nil {
						return err
					}
					for index, target := range []string{"left.value", "right.value", "sum.a", "sum.b"} {
						binding := bindings.At(index)
						source := "left"
						if index%2 != 0 {
							source = "right"
						}
						field := "out"
						if slots {
							field = fmt.Sprintf("values_%d", index%2)
						}
						for _, err := range []error{binding.SetProducer("feeds"), binding.SetNode(source), binding.SetField(field), binding.SetTarget(target)} {
							if err != nil {
								return err
							}
						}
					}
					outputs, err := params.NewOutputs(3)
					if err != nil {
						return err
					}
					for index, name := range []string{"left", "right", "sum"} {
						if err := outputs.Set(index, name); err != nil {
							return err
						}
					}
					upstream, err := params.NewUpstream(2)
					if err != nil {
						return err
					}
					for index, name := range []string{"left", "right"} {
						result := upstream.At(index)
						result.SetInterfaceId(data.Extract_TypeID)
						if err := result.SetProducer("feeds"); err != nil {
							return err
						}
						if err := result.SetNode(name); err != nil {
							return err
						}
						if slots {
							result.SetInterfaceId(store.Grid_TypeID)
							grid, err := store.NewGrid_done_Results(params.Segment())
							if err != nil {
								return err
							}
							values, err := grid.NewValues(2)
							if err != nil {
								return err
							}
							values.Set(0, observation.value)
							values.Set(1, observation.value)
							present, err := grid.NewPresent(2)
							if err != nil {
								return err
							}
							present.Set(0, observation.left)
							present.Set(1, observation.right)
							if err := result.SetValue(capnp.Struct(grid).ToPtr()); err != nil {
								return err
							}
							continue
						}
						extracted, err := data.NewExtracted(params.Segment())
						if err != nil {
							return err
						}
						extracted.SetMissing()
						if index == 0 && observation.left || index == 1 && observation.right {
							extracted.SetOut(observation.value)
						}
						if err := result.SetValue(capnp.Struct(extracted).ToPtr()); err != nil {
							return err
						}
					}
					return nil
				})
				result, err := future.Struct()
				So(err, ShouldBeNil)
				outputs, err := result.Outputs()
				So(err, ShouldBeNil)
				values := map[string]float64{}
				for index := range outputs.Len() {
					output := outputs.At(index)
					name, err := output.Node()
					So(err, ShouldBeNil)
					pointer, err := output.Value()
					So(err, ShouldBeNil)
					if name == "sum" {
						values[name] = arithmetic.Add_done_Results(pointer.Struct()).Out()
						continue
					}
					values[name] = statistic.Mean_done_Results(pointer.Struct()).Out()
				}
				release()
				_, left := values["left"]
				_, right := values["right"]
				_, joined := values["sum"]
				So(left, ShouldEqual, observation.left)
				So(right, ShouldEqual, observation.right)
				So(joined, ShouldEqual, observation.left && observation.right)
				if joined {
					So(values, ShouldResemble, map[string]float64{"left": 20, "right": 25, "sum": 60})
				}
			}
		}
	})
}

func TestProgramStepGather(t *testing.T) {
	Convey("Parallel result bindings fill stable metric coordinates without publishing a partial cut", t, func() {
		graph := Graph{ID: "cut", Nodes: map[string]Node{"cut": {ID: "cut", Type: "data.Gather"}}}
		program, err := Compile(graph, nil)
		So(err, ShouldBeNil)
		client := runtime.StageNode_ServerToClient(program)
		defer client.Release()
		for sequence := range 3 {
			future, release := client.Step(context.Background(), func(params runtime.StageNode_step_Params) error {
				bindings, err := params.NewBindings(2)
				if err != nil {
					return err
				}
				for index := range 2 {
					binding := bindings.At(index)
					for _, err := range []error{binding.SetProducer("signals"), binding.SetNode(fmt.Sprintf("metric%d", index)), binding.SetField("out"), binding.SetTarget(fmt.Sprintf("cut.values_%d", index))} {
						if err != nil {
							return err
						}
					}
				}
				outputs, err := params.NewOutputs(1)
				if err != nil {
					return err
				}
				if err := outputs.Set(0, "cut"); err != nil {
					return err
				}
				results, err := params.NewUpstream(1)
				if err != nil {
					return err
				}
				result := results.At(0)
				result.SetInterfaceId(statistic.Mean_TypeID)
				if err := result.SetProducer("signals"); err != nil {
					return err
				}
				if err := result.SetNode(fmt.Sprintf("metric%d", sequence%2)); err != nil {
					return err
				}
				mean, err := statistic.NewMean_done_Results(params.Segment())
				if err != nil {
					return err
				}
				mean.SetOut(float64((sequence + 1) * 10))
				return result.SetValue(capnp.Struct(mean).ToPtr())
			})
			result, err := future.Struct()
			So(err, ShouldBeNil)
			outputs, err := result.Outputs()
			So(err, ShouldBeNil)
			So(outputs.Len(), ShouldEqual, 1)
			pointer, err := outputs.At(0).Value()
			So(err, ShouldBeNil)
			cut := data.Gathered(pointer.Struct())
			if sequence == 0 {
				So(cut.Which(), ShouldEqual, data.Gathered_Which_idle)
				release()
				continue
			}
			So(cut.Which(), ShouldEqual, data.Gathered_Which_gathered)
			values, err := cut.Gathered().Values()
			So(err, ShouldBeNil)
			expected := []float64{10, 20}
			if sequence == 2 {
				expected[0] = 30
			}
			So([]float64{values.At(0), values.At(1)}, ShouldResemble, expected)
			release()
		}
	})
}

/* TestProgramStepProduction exercises real shipping metric consumers together. */
func TestProgramStepProduction(t *testing.T) {
	Convey("The production metrics process each feed record through native LMAX", t, func() {
		graph, err := DefaultRepository().Load("system")
		So(err, ShouldBeNil)
		for id, node := range graph.Nodes {
			if !strings.HasPrefix(node.Type, "runtime.") && !strings.HasSuffix(id, "_graph") {
				withoutNodes(graph, id)
			}
		}
		withoutLearningStages(graph)
		program, err := Compile(graph, nil, DefaultRepository())
		So(err, ShouldBeNil)
		defer program.Release()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		So(program.Execute(ctx, nil), ShouldBeNil)
		workspace := runtime.Workspace(program.Nodes[program.NodeMap["workspace"]].Client)
		for _, payload := range []string{
			`{"channel":"ticker","capture":{"session":"spot","sequence":0,"record":0,"endpoint":"wss://fixture","receivedAt":"2026-09-26T00:00:00Z"},"data":{"symbol":"BTC/USD","last":101,"bid":100,"ask":102}}`,
			`{"channel":"trade","capture":{"session":"spot","sequence":1,"record":0,"endpoint":"wss://fixture","receivedAt":"2026-09-26T00:00:01Z"},"data":{"symbol":"BTC/USD","price":101,"qty":2,"side":"buy","timestamp":"2026-09-26T00:00:01Z"}}`,
			`{"channel":"ticker","capture":{"session":"spot","sequence":2,"record":0,"endpoint":"wss://fixture","receivedAt":"2026-09-26T00:00:02Z"},"data":{"symbol":"BTC/USD","last":106,"bid":104,"ask":109}}`,
		} {
			So(workspace.Write(ctx, func(args runtime.Workspace_write_Params) error {
				input, err := args.NewData(1)
				if err != nil {
					return err
				}
				return input.Set(0, []byte(payload))
			}), ShouldBeNil)
			So(workspace.WaitStreaming(), ShouldBeNil)
			future, release := workspace.Flush(ctx, nil)
			_, err := future.Struct()
			release()
			So(err, ShouldBeNil)
		}
		consumer := runtime.Consumer(program.Nodes[program.NodeMap["definition-liquidity_ticker_consumer"]].Client)
		future, release := consumer.Done(ctx, nil)
		defer release()
		completed, err := future.Struct()
		So(err, ShouldBeNil)
		So(completed.Completed(), ShouldEqual, 3)
		results, err := completed.Outputs()
		So(err, ShouldBeNil)
		var spread float64
		for index := range results.Len() {
			name, err := results.At(index).Node()
			So(err, ShouldBeNil)
			if name == "spread" {
				pointer, err := results.At(index).Value()
				So(err, ShouldBeNil)
				spread = arithmetic.Subtract_done_Results(pointer.Struct()).Out()
			}
		}
		So(spread, ShouldEqual, 5)
	})
}

func TestProgramStepUI(t *testing.T) {
	Convey("An upstream Cap'n Proto result reaches an authored UI node through a stage", t, func() {
		graph := Graph{ID: "view", Nodes: map[string]Node{"price": {ID: "price", Type: "ui.Text"}}}
		program, err := Compile(graph, nil)
		So(err, ShouldBeNil)
		client := runtime.StageNode_ServerToClient(program)
		defer client.Release()
		future, release := client.Step(context.Background(), func(args runtime.StageNode_step_Params) error {
			bindings, err := args.NewBindings(1)
			if err != nil {
				return err
			}
			for _, err := range []error{bindings.At(0).SetProducer("market"), bindings.At(0).SetNode("last"), bindings.At(0).SetField("out"), bindings.At(0).SetTarget("price.value")} {
				if err != nil {
					return err
				}
			}
			upstream, err := args.NewUpstream(1)
			if err != nil {
				return err
			}
			result := upstream.At(0)
			result.SetInterfaceId(data.Extract_TypeID)
			for _, err := range []error{result.SetProducer("market"), result.SetNode("last")} {
				if err != nil {
					return err
				}
			}
			value, err := data.NewExtracted(args.Segment())
			if err != nil {
				return err
			}
			value.SetOut(123.5)
			return result.SetValue(capnp.Struct(value).ToPtr())
		})
		defer release()
		result, err := future.Struct()
		So(err, ShouldBeNil)
		payload, err := result.Bindings()
		So(err, ShouldBeNil)
		message, err := capnp.Unmarshal(payload)
		So(err, ShouldBeNil)
		defer message.Release()
		bindings, err := ui.ReadRootBindings(message)
		So(err, ShouldBeNil)
		values, err := bindings.Values()
		So(err, ShouldBeNil)
		So(values.Len(), ShouldEqual, 1)
		value, err := values.At(0).Value()
		So(err, ShouldBeNil)
		So(value, ShouldEqual, "123.5")
		graphName, err := values.At(0).Graph()
		So(err, ShouldBeNil)
		So(graphName, ShouldEqual, "view")
	})
}

/* TestProgramStepDurable exercises the shipping stage graph with a local warehouse. */
func TestProgramStepDurable(t *testing.T) {
	Convey("Shipping consumers persist named, stamped cuts through their capability fence", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		directory := t.TempDir()
		database, err := sql.Open("sqlite3", filepath.Join(directory, "catalog.db"))
		So(err, ShouldBeNil)
		defer func() { So(database.Close(), ShouldBeNil) }()
		catalog, err := sqlcat.NewCatalog("test", database, sqlcat.SQLite, iceberg.Properties{"warehouse": "file://" + directory})
		So(err, ShouldBeNil)
		registry := NewRegistry()
		RegisterGeneratedPrimitives(registry)
		registry.Register("tables.IcebergTable", Factory{InterfaceID: tables.IcebergTable_TypeID, New: func(context.Context, []byte) (capnp.Client, error) {
			writer := tables.NewIcebergTable()
			writer.Catalog = catalog
			server := tables.IcebergTable_NewServer(writer)
			server.NewArena = func() capnp.Arena { return capnp.MultiSegment(nil) }
			return capnp.NewClient(server), nil
		}})
		repository := NewRepository()
		So(repository.Save("archive_scan", []byte(`{"id":"archive_scan","nodes":{"input":{"id":"input","type":"controlflow.Once"},"scan":{"id":"scan","type":"tables.IcebergScan","connections":{"outputs":{"out":[{"nodeId":"project","portName":"data"}]}}},"project":{"id":"project","type":"data.Arrow","connections":{"inputs":{"data":[{"nodeId":"scan","portName":"out"}]}}}}}`)), ShouldBeNil)
		graph, err := repository.Load("system")
		So(err, ShouldBeNil)
		for id, node := range graph.Nodes {
			if id != "server" && !strings.HasSuffix(id, "_checkpoint") && !strings.HasPrefix(node.Type, "runtime.") && !strings.HasSuffix(id, "_graph") {
				withoutNodes(graph, id)
			}
		}
		for id, checkpoint := range graph.Nodes {
			if !strings.HasSuffix(id, "_checkpoint") {
				continue
			}
			checkpoint.InputData["path"], err = json.Marshal(filepath.Join(directory, id+".capnp"))
			So(err, ShouldBeNil)
			graph.Nodes[id] = checkpoint
		}
		model := graph.Nodes["model_graph"]
		model.InputData["memory.path"], err = json.Marshal(filepath.Join(directory, "model.capnp"))
		So(err, ShouldBeNil)
		graph.Nodes["model_graph"] = model
		server := graph.Nodes["server"]
		server.InputData["address"] = json.RawMessage(`"127.0.0.1:0"`)
		graph.Nodes["server"] = server
		forward := graph.Nodes["forward_consumer"]
		var selected struct {
			Value []string `json:"value"`
		}
		So(json.Unmarshal(forward.InputData["outputs"], &selected), ShouldBeNil)
		forward.InputData["outputs"], err = json.Marshal(append(selected.Value, "excursion"))
		So(err, ShouldBeNil)
		graph.Nodes["forward_consumer"] = forward
		program, err := Compile(graph, registry, repository)
		So(err, ShouldBeNil)
		defer program.Release()
		So(program.Execute(ctx, nil), ShouldBeNil)
		workspace := runtime.Workspace(program.Nodes[program.NodeMap["workspace"]].Client)
		So(workspace.Write(ctx, func(args runtime.Workspace_write_Params) error {
			arrivals, err := args.NewData(6)
			if err != nil {
				return err
			}
			for index, payload := range []string{
				`{"channel":"ticker","capture":{"session":"spot","sequence":0,"record":0,"endpoint":"wss://fixture","receivedAt":"2026-09-26T00:00:00Z"},"data":{"symbol":"BTC/USD","last":101,"bid":100,"ask":102}}`,
				`{"channel":"trade","capture":{"session":"spot","sequence":1,"record":0,"endpoint":"wss://fixture","receivedAt":"2026-09-26T00:00:01Z"},"data":{"symbol":"BTC/USD","price":101,"qty":2,"side":"buy","timestamp":"2026-09-26T00:00:01Z"}}`,
				`{"channel":"ticker","capture":{"session":"spot","sequence":2,"record":0,"endpoint":"wss://fixture","receivedAt":"2026-09-26T00:00:02Z"},"data":{"symbol":"BTC/USD","last":106,"bid":104,"ask":109}}`,
				`{"channel":"ticker","capture":{"session":"spot","sequence":3,"record":0,"endpoint":"wss://fixture","receivedAt":"2026-09-26T00:00:03Z"},"data":{"symbol":"ETH/USD","last":2000,"bid":1990,"ask":2010}}`,
				`{"channel":"futures_ticker","capture":{"session":"futures","sequence":55,"record":0,"endpoint":"wss://fixture","receivedAt":"2026-09-26T00:00:04Z"},"data":{"symbol":"BTC/USD","last":99999}}`,
				`{"channel":"ticker","capture":{"session":"spot","sequence":5,"record":0,"endpoint":"wss://fixture","receivedAt":"2026-09-26T00:00:05Z"},"data":{"symbol":"BTC/USD","last":108,"bid":107,"ask":109}}`,
			} {
				if err := arrivals.Set(index, []byte(payload)); err != nil {
					return err
				}
			}
			return nil
		}), ShouldBeNil)
		So(workspace.WaitStreaming(), ShouldBeNil)
		So(program.Flush(ctx), ShouldBeNil)
		forwardConsumer := runtime.Consumer(program.Nodes[program.NodeMap["forward_consumer"]].Client)
		completed, release := forwardConsumer.Done(ctx, nil)
		progress, err := completed.Struct()
		So(err, ShouldBeNil)
		results, err := progress.Outputs()
		So(err, ShouldBeNil)
		observedExcursion := false
		for index := range results.Len() {
			name, err := results.At(index).Node()
			So(err, ShouldBeNil)
			if name != "excursion" {
				continue
			}
			observedExcursion = true
			pointer, err := results.At(index).Value()
			So(err, ShouldBeNil)
			excursion := temporal.ExcursionResult(pointer.Struct())
			// Three spot ticks produce two steps; the futures tick is not a spot-price step.
			So(excursion.Steps(), ShouldEqual, 2)
			So(excursion.Legs(), ShouldEqual, 0)
		}
		release()
		So(observedExcursion, ShouldBeTrue)
		table, err := catalog.LoadTable(ctx, []string{"symm", "metric_cuts_v2"})
		So(err, ShouldBeNil)
		So(table.CurrentSnapshot(), ShouldNotBeNil)
		So(table.CurrentSnapshot().Summary.Properties["total-records"], ShouldEqual, "6")
		_, batches, err := table.Scan().ToArrowRecords(ctx)
		So(err, ShouldBeNil)
		seen := make(map[int64]bool)
		for batch, err := range batches {
			So(err, ShouldBeNil)
			for index := 0; index < int(batch.NumRows()); index++ {
				epoch := batch.Column(0).(*array.Int64).Value(index)
				sequence := batch.Column(1).(*array.Int64).Value(index)
				So(epoch, ShouldBeGreaterThan, int64(1)<<53)
				seen[sequence] = true
				symbol := batch.Column(2).(*array.String).Value(index)
				expectedSymbol := "BTC/USD"
				if sequence == 3 {
					expectedSymbol = "ETH/USD"
				}
				So(symbol, ShouldEqual, expectedSymbol)

				var origin struct {
					Session  string
					Sequence int64
					Record   int64
				}
				So(json.Unmarshal([]byte(batch.Column(5).(*array.String).Value(index)), &origin), ShouldBeNil)
				expectedSession, sourceSequence := "spot", sequence
				if sequence == 4 {
					expectedSession, sourceSequence = "futures", 55
				}
				So(origin.Session, ShouldEqual, expectedSession)
				So(origin.Sequence, ShouldEqual, sourceSequence)
				So(origin.Record, ShouldEqual, 0)
				metrics := batch.Column(4).(*array.List)
				start, end := metrics.ValueOffsets(index)
				So(end-start, ShouldEqual, 411)
				rows := metrics.ListValues().(*array.Struct)
				for slot := int(start); slot < int(end); slot++ {
					if !rows.Field(2).(*array.Boolean).Value(slot) {
						continue
					}
					So(rows.Field(3).(*array.Int64).Value(slot), ShouldEqual, epoch)
					stamp := rows.Field(4).(*array.Int64).Value(slot)
					So(stamp, ShouldBeLessThanOrEqualTo, sequence)
					if symbol == "ETH/USD" {
						So(stamp, ShouldEqual, 3)
					}
					identity := rows.Field(0).(*array.String).Value(slot)
					if sequence == 2 && identity == "liquidity_ticker:spread.out" {
						So(rows.Field(1).(*array.Float64).Value(slot), ShouldEqual, 5)
						So(stamp, ShouldEqual, 2)
					}
					if sequence == 2 && identity == "cvd_trade:trade_count.out" {
						So(rows.Field(1).(*array.Float64).Value(slot), ShouldEqual, 1)
						So(stamp, ShouldEqual, 1)
					}
				}
			}
			batch.Release()
		}
		So(seen, ShouldResemble, map[int64]bool{0: true, 1: true, 2: true, 3: true, 4: true, 5: true})
	})
}

/* TestProgramStepModel proves both shipping consumers use one durable model capability. */
func TestProgramStepModel(t *testing.T) {
	Convey("Training evidence reaches live prediction and survives model-node restart", t, func() {
		path := filepath.Join(t.TempDir(), "model.capnp")
		ctx := context.Background()
		for restart := range 2 {
			graph, err := DefaultRepository().Load("system")
			So(err, ShouldBeNil)
			for id := range graph.Nodes {
				if id != "model_graph" && id != "model_train_consumer" && id != "model_live_consumer" {
					withoutNodes(graph, id)
				}
			}
			model := graph.Nodes["model_graph"]
			model.InputData["memory.path"], err = json.Marshal(path)
			So(err, ShouldBeNil)
			graph.Nodes["model_graph"] = model
			program, err := Compile(graph, nil, DefaultRepository())
			So(err, ShouldBeNil)
			So(program.Execute(ctx, nil), ShouldBeNil)
			input := map[string]any{
				"symbol":  "BTC/USD",
				"capture": map[string]string{"session": "entry-1", "endpoint": "spot"},
				"holding": false,
				"settled": map[string]any{"vocabulary": "v1", "tokens": []string{"A", "B"}, "sequence": "A/B"},
				"cursor":  map[string]int{"sequence": 3, "record": 0},
			}
			for _, lane := range []string{"train", "live"} {
				if restart > 0 && lane == "train" {
					continue
				}
				source, name, expected := "forward", "live_with_holding", "ENTER"
				if lane == "train" {
					source, name, expected = "training", "with_holding", ""
					input["event"] = map[string]any{
						"a": map[string]int{"sequence": 1, "record": 0}, "b": map[string]int{"sequence": 3, "record": 0},
						"c": map[string]int{"sequence": 5, "record": 0}, "d": map[string]int{"sequence": 7, "record": 0}, "excursion": 1, "seed": 1,
					}
				}
				if lane == "live" {
					delete(input, "event")
				}
				payload, err := json.Marshal(input)
				So(err, ShouldBeNil)
				client := runtime.StageNode(program.Nodes[program.NodeMap["model_"+lane+"_consumer"]].Client)
				future, release := client.Step(ctx, func(params runtime.StageNode_step_Params) error {
					params.SetEpoch(1)
					params.SetSequence(int64(restart))
					upstream, err := params.NewUpstream(2)
					if err != nil {
						return err
					}
					result := upstream.At(0)
					result.SetInterfaceId(data.Insert_TypeID)
					if err := result.SetProducer(source); err != nil {
						return err
					}
					if err := result.SetNode(name); err != nil {
						return err
					}
					value, err := data.NewInsert_done_Results(params.Segment())
					if err != nil {
						return err
					}
					if err := value.SetOut(payload); err != nil {
						return err
					}
					if err := result.SetValue(capnp.Struct(value).ToPtr()); err != nil {
						return err
					}
					nativeResult := upstream.At(1)
					nativeResult.SetInterfaceId(cognition.ContextBuilder_TypeID)
					if err := nativeResult.SetProducer(source); err != nil {
						return err
					}
					if err := nativeResult.SetNode("contexts"); err != nil {
						return err
					}
					contexts, err := cognition.NewContexts(params.Segment())
					if err != nil {
						return err
					}
					contexts.SetReady()
					var history []string
					if lane == "train" {
						history = []string{"A,B"}
					}
					native, err := modelContextFixture(params.Segment(), "v1", false, []string{"A", "B"}, history)
					if err != nil {
						return err
					}
					if err := contexts.Ready().SetFlat(native); err != nil {
						return err
					}
					return nativeResult.SetValue(capnp.Struct(contexts).ToPtr())
				})
				completed, err := future.Struct()
				So(err, ShouldBeNil)
				outputs, err := completed.Outputs()
				So(err, ShouldBeNil)
				predicted := false
				for index := range outputs.Len() {
					output := outputs.At(index)
					node, err := output.Node()
					So(err, ShouldBeNil)
					if node != "prediction" {
						continue
					}
					value, err := output.Value()
					So(err, ShouldBeNil)
					class, err := cognition.Attractor_done_Results(value.Struct()).Class()
					So(err, ShouldBeNil)
					So(string(class), ShouldEqual, expected)
					predicted = true
				}
				So(predicted, ShouldBeTrue)
				release()
			}
			fenced, release := runtime.StageNode(program.Nodes[program.NodeMap["model_graph"]].Client).Fence(ctx, nil)
			_, err = fenced.Struct()
			So(err, ShouldBeNil)
			release()
			program.Release()
		}
	})
}

/* TestProgramStepReplay exercises stored cuts through the shipping training graph. */
func TestProgramStepReplay(t *testing.T) {
	Convey("Replay selects confirmed fragments and projects saved metrics through the settled regions", t, func() {
		directory := t.TempDir()
		cuts := filepath.Join(directory, "cuts.json")
		fragments := filepath.Join(directory, "fragments.json")
		epoch := int64(1)<<53 + 3
		var rows []map[string]any
		for sequence := range 5 {
			metrics := []map[string]any{}
			for index, identity := range []string{"spread", "flow"} {
				metrics = append(metrics, map[string]any{"identity": identity, "value": float64(sequence*sequence + index), "present": true, "epoch": epoch, "sequence": sequence})
			}
			rows = append(rows, map[string]any{"epoch": epoch, "sequence": sequence, "symbol": "BTC/USD", "complete": true, "metrics": metrics})
		}
		encoded, err := json.Marshal(rows)
		So(err, ShouldBeNil)
		So(os.WriteFile(cuts, encoded, 0600), ShouldBeNil)
		encoded, err = json.Marshal([]map[string]any{{"epoch": epoch, "symbol": "BTC/USD", "anchor_sequence": 1, "ignition_sequence": 2, "extremum_sequence": 3, "confirmation_sequence": 4, "excursion": 0.2, "has_precursor": true}})
		So(err, ShouldBeNil)
		So(os.WriteFile(fragments, encoded, 0600), ShouldBeNil)
		graph, err := DefaultRepository().Load("training")
		So(err, ShouldBeNil)
		query := graph.Nodes["query"]
		var statement struct {
			Value string `json:"value"`
		}
		So(json.Unmarshal(query.InputData["sql"], &statement), ShouldBeNil)
		query.InputData["sql"], err = json.Marshal(strings.ReplaceAll(statement.Value, "symmtables.symm.", ""))
		So(err, ShouldBeNil)
		query.InputData["setup"], err = json.Marshal([]string{
			"CREATE TABLE metric_cuts_v3 AS SELECT * FROM read_json('" + cuts + "')",
			"CREATE TABLE excursion_fragments_v1 AS SELECT * FROM read_json('" + fragments + "')",
		})
		So(err, ShouldBeNil)
		graph.Nodes["query"] = query
		for _, name := range []string{"gather", "prior_cut"} {
			node := graph.Nodes[name]
			node.InputData["identities"] = json.RawMessage(`["spread","flow"]`)
			graph.Nodes[name] = node
		}
		program, err := Compile(graph, nil, DefaultRepository())
		So(err, ShouldBeNil)
		server := runtime.StageNode_NewServer(program)
		server.NewArena = func() capnp.Arena { return capnp.MultiSegment(nil) }
		client := runtime.StageNode(capnp.NewClient(server))
		defer client.Release()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		step := 0
		for step < 5 && ctx.Err() == nil {
			future, release := client.Step(ctx, func(params runtime.StageNode_step_Params) error {
				bindings, err := params.NewBindings(6)
				if err != nil {
					return err
				}
				for index, binding := range []struct{ node, field, target string }{
					{"regions", "settled.vocabulary", ""}, {"state", "values", "authority.prior"},
					{"state", "found", "authority.known"}, {"regions", "settled.regions", "activation.labels"},
					{"regions", "settled.vocabulary", "contexts.vocabulary"},
					{"regions", "settled.vocabulary", "query.parameters_1"},
				} {
					target := bindings.At(index)
					target.SetGate(index == 0)
					for _, err := range []error{target.SetProducer("map"), target.SetNode(binding.node), target.SetField(binding.field), target.SetTarget(binding.target)} {
						if err != nil {
							return err
						}
					}
				}
				outputs, err := params.NewOutputs(3)
				if err != nil {
					return err
				}
				if err := outputs.Set(0, "gather"); err != nil {
					return err
				}
				if err := outputs.Set(1, "observation"); err != nil {
					return err
				}
				if err := outputs.Set(2, "query"); err != nil {
					return err
				}
				upstream, err := params.NewUpstream(2)
				if err != nil {
					return err
				}
				state, err := store.NewVector_done_Results(params.Segment())
				if err != nil {
					return err
				}
				values, err := state.NewValues(6)
				if err != nil {
					return err
				}
				for index, value := range []float64{10, 20, 5, 10, 30, 6} {
					values.Set(index, value)
				}
				known, err := state.NewFound(2)
				if err != nil {
					return err
				}
				known.Set(0, true)
				known.Set(1, true)
				region, err := geometry.NewWatershed(params.Segment())
				if err != nil {
					return err
				}
				region.SetMoving()
				if step > 0 {
					region.SetSettled()
					if err := region.Settled().SetVocabulary("v1"); err != nil {
						return err
					}
					labels, err := region.Settled().NewRegions(2)
					if err != nil {
						return err
					}
					if err := labels.Set(0, "0"); err != nil {
						return err
					}
					if err := labels.Set(1, "1"); err != nil {
						return err
					}
				}
				for index, value := range []capnp.Struct{capnp.Struct(state), capnp.Struct(region)} {
					result := upstream.At(index)
					name, identity := "state", uint64(store.Vector_TypeID)
					if index == 1 {
						name, identity = "regions", geometry.Peak_TypeID
					}
					result.SetInterfaceId(identity)
					if err := result.SetProducer("map"); err != nil {
						return err
					}
					if err := result.SetNode(name); err != nil {
						return err
					}
					if err := result.SetValue(value.ToPtr()); err != nil {
						return err
					}
				}
				return nil
			})
			completed, err := future.Struct()
			So(err, ShouldBeNil)
			outputs, err := completed.Outputs()
			So(err, ShouldBeNil)
			if step == 0 {
				So(outputs.Len(), ShouldEqual, 0)
				release()
				step++
				continue
			}
			var rows []byte
			for index := range outputs.Len() {
				name, err := outputs.At(index).Node()
				So(err, ShouldBeNil)
				if name == "query" {
					value, err := outputs.At(index).Value()
					So(err, ShouldBeNil)
					rows, err = tables.Query_done_Results(value.Struct()).Out()
					So(err, ShouldBeNil)
				}
			}
			if len(rows) == 0 {
				release()
				time.Sleep(time.Millisecond)
				continue
			}
			if step == 4 {
				So(string(rows), ShouldEqual, "[]")
				for index := range outputs.Len() {
					name, err := outputs.At(index).Node()
					So(err, ShouldBeNil)
					So(name, ShouldBeIn, "gather", "observation", "query")
					value, err := outputs.At(index).Value()
					So(err, ShouldBeNil)
					if name == "gather" {
						So(data.Gathered(value.Struct()).Which(), ShouldEqual, data.Gathered_Which_idle)
					}
					if name == "observation" {
						So(data.Extracted(value.Struct()).Which(), ShouldEqual, data.Extracted_Which_missing)
					}
				}
				release()
				step++
				continue
			}
			So(outputs.Len(), ShouldEqual, 3)
			for index := range outputs.Len() {
				output := outputs.At(index)
				name, err := output.Node()
				So(err, ShouldBeNil)
				value, err := output.Value()
				So(err, ShouldBeNil)
				if name == "gather" {
					cut := data.Gathered(value.Struct())
					So(cut.Epoch(), ShouldEqual, epoch)
					So(cut.Sequence(), ShouldEqual, step)
					values, err := cut.Gathered().Values()
					So(err, ShouldBeNil)
					So(values.At(0), ShouldEqual, float64(step*step))
				}
				if name == "observation" {
					encoded, err := data.Extracted(value.Struct()).Json()
					So(err, ShouldBeNil)
					var observation struct {
						Cursor struct{ Sequence int64 }
						Event  struct{ D struct{ Sequence int64 } }
					}
					So(json.Unmarshal(encoded, &observation), ShouldBeNil)
					So(observation.Cursor.Sequence, ShouldEqual, step)
					So(observation.Event.D.Sequence, ShouldEqual, 4)
				}
			}
			release()
			step++
		}
		So(ctx.Err(), ShouldBeNil)
		So(step, ShouldEqual, 5)
	})
}

/* TestProgramStepMarkets proves symbol isolation through the shipping LMAX metric groups. */
func TestProgramStepMarkets(t *testing.T) {
	Convey("Alternating markets survive ring wrap without sharing metric history", t, func() {
		graph, err := DefaultRepository().Load("system")
		So(err, ShouldBeNil)
		for identifier, node := range graph.Nodes {
			if !strings.HasPrefix(node.Type, "runtime.") && !strings.HasSuffix(identifier, "_graph") {
				withoutNodes(graph, identifier)
			}
		}
		withoutLearningStages(graph)
		workspaceNode := graph.Nodes["workspace"]
		workspaceNode.InputData["capacity"] = json.RawMessage(`8`)
		graph.Nodes["workspace"] = workspaceNode
		program, err := Compile(graph, nil, DefaultRepository())
		So(err, ShouldBeNil)
		defer program.Release()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		So(program.Execute(ctx, nil), ShouldBeNil)
		workspace := runtime.Workspace(program.Nodes[program.NodeMap["workspace"]].Client)
		// Sixty-four alternating ticks wrap the eight-slot ring eight times.
		So(workspace.Write(ctx, func(args runtime.Workspace_write_Params) error {
			arrivals, err := args.NewData(64)
			if err != nil {
				return err
			}
			for sequence := range 64 {
				symbol, price := "BTC/USD", float64(100+sequence)
				if sequence%2 == 1 {
					symbol, price = "ETH/USD", float64(1000+10*sequence)
				}
				payload := fmt.Sprintf(`{"channel":"ticker","capture":{"session":"spot","sequence":%d,"record":0,"endpoint":"wss://fixture","receivedAt":"2026-09-26T00:00:00Z"},"data":{"symbol":%q,"last":%g,"bid":%g,"ask":%g}}`, sequence, symbol, price, price-1, price+1)
				if err := arrivals.Set(sequence, []byte(payload)); err != nil {
					return err
				}
			}
			return nil
		}), ShouldBeNil)
		So(workspace.WaitStreaming(), ShouldBeNil)
		So(program.Flush(ctx), ShouldBeNil)
		consumer := runtime.Consumer(program.Nodes[program.NodeMap["definition-pumpdump_ticker_consumer"]].Client)
		future, release := consumer.Done(ctx, nil)
		defer release()
		completed, err := future.Struct()
		So(err, ShouldBeNil)
		So(completed.Completed(), ShouldEqual, 64)
		outputs, err := completed.Outputs()
		So(err, ShouldBeNil)
		found := false
		for index := range outputs.Len() {
			output := outputs.At(index)
			name, err := output.Node()
			So(err, ShouldBeNil)
			if name != "midpoint_log_return" {
				continue
			}
			found = true
			value, err := output.Value()
			So(err, ShouldBeNil)
			So(temporal.LogReturns_done_Results(value.Struct()).Out(), ShouldAlmostEqual, math.Log(1630.0/1610.0))
			So(output.Sequence(), ShouldEqual, 63)
		}
		So(found, ShouldBeTrue)
		cutConsumer := runtime.Consumer(program.Nodes[program.NodeMap["cut_consumer"]].Client)
		cutFuture, release := cutConsumer.Done(ctx, nil)
		defer release()
		cutDone, err := cutFuture.Struct()
		So(err, ShouldBeNil)
		cuts, err := cutDone.Outputs()
		So(err, ShouldBeNil)
		So(cuts.Len(), ShouldEqual, 1)
		pointer, err := cuts.At(0).Value()
		So(err, ShouldBeNil)
		cut := data.Gathered(pointer.Struct())
		So(cut.Which(), ShouldEqual, data.Gathered_Which_idle)
		row, err := cut.Row()
		So(err, ShouldBeNil)
		storedPointer, err := row.Value()
		So(err, ShouldBeNil)
		stored := data.MetricCut(storedPointer.Struct())
		symbol, err := stored.Symbol()
		So(err, ShouldBeNil)
		So(symbol, ShouldEqual, "ETH/USD")
		So(stored.Sequence(), ShouldEqual, 63)
		metrics, err := stored.Metrics()
		So(err, ShouldBeNil)
		for index := range metrics.Len() {
			metric := metrics.At(index)
			if metric.Present() {
				So(metric.Sequence()%2, ShouldEqual, 1)
			}
		}
		for _, identifier := range []string{"definition-pumpdump_ticker_graph", "cut_graph"} {
			factory := runtime.StageFactory(program.Nodes[program.NodeMap[identifier]].Client)
			future, release := factory.Done(ctx, nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Partitions(), ShouldEqual, 2)
			release()
		}
	})
}

/* TestProgramStepSharedGrid preserves per-market differences in the one shared region graph. */
func TestProgramStepSharedGrid(t *testing.T) {
	Convey("The shared grid compares each metric with the same market's previous cut", t, func() {
		graph, err := DefaultRepository().Load("impulse_map")
		So(err, ShouldBeNil)
		for identifier := range graph.Nodes {
			if identifier != "previous" && identifier != "change" {
				withoutNodes(graph, identifier)
			}
		}
		program, err := Compile(graph, nil, DefaultRepository())
		So(err, ShouldBeNil)
		client := runtime.StageNode_ServerToClient(program)
		defer client.Release()
		for _, sample := range []struct {
			symbol        string
			value, change float64
			defined       bool
		}{
			{"BTC/USD", 10, 0, false}, {"ETH/USD", 1000, 0, false}, {"BTC/USD", 13, 3, true}, {"ETH/USD", 990, -10, true}, {"BTC/USD", 18, 5, true},
		} {
			future, release := client.Step(context.Background(), func(args runtime.StageNode_step_Params) error {
				bindings, err := args.NewBindings(3)
				if err != nil {
					return err
				}
				for index, binding := range []struct{ field, target string }{{"values", "change.value"}, {"scope", "previous.scope_0"}, {"present", "change.present"}} {
					for _, err := range []error{bindings.At(index).SetProducer("cut"), bindings.At(index).SetNode("grid"), bindings.At(index).SetField(binding.field), bindings.At(index).SetTarget(binding.target)} {
						if err != nil {
							return err
						}
					}
				}
				upstream, err := args.NewUpstream(1)
				if err != nil {
					return err
				}
				result := upstream.At(0)
				result.SetInterfaceId(store.Grid_TypeID)
				if err := result.SetProducer("cut"); err != nil {
					return err
				}
				if err := result.SetNode("grid"); err != nil {
					return err
				}
				grid, err := store.NewGrid_done_Results(args.Segment())
				if err != nil {
					return err
				}
				if err := grid.SetScope(sample.symbol); err != nil {
					return err
				}
				values, err := grid.NewValues(1)
				if err != nil {
					return err
				}
				values.Set(0, sample.value)
				present, err := grid.NewPresent(1)
				if err != nil {
					return err
				}
				present.Set(0, true)
				if err := result.SetValue(capnp.Struct(grid).ToPtr()); err != nil {
					return err
				}
				selected, err := args.NewOutputs(1)
				if err != nil {
					return err
				}
				return selected.Set(0, "change")
			})
			result, err := future.Struct()
			So(err, ShouldBeNil)
			outputs, err := result.Outputs()
			So(err, ShouldBeNil)
			So(outputs.Len(), ShouldEqual, 1)
			pointer, err := outputs.At(0).Value()
			So(err, ShouldBeNil)
			changed := calculus.Changed(pointer.Struct())
			values, err := changed.Change()
			So(err, ShouldBeNil)
			defined, err := changed.Defined()
			So(err, ShouldBeNil)
			So(values.Len(), ShouldEqual, 1)
			So(defined.At(0), ShouldEqual, sample.defined)
			So(values.At(0), ShouldEqual, sample.change)
			release()
		}
	})
}

/* TestProgramStepCapture verifies the shipping bindings preserve raw frames and provenance. */
func TestProgramStepCapture(t *testing.T) {
	Convey("Source group results reach the one raw archive through native bindings", t, func() {
		ctx := context.Background()
		graph, err := DefaultRepository().Load("raw_capture")
		So(err, ShouldBeNil)
		withoutNodes(graph, "capture")
		program, err := Compile(graph, nil, DefaultRepository())
		So(err, ShouldBeNil)
		stage := runtime.StageNode_ServerToClient(program)
		defer stage.Release()
		root, err := DefaultRepository().Load("system")
		So(err, ShouldBeNil)
		var configuration struct {
			Value string `json:"value"`
		}
		So(json.Unmarshal(root.Nodes["raw_capture_consumer"].InputData["bindings"], &configuration), ShouldBeNil)
		var bindings []struct{ Producer, Node, Field, Target string }
		So(json.Unmarshal([]byte(configuration.Value), &bindings), ShouldBeNil)
		for generation := range 2 {
			for ordinal, feed := range []string{"spot", "level3", "futures"} {
				payload := fmt.Sprintf("snapshot:%s:%d", feed, generation)
				future, release := stage.Step(ctx, func(params runtime.StageNode_step_Params) error {
					params.SetEpoch(91)
					params.SetSequence(int64(generation*3 + ordinal))
					selected, err := params.NewOutputs(1)
					if err != nil {
						return err
					}
					if err := selected.Set(0, "envelope"); err != nil {
						return err
					}
					inputs, err := params.NewBindings(int32(len(bindings)))
					if err != nil {
						return err
					}
					for index, binding := range bindings {
						for _, err := range []error{inputs.At(index).SetProducer(binding.Producer), inputs.At(index).SetNode(binding.Node), inputs.At(index).SetField(binding.Field), inputs.At(index).SetTarget(binding.Target)} {
							if err != nil {
								return err
							}
						}
					}
					upstream, err := params.NewUpstream(1)
					if err != nil {
						return err
					}
					output := upstream.At(0)
					output.SetEpoch(91)
					output.SetSequence(int64(generation*3 + ordinal))
					output.SetInterfaceId(websocket.WebSocketClient_TypeID)
					name := "socket"
					if feed == "level3" {
						name = "shards"
						output.SetInterfaceId(websocket.Shards_TypeID)
					}
					if err := output.SetProducer(feed); err != nil {
						return err
					}
					if err := output.SetNode(name); err != nil {
						return err
					}
					received, err := websocket.NewReceived(params.Segment())
					if err != nil {
						return err
					}
					received.SetFrame()
					if err := received.Frame().SetRead([]byte(payload)); err != nil {
						return err
					}
					provenance := fmt.Sprintf(`{"session":%q,"sequence":%d,"endpoint":%q,"receivedAt":"2026-09-26T00:00:00Z"}`, feed, generation, "wss://fixture/"+feed)
					if err := received.Frame().SetProvenance([]byte(provenance)); err != nil {
						return err
					}
					return output.SetValue(capnp.Struct(received).ToPtr())
				})
				result, err := future.Struct()
				So(err, ShouldBeNil)
				outputs, err := result.Outputs()
				So(err, ShouldBeNil)
				So(outputs.Len(), ShouldEqual, 1)
				pointer, err := outputs.At(0).Value()
				So(err, ShouldBeNil)
				captured := store.Captured(pointer.Struct())
				So(captured.Which(), ShouldEqual, store.Captured_Which_row)
				actual, err := captured.Row().Payload()
				So(err, ShouldBeNil)
				So(string(actual), ShouldEqual, payload)
				release()
			}
		}
	})
}

func TestProgramRecord(t *testing.T) {
	Convey("An authored source output preserves Data and union presence", t, func() {
		program, err := CompileJSON([]byte(`{"nodes":{"source":{"id":"source","type":"controlflow.Once","inputData":{"trigger":true,"through":"observation"}},"numeric":{"id":"numeric","type":"arithmetic.Add","inputData":{"a":1,"b":2}}}}`), nil, nil)
		So(err, ShouldBeNil)
		defer program.Release()
		So(program.Execute(context.Background(), nil), ShouldBeNil)
		payload, err := program.record("source.out")
		So(err, ShouldBeNil)
		So(string(payload), ShouldEqual, "observation")
		for _, address := range []string{"missing.out", "source.missing", "numeric.out"} {
			_, err := program.record(address)
			So(err, ShouldNotBeNil)
		}
		So(program.Execute(context.Background(), nil), ShouldBeNil)
		payload, err = program.record("source.out")
		So(err, ShouldBeNil)
		So(payload, ShouldBeEmpty)
	})
}

func TestProgramStepBookSignals(t *testing.T) {
	Convey("Production groups compute book, trade and derivatives signals through a market reversal", t, func() {
		graph, err := DefaultRepository().Load("system")
		So(err, ShouldBeNil)
		for identifier, node := range graph.Nodes {
			if !strings.HasPrefix(node.Type, "runtime.") && !strings.HasSuffix(identifier, "_graph") {
				withoutNodes(graph, identifier)
			}
		}
		withoutLearningStages(graph)
		program, err := Compile(graph, nil, DefaultRepository())
		So(err, ShouldBeNil)
		defer program.Release()
		ctx := context.Background()
		So(program.Execute(ctx, nil), ShouldBeNil)
		workspace := runtime.Workspace(program.Nodes[program.NodeMap["workspace"]].Client)
		sequence := 0
		for index, price := range marketfixture.Reversal() {
			stamp := time.Unix(1700000000+int64(index), 0).UTC().Format(time.RFC3339Nano)
			// Vary displayed depth and trade direction independently of the
			// rise/fall/recovery to exercise nonconstant fractions and spreads.
			spread, quantity := float64(index%3+1), float64(index%5+1)
			orders := [2][]marketfixture.Order{
				{{fmt.Sprint(price - spread), fmt.Sprint(quantity), stamp}},
				{{fmt.Sprint(price + spread), "3", stamp}},
			}
			bookFrame := marketfixture.Level3Frame("snapshot", "BTC/USD", [2][]marketfixture.Order{}, orders, "")
			var bookRecord map[string]any
			So(json.Unmarshal(bookFrame, &bookRecord), ShouldBeNil)
			bookRecord["capture"] = map[string]any{"session": "fixture", "sequence": sequence, "record": 0, "endpoint": "wss://fixture", "receivedAt": stamp}
			bookFrame, err = json.Marshal(bookRecord)
			So(err, ShouldBeNil)
			side, tradePrice := "sell", price-spread
			if index%2 == 1 {
				side, tradePrice = "buy", price+spread
			}
			capture := fmt.Sprintf(`"capture":{"session":"fixture","sequence":%d,"record":0,"endpoint":"wss://fixture","receivedAt":%q}`, sequence, stamp)
			frames := [][]byte{
				bookFrame,
				[]byte(fmt.Sprintf(`{"channel":"ticker",%s,"data":{"symbol":"BTC/USD","last":%g,"bid":%g,"ask":%g}}`, capture, price, price-spread, price+spread)),
				[]byte(fmt.Sprintf(`{"channel":"trade",%s,"data":{"symbol":"BTC/USD","side":%q,"price":%g,"qty":%g,"timestamp":%q}}`, capture, side, tradePrice, quantity, stamp)),
				[]byte(fmt.Sprintf(`{"channel":"futures_ticker",%s,"data":{"symbol":"BTC/USD","last":%g,"index":%g,"openInterest":%g}}`, capture, price+spread, price, quantity+100)),
			}
			for _, frame := range frames {
				So(workspace.Write(ctx, func(args runtime.Workspace_write_Params) error {
					arrivals, err := args.NewData(1)
					if err != nil {
						return err
					}
					return arrivals.Set(0, frame)
				}), ShouldBeNil)
				So(workspace.WaitStreaming(), ShouldBeNil)
				sequence++
			}
		}
		So(program.Flush(ctx), ShouldBeNil)
		consumer := runtime.Consumer(program.Nodes[program.NodeMap["cut_consumer"]].Client)
		future, release := consumer.Done(ctx, nil)
		defer release()
		result, err := future.Struct()
		So(err, ShouldBeNil)
		outputs, err := result.Outputs()
		So(err, ShouldBeNil)
		So(outputs.Len(), ShouldEqual, 1)
		pointer, err := outputs.At(0).Value()
		So(err, ShouldBeNil)
		row, err := data.Gathered(pointer.Struct()).Row()
		So(err, ShouldBeNil)
		stored, err := row.Value()
		So(err, ShouldBeNil)
		metrics, err := data.MetricCut(stored.Struct()).Metrics()
		So(err, ShouldBeNil)
		observed := map[string]bool{}
		for index := range metrics.Len() {
			identity, err := metrics.At(index).Identity()
			So(err, ShouldBeNil)
			observed[identity] = metrics.At(index).Present()
		}
		for _, identity := range []string{
			"depthflow_level3:observed_notional_imbalance_zscore.out",
			"pumpdump_level3:spread_zscore.out",
			"cvd_trade:signed_net_fraction_zscore.out",
			"toxicity_trade:fill_fraction_zscore:bid.out",
			"toxicity_trade:fill_fraction_zscore:ask.out",
			"derivatives_ticker:basis_zscore.out",
		} {
			So(observed[identity], ShouldBeTrue)
		}
	})
}

func TestProgramStepCohort(t *testing.T) {
	Convey("The shipping sentiment stage sees the latest return from every initialized market", t, func() {
		graph, err := DefaultRepository().Load("system")
		So(err, ShouldBeNil)
		for identifier, node := range graph.Nodes {
			if !strings.HasPrefix(node.Type, "runtime.") && !strings.HasSuffix(identifier, "_graph") {
				withoutNodes(graph, identifier)
			}
		}
		withoutLearningStages(graph)
		program, err := Compile(graph, nil, DefaultRepository())
		So(err, ShouldBeNil)
		defer program.Release()
		ctx := context.Background()
		So(program.Execute(ctx, nil), ShouldBeNil)
		workspace := runtime.Workspace(program.Nodes[program.NodeMap["workspace"]].Client)
		for sequence, price := range []float64{100, 100, 100, 110, 110, 90, 121} {
			symbol := []string{"BTC/USD", "ETH/USD", "SOL/USD"}[sequence%3]
			So(workspace.Write(ctx, func(args runtime.Workspace_write_Params) error {
				arrivals, err := args.NewData(1)
				if err != nil {
					return err
				}
				return arrivals.Set(0, []byte(fmt.Sprintf(`{"channel":"ticker","capture":{"session":"spot","sequence":%d,"record":0,"endpoint":"wss://fixture","receivedAt":"2026-09-26T00:00:00Z"},"data":{"symbol":%q,"last":%g,"bid":%g,"ask":%g}}`, sequence, symbol, price, price-1, price+1)))
			}), ShouldBeNil)
			So(workspace.WaitStreaming(), ShouldBeNil)
		}
		So(program.Flush(ctx), ShouldBeNil)
		consumer := runtime.Consumer(program.Nodes[program.NodeMap["definition-sentiment_ticker_consumer"]].Client)
		future, release := consumer.Done(ctx, nil)
		defer release()
		result, err := future.Struct()
		So(err, ShouldBeNil)
		outputs, err := result.Outputs()
		So(err, ShouldBeNil)
		found := false
		peers := map[string]float64{}
		for index := range outputs.Len() {
			name, err := outputs.At(index).Node()
			So(err, ShouldBeNil)
			pointer, err := outputs.At(index).Value()
			So(err, ShouldBeNil)
			if name == "same_direction_peer_count" {
				peers[name] = arithmetic.Subtract_done_Results(pointer.Struct()).Out()
			}
			if name == "opposite_direction_peer_count" {
				peers[name] = arithmetic.Quotient(pointer.Struct()).Out()
			}
			if name != "crossSection" {
				continue
			}
			found = true
			section := statistic.Order_done_Results(pointer.Struct())
			So(section.Count(), ShouldEqual, 3)
			So(section.Positive(), ShouldEqual, 2)
			So(section.Negative(), ShouldEqual, 1)
			So(section.Zero(), ShouldEqual, 0)
			So(section.Median(), ShouldAlmostEqual, math.Log(1.1))
			So(section.ExtremeMagnitude(), ShouldAlmostEqual, -math.Log(0.9))
		}
		So(found, ShouldBeTrue)
		So(peers, ShouldResemble, map[string]float64{"same_direction_peer_count": 1, "opposite_direction_peer_count": 1})
	})
}

/* stageBindingFixture uses the production cut width and native scalar results. */
func stageBindingFixture(t testing.TB) (*Program, runtime.StageNode_step_Params, int) {
	t.Helper()
	graph, err := DefaultRepository().Load("cut")
	if err != nil {
		t.Fatal(err)
	}
	var identities struct {
		Value []string `json:"value"`
	}
	if err := json.Unmarshal(graph.Nodes["gather"].InputData["identities"], &identities); err != nil {
		t.Fatal(err)
	}
	count := len(identities.Value)
	program, err := CompileJSON([]byte(`{"nodes":{"grid":{"id":"grid","type":"store.Grid"}}}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(program.Release)
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(message.Release)
	message.ResetReadLimit(math.MaxUint64)
	input, err := runtime.NewStageNode_step_Params(segment)
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := input.NewBindings(int32(count))
	if err != nil {
		t.Fatal(err)
	}
	upstream, err := input.NewUpstream(int32(count))
	if err != nil {
		t.Fatal(err)
	}
	for index, identity := range identities.Value {
		producer, node, found := strings.Cut(identity, ":")
		if !found {
			t.Fatalf("metric has no producer: %s", identity)
		}
		node = fmt.Sprintf("%s/%d", node, index)
		binding := bindings.At(index)
		result := upstream.At(count - index - 1)
		for _, err := range []error{binding.SetProducer(producer), binding.SetNode(node), binding.SetField("out"), binding.SetTarget(fmt.Sprintf("grid.metrics_%d", index)), result.SetProducer(producer), result.SetNode(node)} {
			if err != nil {
				t.Fatal(err)
			}
		}
		result.SetInterfaceId(arithmetic.Add_TypeID)
		value, err := arithmetic.NewAdd_done_Results(segment)
		if err != nil {
			t.Fatal(err)
		}
		value.SetOut(float64(index + 1))
		if err := result.SetValue(capnp.Struct(value).ToPtr()); err != nil {
			t.Fatal(err)
		}
	}
	return program, input, count
}

func TestProgramSeedBindings(t *testing.T) {
	Convey("Every native metric finds its producer even when upstream order is reversed", t, func() {
		program, input, count := stageBindingFixture(t)
		for pass := range 2 {
			frames := make([]nodeFrame, len(program.Nodes))
			ready, err := program.seedBindings(frames, input)
			So(err, ShouldBeNil)
			So(ready, ShouldBeTrue)
			args := store.Grid_write_Params(frames[program.NodeMap["grid"]].args)
			values, err := args.Metrics()
			So(err, ShouldBeNil)
			present, err := args.Present()
			So(err, ShouldBeNil)
			So(values.Len(), ShouldEqual, count)
			for index := range count {
				So(present.At(index), ShouldEqual, pass == 0 || index%2 == 0)
				if present.At(index) {
					So(values.At(index), ShouldEqual, index+1)
				}
			}
			args.Message().Release()
			upstream, err := input.Upstream()
			So(err, ShouldBeNil)
			for index := range count {
				if index%2 == 1 {
					So(upstream.At(count-index-1).SetProducer("absent"), ShouldBeNil)
				}
			}
		}
	})
}

func BenchmarkProgramSeedBindings(b *testing.B) {
	program, input, _ := stageBindingFixture(b)
	b.ReportAllocs()
	for b.Loop() {
		frames := make([]nodeFrame, len(program.Nodes))
		if _, err := program.seedBindings(frames, input); err != nil {
			b.Fatal(err)
		}
		for _, frame := range frames {
			if frame.args.IsValid() {
				frame.args.Message().Release()
			}
		}
	}
}

func TestProgramStepRelations(t *testing.T) {
	Convey("The shipping graph sends cross-market correlation and measured lags into the cut", t, func() {
		graph, err := DefaultRepository().Load("system")
		So(err, ShouldBeNil)
		for identifier, node := range graph.Nodes {
			if !strings.HasPrefix(node.Type, "runtime.") && !strings.HasSuffix(identifier, "_graph") {
				withoutNodes(graph, identifier)
			}
		}
		withoutLearningStages(graph)
		program, err := Compile(graph, nil, DefaultRepository())
		So(err, ShouldBeNil)
		defer program.Release()
		ctx := context.Background()
		So(program.Execute(ctx, nil), ShouldBeNil)
		workspace := runtime.Workspace(program.Nodes[program.NodeMap["workspace"]].Client)
		prices := marketfixture.Reversal()
		sequence := 0
		for index := range prices {
			for market, symbol := range []string{"BTC/USD", "ETH/USD", "SOL/USD"} {
				price := prices[max(0, index-market*2)] + float64((index+market)%5)/10
				stamp := time.Unix(1700000000+int64(index), 0).UTC().Format(time.RFC3339Nano)
				frame := []byte(fmt.Sprintf(`{"channel":"ticker","capture":{"session":"spot","sequence":%d,"record":0,"endpoint":"wss://fixture","receivedAt":%q},"data":{"symbol":%q,"last":%g,"bid":%g,"ask":%g}}`, sequence, stamp, symbol, price, price-1, price+1))
				So(workspace.Write(ctx, func(args runtime.Workspace_write_Params) error {
					arrivals, err := args.NewData(1)
					if err != nil {
						return err
					}
					return arrivals.Set(0, frame)
				}), ShouldBeNil)
				So(workspace.WaitStreaming(), ShouldBeNil)
				sequence++
			}
		}
		So(program.Flush(ctx), ShouldBeNil)
		consumer := runtime.Consumer(program.Nodes[program.NodeMap["cut_consumer"]].Client)
		future, release := consumer.Done(ctx, nil)
		defer release()
		result, err := future.Struct()
		So(err, ShouldBeNil)
		outputs, err := result.Outputs()
		So(err, ShouldBeNil)
		So(outputs.Len(), ShouldEqual, 1)
		pointer, err := outputs.At(0).Value()
		So(err, ShouldBeNil)
		row, err := data.Gathered(pointer.Struct()).Row()
		So(err, ShouldBeNil)
		stored, err := row.Value()
		So(err, ShouldBeNil)
		metrics, err := data.MetricCut(stored.Struct()).Metrics()
		So(err, ShouldBeNil)
		observed := map[string]bool{}
		for index := range metrics.Len() {
			identity, err := metrics.At(index).Identity()
			So(err, ShouldBeNil)
			observed[identity] = metrics.At(index).Present()
		}
		for _, identity := range []string{"correlation_ticker:hy.correlation", "correlation_ticker:zscore.out", "correlation_ticker:cohortSigned.mean", "leadlag_ticker:best_lag_correlation.out", "leadlag_ticker:best_lag_seconds.out", "leadlag_ticker:search_count.out"} {
			SoMsg(identity, observed[identity], ShouldBeTrue)
		}
	})
}

func TestProgramSeedBindingsDuplicate(t *testing.T) {
	Convey("Ambiguous producer identities fail before routing any values", t, func() {
		program, input, _ := stageBindingFixture(t)
		upstream, err := input.Upstream()
		So(err, ShouldBeNil)
		producer, err := upstream.At(0).Producer()
		So(err, ShouldBeNil)
		node, err := upstream.At(0).Node()
		So(err, ShouldBeNil)
		So(upstream.At(1).SetProducer(producer), ShouldBeNil)
		So(upstream.At(1).SetNode(node), ShouldBeNil)
		_, err = program.seedBindings(make([]nodeFrame, len(program.Nodes)), input)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "duplicate upstream")
	})
}

func TestProgramCopyStamp(t *testing.T) {
	Convey("Exact native observation counters reach native inputs and UI nodes", t, func() {
		program, err := CompileJSON([]byte(`{"nodes":{"sequence":{"id":"sequence","type":"store.Sequence"}}}`), nil)
		So(err, ShouldBeNil)
		defer program.Release()
		message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)
		defer message.Release()
		result, err := runtime.NewResult(segment)
		So(err, ShouldBeNil)
		result.SetEpoch(9007199254740993)
		result.SetSequence(9007199254740995)
		result.SetCompleted(18446744073709551615)
		binding, err := runtime.NewBinding(segment)
		So(err, ShouldBeNil)
		binding.SetStamp(runtime.Stamp_completed)
		So(binding.SetTarget("sequence.index"), ShouldBeNil)
		frames := make([]nodeFrame, len(program.Nodes))
		present, err := program.copyStamp(frames, binding, result)
		So(err, ShouldBeNil)
		So(present, ShouldBeTrue)
		args := store.Sequence_write_Params(frames[program.NodeMap["sequence"]].args)
		So(args.Index(), ShouldEqual, uint64(18446744073709551615))
		defer args.Message().Release()
		program.Bindings = &BindingPlan{Inputs: map[string]Origin{"stage": {Definition: "ui_diagnostics", Node: "stage"}}}
		for _, test := range []struct {
			stamp    runtime.Stamp
			expected string
		}{
			{runtime.Stamp_epoch, `"9007199254740993"`},
			{runtime.Stamp_sequence, `"9007199254740995"`},
			{runtime.Stamp_completed, `"18446744073709551615"`},
		} {
			binding.SetStamp(test.stamp)
			So(binding.SetTarget("stage.completed"), ShouldBeNil)
			present, err := program.copyStamp(frames, binding, result)
			So(err, ShouldBeNil)
			So(present, ShouldBeTrue)
			So(program.stageArrivals[len(program.stageArrivals)-1].value, ShouldEqual, test.expected)
		}
		Convey("Signed stamps cannot silently become unsigned counts", func() {
			binding.SetStamp(runtime.Stamp_epoch)
			So(binding.SetTarget("sequence.index"), ShouldBeNil)
			_, err := program.copyStamp(frames, binding, result)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestProgramSeedBindingsListSlots(t *testing.T) {
	Convey("Numbered list results merge into the wider cut without replacing scalar neighbours", t, func() {
		program, input, count := stageBindingFixture(t)
		bindings, err := input.Bindings()
		So(err, ShouldBeNil)
		upstream, err := input.Upstream()
		So(err, ShouldBeNil)
		result := upstream.At(count - 1)
		result.SetInterfaceId(store.Grid_TypeID)
		wire, err := store.NewGrid_done_Results(input.Segment())
		So(err, ShouldBeNil)
		values, err := wire.NewValues(2)
		So(err, ShouldBeNil)
		values.Set(0, 42)
		values.Set(1, -7)
		presence, err := wire.NewPresent(2)
		So(err, ShouldBeNil)
		presence.Set(0, true)
		So(result.SetValue(capnp.Struct(wire).ToPtr()), ShouldBeNil)
		for _, field := range []string{"values_0", "values_1"} {
			So(bindings.At(0).SetField(field), ShouldBeNil)
			frames := make([]nodeFrame, len(program.Nodes))
			ready, err := program.seedBindings(frames, input)
			So(err, ShouldBeNil)
			So(ready, ShouldBeTrue)
			args := store.Grid_write_Params(frames[program.NodeMap["grid"]].args)
			actual, err := args.Metrics()
			So(err, ShouldBeNil)
			present, err := args.Present()
			So(err, ShouldBeNil)
			So(actual.Len(), ShouldEqual, count)
			So(present.At(0), ShouldEqual, field == "values_0")
			if present.At(0) {
				So(actual.At(0), ShouldEqual, 42)
			}
			for index := 1; index < count; index++ {
				So(actual.At(index), ShouldEqual, index+1)
				So(present.At(index), ShouldBeTrue)
			}
			args.Message().Release()
		}
	})
}

/* TestProgramStepCompleteMarket exercises every required signal coordinate with all authored feeds. */
func TestProgramStepCompleteMarket(t *testing.T) {
	Convey("Every required metric initializes on complete multi-market feed observations", t, func() {
		graph, err := DefaultRepository().Load("system")
		So(err, ShouldBeNil)
		for identifier, node := range graph.Nodes {
			if !strings.HasPrefix(node.Type, "runtime.") && !strings.HasSuffix(identifier, "_graph") {
				withoutNodes(graph, identifier)
			}
		}
		withoutLearningStages(graph)
		program, err := Compile(graph, nil, DefaultRepository())
		So(err, ShouldBeNil)
		defer program.Release()
		ctx := context.Background()
		So(program.Execute(ctx, nil), ShouldBeNil)
		workspace := runtime.Workspace(program.Nodes[program.NodeMap["workspace"]].Client)
		sequence := 0
		prices := marketfixture.Reversal()
		previousBooks := make(map[string][2][]marketfixture.Order)
		low, high := slices.Min(prices)-10, slices.Max(prices)+10
		for index := range prices {
			for ordinal, symbol := range []string{"BTC/USD", "ETH/USD", "SOL/USD"} {
				price := prices[max(0, index-ordinal*2)] + float64((index+ordinal)%5)/10
				stamp := time.Unix(1700000000+int64(index*index+ordinal), 0).UTC().Format(time.RFC3339Nano)
				spread, quantity := float64((index+ordinal)%3+1), float64((index+ordinal)%5+1)
				orders := [2][]marketfixture.Order{
					{{fmt.Sprint(price - spread), fmt.Sprint(quantity), stamp}, {fmt.Sprint(price - spread - 1), fmt.Sprint(quantity + 1), stamp}},
					{{fmt.Sprint(price + spread), fmt.Sprint(6 - quantity), stamp}, {fmt.Sprint(price + spread + 1), fmt.Sprint(quantity + 2), stamp}},
				}
				anchors := [2][]marketfixture.Order{{{fmt.Sprint(low), "10", "2023-11-14T22:00:00Z"}}, {{fmt.Sprint(high), "10", "2023-11-14T22:00:00Z"}}}
				var bookFrames [][]byte
				previous, initialized := previousBooks[symbol]
				if initialized {
					// The fixture alternates same-price depletion and touch retreat,
					// so withdrawal and retreat statistics both see differing regimes.
					modified := [2][]marketfixture.Order{{previous[0][0]}, {previous[1][0]}}
					for side := range modified {
						quantity, err := strconv.ParseFloat(modified[side][0].Quantity, 64)
						So(err, ShouldBeNil)
						modified[side][0].Quantity = fmt.Sprint(quantity * float64((index+side)%3+1) / 4)
					}
					bookFrames = append(bookFrames, marketfixture.Level3Frame("update", symbol, previous, modified, "modify"))
					previous[0][0], previous[1][0] = modified[0][0], modified[1][0]
					removed := [2][]marketfixture.Order{previous[0][:2], previous[1][:2]}
					bookFrames = append(bookFrames, marketfixture.Level3Frame("update", symbol, previous, removed, "delete"))
					bookFrames = append(bookFrames, marketfixture.Level3Frame("update", symbol, anchors, orders, "add"))
				}
				combined := [2][]marketfixture.Order{append(append([]marketfixture.Order{}, orders[0]...), anchors[0]...), append(append([]marketfixture.Order{}, orders[1]...), anchors[1]...)}
				if !initialized {
					bookFrames = append(bookFrames, marketfixture.Level3Frame("snapshot", symbol, [2][]marketfixture.Order{}, combined, ""))
				}
				previousBooks[symbol] = combined
				for index, frame := range bookFrames {
					var record map[string]any
					So(json.Unmarshal(frame, &record), ShouldBeNil)
					record["capture"] = map[string]any{"session": "fixture", "sequence": sequence + index, "record": 0, "endpoint": "wss://fixture", "receivedAt": stamp}
					bookFrames[index], err = json.Marshal(record)
					So(err, ShouldBeNil)
				}
				side, tradePrice, kind := "sell", price-spread, "fill"
				if (index+ordinal)%3 == 1 {
					side, tradePrice = "buy", price+spread
				}
				if index%7 == ordinal {
					kind = "liquidation"
				}
				capture := fmt.Sprintf(`"capture":{"session":"fixture","sequence":%d,"record":0,"endpoint":"wss://fixture","receivedAt":%q}`, sequence, stamp)
				frames := append(bookFrames, [][]byte{
					[]byte(fmt.Sprintf(`{"channel":"ticker",%s,"data":{"symbol":%q,"last":%g,"bid":%g,"ask":%g,"bid_qty":%g,"ask_qty":%g}}`, capture, symbol, price, price-spread, price+spread, quantity, 6-quantity)),
					[]byte(fmt.Sprintf(`{"channel":"trade",%s,"data":{"symbol":%q,"side":%q,"price":%g,"qty":%g,"timestamp":%q}}`, capture, symbol, side, tradePrice, quantity/10, stamp)),
					[]byte(fmt.Sprintf(`{"channel":"futures_ticker",%s,"data":{"symbol":%q,"last":%g,"index":%g,"mark":%g,"openInterest":%g}}`, capture, symbol, price+spread, price, price-spread/2, quantity+100)),
					[]byte(fmt.Sprintf(`{"channel":"futures_trade",%s,"data":{"symbol":%q,"side":%q,"price":%g,"qty":%g,"timestamp":%q,"type":%q}}`, capture, symbol, side, tradePrice, quantity/5, stamp, kind)),
				}...)
				for _, frame := range frames {
					var stamped map[string]any
					So(json.Unmarshal(frame, &stamped), ShouldBeNil)
					stamped["capture"].(map[string]any)["receivedAt"] = time.Unix(1700000000, int64(sequence)*int64(time.Millisecond)).UTC().Format(time.RFC3339Nano)
					frame, err = json.Marshal(stamped)
					So(err, ShouldBeNil)
					So(workspace.Write(ctx, func(args runtime.Workspace_write_Params) error {
						arrivals, err := args.NewData(1)
						if err != nil {
							return err
						}
						return arrivals.Set(0, frame)
					}), ShouldBeNil)
					So(workspace.WaitStreaming(), ShouldBeNil)
					sequence++
				}
			}
		}
		So(program.Flush(ctx), ShouldBeNil)
		consumer := runtime.Consumer(program.Nodes[program.NodeMap["cut_consumer"]].Client)
		future, release := consumer.Done(ctx, nil)
		defer release()
		result, err := future.Struct()
		So(err, ShouldBeNil)
		outputs, err := result.Outputs()
		So(err, ShouldBeNil)
		So(outputs.Len(), ShouldEqual, 1)
		pointer, err := outputs.At(0).Value()
		So(err, ShouldBeNil)
		row, err := data.Gathered(pointer.Struct()).Row()
		So(err, ShouldBeNil)
		stored, err := row.Value()
		So(err, ShouldBeNil)
		cut := data.MetricCut(stored.Struct())
		metrics, err := cut.Metrics()
		So(err, ShouldBeNil)
		missing := []string{}
		for index := range metrics.Len() {
			if !metrics.At(index).Present() {
				identity, err := metrics.At(index).Identity()
				So(err, ShouldBeNil)
				missing = append(missing, identity)
			}
		}
		So(missing, ShouldBeEmpty)
		So(cut.Complete(), ShouldBeTrue)
	})
}

func TestProgramCopyBinding(t *testing.T) {
	Convey("Native absent records remain absent and numbered UI ports receive their own value", t, func() {
		program, err := CompileJSON([]byte(`{"nodes":{"grid":{"id":"grid","type":"store.Grid"}}}`), nil)
		So(err, ShouldBeNil)
		defer program.Release()
		program.Bindings = &BindingPlan{Inputs: map[string]Origin{"stage": {Definition: "ui_diagnostics", Node: "stage"}}}
		message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)
		defer message.Release()
		result, err := runtime.NewResult(segment)
		So(err, ShouldBeNil)
		binding, err := runtime.NewBinding(segment)
		So(err, ShouldBeNil)
		So(binding.SetTarget("stage.value"), ShouldBeNil)
		frames := make([]nodeFrame, len(program.Nodes))
		Convey("An idle gather does not send a null record as an initialized value", func() {
			gathered, err := data.NewGathered(segment)
			So(err, ShouldBeNil)
			result.SetInterfaceId(data.Gather_TypeID)
			So(result.SetValue(capnp.Struct(gathered).ToPtr()), ShouldBeNil)
			So(binding.SetField("row"), ShouldBeNil)
			present, err := program.copyBinding(frames, binding, result)
			So(err, ShouldBeNil)
			So(present, ShouldBeFalse)
			So(program.stageArrivals, ShouldBeEmpty)
		})
		Convey("List index and presence govern a scalar UI binding", func() {
			output, err := store.NewGrid_done_Results(segment)
			So(err, ShouldBeNil)
			values, err := output.NewValues(2)
			So(err, ShouldBeNil)
			values.Set(0, 42)
			values.Set(1, -7)
			presence, err := output.NewPresent(2)
			So(err, ShouldBeNil)
			presence.Set(0, true)
			result.SetInterfaceId(store.Grid_TypeID)
			So(result.SetValue(capnp.Struct(output).ToPtr()), ShouldBeNil)
			for _, field := range []string{"values_1", "values_0"} {
				So(binding.SetField(field), ShouldBeNil)
				present, err := program.copyBinding(frames, binding, result)
				So(err, ShouldBeNil)
				So(present, ShouldEqual, field == "values_0")
			}
			So(len(program.stageArrivals), ShouldEqual, 1)
			So(program.stageArrivals[0].value, ShouldEqual, "42")
		})
	})
}

/* TestProgramStepPaper exercises the authored paper consumer behind its production LMAX group. */
func TestProgramStepPaper(t *testing.T) {
	Convey("The production book projection activates one native paper wallet", t, func() {
		t.Setenv("KRAKEN_API_KEY", "fixture-key")
		t.Setenv("KRAKEN_API_SECRET", "Zml4dHVyZS1zZWNyZXQ=")
		repository := NewRepository()
		paperGraph, err := repository.Load("paper_exchange")
		So(err, ShouldBeNil)
		withoutNodes(paperGraph, "round_trips", "live_round_trips")
		liveCheckpoint := paperGraph.Nodes["live_checkpoint"]
		liveCheckpoint.InputData["path"], err = json.Marshal(filepath.Join(t.TempDir(), "live-account.capnp"))
		So(err, ShouldBeNil)
		paperGraph.Nodes["live_checkpoint"] = liveCheckpoint
		encoded, err := json.Marshal(paperGraph)
		So(err, ShouldBeNil)
		So(repository.Save("paper_exchange", encoded), ShouldBeNil)
		graph, err := repository.Load("system")
		So(err, ShouldBeNil)
		keep := map[string]bool{"workspace": true, "projection_graph": true, "projection_consumer": true, "projection_group": true, "paper_graph": true, "paper_consumer": true, "paper_group": true, "paper_checkpoint": true}
		for id := range graph.Nodes {
			if !keep[id] {
				withoutNodes(graph, id)
			}
		}
		checkpoint := graph.Nodes["paper_checkpoint"]
		checkpoint.InputData["path"], err = json.Marshal(filepath.Join(t.TempDir(), "paper.capnp"))
		So(err, ShouldBeNil)
		graph.Nodes["paper_checkpoint"] = checkpoint
		program, err := Compile(graph, nil, repository)
		So(err, ShouldBeNil)
		defer program.Release()
		ctx := context.Background()
		So(program.Execute(ctx, nil), ShouldBeNil)
		workspace := runtime.Workspace(program.Nodes[program.NodeMap["workspace"]].Client)
		stamp := "2026-09-26T10:00:00Z"
		frame := marketfixture.Level3Frame("snapshot", "BTC/USD", [2][]marketfixture.Order{}, [2][]marketfixture.Order{{{Price: "99", Quantity: "2", At: stamp}}, {{Price: "100", Quantity: "3", At: stamp}}}, "")
		var record map[string]any
		So(json.Unmarshal(frame, &record), ShouldBeNil)
		record["capture"] = map[string]any{"session": "fixture", "sequence": 0, "record": 0, "endpoint": "wss://fixture", "receivedAt": stamp}
		frame, err = json.Marshal(record)
		So(err, ShouldBeNil)
		So(workspace.Write(ctx, func(args runtime.Workspace_write_Params) error {
			data, err := args.NewData(1)
			if err != nil {
				return err
			}
			return data.Set(0, frame)
		}), ShouldBeNil)
		So(workspace.WaitStreaming(), ShouldBeNil)
		So(program.Flush(ctx), ShouldBeNil)
		consumer := runtime.Consumer(program.Nodes[program.NodeMap["paper_consumer"]].Client)
		future, release := consumer.Done(ctx, nil)
		defer release()
		result, err := future.Struct()
		So(err, ShouldBeNil)
		outputs, err := result.Outputs()
		So(err, ShouldBeNil)
		So(outputs.Len(), ShouldEqual, 2)
		var accountIndex int
		for index := range outputs.Len() {
			name, err := outputs.At(index).Node()
			So(err, ShouldBeNil)
			if name == "account" {
				accountIndex = index
			}
		}
		value, err := outputs.At(accountIndex).Value()
		So(err, ShouldBeNil)
		account := execution.AccountState(value.Struct())
		cash, err := account.Cash()
		So(err, ShouldBeNil)
		So(cash, ShouldEqual, "200")
		So(account.Observations(), ShouldEqual, 1)
		So(account.Decisions(), ShouldEqual, 0)
	})
}

func TestProgramSeedBindingsOptional(t *testing.T) {
	Convey("A causal cut can record absent optional logic without fabricating its stamp", t, func() {
		program, err := CompileJSON([]byte(`{"nodes":{"cut":{"id":"cut","type":"data.ExtendCut","inputData":{"identities":{"value":["logic:late"]}}}}}`), nil)
		So(err, ShouldBeNil)
		client := runtime.State_ServerToClient(program)
		defer client.Release()
		for _, optional := range []bool{true, false} {
			future, release := client.Step(context.Background(), func(args runtime.StageNode_step_Params) error {
				outputs, err := args.NewOutputs(1)
				if err != nil {
					return err
				}
				if err := outputs.Set(0, "cut"); err != nil {
					return err
				}
				bindings, err := args.NewBindings(2)
				if err != nil {
					return err
				}
				for index, entry := range []struct{ producer, field, target string }{{"signal", "row", "cut.base"}, {"absent", "epoch", "cut.epoch"}} {
					binding := bindings.At(index)
					for _, err := range []error{binding.SetProducer(entry.producer), binding.SetNode("producer"), binding.SetField(entry.field), binding.SetTarget(entry.target)} {
						if err != nil {
							return err
						}
					}
					binding.SetOptional(index == 1 && optional)
				}
				upstream, err := args.NewUpstream(1)
				if err != nil {
					return err
				}
				result := upstream.At(0)
				result.SetInterfaceId(data.Gather_TypeID)
				if err := result.SetProducer("signal"); err != nil {
					return err
				}
				if err := result.SetNode("producer"); err != nil {
					return err
				}
				gathered, err := data.NewGathered(args.Segment())
				if err != nil {
					return err
				}
				row, err := gathered.NewRow()
				if err != nil {
					return err
				}
				row.SetTypeId(data.MetricCut_TypeID)
				cut, err := data.NewMetricCut(args.Segment())
				if err != nil {
					return err
				}
				cut.SetEpoch(7)
				cut.SetSequence(9)
				cut.SetComplete(true)
				if err := cut.SetSymbol("BTC/USD"); err != nil {
					return err
				}
				metrics, err := cut.NewMetrics(1)
				if err != nil {
					return err
				}
				metric := metrics.At(0)
				if err := metric.SetIdentity("signal:observed"); err != nil {
					return err
				}
				metric.SetValue(42)
				metric.SetPresent(true)
				metric.SetEpoch(7)
				metric.SetSequence(8)
				if err := row.SetValue(capnp.Struct(cut).ToPtr()); err != nil {
					return err
				}
				return result.SetValue(capnp.Struct(gathered).ToPtr())
			})
			result, err := future.Struct()
			So(err, ShouldBeNil)
			outputs, err := result.Outputs()
			So(err, ShouldBeNil)
			if !optional {
				So(outputs.Len(), ShouldEqual, 0)
				release()
				continue
			}
			So(outputs.Len(), ShouldEqual, 1)
			pointer, err := outputs.At(0).Value()
			So(err, ShouldBeNil)
			row, err := data.Gathered(pointer.Struct()).Row()
			So(err, ShouldBeNil)
			stored, err := row.Value()
			So(err, ShouldBeNil)
			cut := data.MetricCut(stored.Struct())
			So(cut.Complete(), ShouldBeFalse)
			metrics, err := cut.Metrics()
			So(err, ShouldBeNil)
			So(metrics.At(0).Value(), ShouldEqual, 42)
			So(metrics.At(0).Sequence(), ShouldEqual, 8)
			So(metrics.At(1).Present(), ShouldBeFalse)
			release()
		}
	})
}
