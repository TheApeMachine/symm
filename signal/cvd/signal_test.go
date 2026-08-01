package cvd

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

func drainCVDTickers(sub *types.Subscription[any]) []kraken.TickerData {
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

func drainCVDTrades(sub *types.Subscription[any]) []kraken.TradeData {
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

func causalEntryCount(thesis *types.Thesis) int {
	count := 0

	thesis.Causal.Range(func(_, _ any) bool {
		count++
		return true
	})

	return count
}

func measureCVD(t *testing.T, state tests.MarketState, focus ...string) ([]*types.Measurement, int) {
	market := tests.NewMarket(t.Context(), 3)
	So(market.Bootstrap(), ShouldBeNil)
	defer market.Close()

	thesis := types.NewThesis()
	signal := &Signal{ui: make(chan []byte, 32)}
	tickerSub := market.Public.Subscribe("ticker")
	tradeSub := market.Public.Subscribe("trade")

	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"ticker","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)
	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"trade","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)

	for _, ticker := range drainCVDTickers(tickerSub) {
		thesis.Tickers.Store(ticker.Symbol, ticker)
	}

	for _, trade := range drainCVDTrades(tradeSub) {
		thesis.Trades.Store(trade.Symbol, trade)
	}

	thesis.Tick++
	signal.Measure(thesis)

	consume := func(into *[]*types.Measurement) func() error {
		return func() error {
			for _, ticker := range drainCVDTickers(tickerSub) {
				thesis.Tickers.Store(ticker.Symbol, ticker)
			}

			for _, trade := range drainCVDTrades(tradeSub) {
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

	return rows, causalEntryCount(thesis)
}

func TestCalculate(t *testing.T) {
	Convey("CVD distinguishes directional drive from absorption", t, func() {
		metrics := []types.MetricType{types.MetricDrive, types.MetricAbsorption}
		baselineRows, baselineCausal := measureCVD(t, tests.MarketStateBaseline)

		baseline := tests.PeakMagnitudeMeasurements(
			baselineRows,
			types.SourceCVD,
			metrics,
		)

		pumpRows, pumpCausal := measureCVD(t, tests.MarketStateFastPump)
		pump := tests.PeakMagnitudeMeasurements(
			pumpRows,
			types.SourceCVD,
			metrics,
		)

		absorptionRows, absorptionCausal := measureCVD(t, tests.MarketStateVolumeAbsorption)
		absorption := tests.PeakMagnitudeMeasurements(
			absorptionRows,
			types.SourceCVD,
			metrics,
		)

		So(pump[types.MetricDrive]["SIM1/USD"], ShouldNotEqual, baseline[types.MetricDrive]["SIM1/USD"])
		So(absorption[types.MetricAbsorption]["SIM1/USD"], ShouldNotEqual, baseline[types.MetricAbsorption]["SIM1/USD"])
		So(baselineCausal, ShouldEqual, 0)
		So(pumpCausal, ShouldEqual, 0)
		So(absorptionCausal, ShouldEqual, 0)
	})
}
