package liquidity

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

func drainLiquidity(sub *types.Subscription[any]) []*kraken.Ticker {
	out := make([]*kraken.Ticker, 0)

	for {
		select {
		case frame := <-sub.Channel:
			if ticker, ok := frame.(*kraken.Ticker); ok {
				out = append(out, ticker)
			}
		default:
			return out
		}
	}
}

func measureLiquidity(
	t *testing.T,
	state tests.MarketState,
	focus ...string,
) []*types.Measurement {
	market := tests.NewMarket(t.Context(), 3)
	So(market.Bootstrap(), ShouldBeNil)
	defer market.Close()

	thesis := types.NewThesis()
	signal := &Signal{ui: make(chan []byte, 32)}
	tickerSub := market.Public.Subscribe("ticker")

	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"ticker","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)

	for _, ticker := range drainLiquidity(tickerSub) {
		for _, row := range ticker.Data {
			thesis.Tickers.Store(row.Symbol, row)
		}
		thesis.Tick++
		signal.Measure(thesis)
	}

	consume := func(into *[]*types.Measurement) func() error {
		return func() error {
			for _, ticker := range drainLiquidity(tickerSub) {
				for _, row := range ticker.Data {
					thesis.Tickers.Store(row.Symbol, row)
				}
				thesis.Tick++
				*into = append(*into, signal.Measure(thesis)...)
			}

			return nil
		}
	}

	So(market.Warmup(consume(&[]*types.Measurement{})), ShouldBeNil)
	rows := make([]*types.Measurement, 0)
	So(market.Transition(state, consume(&rows), focus...), ShouldBeNil)

	return rows
}

func TestCalculate(t *testing.T) {
	Convey("Liquidity measures thin and loaded touch states from market fixtures", t, func() {
		metrics := []types.MetricType{
			types.MetricRelativeTouchDepth,
			types.MetricScarcityScore,
		}

		baseline := tests.LatestMeasurements(
			measureLiquidity(t, tests.MarketStateBaseline),
			types.SourceLiquidity,
			metrics,
		)
		thin := tests.LatestMeasurements(
			measureLiquidity(t, tests.MarketStateThinLiquidity, "SIM1/USD"),
			types.SourceLiquidity,
			metrics,
		)
		loaded := tests.LatestMeasurements(
			measureLiquidity(t, tests.MarketStateLoadedLiquidity, "SIM1/USD"),
			types.SourceLiquidity,
			metrics,
		)

		So(thin[types.MetricRelativeTouchDepth]["SIM1/USD"], ShouldBeLessThan, baseline[types.MetricRelativeTouchDepth]["SIM1/USD"])
		So(thin[types.MetricScarcityScore]["SIM1/USD"], ShouldBeGreaterThanOrEqualTo, baseline[types.MetricScarcityScore]["SIM1/USD"])
		So(loaded[types.MetricRelativeTouchDepth]["SIM1/USD"], ShouldBeGreaterThanOrEqualTo, baseline[types.MetricRelativeTouchDepth]["SIM1/USD"])
	})
}
