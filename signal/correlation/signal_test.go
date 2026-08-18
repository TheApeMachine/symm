package correlation

import (
	"context"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
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
			market := types.NewSymbol("AAA/USD", nil)
			appendCorrelationTickers(market,
				correlationTicker("AAA/USD", prices[0], at),
				correlationTicker("BBB/USD", prices[1], at),
			)
			_ = slices.Collect(signal.Measure(market))
		}

		Convey("It reuses the section group and constructs only the event subject", func() {
			at := start.Add(3 * time.Second)
			market := types.NewSymbol("AAA/USD", nil)
			appendCorrelationTickers(market,
				correlationTicker("AAA/USD", 133.1, at),
				correlationTicker("BBB/USD", 266.2, at),
			)

			measurements := slices.Collect(signal.Measure(market))
			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].Symbol, ShouldEqual, "AAA/USD")

			for _, measurement := range measurements {
				separation := measurement.Sample(types.MetricHypothesisSeparation, types.SideNone)
				So(separation.Normalized, ShouldNotBeNil)
				So(separation.Raw, ShouldEqual, math.Nextafter(1, 0))
			}
		})
	})

	Convey("Given complete symbol histories behind symbol-local cursors", t, func() {
		signal := &Signal{ctx: context.Background(), section: NewSection()}
		start := time.Unix(1_700_006_000, 0).UTC()
		market := types.NewSymbol("AAA/USD", nil)
		appendCorrelationTickers(market,
			correlationTicker("AAA/USD", 100, start),
			correlationTicker("AAA/USD", 110, start.Add(time.Second)),
			correlationTicker("AAA/USD", 121, start.Add(2*time.Second)),
			correlationTicker("BBB/USD", 200, start),
			correlationTicker("BBB/USD", 220, start.Add(time.Second)),
			correlationTicker("BBB/USD", 242, start.Add(2*time.Second)),
		)

		measurements := slices.Collect(signal.Measure(market))

		for _, measurement := range measurements {
			market.AppendMeasurement(measurement)
		}

		Convey("It should consume each symbol history with its own cursor", func() {
			So(measurements, ShouldHaveLength, 1)

			recalled := slices.Collect(signal.Measure(market))

			So(recalled, ShouldHaveLength, 1)
			So(recalled[0].Source, ShouldEqual, types.SourceCorrelation)
			So(recalled[0].ID, ShouldEqual, measurements[0].ID)
			So(recalled[0].At, ShouldEqual, measurements[0].At)
		})
	})

	Convey("Given a ready cohort where only one peer receives another ticker", t, func() {
		signal := &Signal{ctx: context.Background(), section: NewSection()}
		start := time.Unix(1_700_007_000, 0).UTC()

		for leg, prices := range [][]float64{
			{100, 200},
			{110, 220},
			{121, 242},
		} {
			at := start.Add(time.Duration(leg) * time.Second)
			market := types.NewSymbol("AAA/USD", nil)
			appendCorrelationTickers(market,
				correlationTicker("AAA/USD", prices[0], at),
				correlationTicker("BBB/USD", prices[1], at),
			)
			_ = slices.Collect(signal.Measure(market))
		}

		market := types.NewSymbol("BBB/USD", nil)
		appendCorrelationTickers(market,
			correlationTicker("BBB/USD", 266.2, start.Add(3*time.Second)),
		)
		measurements := slices.Collect(signal.Measure(market))

		Convey("It should emit only the peer changed by this event", func() {
			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].Symbol, ShouldEqual, "BBB/USD")
			So(measurements[0].At, ShouldResemble, start.Add(3*time.Second))
			So(measurements[0].Sample(
				types.MetricLastPrice, types.SideNone,
			).Raw, ShouldEqual, 266.2)
		})
	})
}

func correlationTicker(symbol string, price float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol: symbol, Last: decimal.NewFromFloat64(price), Timestamp: at,
	}
}

func appendCorrelationTickers(market *types.Symbol, rows ...kraken.TickerData) {
	for _, row := range rows {
		market.AppendTicker(row, types.TickerReceivers)
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
		})

		Convey("It should retain signed and relative evidence without fake scaling", func() {
			So(valid, ShouldBeTrue)
			So(*metrics[types.MetricKey(types.MetricSigned, types.SideNone)].Normalized,
				ShouldEqual, -0.6)
			So(*metrics[types.MetricKey(types.MetricRelativeEnergy, types.SideNone)].Normalized,
				ShouldEqual, 1.5)
			So(metrics, ShouldHaveLength, 7)
			_, hasPeak := metrics["peak_score"]
			_, hasStrength := metrics[types.MetricKey(types.MetricStrength, types.SideNone)]
			So(hasPeak, ShouldBeFalse)
			So(hasStrength, ShouldBeFalse)
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

	Convey("Given finite scores outside their mathematical domains", t, func() {
		validScores := map[string]float64{
			"correlation": 0.8, "signed": -0.6, "relativeEnergy": 1.5,
			"herdScore": 0.2, "alphaScore": 0.3, "noiseScore": 0.4,
			"stressScore": 0.5,
		}
		invalid := map[string]float64{
			"correlation":    math.Nextafter(1, 2),
			"signed":         math.Nextafter(-1, -2),
			"relativeEnergy": math.Nextafter(0, -1),
			"herdScore":      math.Nextafter(0, -1),
			"alphaScore":     math.Nextafter(1, 2),
			"noiseScore":     math.Nextafter(0, -1),
			"stressScore":    math.Nextafter(1, 2),
		}

		Convey("It should reject every invalid bundle instead of normalizing corrupt evidence", func() {
			for name, value := range invalid {
				scores := make(map[string]float64, len(validScores))

				for validName, validValue := range validScores {
					scores[validName] = validValue
				}

				scores[name] = value
				metrics, valid := correlationMetrics(scores)
				So(valid, ShouldBeFalse)
				So(metrics, ShouldHaveLength, 7)
			}
		})
	})
}

func BenchmarkCorrelationMetrics(b *testing.B) {
	scores := map[string]float64{
		"correlation": 0.8, "signed": 0.6, "relativeEnergy": 1.5,
		"herdScore": 0.2, "alphaScore": 0.3, "noiseScore": 0.4,
		"stressScore": 0.5,
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = correlationMetrics(scores)
	}
}

func BenchmarkMeasure(b *testing.B) {
	signal := &Signal{ctx: context.Background(), section: NewSection()}
	start := time.Unix(1_700_008_000, 0).UTC()

	for leg, prices := range [][]float64{
		{100, 200},
		{110, 220},
		{121, 242},
	} {
		at := start.Add(time.Duration(leg) * time.Second)
		market := types.NewSymbol("AAA/USD", nil)
		appendCorrelationTickers(market,
			correlationTicker("AAA/USD", prices[0], at),
			correlationTicker("BBB/USD", prices[1], at),
		)
		_ = slices.Collect(signal.Measure(market))
	}

	sequence := 3
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		sequence++
		market := types.NewSymbol("AAA/USD", nil)
		appendCorrelationTickers(market,
			correlationTicker(
				"BBB/USD",
				242+float64(sequence%7),
				start.Add(time.Duration(sequence)*time.Second),
			),
		)
		_ = slices.Collect(signal.Measure(market))
	}
}
