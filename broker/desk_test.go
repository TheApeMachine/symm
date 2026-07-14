package broker

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/tests"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
	"github.com/theapemachine/symm/types"
)

func TestNewDesk(t *testing.T) {
	Convey("Given a newly constructed Desk", t, func() {
		desk := NewDesk(nil, nil, nil, make(chan []byte, 1))

		Convey("Then it waits for balance and price before hydrating", func() {
			So(desk.Status(), ShouldEqual, types.INITIALIZING)
		})
	})
}

func TestDeskQuantity(t *testing.T) {
	Convey("Given quote capital and Kraken quantity constraints", t, func() {
		price := &Price{status: types.READY, fees: &sync.Map{}, tickers: &sync.Map{}}
		price.fees.Store("TEST/USD", kraken.TradeVolumeFees{Fee: "0.2600"})
		price.TickerAck([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"TEST/USD","last":"10","bid":"9.9","ask":"10"}]}`))
		costMin, err := decimal.NewFromString("5")
		So(err, ShouldBeNil)
		desk := &Desk{price: price}

		quantity, executablePrice, err := desk.quantity("TEST/USD", 100, kraken.InstrumentPair{
			Symbol: "TEST/USD", QtyIncrement: 0.1, QtyPrecision: 1,
			QtyMin: 0.1, CostMin: *costMin,
		})

		Convey("Then quantity rounds down and leaves room for the taker fee", func() {
			So(err, ShouldBeNil)
			So(quantity.Float64(), ShouldEqual, 9.9)
			So(executablePrice.Float64(), ShouldEqual, 10.0)
		})
	})
}

func BenchmarkDeskQuantity(b *testing.B) {
	price := &Price{status: types.READY, fees: &sync.Map{}, tickers: &sync.Map{}}
	price.fees.Store("TEST/USD", kraken.TradeVolumeFees{Fee: "0.2600"})
	price.TickerAck([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"TEST/USD","last":"10","bid":"9.9","ask":"10"}]}`))
	costMin, _ := decimal.NewFromString("5")
	desk := &Desk{price: price}
	pair := kraken.InstrumentPair{
		Symbol: "TEST/USD", QtyIncrement: 0.1, QtyPrecision: 1,
		QtyMin: 0.1, CostMin: *costMin,
	}

	b.ReportAllocs()

	for b.Loop() {
		quantity, _, err := desk.quantity("TEST/USD", 100, pair)

		if err != nil || quantity.Float64() != 9.9 {
			b.Fatal("invalid executable quantity")
		}
	}
}

