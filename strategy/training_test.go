package strategy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/store/storetest"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

type testFillingDesk struct {
	writes  chan string
	volumes chan *decimal.Decimal
}

func (filling *testFillingDesk) Read(keys ...string) ([]byte, error) {
	return nil, nil
}

func (filling *testFillingDesk) Write(buf []byte) error {
	message := kraken.AddOrderMessage{}
	if err := sonic.Unmarshal(buf, &message); err != nil {
		return err
	}

	quantity, err := decimal.NewFromString(message.Params.Volume)
	if err != nil {
		return err
	}

	filling.volumes <- quantity
	filling.writes <- message.Params.Type
	return nil
}

/*
paperFixture is a Training over a seeded BTC/USD book (asks 50000 x 0.01,
50100 x 0.05; bids 49990 x 0.02, 49900 x 0.05), a 0.1% fee, $1000 of
reference cash, and a desk whose orders land on filling.
*/
type paperFixture struct {
	training *Training
	desk     *broker.Desk
	filling  *testFillingDesk
	hub      *bytes.Buffer
	records  *bytes.Buffer
}

func newPaperTraining(tree *CognitionTree) *paperFixture {
	ctx := context.Background()
	normalizer := spot.NewNormalizer()
	tickSize, _ := decimal.NewFromString("0.1")
	normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{"BTC": {AltName: "BTC"}, "USD": {AltName: "USD"}},
		NewPairs: map[string]spot.AssetPair{"BTCUSD": {
			WSName: "BTC/USD", Base: "BTC", Quote: "USD",
			PairDecimals: 1, LotDecimals: 8, LotMultiplier: 1, TickSize: tickSize,
		}},
	})

	book := broker.NewBook(ctx, normalizer)
	order := func(id string, price, qty float64) kraken.Level3Order {
		return kraken.Level3Order{
			OrderID: id, LimitPrice: decimal.NewFromFloat64(price), OrderQty: decimal.NewFromFloat64(qty),
			Timestamp: time.Now().UTC(), Event: "add",
		}
	}

	So(book.Update(&kraken.Level3{Channel: "level3", Type: "snapshot", Data: []kraken.Level3Data{{
		Symbol:    "BTC/USD",
		Asks:      []kraken.Level3Order{order("a1", 50000, 0.01), order("a2", 50100, 0.05)},
		Bids:      []kraken.Level3Order{order("b1", 49990, 0.02), order("b2", 49900, 0.05)},
		Timestamp: time.Now().UTC(),
	}}}), ShouldBeNil)

	price := broker.NewPrice(ctx, book, nil, nil, normalizer)
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.1)})
	price.SetReferenceCash(decimal.NewFromFloat64(1000))

	fixture := &paperFixture{
		filling: &testFillingDesk{writes: make(chan string, 64), volumes: make(chan *decimal.Decimal, 64)},
		hub:     &bytes.Buffer{},
		records: &bytes.Buffer{},
	}

	fixture.desk = broker.NewDesk(ctx, fixture.filling, price, nil)

	training, err := NewTraining(ctx, price, fixture.desk, tree, fixture.hub, fixture.records)
	So(err, ShouldBeNil)
	fixture.training = training

	return fixture
}

/*
fill is one venue execution row for symbol.
*/
func fill(side string, qty *decimal.Decimal, price, fee float64, at time.Time) *kraken.Execution {
	return &kraken.Execution{Channel: "executions", Data: []kraken.ExecutionData{{
		OrderID: side + "-1", Symbol: "BTC/USD", Side: side, LastQty: qty, LastPrice: decimal.NewFromFloat64(price),
		FeeUsdEquiv: decimal.NewFromFloat64(fee), Timestamp: at, OrderStatus: "filled", ExecType: "trade",
		CumQty: qty, AvgPrice: decimal.NewFromFloat64(price),
	}}}
}

func readValues(buf *bytes.Buffer) []*data.Value {
	reader := bufio.NewReader(bytes.NewReader(buf.Bytes()))
	values := make([]*data.Value, 0)

	for {
		value := data.New()
		n, err := value.ReadFrom(reader)

		if n == 0 && errors.Is(err, io.EOF) {
			return values
		}

		So(err, ShouldBeNil)
		values = append(values, value)
	}
}

