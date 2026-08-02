package pumpdump

import (
	"encoding/json"
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

func pumpdumpCausalEntryCount(thesis *types.Thesis) int {
	count := 0

	thesis.Causal.Range(func(_, _ any) bool {
		count++
		return true
	})

	return count
}

func measurePumpdump(
	t *testing.T,
	state tests.MarketState,
	focus ...string,
) ([]*types.Measurement, int) {
	market := tests.NewMarket(t.Context(), 3)
	So(market.Bootstrap(), ShouldBeNil)
	defer market.Close()

	thesis := types.NewThesis()
	_ = bookflow.NewBook
	signal := &Signal{algo: equation.NewIgnition(256), ui: make(chan []byte, 32)}
	tickerSub := market.Public.Subscribe("ticker")
	tradeSub := market.Public.Subscribe("trade")

	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"trade","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)
	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"book","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)
	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"ticker","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)

	for _, ticker := range drainPumpTickers(tickerSub) {
		thesis.Tickers.Store(ticker.Symbol, ticker)
	}
	for _, trade := range drainPumpTrades(tradeSub) {
		thesis.Trades.Store(trade.Symbol, trade)
	}
	thesis.Tick++
	signal.Measure(thesis)

	consume := func(into *[]*types.Measurement) func() error {
		return func() error {
			for _, ticker := range drainPumpTickers(tickerSub) {
				thesis.Tickers.Store(ticker.Symbol, ticker)
			}
			for _, trade := range drainPumpTrades(tradeSub) {
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

	return rows, pumpdumpCausalEntryCount(thesis)
}

func TestCalculate(t *testing.T) {
	Convey("Pumpdump raises ignition and trend on fast directional tapes", t, func() {
		metrics := []types.MetricType{types.MetricIgnition, types.MetricTrend, types.MetricStrength}
		baselineRows, baselineCausal := measurePumpdump(t, tests.MarketStateBaseline)

		baseline := tests.PeakMeasurements(baselineRows, types.SourcePumpDump, metrics)
		pumpRows, pumpCausal := measurePumpdump(t, tests.MarketStateFastPump)
		pump := tests.PeakMeasurements(pumpRows, types.SourcePumpDump, metrics)

		So(pump[types.MetricIgnition]["SIM1/USD"], ShouldBeGreaterThanOrEqualTo, baseline[types.MetricIgnition]["SIM1/USD"])
		So(pump[types.MetricTrend]["SIM1/USD"], ShouldBeGreaterThanOrEqualTo, baseline[types.MetricTrend]["SIM1/USD"])
		So(baselineCausal, ShouldEqual, 0)
		So(pumpCausal, ShouldEqual, 0)
	})

	Convey("Pumpdump reports estimator readiness without fabricating a scale epoch", t, func() {
		rows, _ := measurePumpdump(t, tests.MarketStateBaseline)
		So(rows, ShouldNotBeEmpty)

		for _, measurement := range rows {
			So(measurement.ObservedFrom.IsZero(), ShouldBeTrue)
			So(measurement.Scale.From.IsZero(), ShouldBeTrue)
			So(measurement.Scale.Through.IsZero(), ShouldBeTrue)
			So(measurement.Validity.Readiness, ShouldEqual, types.ReadinessObservation)

			if measurement.Maturity == 0 {
				So(measurement.Validity.State, ShouldEqual, types.ValidityProvisional)
			}
		}
	})
}
