package pumpdump

import (
	"context"
	"iter"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
	"github.com/theapemachine/symm/types"
)

/*
drive replays a ticker timeline through Calculate one frame at a time, warming
the shared ignition state exactly as the live feed does, and returns the last
frame's measurements.
*/
func drive(signal *Signal, frames iter.Seq[tests.Frame]) []*types.Measurement {
	var last []*types.Measurement

	for frame := range frames {
		rows := kraken.NewTicker(frame.Payload).Data

		if len(rows) == 0 {
			continue
		}

		measurements, err := signal.Calculate(&types.MarketFrame{
			Tickers:      rows,
			CrossSection: types.NewCrossSection(),
		})

		if err != nil {
			continue
		}

		last = measurements
	}

	return last
}

/*
peakField replays a timeline and returns the greatest ALGO/USD value observed
for the metric, so a transient spike mid-stream is not masked by the calm tail.
*/
func peakField(
	signal *Signal, frames iter.Seq[tests.Frame], metric types.MetricType,
) (float64, bool) {
	peak := 0.0
	found := false

	for frame := range frames {
		rows := kraken.NewTicker(frame.Payload).Data

		if len(rows) == 0 {
			continue
		}

		measurements, err := signal.Calculate(&types.MarketFrame{
			Tickers:      rows,
			CrossSection: types.NewCrossSection(),
		})

		if err != nil {
			continue
		}

		for _, measurement := range measurements {
			if measurement.Symbol == "ALGO/USD" && measurement.Metric == metric {
				found = true

				if measurement.Raw > peak {
					peak = measurement.Raw
				}
			}
		}
	}

	return peak, found
}

func newSignal() *Signal {
	return &Signal{
		ctx:      context.Background(),
		ignition: equation.NewIgnition(),
	}
}

func TestSignal_MeasureFromMarket(testingTB *testing.T) {
	Convey("Given a pumpdump signal fed by a market replay", testingTB, func() {
		market := tests.NewMarket().
			Feed(tickerfixture.NewFixture(tickerfixture.UPDATE, 32))

		Convey("When calm and pumped ticker timelines are measured", func() {
			calm, hasCalm := peakField(newSignal(), market.Frames(), types.MetricRVOL)
			pumped, hasPumped := peakField(
				newSignal(), tests.Spike(market.Frames(), 16, 1.25, 8), types.MetricRVOL,
			)

			Convey("Then the pumped stream should lift relative volume", func() {
				So(hasCalm, ShouldBeTrue)
				So(hasPumped, ShouldBeTrue)
				So(pumped, ShouldBeGreaterThan, calm)
			})
		})
	})
}

func TestSignal_MeasureSkipsIncompleteRow(testingTB *testing.T) {
	Convey("Given a partial Kraken ticker row", testingTB, func() {
		signal := newSignal()

		Convey("When measure runs", func() {
			result, err := signal.Calculate(&types.MarketFrame{
				Tickers: []kraken.TickerData{
					{Symbol: "BTC/USD"},
				},
				CrossSection: types.NewCrossSection(),
			})

			Convey("Then it should wait without publishing metrics", func() {
				So(err, ShouldBeNil)
				So(result, ShouldBeEmpty)
			})
		})
	})
}

func TestSignal_Measure(testingTB *testing.T) {
	Convey("Given a replayed ticker timeline", testingTB, func() {
		signal := newSignal()
		fixture := tickerfixture.NewFixture(tickerfixture.UPDATE, 32)

		result := drive(signal, fixture.Frames())

		Convey("It should publish ignition metrics without categories", func() {
			ignition := 0.0

			for _, measurement := range result {
				if measurement.Symbol == "ALGO/USD" && measurement.Metric == types.MetricIgnition {
					ignition = measurement.Raw
				}
			}

			So(ignition, ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	market := tests.NewMarket().
		Feed(tickerfixture.NewFixture(tickerfixture.UPDATE, 32))

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_ = drive(newSignal(), market.Frames())
	}
}
