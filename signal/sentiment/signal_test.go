package sentiment

import (
	"context"
	"iter"
	"math"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
	"github.com/theapemachine/symm/types"
)

func TestSignal_MeasureFromMarket(testingTB *testing.T) {
	Convey("Given a sentiment signal fed by a market replay", testingTB, func() {
		signal := &Signal{
			ctx: context.Background(),
			ticker: &Ticker{
				cache: []kraken.TickerData{},
			},
		}
		handlers := tests.Handlers{
			"ticker": signal.ticker.On,
		}
		market := tests.NewMarket().
			Feed(tickerfixture.NewFixture(tickerfixture.UPDATE, 32))

		Convey("When calm and pumped ticker timelines are measured", func() {
			calm := measureField(signal, handlers, market.Frames(), "change")
			pumped := measureField(
				signal,
				handlers,
				tests.Spike(market.Frames(), 16, 1.25, 1),
				"change",
			)

			Convey("Then the pumped stream should amplify measured change", func() {
				So(len(signal.ticker.cache), ShouldEqual, 0)
				So(math.Abs(pumped), ShouldBeGreaterThan, math.Abs(calm))
			})
		})
	})
}

func TestSignal_Measure(testingTB *testing.T) {
	Convey("Given cached ticker rows with enough history to establish a leadership threshold", testingTB, func() {
		now := time.Now()
		signal := &Signal{
			ctx:    context.Background(),
			ticker: &Ticker{cache: []kraken.TickerData{}},
		}

		/*
			LeadershipThreshold only counts a symbol once it has at least
			MinBars observations, so a single tick per symbol can never
			establish a threshold and IsLeader always reports false. Seed
			each symbol with MinBars bars to reflect that a leadership
			signal needs real history behind it, not a synthetic minimum
			the test invents on its own.
		*/
		minBars := types.DefaultCrossSectionConfig().MinBars
		seed := func(symbol string, changePct, last float64) []kraken.TickerData {
			rows := make([]kraken.TickerData, 0, minBars)

			for bar := range minBars {
				rows = append(rows, kraken.TickerData{
					Symbol:    symbol,
					ChangePct: changePct,
					Last:      krakendecimal.NewFromFloat64(last),
					Timestamp: now.Add(time.Duration(bar) * time.Second),
				})
			}

			return rows
		}

		signal.ticker.cache = append(signal.ticker.cache, seed("BTC/USD", 5, 105)...)
		signal.ticker.cache = append(signal.ticker.cache, seed("ETH/USD", 2, 102)...)
		signal.ticker.cache = append(signal.ticker.cache, seed("SOL/USD", -1, 99)...)

		thesis := types.NewThesis(nil)

		result := signal.Measure(thesis)

		Convey("It should publish breadth and leader scores without categories", func() {
			raw, ok := result.Measurements.Load("sentiment")
			So(ok, ShouldBeTrue)

			out, ok := raw.(datura.Map[datura.Map[*krakendecimal.Decimal]])
			So(ok, ShouldBeTrue)
			So(out["BTC/USD"]["breadth"].Float64(), ShouldAlmostEqual, 2.0/3.0, 0.0001)
			So(out["BTC/USD"]["surgeScore"].Float64(), ShouldBeGreaterThan, 0)
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

	raw, ok := result.Measurements.Load("sentiment")

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
	now := time.Now()
	signal := &Signal{
		ctx: context.Background(),
		ticker: &Ticker{
			cache: []kraken.TickerData{
				{
					Symbol:    "BTC/USD",
					ChangePct: 5,
					Last:      krakendecimal.NewFromFloat64(105),
					Timestamp: now,
				},
				{
					Symbol:    "ETH/USD",
					ChangePct: 2,
					Last:      krakendecimal.NewFromFloat64(102),
					Timestamp: now,
				},
			},
		},
	}
	thesis := types.NewThesis(nil)

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		signal.ticker.cache = []kraken.TickerData{
			{
				Symbol:    "BTC/USD",
				ChangePct: 5,
				Last:      krakendecimal.NewFromFloat64(105),
				Timestamp: now,
			},
			{
				Symbol:    "ETH/USD",
				ChangePct: 2,
				Last:      krakendecimal.NewFromFloat64(102),
				Timestamp: now,
			},
		}
		_ = signal.Measure(thesis)
	}
}
