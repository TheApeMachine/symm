package exhaust

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

func drainExhaustTrades(sub *types.Subscription[any]) []kraken.TradeData {
	out := make([]kraken.TradeData, 0)

	for {
		select {
		case frame := <-sub.Channel:
			if trade, ok := frame.(*kraken.Trade); ok {
				out = append(out, trade.Data...)
			}
		default:
			return out
		}
	}
}

func measureExhaust(t *testing.T, state tests.MarketState, focus ...string) []*types.Measurement {
	market := tests.NewMarket(t.Context(), 3)
	So(market.Bootstrap(), ShouldBeNil)
	defer market.Close()

	thesis := types.NewThesis()
	signal := &Signal{
		sample: algorithm.NewDecaySample(),
		decay:  equation.NewDecay(),
		ui:     make(chan []byte, 32),
	}
	tradeSub := market.Public.Subscribe("trade")

	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"trade","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)
	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"book","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)

	for _, trade := range drainExhaustTrades(tradeSub) {
		thesis.Trades.Store(trade.Symbol, trade)
	}
	thesis.Tick++
	signal.Measure(thesis)

	consume := func(into *[]*types.Measurement) func() error {
		return func() error {
			for _, trade := range drainExhaustTrades(tradeSub) {
				thesis.Trades.Store(trade.Symbol, trade)
			}
			thesis.Tick++
			*into = append(*into, signal.Measure(thesis)...)

			return nil
		}
	}

	So(market.Warmup(consume(&[]*types.Measurement{})), ShouldBeNil)
	rows := make([]*types.Measurement, 0)
	So(market.Transition(state, consume(&rows), focus...), ShouldBeNil)

	return rows
}

func TestCalculate(t *testing.T) {
	Convey("Exhaust raises urgency on rejecting and fast directional tapes", t, func() {
		baseline := measureExhaust(t, tests.MarketStateBaseline)
		rejection := measureExhaust(t, tests.MarketStateFastDump)

		So(baseline, ShouldNotBeEmpty)
		So(rejection, ShouldNotBeEmpty)

		foundUrgency := false

		for _, measurement := range rejection {
			measurement.EachMetric(func(metric types.MetricType, side types.MeasurementSide, sample types.MetricSample) bool {
				if metric == types.MetricUrgency && (side == types.SideBuy || side == types.SideSell) && sample.Raw > 0 {
					foundUrgency = true
				}

				return true
			})
		}

		So(foundUrgency, ShouldBeTrue)
	})

	Convey("Exhaust emits scored frames as point observations", t, func() {
		rows := measureExhaust(t, tests.MarketStateBaseline)
		So(rows, ShouldNotBeEmpty)

		for _, measurement := range rows {
			So(measurement.ObservedFrom.IsZero(), ShouldBeTrue)
			So(measurement.Horizon, ShouldEqual, 0)
			So(measurement.Scale.From.IsZero(), ShouldBeTrue)
			So(measurement.Scale.Through.IsZero(), ShouldBeTrue)
			So(measurement.ValidateStruct(), ShouldBeNil)
		}
	})
}
