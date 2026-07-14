package broker

import (
	"sync"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/strategy"
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
			Data: &PositionData{
				Qty: *decimal.NewFromInt64(1), Mark: *decimal.NewFromInt64(100),
			},
		})
		desk.positions.Store("ETH/USD", &Position{
			thesis: eth,
			Data: &PositionData{
				Qty: *decimal.NewFromInt64(1), Mark: *decimal.NewFromInt64(100),
			},
		})

		exposures := desk.Exposures()

		Convey("Then strategy receives the exact lifecycle for each position", func() {
			So(exposures, ShouldHaveLength, 2)
			So(exposures["BTC/USD"].Thesis, ShouldEqual, btc)
			So(exposures["ETH/USD"].Thesis, ShouldEqual, eth)
		})
	})
}

func TestDeskExecuteRecordsRejection(t *testing.T) {
	Convey("Given a selected action the broker cannot execute", t, func() {
		thesis := types.NewThesis(nil)
		So(thesis.Transition(
			"BTC/USD", types.LifecycleShaped, time.Unix(1, 0),
		), ShouldBeNil)
		So(thesis.Transition(
			"BTC/USD", types.LifecycleEntrySelected, time.Unix(2, 0),
		), ShouldBeNil)
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
}

func TestDeskPostExitAndFinalize(t *testing.T) {
	Convey("Given a closed position retaining its completed trade Thesis", t, func() {
		desk := &Desk{
			positions: &sync.Map{},
			ui:        make(chan []byte, 1),
			balance:   &Balance{},
		}
		thesis := types.NewThesis(nil)
		thesis.Forecasts = append(thesis.Forecasts, types.Forecasts{
			Symbol: "BTC/USD", At: time.Unix(2, 0), SourceEpoch: 10, HorizonEvents: 1,
		})
		thesis.Lifecycle["BTC/USD"] = types.LifecycleClosed
		thesis.TradeJournal = append(thesis.TradeJournal, types.TradeObservation{
			Kind: "lifecycle_transition", Symbol: "BTC/USD",
			Status: types.LifecycleClosed, At: time.Unix(3, 0),
		})
		desk.positions.Store("BTC/USD", &Position{
			status: types.CLOSED,
			thesis: thesis,
			Data:   &PositionData{Symbol: "BTC/USD"},
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