func testTree(actions map[string]string) *CognitionTree {
	pol := &policy{epoch: 7, actions: actions, metrics: map[string]map[string]float64{}}

	for path, action := range actions {
		key := path + "/" + action + ".bin"
		pol.keys = append(pol.keys, key)
		pol.metrics[key] = map[string]float64{"gain": 0.02, "hold_seconds": 30, "visits": 3, "mean_reward": 0.02, "confidence": 0.66}
	}

	tree, err := newCognitionTree(pol, 2)
	So(err, ShouldBeNil)
	return tree
}

func TestCognitionTree(t *testing.T) {
	Convey("Given a cognition tree over stored paths", t, func() {
		tree := testTree(map[string]string{"R01/R04": "enter", "R02/R01/R04": "exit", "R07/R08": ""})

		Convey("It matches the deepest stored path the tokens end on", func() {
			match, ok := tree.Match([]string{"R09", "R02", "R01", "R04"})
			So(ok, ShouldBeTrue)
			So(match, ShouldResemble, CognitionMatch{Path: "R02/R01/R04", Depth: 3, Action: "exit"})

			match, ok = tree.Match([]string{"R03", "R01", "R04"})
			So(ok, ShouldBeTrue)
			So(match, ShouldResemble, CognitionMatch{Path: "R01/R04", Depth: 2, Action: "enter"})
		})

		Convey("A path stored with disagreeing actions matches with no action", func() {
			match, ok := tree.Match([]string{"R07", "R08"})
			So(ok, ShouldBeTrue)
			So(match.Action, ShouldBeEmpty)
		})

		Convey("Tokens that end on no stored path, or are shorter than the minimum depth, do not match", func() {
			_, ok := tree.Match([]string{"R01", "R04", "R05"})
			So(ok, ShouldBeFalse)

			_, ok = tree.Match([]string{"R04"})
			So(ok, ShouldBeFalse)
		})

		Convey("Its depth is the longest stored path", func() {
			So(tree.Depth(), ShouldEqual, 3)
			So(tree.Paths(), ShouldEqual, 3)
			So(tree.Epoch(), ShouldEqual, 7)
		})

		Convey("A minimum depth below one is rejected", func() {
			_, err := newCognitionTree(&policy{actions: map[string]string{}}, 0)
			So(err, ShouldNotBeNil)
		})

		Convey("Export lists each path with its stored policy values", func() {
			only := testTree(map[string]string{"R01/R04": "enter"})
			export, err := only.Export()
			So(err, ShouldBeNil)
			So(export.Keys, ShouldResemble, []string{"R01/R04/enter.bin"})
			So(export.Branches, ShouldResemble, []ui.TrieBranchJSON{{
				Hash: "R01/R04", Depth: 2, Visits: 3, MeanEdge: 0.02, Confidence: 0.66, Policy: "ENTER",
			}})
		})

		Convey("Export refuses a path without stored values", func() {
			broken := testTree(map[string]string{"R01/R04": "enter"})
			broken.policy.metrics = map[string]map[string]float64{}
			_, err := broken.Export()
			So(err, ShouldNotBeNil)
		})
	})
}

