package pumpdump

import (
	"encoding/json"
	"sync"
	"testing"

	bookflow "github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

func drainPumpTickers(sub *types.Subscription[any]) []kraken.TickerData {
	out := make([]kraken.TickerData, 0)

	for {
		select {
		case frame := <-sub.Channel:
			if ticker, ok := frame.(*kraken.Ticker); ok {
				out = append(out, ticker.Data...)
			}
		default:
			return out
		}
	}
}

func drainPumpTrades(sub *types.Subscription[any]) []kraken.TradeData {
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

func drainPumpBooks(sub *types.Subscription[any]) []kraken.BookData {
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

func measurePumpdump(t *testing.T, state tests.MarketState, focus ...string) []*types.Measurement {
	market := tests.NewMarket(t.Context(), 3)
	So(market.Bootstrap(), ShouldBeNil)
	defer market.Close()

	thesis := types.NewThesis()
	thesis.Causal.Store("signal:pumpdump:ignition", equation.NewIgnition(256))
	thesis.Causal.Store("signal:pumpdump:volume", &sync.Map{})
	thesis.Causal.Store("signal:pumpdump:books", &sync.Map{})
	thesis.Causal.Store("signal:pumpdump:increments", &sync.Map{})
	thesis.Causal.Store("signal:pumpdump:lastAt", &sync.Map{})
	thesis.Causal.Store("signal:pumpdump:lastWire", &sync.Map{})
	_ = bookflow.NewBook
	signal := &Signal{thesis: thesis, ui: make(chan []byte, 32)}
	tickerSub := market.Public.Subscribe("ticker")
	tradeSub := market.Public.Subscribe("trade")
	bookSub := market.Public.Subscribe("book")

	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"trade","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)
	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"book","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)
	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"ticker","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)

	signal.thesis.Tick++
	signal.Calculate(drainPumpTickers(tickerSub), drainPumpTrades(tradeSub), drainPumpBooks(bookSub))

	consume := func(into *[]*types.Measurement) func() error {
		return func() error {
			signal.thesis.Tick++
			*into = append(*into, signal.Calculate(
				drainPumpTickers(tickerSub),
				drainPumpTrades(tradeSub),
				drainPumpBooks(bookSub),
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
	Convey("Pumpdump raises ignition and trend on fast directional tapes", t, func() {
		metrics := []types.MetricType{types.MetricIgnition, types.MetricTrend, types.MetricStrength}

		baseline := tests.PeakMeasurements(measurePumpdump(t, tests.MarketStateBaseline), types.SourcePumpDump, metrics)
		pump := tests.PeakMeasurements(measurePumpdump(t, tests.MarketStateFastPump), types.SourcePumpDump, metrics)

		So(pump[types.MetricIgnition]["SIM1/USD"], ShouldBeGreaterThanOrEqualTo, baseline[types.MetricIgnition]["SIM1/USD"])
		So(pump[types.MetricTrend]["SIM1/USD"], ShouldBeGreaterThanOrEqualTo, baseline[types.MetricTrend]["SIM1/USD"])
	})
}
