package correlation

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

func drainCorrelation(sub *types.Subscription[any]) []*kraken.Ticker {
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

func measureCorrelation(
	t *testing.T,
	state tests.MarketState,
	focus ...string,
) []*types.Measurement {
	market := tests.NewMarket(t.Context(), 3)
	So(market.Bootstrap(), ShouldBeNil)
	defer market.Close()

	signal := &Signal{
		section: NewSection(),
		ui:      make(chan []byte, 32),
	}
	thesis := types.NewThesis()
	tickerSub := market.Public.Subscribe("ticker")

	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"ticker","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)

	for _, ticker := range drainCorrelation(tickerSub) {
		for _, row := range ticker.Data {
			thesis.Tickers.Store(row.Symbol, row)
		}

		signal.Measure(thesis)
	}

	consume := func(into *[]*types.Measurement) func() error {
		return func() error {
			for _, ticker := range drainCorrelation(tickerSub) {
				for _, row := range ticker.Data {
					thesis.Tickers.Store(row.Symbol, row)
				}

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
	Convey("Correlation measures herd and isolated alpha from market fixtures", t, func() {
		metrics := []types.MetricType{
			types.MetricHerdScore,
			types.MetricAlphaScore,
		}

		baseline := tests.PeakMeasurements(
			measureCorrelation(t, tests.MarketStateBaseline),
			types.SourceCorrelation,
			metrics,
		)
		pump := tests.PeakMeasurements(
			measureCorrelation(t, tests.MarketStateFastPump),
			types.SourceCorrelation,
			metrics,
		)
		isolated := tests.PeakMeasurements(
			measureCorrelation(t, tests.MarketStateFastPump, "SIM1/USD"),
			types.SourceCorrelation,
			metrics,
		)

		for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
			So(pump[types.MetricHerdScore][symbol], ShouldBeGreaterThanOrEqualTo, baseline[types.MetricHerdScore][symbol])
		}

		So(isolated[types.MetricAlphaScore]["SIM1/USD"], ShouldBeGreaterThan, 0)
		So(isolated[types.MetricAlphaScore]["SIM1/USD"], ShouldBeGreaterThanOrEqualTo, pump[types.MetricAlphaScore]["SIM1/USD"])
	})
}
