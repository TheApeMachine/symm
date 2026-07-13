package pumpdump

import (
	"context"
	"iter"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
	"github.com/theapemachine/symm/types"
)

func TestSignal_MeasureFromMarket(testingTB *testing.T) {
	Convey("Given a pumpdump signal fed by a market replay", testingTB, func() {
		signal := &Signal{
			ctx: context.Background(),
			ticker: &Ticker{
				cache: []kraken.TickerData{},
			},
			ignition: equation.NewIgnition(),
		}
		handlers := tests.Handlers{
			"ticker": signal.ticker.On,
		}
		market := tests.NewMarket().
			Feed(tickerfixture.NewFixture(tickerfixture.UPDATE, 32))

		Convey("When calm and pumped ticker timelines are measured", func() {
			calm := measureField(signal, handlers, market.Frames(), "rvol")
			pumped := measureField(
				signal,
				handlers,
				tests.Spike(market.Frames(), 16, 1.25, 8),
				"rvol",
			)

			Convey("Then the pumped stream should lift relative volume", func() {
				So(len(signal.ticker.cache), ShouldEqual, 0)
				So(pumped, ShouldBeGreaterThan, calm)
			})
		})
	})
}

func TestSignal_MeasureSkipsIncompleteRow(testingTB *testing.T) {
	Convey("Given a partial Kraken ticker row", testingTB, func() {
		signal := &Signal{
			ctx:      context.Background(),
			ticker:   &Ticker{cache: []kraken.TickerData{}},
			ignition: equation.NewIgnition(),
		}

		signal.ticker.cache = append(signal.ticker.cache, kraken.TickerData{
			Symbol:    "BTC/USD",
			Timestamp: time.Now(),
		})

		Convey("When measure runs", func() {
			result := signal.Measure(types.NewThesis(nil))

			Convey("Then it should wait without publishing metrics", func() {
				raw, ok := result.Measurements.Load("pumpdump")
				So(ok, ShouldBeTrue)

				out := raw.(datura.Map[datura.Map[*krakendecimal.Decimal]])
				So(out, ShouldBeEmpty)
			})
		})
	})
}

func TestSignal_Measure(testingTB *testing.T) {
	Convey("Given cached ticker rows", testingTB, func() {
		signal := &Signal{
			ctx: context.Background(),
			ticker: &Ticker{
				cache: []kraken.TickerData{},
			},
			ignition: equation.NewIgnition(),
		}
		fixture := tickerfixture.NewFixture(tickerfixture.UPDATE, 32)

		tests.Replay(tests.Handlers{"ticker": signal.ticker.On}, fixture.Frames())

		result := signal.Measure(types.NewThesis(nil))

		Convey("It should publish ignition metrics without categories", func() {
			raw, ok := result.Measurements.Load("pumpdump")
			So(ok, ShouldBeTrue)

			out, ok := raw.(datura.Map[datura.Map[*krakendecimal.Decimal]])
			So(ok, ShouldBeTrue)
			So(out["ALGO/USD"]["ignition"].Float64(), ShouldBeGreaterThan, 0)
			So(len(signal.ticker.cache), ShouldEqual, 0)
		})
	})
}

func measureField(
	signal *Signal,
	handlers tests.Handlers,
	frames iter.Seq[tests.Frame],
	key string,
) float64 {
	signal.ticker.cache = signal.ticker.cache[:0]
	tests.Replay(handlers, frames)

	thesis := types.NewThesis(nil)
	result := signal.Measure(thesis)

	raw, ok := result.Measurements.Load("pumpdump")

	if !ok {
		return 0
	}

	out, ok := raw.(datura.Map[datura.Map[*krakendecimal.Decimal]])

	if !ok || out["ALGO/USD"] == nil || out["ALGO/USD"][key] == nil {
		return 0
	}

	return out["ALGO/USD"][key].Float64()
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	signal := &Signal{
		ctx: context.Background(),
		ticker: &Ticker{
			cache: []kraken.TickerData{},
		},
		ignition: equation.NewIgnition(),
	}
	handlers := tests.Handlers{
		"ticker": signal.ticker.On,
	}
	market := tests.NewMarket().
		Feed(tickerfixture.NewFixture(tickerfixture.UPDATE, 32))

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		signal.ticker.cache = signal.ticker.cache[:0]
		tests.Replay(handlers, market.Frames())
		_ = signal.Measure(types.NewThesis(nil))
	}
}
