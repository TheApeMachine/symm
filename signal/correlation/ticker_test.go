package correlation

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	nomcorrelation "github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

/*
correlatedPricePaths generates two positive price paths of length count:
left is a plain geometric random walk, right is 60% left's log-return plus
40% independent noise, so the pair is genuinely, but not perfectly,
correlated — a stronger property-test fixture than two exactly
proportional paths, which would trivially correlate at 1 regardless of
whether scale invariance holds.
*/
func correlatedPricePaths(seed int64, count int) (left, right []float64) {
	source := rand.New(rand.NewSource(seed))
	left = make([]float64, count)
	noise := make([]float64, count)
	right = make([]float64, count)
	left[0] = 100
	noise[0] = 50
	right[0] = 50

	for index := 1; index < count; index++ {
		left[index] = left[index-1] * math.Exp(source.NormFloat64()*0.01)
		noise[index] = noise[index-1] * math.Exp(source.NormFloat64()*0.01)

		leftReturn := math.Log(left[index] / left[index-1])
		noiseReturn := math.Log(noise[index] / noise[index-1])
		right[index] = right[index-1] * math.Exp(0.6*leftReturn+0.4*noiseReturn)
	}

	return left, right
}

func samplesFromPrices(
	prices []float64, start time.Time, step time.Duration,
) []nomcorrelation.Sample {
	samples := make([]nomcorrelation.Sample, len(prices))

	for index, price := range prices {
		samples[index] = nomcorrelation.Sample{
			At:    start.Add(time.Duration(index) * step),
			Value: price,
		}
	}

	return samples
}

func rescaledSamples(
	samples []nomcorrelation.Sample, factor float64,
) []nomcorrelation.Sample {
	rescaled := make([]nomcorrelation.Sample, len(samples))

	for index, sample := range samples {
		rescaled[index] = nomcorrelation.Sample{At: sample.At, Value: sample.Value * factor}
	}

	return rescaled
}

// rescaleFactors spans sub-unit denominations (a coin quoted in a fraction
// of a cent) through arbitrarily large ones (a coin quoted in a rebased
// unit worth a million of the original), the exact span the plan calls
// out: correlation is computed from log-returns, so no positive rescale of
// either leg should move it.
var rescaleFactors = []float64{0.0001, 0.001, 0.5, 2, 1000, 1_000_000}

func TestTickerCorrelationScaleInvariant(t *testing.T) {
	Convey("Given two genuinely correlated, non-proportional price paths", t, func() {
		start := time.Unix(1_700_000_000, 0)
		leftPrices, rightPrices := correlatedPricePaths(7, 128)
		leftSamples := samplesFromPrices(leftPrices, start, time.Second)
		rightSamples := samplesFromPrices(rightPrices, start, time.Second)

		ticker := NewTicker()
		baseline, baselineOK := ticker.correlation(leftSamples, rightSamples)
		So(baselineOK, ShouldBeTrue)
		So(baseline, ShouldBeGreaterThan, 0)

		Convey("When the left leg is rescaled by sub-unit and arbitrarily large positive factors", func() {
			for _, factor := range rescaleFactors {
				value, ok := ticker.correlation(rescaledSamples(leftSamples, factor), rightSamples)

				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, baseline, 1e-9)
			}
		})

		Convey("When the right leg is rescaled by sub-unit and arbitrarily large positive factors", func() {
			for _, factor := range rescaleFactors {
				value, ok := ticker.correlation(leftSamples, rescaledSamples(rightSamples, factor))

				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, baseline, 1e-9)
			}
		})

		Convey("When both legs are rescaled independently by different positive factors", func() {
			for _, factor := range rescaleFactors {
				value, ok := ticker.correlation(
					rescaledSamples(leftSamples, factor),
					rescaledSamples(rightSamples, 1/factor),
				)

				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, baseline, 1e-9)
			}
		})
	})
}

func denominationRow(symbol string, price float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Last:      decimal.NewFromFloat64(price),
		Timestamp: at,
	}
}

func TestTickerMeasureDenominationInvariant(t *testing.T) {
	Convey("Given a subject and a peer with genuinely correlated, non-proportional price paths", t, func() {
		start := time.Unix(1_700_000_000, 0)
		subjectPrices, peerPrices := correlatedPricePaths(11, 96)

		buildCrossSection := func(subjectFactor float64) *types.CrossSection {
			crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())
			So(err, ShouldBeNil)

			rows := make([]kraken.TickerData, 0, len(subjectPrices)+len(peerPrices))

			for index := range subjectPrices {
				at := start.Add(time.Duration(index) * time.Second)
				rows = append(rows,
					denominationRow("BTC/USD", subjectPrices[index]*subjectFactor, at),
					denominationRow("ETH/USD", peerPrices[index], at),
				)
			}

			So(crossSection.Observe(rows), ShouldBeNil)

			return crossSection
		}

		ticker := NewTicker()
		baselineCrossSection := buildCrossSection(1)
		baselineRow := denominationRow(
			"BTC/USD",
			subjectPrices[len(subjectPrices)-1],
			start.Add(time.Duration(len(subjectPrices)-1)*time.Second),
		)
		baseline, err := ticker.Measure(baselineRow, baselineCrossSection)
		So(err, ShouldBeNil)
		So(baseline, ShouldHaveLength, 1)

		Convey("When the subject's entire history is requoted in a different denomination", func() {
			for _, factor := range rescaleFactors {
				rescaledCrossSection := buildCrossSection(factor)
				rescaledRow := denominationRow(
					"BTC/USD",
					subjectPrices[len(subjectPrices)-1]*factor,
					start.Add(time.Duration(len(subjectPrices)-1)*time.Second),
				)
				result, measureErr := ticker.Measure(rescaledRow, rescaledCrossSection)

				So(measureErr, ShouldBeNil)
				So(result, ShouldHaveLength, 1)

				// Every exported metric is unchanged, since all of them
				// derive from log-returns.
				for key, value := range baseline[0].Metrics {
					So(result[0].Metrics[key], ShouldAlmostEqual, value, 1e-9)
				}
			}
		})
	})
}