func TestDeskPublish(t *testing.T) {
	Convey("Given a desk holding one open position", t, func() {
		ui := make(chan []byte, 1)
		desk := NewDesk(nil, nil, &Balance{}, ui)
		desk.positions.Store("BTC/USD", &Position{
			Data: &PositionData{
				Symbol:     "BTC/USD",
				Qty:        *decimal.NewFromFloat64(0.0001),
				EntryPrice: *decimal.NewFromFloat64(64129.900),
				Mark:       *decimal.NewFromFloat64(63039.400),
				PnL:        *decimal.NewFromFloat64(-0.142114),
				ReturnPct:  -0.0222,
			},
		})

		desk.Publish()

		Convey("It should publish flat position snapshots the frontend can parse", func() {
			var frame map[string]any

			select {
			case payload := <-ui:
				err := sonic.Unmarshal(payload, &frame)
				So(err, ShouldBeNil)
			default:
				t.Fatal("desk publish did not emit a frame")
			}

			positions, ok := frame["positions"].([]any)
			So(ok, ShouldBeTrue)
			So(positions, ShouldHaveLength, 1)

			position, ok := positions[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(position["symbol"], ShouldEqual, "BTC/USD")
		})
	})
}

func TestDeskExposures(t *testing.T) {
	Convey("Given positions associated with their originating Theses", t, func() {
		desk := &Desk{positions: &sync.Map{}, price: &Price{}}
		btc := types.NewThesis(nil)
		eth := types.NewThesis(nil)
		desk.positions.Store("BTC/USD", &Position{
			thesis: btc,
			Data:   &PositionData{Qty: *decimal.NewFromInt64(1), Mark: *decimal.NewFromInt64(100)},
		})
		desk.positions.Store("ETH/USD", &Position{
			thesis: eth,
			Data:   &PositionData{Qty: *decimal.NewFromInt64(1), Mark: *decimal.NewFromInt64(100)},
		})

		exposures := desk.Exposures()

		Convey("Then strategy receives the exact lifecycle for each position", func() {
			So(exposures, ShouldHaveLength, 2)
			So(exposures["BTC/USD"].Thesis, ShouldEqual, btc)
			So(exposures["ETH/USD"].Thesis, ShouldEqual, eth)
		})
	})
}

func TestDeskOpenPositions(t *testing.T) {
	Convey("Given one pending entry with no fill and one retained closed position", t, func() {
		desk := &Desk{positions: &sync.Map{}}
		desk.positions.Store("PENDING/USD", &Position{
			status: types.PENDING,
			Data:   &PositionData{Symbol: "PENDING/USD", Qty: *decimal.NewFromInt64(0)},
		})
		desk.positions.Store("CLOSED/USD", &Position{
			status: types.CLOSED,
			Data:   &PositionData{Symbol: "CLOSED/USD", Qty: *decimal.NewFromInt64(0)},
		})

		Convey("Then the order reserves one slot without becoming strategy exposure", func() {
			So(desk.OpenPositions(), ShouldEqual, 1)
			So(desk.Exposures(), ShouldBeEmpty)
		})
	})
}

func BenchmarkDeskOpenPositions(b *testing.B) {
	desk := &Desk{positions: &sync.Map{}}

	for index := range 40 {
		desk.positions.Store(index, &Position{
			status: types.OPEN,
			Data:   &PositionData{Symbol: "TEST/USD", Qty: *decimal.NewFromInt64(1)},
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		if desk.OpenPositions() != 40 {
			b.Fatal("open position count changed")
		}
	}
}

func TestDeskExecute(t *testing.T) {
	Convey("Given a selected action the broker cannot execute", t, func() {
		thesis := types.NewThesis(nil)
		So(thesis.Transition("BTC/USD", types.LifecycleShaped, time.Unix(1, 0)), ShouldBeNil)
		So(
			thesis.Transition("BTC/USD", types.LifecycleEntrySelected, time.Unix(2, 0)),
			ShouldBeNil,
		)
		thesis.Decisions = append(thesis.Decisions, types.Decision{
			Action: "enter", Symbol: "BTC/USD",
		})
		desk := &Desk{}

		err := desk.Execute(strategy.Intent{Thesis: thesis}, kraken.InstrumentPair{})

		Convey("Then the original Decision is unchanged and rejection is journaled", func() {
			So(err, ShouldNotBeNil)
			So(thesis.Decisions[0].Action, ShouldEqual, "enter")
			So(thesis.LifecycleState("BTC/USD"), ShouldEqual, types.LifecycleRejected)
			So(thesis.TradeJournal, ShouldHaveLength, 5)
			So(thesis.TradeJournal[2].Kind, ShouldEqual, "intent_submission")
			So(thesis.TradeJournal[3].Status, ShouldEqual, types.LifecycleRejected)
			So(thesis.TradeJournal[4].Kind, ShouldEqual, "broker_rejection")
			So(thesis.TradeJournal[4].Error, ShouldNotBeEmpty)
		})
	})

	Convey("Given a sequenced market and a calibrated round-trip opportunity", t, func() {
		previousModel := viper.Get("trading.model")
		viper.Set("trading.model", "live")
		defer viper.Set("trading.model", previousModel)
		ctx := context.Background()
		mock := tests.NewMockAPI()
		api := websocket.NewAPI(ctx, mock.Public(), mock.Private(), nil)
		price := NewPrice(api)
		price.status = types.READY
		price.fees.Store("ALGO/USD", kraken.TradeVolumeFees{Fee: "0.2600"})
		tests.NewMarket().
			Prefix(tickerfixture.NewFixture(tickerfixture.SNAPSHOT, 1)).
			Replay(tests.Handlers{"ticker": price.TickerAck})

		balance := &Balance{
			status: types.READY,
			model: &kraken.Balance{Data: []kraken.BalanceData{{
				Asset: "USD", Balance: *decimal.NewFromInt64(1000),
				Available: *decimal.NewFromInt64(1000),
			}}},
			quote: "USD",
		}
		desk := NewDesk(api, price, balance, make(chan []byte, 64))
		desk.status = types.READY
		desk.maxPositions = 2
		planner := strategy.NewPlanner(ctx, nil, nil, nil)
		entryAt := time.Now().UTC()
		thesis := types.NewThesis(nil)
		thesis.Forecasts = append(thesis.Forecasts, types.Forecasts{
			Source: "manifold_forecast", Symbol: "ALGO/USD", At: entryAt,
			ObservedInterval: time.Second, SourceEpoch: 10, HorizonEvents: 1,
			ExpiresEpoch: 11, Target: "return", ModelVersion: "fixture", Ready: true,
			Calibrated: true, FrictionReady: true, CalibrationSamples: 30,
			IncrementalMSE:           0.0001,
			IncrementalMSELowerBound: 0.00005, ExpectedReturn: 0.02,
			ReferencePrice: 0.10036, BuyCapacity: 200, SellCapacity: 200,
			ExpectedFees: 0.0026, ExpectedSpread: 0.001, ExpectedImpact: 0.0001,
			ExpectedAdverseSelection: 0.0001, Uncertainty: 0.001, Confidence: 0.8,
		})

		intents := planner.Decide(
			thesis, desk.Exposures(), map[string]float64{"ALGO/USD": 0.0026},
			100, desk.Slots(),
		)
		So(intents, ShouldHaveLength, 1)
		So(intents[0].Selected().Action, ShouldEqual, "enter")
		costMin, err := decimal.NewFromString("1")
		So(err, ShouldBeNil)
		pair := kraken.InstrumentPair{
			Symbol: "ALGO/USD", QtyIncrement: 0.1, QtyPrecision: 1,
			QtyMin: 0.1, CostMin: *costMin,
		}
		So(desk.Execute(intents[0], pair), ShouldBeNil)

		stored, exists := desk.positions.Load("ALGO/USD")
		So(exists, ShouldBeTrue)
		position := stored.(*Position)
		entryQuantity := position.requestedQty.Float64()
		entryHalf := entryQuantity / 2
		entryPrice := 0.10036
		entryFee := entryHalf * entryPrice * 0.0026
		handlers := tests.Handlers{
			"add_order":  func(payload []byte) { mock.Private().Emit("add_order", payload) },
			"executions": func(payload []byte) { mock.Private().Emit("executions", payload) },
		}
		entryPartial := tests.NewReplayFixture([]byte(fmt.Sprintf(`
{"channel":"add_order","type":"update","method":"add_order","result":{"order_id":"entry-order"},"success":true,"req_id":%d,"time_out":"%s"}
{"channel":"executions","type":"update","data":[{"order_id":"entry-order","exec_id":"entry-partial","exec_type":"trade","symbol":"ALGO/USD","side":"buy","last_qty":%.12f,"last_price":"%.8f","cost":"%.12f","order_status":"partially_filled","cum_qty":%.12f,"cum_cost":"%.12f","avg_price":"%.8f","fee_usd_equiv":"%.12f","timestamp":"%s"}]}
`, position.reqID,
			entryAt.Add(time.Second).Format(time.RFC3339Nano), entryHalf,
			entryPrice, entryHalf*entryPrice, entryHalf,
			entryHalf*entryPrice, entryPrice, entryFee,
			entryAt.Add(time.Second).Format(time.RFC3339Nano),
		)))
		tests.NewMarket().Feed(entryPartial).Replay(handlers)
		So(position.Status(), ShouldEqual, types.PARTIAL_FILLED)
		So(thesis.LifecycleState("ALGO/USD"), ShouldEqual, types.LifecyclePartiallyEntered)

		entryFilled := tests.NewReplayFixture([]byte(fmt.Sprintf(`
{"channel":"executions","type":"update","data":[{"order_id":"entry-order","exec_id":"entry-filled","exec_type":"trade","symbol":"ALGO/USD","side":"buy","last_qty":%.12f,"last_price":"%.8f","cost":"%.12f","order_status":"filled","cum_qty":%.12f,"cum_cost":"%.12f","avg_price":"%.8f","fee_usd_equiv":"%.12f","timestamp":"%s"}]}
`,
			entryHalf, entryPrice, entryHalf*entryPrice, entryQuantity,
			entryQuantity*entryPrice, entryPrice, entryFee,
			entryAt.Add(2*time.Second).Format(time.RFC3339Nano),
		)))
		tests.NewMarket().Feed(entryFilled).Replay(handlers)
		So(position.Status(), ShouldEqual, types.FILLED)
		So(thesis.LifecycleState("ALGO/USD"), ShouldEqual, types.LifecycleEntered)

		management := types.NewThesis(nil)
		management.Forecasts = append(management.Forecasts, types.Forecasts{
			Source: "manifold_forecast", Symbol: "ALGO/USD",
			At: entryAt.Add(3 * time.Second), ObservedInterval: time.Second,
			SourceEpoch: 11, HorizonEvents: 1, ExpiresEpoch: 12,
			Target: "return", ModelVersion: "fixture", Ready: true,
			Calibrated: true, FrictionReady: true, CalibrationSamples: 31,
			IncrementalMSE: 0.0001, IncrementalMSELowerBound: 0.00005,
			ExpectedReturn: -0.02, ReferencePrice: entryPrice,
			BuyCapacity: 200, SellCapacity: 200, ExpectedFees: 0.0026,
			ExpectedSpread: 0.001, ExpectedImpact: 0.0001,
			ExpectedAdverseSelection: 0.0001, Uncertainty: 0.001,
			Confidence: 0.8,
		})
		exitIntents := planner.Decide(
			management, desk.Exposures(), map[string]float64{"ALGO/USD": 0.0026},
			0, desk.Slots(),
		)
		So(exitIntents, ShouldHaveLength, 1)
		So(exitIntents[0].Selected().Action, ShouldEqual, "exit")
		So(exitIntents[0].Thesis, ShouldEqual, thesis)
		So(desk.Execute(exitIntents[0], pair), ShouldBeNil)

		exitQuantity := position.requestedQty.Float64()
		exitHalf := exitQuantity / 2
		exitPrice := 0.11
		exitFee := exitHalf * exitPrice * 0.0026
		exitPartial := tests.NewReplayFixture([]byte(fmt.Sprintf(`
{"channel":"add_order","type":"update","method":"add_order","result":{"order_id":"exit-order"},"success":true,"req_id":%d,"time_out":"%s"}
{"channel":"executions","type":"update","data":[{"order_id":"exit-order","exec_id":"exit-partial","exec_type":"trade","symbol":"ALGO/USD","side":"sell","last_qty":%.12f,"last_price":"%.8f","cost":"%.12f","order_status":"partially_filled","cum_qty":%.12f,"cum_cost":"%.12f","avg_price":"%.8f","fee_usd_equiv":"%.12f","timestamp":"%s"}]}
`, position.reqID,
			entryAt.Add(4*time.Second).Format(time.RFC3339Nano), exitHalf,
			exitPrice, exitHalf*exitPrice, exitHalf,
			exitHalf*exitPrice, exitPrice, exitFee,
			entryAt.Add(4*time.Second).Format(time.RFC3339Nano),
		)))
		tests.NewMarket().Feed(exitPartial).Replay(handlers)
		So(position.Status(), ShouldEqual, types.PARTIAL_FILLED)
		So(thesis.LifecycleState("ALGO/USD"), ShouldEqual, types.LifecyclePartiallyExited)

		exitFilled := tests.NewReplayFixture([]byte(fmt.Sprintf(`
{"channel":"executions","type":"update","data":[{"order_id":"exit-order","exec_id":"exit-filled","exec_type":"trade","symbol":"ALGO/USD","side":"sell","last_qty":%.12f,"last_price":"%.8f","cost":"%.12f","order_status":"filled","cum_qty":%.12f,"cum_cost":"%.12f","avg_price":"%.8f","fee_usd_equiv":"%.12f","timestamp":"%s"}]}
`,
			exitHalf, exitPrice, exitHalf*exitPrice, exitQuantity,
			exitQuantity*exitPrice, exitPrice, exitFee,
			entryAt.Add(5*time.Second).Format(time.RFC3339Nano),
		)))
		tests.NewMarket().Feed(exitFilled).Replay(handlers)
		So(position.Status(), ShouldEqual, types.CLOSED)
		So(thesis.LifecycleState("ALGO/USD"), ShouldEqual, types.LifecycleClosed)

		postExit := types.NewThesis(nil)
		postExit.Forecasts = append(postExit.Forecasts, types.Forecasts{
			Symbol: "ALGO/USD", At: entryAt.Add(6 * time.Second),
			SourceEpoch: 12, HorizonEvents: 1,
		})
		ready := desk.PostExit(postExit)
		So(ready["ALGO/USD"], ShouldEqual, thesis)
		So((&strategy.PostMortem{}).Evaluate(thesis, "ALGO/USD"), ShouldBeNil)
		So(desk.Finalize("ALGO/USD", thesis), ShouldBeNil)

		_, exists = desk.positions.Load("ALGO/USD")
		So(exists, ShouldBeFalse)
		So(thesis.LifecycleState("ALGO/USD"), ShouldEqual, types.LifecycleEvaluated)
		So(thesis.Decisions, ShouldHaveLength, 2)
		So(thesis.Decisions[0].Action, ShouldEqual, "enter")
		So(thesis.Decisions[1].Action, ShouldEqual, "exit")
		So(thesis.Findings, ShouldHaveLength, 3)
	})
}

func TestDeskPostExitAndFinalize(t *testing.T) {
	Convey("Given a closed position retaining its completed trade Thesis", t, func() {
		desk := &Desk{positions: &sync.Map{}, ui: make(chan []byte, 1), balance: &Balance{}}
		thesis := types.NewThesis(nil)
		thesis.Forecasts = append(thesis.Forecasts, types.Forecasts{
			Symbol: "BTC/USD", At: time.Unix(2, 0),
			SourceEpoch: 10, HorizonEvents: 1,
		})
		thesis.Lifecycle["BTC/USD"] = types.LifecycleClosed
		thesis.TradeJournal = append(thesis.TradeJournal, types.TradeObservation{
			Kind: "lifecycle_transition", Symbol: "BTC/USD", Status: types.LifecycleClosed,
			At: time.Unix(3, 0),
		})
		desk.positions.Store("BTC/USD", &Position{
			status: types.CLOSED, thesis: thesis, Data: &PositionData{Symbol: "BTC/USD"},
		})
		current := types.NewThesis(nil)
		current.Forecasts = append(current.Forecasts, types.Forecasts{
			Symbol: "BTC/USD", At: time.Unix(4, 0), SourceEpoch: 11,
		})

		ready := desk.PostExit(current)

		Convey("Then the configured forecast horizon makes the same Thesis ready", func() {
			So(ready["BTC/USD"], ShouldEqual, thesis)
			So(thesis.LifecycleState("BTC/USD"), ShouldEqual, types.LifecyclePostMortemReady)
		})

		thesis.Lifecycle["BTC/USD"] = types.LifecycleEvaluated
		So(desk.Finalize("BTC/USD", thesis), ShouldBeNil)

		Convey("Then evaluated runtime state is removed and later entry is possible", func() {
			_, exists := desk.positions.Load("BTC/USD")
			So(exists, ShouldBeFalse)
		})
	})
}