func TestLoadCognitionTree(t *testing.T) {
	Convey("Given an in-memory bucket (unit tests never reach the real one)", t, func() {
		ctx := context.Background()
		backend := storetest.NewBackend()
		server := httptest.NewServer(backend)
		defer server.Close()

		Convey("A missing bucket is an error, not an empty model", func() {
			_, err := store.NewReader(ctx, "symm", storetest.Options(server.URL))
			So(err, ShouldNotBeNil)
		})

		backend.Buckets["symm"] = true
		reader, err := store.NewReader(ctx, "symm", storetest.Options(server.URL))
		So(err, ShouldBeNil)

		run := gateSettings(system.Cfg.Resonance)
		run.TickEpoch = 3

		publishGated := func(epoch, tick uint64, action, path string, vector []float64, gate *data.PolicyGate) {
			value := &data.Value{Origin: data.PolicyOrigin, Role: action, Scope: data.PolicyScope, Epoch: epoch, Tick: tick}
			value.FormatPrefix()
			data.AppendVector(value, vector, append(append([]string{}, data.PolicyKeys[:len(vector)]...), data.PolicyPathTag+path)...)

			if gate != nil {
				So(data.AppendPolicyGate(value, *gate), ShouldBeNil)
			}

			backend.Objects[value.Prefix] = value.Bytes()
		}

		publish := func(epoch, tick uint64, action, path string, vector []float64) {
			publishGated(epoch, tick, action, path, vector, &run)
		}

		Convey("No published epoch is an empty tree", func() {
			tree, err := LoadCognitionTree(reader, 2, system.Cfg.Resonance)
			So(err, ShouldBeNil)
			So(tree.Empty(), ShouldBeTrue)
		})

		Convey("The newest epoch's paths load with their actions and gate", func() {
			publish(5, 1, "exit", "R09/R09", []float64{0.01, 10, 2, 0.01, 0.5})
			publish(7, 1, "enter", "R01/R04", []float64{0.02, 30, 3, 0.02, 0.66})

			tree, err := LoadCognitionTree(reader, 2, system.Cfg.Resonance)
			So(err, ShouldBeNil)
			So(tree.Epoch(), ShouldEqual, 7)
			So(tree.policy.actions, ShouldResemble, map[string]string{"R01/R04": "enter"})
			So(tree.Gate().Same(run), ShouldBeTrue)
			So(tree.Gate().Epoch, ShouldEqual, 7)
			So(tree.Gate().TickEpoch, ShouldEqual, 3)
		})

		Convey("A policy epoch trained under other gate settings is refused, naming both", func() {
			other := run
			other.Enabled = !run.Enabled
			publishGated(7, 1, "enter", "R01/R04", []float64{0.02, 30, 3, 0.02, 0.66}, &other)

			_, err := LoadCognitionTree(reader, 2, system.Cfg.Resonance)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "policy epoch 7 was trained under")
			So(err.Error(), ShouldContainSubstring, "this run is configured for")

			other = run
			other.SurpriseZ = run.SurpriseZ + 0.5
			publishGated(7, 1, "enter", "R01/R04", []float64{0.02, 30, 3, 0.02, 0.66}, &other)

			_, err = LoadCognitionTree(reader, 2, system.Cfg.Resonance)
			So(err, ShouldNotBeNil)
		})

		Convey("A policy without a gate tag (trained before the gate) is refused", func() {
			publishGated(7, 1, "enter", "R01/R04", []float64{0.02, 30, 3, 0.02, 0.66}, nil)

			_, err := LoadCognitionTree(reader, 2, system.Cfg.Resonance)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "retrain")
		})

		Convey("Policies of one epoch that disagree on their gate are refused", func() {
			other := run
			other.Warmup.SlowRate = run.Warmup.SlowRate * 2
			publish(7, 1, "enter", "R01/R04", []float64{0.02, 30, 3, 0.02, 0.66})
			publishGated(7, 2, "exit", "R02/R03", []float64{0.02, 30, 3, 0.02, 0.66}, &other)

			_, err := LoadCognitionTree(reader, 2, system.Cfg.Resonance)
			So(err, ShouldNotBeNil)
		})

		Convey("A stored policy missing a PolicyKey is an error", func() {
			publish(7, 1, "enter", "R01/R04", []float64{0.02, 30})

			_, err := LoadCognitionTree(reader, 2, system.Cfg.Resonance)
			So(err, ShouldNotBeNil)
		})

		Convey("An object that does not decode is an error", func() {
			backend.Objects[data.Prefix(data.PolicyOrigin, "enter", data.PolicyScope, 7)+"0000000000000000001.bin"] = []byte("garbage")

			_, err := LoadCognitionTree(reader, 2, system.Cfg.Resonance)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestPublishTree(t *testing.T) {
	Convey("Given the hub's retained writer", t, func() {
		sink := &frameSink{}
		read := func() map[string]any {
			So(sink.cognition, ShouldHaveLength, 1)
			value := sink.cognition[0]
			So(value.Origin, ShouldEqual, data.CognitionOrigin)
			So(value.Role, ShouldEqual, data.CognitionTreeRole)
			So(value.Scope, ShouldEqual, data.PolicyScope)

			fields := map[string]any{}
			So(json.Unmarshal(*data.Layer[[]byte](value, 0), &fields), ShouldBeNil)
			return fields
		}

		Convey("An empty tree says nothing is trained and nothing trades", func() {
			tree, err := newCognitionTree(&policy{actions: map[string]string{}}, 2)
			So(err, ShouldBeNil)
			So(PublishTree(sink, tree, false), ShouldBeNil)

			fields := read()
			So(fields["paths"], ShouldEqual, 0)
			So(fields["trading"], ShouldEqual, false)
			So(fields["gate"], ShouldBeNil)
			So(fields["keys"], ShouldBeEmpty)
		})

		Convey("A loaded tree carries its epoch, keys, stored values and gate", func() {
			tree := testTree(map[string]string{"R01/R04": "enter"})
			tree.policy.epoch = 1792195368165650000
			tree.policy.gate = data.PolicyGate{Epoch: 1792195368165650000, TickEpoch: 1792189318815706000, Enabled: true, SurpriseZ: 1, Warmup: data.PolicyWarmup{Rule: data.WarmupSettled, FastRate: 0.15, SlowRate: 0.01}, LearningRate: 0.01}
			So(PublishTree(sink, tree, true), ShouldBeNil)

			fields := read()
			So(fields["epoch"], ShouldEqual, "1792195368165650000")
			So(fields["paths"], ShouldEqual, 1)
			So(fields["trading"], ShouldEqual, true)
			So(fields["keys"], ShouldResemble, []any{"R01/R04/enter.bin"})
			So(fields["gate"], ShouldResemble, map[string]any{
				"tickEpoch": "1792189318815706000", "enabled": true, "surpriseZ": 1.0, "warmup": "settled: error average at 0.15 no longer below its average at 0.01", "learningRate": 0.01,
			})

			branch := fields["branches"].([]any)[0].(map[string]any)
			So(branch["visits"], ShouldEqual, 3)
			So(branch["meanEdge"], ShouldEqual, 0.02)
			So(branch["confidence"], ShouldEqual, 0.66)
			So(branch["policy"], ShouldEqual, "ENTER")
		})
	})
}

func TestTraining(t *testing.T) {
	Convey("Given a paper trader over a cognition tree", t, func() {
		enter := CognitionMatch{Path: "R01/R04", Depth: 2, Action: "enter"}
		exit := CognitionMatch{Path: "R02/R03", Depth: 2, Action: "exit"}
		tree := testTree(map[string]string{enter.Path: "enter", exit.Path: "exit"})

		Convey("It refuses anything but the paper market model", func() {
			model := system.Cfg.Market.Model
			system.Cfg.Market.Model = "live"
			defer func() { system.Cfg.Market.Model = model }()

			_, err := NewTraining(context.Background(), &broker.Price{}, &broker.Desk{}, tree, &bytes.Buffer{}, &bytes.Buffer{})
			So(err, ShouldNotBeNil)
		})

		Convey("It refuses an empty tree and missing parts", func() {
			empty, err := newCognitionTree(&policy{actions: map[string]string{}}, 2)
			So(err, ShouldBeNil)

			_, err = NewTraining(context.Background(), &broker.Price{}, &broker.Desk{}, empty, &bytes.Buffer{}, &bytes.Buffer{})
			So(err, ShouldNotBeNil)

			_, err = NewTraining(context.Background(), nil, &broker.Desk{}, tree, &bytes.Buffer{}, &bytes.Buffer{})
			So(err, ShouldNotBeNil)
		})

		paper := newPaperTraining(tree)

		Convey("A round trip emits one record per decision to the file and the hub", func() {
			So(paper.training.Act("BTC/USD", enter), ShouldEqual, "enter submitted")
			So(<-paper.filling.writes, ShouldEqual, "buy")
			qty := <-paper.filling.volumes

			entered := time.Now().Add(150 * time.Millisecond)
			bought := fill("buy", qty, 50010, 0.25, entered)
			paper.desk.Apply(bought)
			So(paper.training.Filled(bought), ShouldBeNil)

			// Held: it is listed as open, with no record written for it.
			open := paper.training.Open()
			So(open, ShouldHaveLength, 1)
			So(open[0], ShouldStartWith, fmt.Sprintf("BTC/USD qty %g of %g ordered", qty.Float64(), qty.Float64()))
			So(open[0], ShouldContainSubstring, "(filled) entry 50010")
			So(paper.records.Len(), ShouldEqual, 0)

			So(paper.training.Act("BTC/USD", enter), ShouldEqual, "holding: enter ignored")
			So(paper.training.Act("BTC/USD", exit), ShouldEqual, "exit submitted")
			So(<-paper.filling.writes, ShouldEqual, "sell")
			So((<-paper.filling.volumes).Float64(), ShouldAlmostEqual, qty.Float64(), 1e-12)

			exited := entered.Add(30 * time.Second)
			sold := fill("sell", qty, 50490, 0.25, exited)
			paper.desk.Apply(sold)
			So(paper.training.Filled(sold), ShouldBeNil)

			records := readValues(paper.records)
			So(records, ShouldHaveLength, 2)
			So(paper.training.Open(), ShouldBeEmpty)
			So(paper.hub.Bytes(), ShouldResemble, paper.records.Bytes())

			q := qty.Float64()
			pnl := q*50490 - 0.25 - q*50010 - 0.25

			for idx, decision := range []struct {
				action, path   string
				intended, fill float64
				at             time.Time
			}{
				{"enter", enter.Path, 50000, 50010, entered},
				{"exit", exit.Path, 49990, 50490, exited},
			} {
				record, err := data.PaperFields(records[idx])
				So(err, ShouldBeNil)
				So(records[idx].Prefix, ShouldStartWith, "paper/"+decision.action+"/BTC/USD/")
				So(record.Action, ShouldEqual, decision.action)
				So(record.Path, ShouldEqual, decision.path)
				So(record.PolicyEpoch, ShouldEqual, 7)

				fields := record.Fields
				So(fields["depth"], ShouldEqual, 2)
				So(fields["intended_price"], ShouldEqual, decision.intended)
				So(fields["fill_price"], ShouldAlmostEqual, decision.fill, 1e-6)
				So(fields["filled_at"], ShouldEqual, float64(decision.at.UnixNano()))
				So(fields["latency_seconds"], ShouldBeGreaterThan, 0)
				So(fields["qty"], ShouldAlmostEqual, q, 1e-12)
				So(fields["entry_price"], ShouldAlmostEqual, 50010, 1e-6)
				So(fields["exit_price"], ShouldAlmostEqual, 50490, 1e-6)
				So(fields["fees"], ShouldAlmostEqual, 0.5, 1e-9)
				So(fields["pnl"], ShouldAlmostEqual, pnl, 1e-9)
				So(fields["net_return"], ShouldAlmostEqual, pnl/(q*50010+0.25), 1e-12)
			}

			// Entry filled 10 above the ask; exit 500 above the bid.
			enterFields, _ := data.PaperFields(records[0])
			exitFields, _ := data.PaperFields(records[1])
			So(enterFields.Fields["slippage"], ShouldAlmostEqual, 10.0/50000, 1e-12)
			So(exitFields.Fields["slippage"], ShouldAlmostEqual, -500.0/49990, 1e-12)
		})

		Convey("An exit without a filled entry from this run is refused", func() {
			So(paper.training.Act("BTC/USD", exit), ShouldEqual, "flat: exit ignored")
			So(paper.training.Act("BTC/USD", enter), ShouldEqual, "enter submitted")
			So(paper.training.Act("BTC/USD", exit), ShouldEqual, "exit refused: no filled entry this run")
		})

		Convey("Noop and disagreeing paths place nothing", func() {
			So(paper.training.Act("BTC/USD", CognitionMatch{Path: "R05/R06", Depth: 2, Action: "noop"}), ShouldStartWith, "noop")
			So(paper.training.Act("BTC/USD", CognitionMatch{Path: "R05/R06", Depth: 2}), ShouldStartWith, "no action")
			So(paper.filling.writes, ShouldHaveLength, 0)
		})

		Convey("A fill with no decision, or without its fee, is an error", func() {
			qty := decimal.NewFromFloat64(0.001)
			So(paper.training.Filled(fill("buy", qty, 50000, 0.1, time.Now())), ShouldNotBeNil)

			So(paper.training.Act("BTC/USD", enter), ShouldEqual, "enter submitted")
			<-paper.filling.writes
			ordered := <-paper.filling.volumes
			noFee := fill("buy", ordered, 50000, 0, time.Now())
			noFee.Data[0].FeeUsdEquiv = nil
			So(paper.training.Filled(noFee), ShouldNotBeNil)
		})

		Convey("A canceled order drops its decision loudly", func() {
			So(paper.training.Act("BTC/USD", enter), ShouldEqual, "enter submitted")
			canceled := fill("buy", decimal.NewFromFloat64(0), 0, 0, time.Now())
			canceled.Data[0].OrderStatus = "canceled"
			So(paper.training.Filled(canceled), ShouldNotBeNil)
			So(paper.training.trips, ShouldBeEmpty)
		})
	})
}
