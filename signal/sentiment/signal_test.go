package sentiment

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

func drainSentiment(sub *types.Subscription[any]) []*kraken.Ticker {
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

func measureSentiment(
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

	for _, ticker := range drainSentiment(tickerSub) {
		for _, row := range ticker.Data {
			thesis.Tickers.Store(row.Symbol, row)
		}
		thesis.Tick++
		signal.Measure(thesis)
	}

	consume := func(into *[]*types.Measurement) func() error {
		return func() error {
			for _, ticker := range drainSentiment(tickerSub) {
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
	Convey("Sentiment distinguishes cohort surge from isolated divergence", t, func() {
		metrics := []types.MetricType{
			types.MetricSurgeScore,
			types.MetricDivergentScore,
		}

		pump := tests.PeakMeasurements(
			measureSentiment(t, tests.MarketStateFastPump),
			types.SourceSentiment,
			metrics,
		)
		isolated := tests.PeakMeasurements(
			measureSentiment(t, tests.MarketStateFastPump, "SIM1/USD"),
			types.SourceSentiment,
			metrics,
		)

		for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
			So(pump[types.MetricSurgeScore][symbol], ShouldBeGreaterThanOrEqualTo, 0)
		}

		So(isolated[types.MetricDivergentScore]["SIM1/USD"], ShouldBeGreaterThan, 0)
	})

	Convey("Sentiment anchors cross-sectional scores to the complete cohort epoch", t, func() {
		rows := measureSentiment(t, tests.MarketStateBaseline)
		So(rows, ShouldNotBeEmpty)
		from := rows[0].Scale.From
		through := rows[0].Scale.Through
		So(from.IsZero(), ShouldBeFalse)
		So(through.Before(from), ShouldBeFalse)

		for _, measurement := range rows {
			So(measurement.ObservedFrom.IsZero(), ShouldBeTrue)
			So(measurement.Scale.Kind, ShouldEqual, types.ScaleObservationWindow)
			So(measurement.Scale.From, ShouldResemble, from)
			So(measurement.Scale.Through, ShouldResemble, through)
		}
	})
}
