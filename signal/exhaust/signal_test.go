package exhaust

import (
	"encoding/json"
	"testing"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"

	. "github.com/smartystreets/goconvey/convey"
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

func drainExhaustBooks(sub *types.Subscription[any]) []kraken.BookData {
	out := make([]kraken.BookData, 0)

	for {
		select {
		case frame := <-sub.Channel:
			if book, ok := frame.(*kraken.Book); ok {
				out = append(out, book.Data...)
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
	thesis.Causal.Store("signal:exhaust:sample", algorithm.NewDecaySample())
	thesis.Causal.Store("signal:exhaust:decay", equation.NewDecay())
	signal := &Signal{thesis: thesis, ui: make(chan []byte, 32)}
	tradeSub := market.Public.Subscribe("trade")
	bookSub := market.Public.Subscribe("book")

	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"trade","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)
	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"book","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)

	signal.thesis.Tick++
	signal.Calculate(nil, drainExhaustTrades(tradeSub), drainExhaustBooks(bookSub))

	consume := func(into *[]*types.Measurement) func() error {
		return func() error {
			signal.thesis.Tick++
			*into = append(*into, signal.Calculate(
				nil,
				drainExhaustTrades(tradeSub),
				drainExhaustBooks(bookSub),
			)...)

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
}
