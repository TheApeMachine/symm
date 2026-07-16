package correlation

import (
	"context"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	nomcorrelation "github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

func measurementFields(measurements []*types.Measurement, symbol string) map[types.MetricType]float64 {
	return tests.MeasurementFields(measurements, symbol)
}

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

func TestTickerOn(t *testing.T) {
	Convey("Given a correlation ticker ingestor", t, func() {
		ticker := &Ticker{cache: tickerCache()}
		payload := []byte(`{"channel":"ticker","type":"update","data":[{"symbol":"ALGO/USD","bid":0.10025,"bid_qty":740,"ask":0.10035,"ask_qty":740,"last":0.10035,"volume":997038.98,"vwap":0.10148,"low":0.09979,"high":0.10285,"change_pct":-0.17,"timestamp":"2023-09-25T09:04:31.742648Z"}]}`)

		Convey("When a ticker frame arrives", func() {
			ticker.On(payload)

			Convey("Then ticker rows should accumulate in cache", func() {
				So(len(tickerRows(ticker.cache)), ShouldEqual, 1)
				So(tickerRows(ticker.cache)[0].Symbol, ShouldEqual, "ALGO/USD")
			})
		})
	})
}

func TestSectionCorrelationScaleInvariant(t *testing.T) {
	Convey("Given two genuinely correlated, non-proportional price paths", t, func() {
		start := time.Unix(1_700_000_000, 0)
		leftPrices, rightPrices := correlatedPricePaths(7, 128)
		leftSamples := samplesFromPrices(leftPrices, start, time.Second)
		rightSamples := samplesFromPrices(rightPrices, start, time.Second)

		section := NewSection()
		baseline, baselineOK := section.correlation(leftSamples, rightSamples)
		So(baselineOK, ShouldBeTrue)
		So(baseline, ShouldBeGreaterThan, 0)

		Convey("When the left leg is rescaled by sub-unit and arbitrarily large positive factors", func() {
			for _, factor := range rescaleFactors {
				value, ok := section.correlation(rescaledSamples(leftSamples, factor), rightSamples)

				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, baseline, 1e-9)
			}
		})

		Convey("When the right leg is rescaled by sub-unit and arbitrarily large positive factors", func() {
			for _, factor := range rescaleFactors {
				value, ok := section.correlation(leftSamples, rescaledSamples(rightSamples, factor))

				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, baseline, 1e-9)
			}
		})

		Convey("When both legs are rescaled independently by different positive factors", func() {
			for _, factor := range rescaleFactors {
				value, ok := section.correlation(
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

func TestSignal_MeasureDenominationInvariant(t *testing.T) {
	Convey("Given a subject and a peer with genuinely correlated, non-proportional price paths", t, func() {
		start := time.Unix(1_700_000_000, 0)
		subjectPrices, peerPrices := correlatedPricePaths(11, 96)

		measure := func(subjectFactor float64) *types.Thesis {
			signal := &Signal{
				ctx:     context.Background(),
				ticker:  &Ticker{cache: tickerCache()},
				section: NewSection(),
			}
			history := make([]kraken.TickerData, 0, len(subjectPrices)*2)

			for index := range subjectPrices {
				at := start.Add(time.Duration(index) * time.Second)
				history = append(history,
					denominationRow("BTC/USD", subjectPrices[index]*subjectFactor, at),
					denominationRow("ETH/USD", peerPrices[index], at),
				)
			}

			signal.ticker.cache = tickerCache(history...)

			return signal.Measure(types.NewThesis(nil))
		}

		baselineResult := measure(1)
		baselineMetrics := measurementFields(baselineResult.Measurements, "BTC/USD")
		So(baselineMetrics, ShouldNotBeEmpty)
		So(baselineMetrics[types.MetricStrength], ShouldBeLessThanOrEqualTo, 1)
		So(baselineMetrics[types.MetricAlphaScore], ShouldBeLessThanOrEqualTo, 1)
		So(baselineMetrics[types.MetricStressScore], ShouldBeLessThanOrEqualTo, 1)

		Convey("When the subject's entire history is requoted in a different denomination", func() {
			for _, factor := range rescaleFactors {
				result := measure(factor)
				metrics := measurementFields(result.Measurements, "BTC/USD")
				So(metrics, ShouldNotBeEmpty)

				for metric, value := range baselineMetrics {
					So(metrics[metric], ShouldAlmostEqual, value, 1e-9)
				}
			}
		})
	})
}

func TestSignal_MeasureRequiresCrossSection(t *testing.T) {
	Convey("Given a lone ticker row with no peer history", t, func() {
		signal := &Signal{
			ctx:     context.Background(),
			ticker:  &Ticker{cache: tickerCache(denominationRow("BTC/USD", 100, time.Now()))},
			section: NewSection(),
		}

		result := signal.Measure(types.NewThesis(nil))

		Convey("Then correlation emits nothing for that symbol", func() {
			So(measurementFields(result.Measurements, "BTC/USD"), ShouldBeEmpty)
		})
	})
}

func TestSignal_MeasureUsesTrimmedSymbolTimestamp(t *testing.T) {
	Convey("Given ticker rows whose symbols need trimming", t, func() {
		start := time.Unix(1_700_000_100, 0)
		subjectPrices, peerPrices := correlatedPricePaths(3, 3)
		signal := &Signal{
			ctx:     context.Background(),
			ticker:  &Ticker{cache: tickerCache()},
			section: NewSection(),
		}
		history := make([]kraken.TickerData, 0, len(subjectPrices)*2)

		for index := range subjectPrices {
			at := start.Add(time.Duration(index) * time.Second)
			subjectRow := denominationRow("BTC/USD", subjectPrices[index], at)
			peerRow := denominationRow("ETH/USD", peerPrices[index], at)
			subjectRow.Symbol = " BTC/USD "
			history = append(history, subjectRow, peerRow)
		}

		signal.ticker.cache = tickerCache(history...)

		result := signal.Measure(types.NewThesis(nil))

		Convey("Then measurements carry the source row timestamp", func() {
			metrics := measurementFields(result.Measurements, "BTC/USD")
			So(metrics, ShouldNotBeEmpty)

			for _, measurement := range result.Measurements {
				if measurement.Symbol != "BTC/USD" {
					continue
				}

				So(measurement.At, ShouldEqual, start.Add(2*time.Second))
				So(measurement.At.IsZero(), ShouldBeFalse)
			}
		})
	})
}

func BenchmarkSignal_Measure(b *testing.B) {
	start := time.Unix(1_700_000_000, 0)
	subjectPrices, peerPrices := correlatedPricePaths(13, 64)
	signal := &Signal{
		ctx:     context.Background(),
		ticker:  &Ticker{cache: tickerCache()},
		section: NewSection(),
	}
	history := make([]kraken.TickerData, 0, len(subjectPrices)*2)

	for index := range subjectPrices {
		at := start.Add(time.Duration(index) * time.Second)
		history = append(history,
			denominationRow("BTC/USD", subjectPrices[index], at),
			denominationRow("ETH/USD", peerPrices[index], at),
		)
	}

	signal.section.Measure(history)
	tick := 0

	b.ReportAllocs()

	for b.Loop() {
		tick++
		at := start.Add(time.Duration(len(subjectPrices)+tick) * time.Second)
		signal.ticker.cache = tickerCache(
			denominationRow("BTC/USD", subjectPrices[len(subjectPrices)-1]+float64(tick%7), at),
			denominationRow("ETH/USD", peerPrices[len(peerPrices)-1]+float64(tick%5), at),
		)
		_ = signal.Measure(types.NewThesis(nil))
	}
}
