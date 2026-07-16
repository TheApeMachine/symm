package sentiment

import (
	"context"
	"math"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func lastMeasurement(
	measurements []*types.Measurement, symbol string, metric types.MetricType,
) (*types.Measurement, bool) {
	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Symbol == symbol && measurement.Metric == metric {
			return measurement, true
		}
	}

	return nil, false
}

/*
frameFrom builds an immutable market cut from raw ticker rows, mirroring the
central feed: it measures the cross-section once and shares it with Calculate.
*/
func frameFrom(rows ...kraken.TickerData) *types.MarketFrame {
	crossSection := types.NewCrossSection()
	crossSection.Measure(rows)

	return &types.MarketFrame{
		Tickers:      rows,
		CrossSection: crossSection,
	}
}

func TestSignal_MeasureFromMarket(testingTB *testing.T) {
	Convey("Given a sentiment signal fed by a market cut", testingTB, func() {
		signal := &Signal{ctx: context.Background()}
		now := time.Now()

		calmRows := []kraken.TickerData{
			{Symbol: "ALGO/USD", ChangePct: 1, Last: krakendecimal.NewFromFloat64(101), Volume: 10, Timestamp: now},
			{Symbol: "ETH/USD", ChangePct: 0.2, Last: krakendecimal.NewFromFloat64(100), Volume: 10, Timestamp: now},
		}
		pumpedRows := []kraken.TickerData{
			{Symbol: "ALGO/USD", ChangePct: 25, Last: krakendecimal.NewFromFloat64(125), Volume: 10, Timestamp: now},
			{Symbol: "ETH/USD", ChangePct: 0.2, Last: krakendecimal.NewFromFloat64(100), Volume: 10, Timestamp: now},
		}

		Convey("When calm and pumped ticker cohorts are measured", func() {
			calm := measureField(signal, calmRows, types.MetricChange)
			pumped := measureField(signal, pumpedRows, types.MetricChange)

			Convey("Then the pumped stream should amplify measured change", func() {
				So(math.Abs(pumped), ShouldBeGreaterThan, math.Abs(calm))
			})
		})
	})
}

func TestSignal_Measure(testingTB *testing.T) {
	Convey("Given current ticker rows with one clear cohort leader", testingTB, func() {
		now := time.Now()
		signal := &Signal{ctx: context.Background()}

		frame := frameFrom(
			kraken.TickerData{
				Symbol:    "BTC/USD",
				ChangePct: 5,
				Last:      krakendecimal.NewFromFloat64(105),
				Volume:    10,
				Timestamp: now,
			},
			kraken.TickerData{
				Symbol:    "ETH/USD",
				ChangePct: 2,
				Last:      krakendecimal.NewFromFloat64(102),
				Volume:    10,
				Timestamp: now,
			},
			kraken.TickerData{
				Symbol:    "SOL/USD",
				ChangePct: -1,
				Last:      krakendecimal.NewFromFloat64(99),
				Volume:    10,
				Timestamp: now,
			},
		)

		result, err := signal.Calculate(frame)
		So(err, ShouldBeNil)

		Convey("It should publish breadth and leader scores without categories", func() {
			breadth, ok := lastMeasurement(result, "BTC/USD", types.MetricBreadth)
			So(ok, ShouldBeTrue)
			So(breadth.Raw, ShouldAlmostEqual, 2.0/3.0, 0.0001)

			surge, ok := lastMeasurement(result, "BTC/USD", types.MetricSurgeScore)
			So(ok, ShouldBeTrue)
			So(surge.Raw, ShouldBeGreaterThan, 0)

			strength, ok := lastMeasurement(result, "BTC/USD", types.MetricStrength)
			So(ok, ShouldBeTrue)
			So(strength.Raw, ShouldBeLessThanOrEqualTo, 1)
		})
	})
}

func measureField(
	signal *Signal,
	rows []kraken.TickerData,
	metric types.MetricType,
) float64 {
	result, err := signal.Calculate(frameFrom(rows...))

	if err != nil {
		return 0
	}

	measurement, ok := lastMeasurement(result, "ALGO/USD", metric)

	if !ok {
		return 0
	}

	return measurement.Raw
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	now := time.Now()
	signal := &Signal{ctx: context.Background()}
	frame := frameFrom(
		kraken.TickerData{
			Symbol:    "BTC/USD",
			ChangePct: 5,
			Last:      krakendecimal.NewFromFloat64(105),
			Volume:    10,
			Timestamp: now,
		},
		kraken.TickerData{
			Symbol:    "ETH/USD",
			ChangePct: 2,
			Last:      krakendecimal.NewFromFloat64(102),
			Volume:    10,
			Timestamp: now,
		},
	)

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _ = signal.Calculate(frame)
	}
}
