package correlation

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMeasure(t *testing.T) {
	Convey("Given repeated multi-symbol correlation cuts", t, func() {
		signal := &Signal{ctx: context.Background(), section: NewSection()}
		start := time.Unix(1_700_005_000, 0).UTC()

		Reset(func() {
			signal.Close()
		})

		for leg, prices := range [][]float64{
			{100, 200},
			{110, 220},
			{121, 242},
		} {
			at := start.Add(time.Duration(leg) * time.Second)
			thesis := types.NewThesis(t.Context(), nil)
			thesis.Tickers.Store("AAA/USD", []kraken.TickerData{
				correlationTicker("AAA/USD", prices[0], at),
			})
			thesis.Tickers.Store("BBB/USD", []kraken.TickerData{
				correlationTicker("BBB/USD", prices[1], at),
			})
			_ = signal.Measure(thesis)
		}

		Convey("It reuses the section group and constructs every symbol measurement", func() {
			at := start.Add(3 * time.Second)
			thesis := types.NewThesis(t.Context(), nil)
			thesis.Tickers.Store("AAA/USD", []kraken.TickerData{
				correlationTicker("AAA/USD", 133.1, at),
			})
			thesis.Tickers.Store("BBB/USD", []kraken.TickerData{
				correlationTicker("BBB/USD", 266.2, at),
			})

			measurements := signal.Measure(thesis)
			So(measurements, ShouldHaveLength, 2)
		})
	})
}

func correlationTicker(symbol string, price float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol: symbol, Last: decimal.NewFromFloat64(price), Timestamp: at,
	}
}

func TestCorrelationMetrics(t *testing.T) {
	Convey("Given cohort scores with their equation-defined domains", t, func() {
		metrics, valid := correlationMetrics(map[string]float64{
			"correlation":    0.8,
			"signed":         -0.6,
			"relativeEnergy": 1.5,
			"herdScore":      0.2,
			"alphaScore":     0.3,
			"noiseScore":     0.4,
			"stressScore":    0.5,
			"peakScore":      0.5,
			"strength":       0.5,
		})

		Convey("It should retain signed and relative evidence without fake scaling", func() {
			So(valid, ShouldBeTrue)
			So(*metrics[types.MetricKey(types.MetricSigned, types.SideNone)].Normalized,
				ShouldAlmostEqual, -0.6, 1e-12)
			So(*metrics[types.MetricKey(types.MetricRelativeEnergy, types.SideNone)].Normalized,
				ShouldAlmostEqual, 1.5, 1e-12)
		})
	})

	Convey("Given a missing cohort score", t, func() {
		metrics, valid := correlationMetrics(map[string]float64{})

		Convey("It should expose the incomplete bundle as invalid", func() {
			So(valid, ShouldBeFalse)
			So(metrics[types.MetricKey(types.MetricRelativeEnergy, types.SideNone)].Normalized,
				ShouldBeNil)
		})
	})
}

func BenchmarkCorrelationMetrics(b *testing.B) {
	scores := map[string]float64{
		"correlation": 0.8, "signed": 0.6, "relativeEnergy": 1.5,
		"herdScore": 0.2, "alphaScore": 0.3, "noiseScore": 0.4,
		"stressScore": 0.5, "peakScore": 0.5, "strength": 0.5,
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = correlationMetrics(scores)
	}
}
