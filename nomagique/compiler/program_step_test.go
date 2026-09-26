package compiler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
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
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/store/tables"
	"github.com/theapemachine/symm/nomagique/temporal"

	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/ui"
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
			if id != "server" && id != "grid_checkpoint" && !strings.HasPrefix(node.Type, "runtime.") && !strings.HasSuffix(id, "_graph") {
				withoutNodes(graph, id)
			}
		}
		checkpoint := graph.Nodes["grid_checkpoint"]
		checkpoint.InputData["path"], err = json.Marshal(filepath.Join(directory, "grid.capnp"))
		So(err, ShouldBeNil)
		graph.Nodes["grid_checkpoint"] = checkpoint
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
			"CREATE TABLE metric_cuts_v2 AS SELECT * FROM read_json('" + cuts + "')",
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
		var stored struct {
			Symbol   string
			Sequence int64
			Metrics  []struct {
				Identity string
				Present  bool
				Sequence int64
			}
		}
		So(json.Unmarshal(row, &stored), ShouldBeNil)
		So(stored.Symbol, ShouldEqual, "ETH/USD")
		So(stored.Sequence, ShouldEqual, 63)
		for _, metric := range stored.Metrics {
			if metric.Present {
				So(metric.Sequence%2, ShouldEqual, 1)
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
