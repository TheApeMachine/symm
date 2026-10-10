package audit

import (
	"math"
	"math/rand"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestAuditStages(t *testing.T) {
	Convey("Given simulated multi-metric observations", t, func() {
		ticks := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
		rawSeries := make(map[string]map[int64]float64)
		canonicalSeries := make(map[string]map[int64]float64)

		rawSeries["sensor_sin@PEER"] = make(map[int64]float64)
		rawSeries["sensor_clone@PEER"] = make(map[int64]float64)
		rawSeries["sensor_inv@PEER"] = make(map[int64]float64)
		rawSeries["sensor_dead@PEER"] = make(map[int64]float64)

		canonicalSeries["sensor_sin"] = make(map[int64]float64)
		canonicalSeries["sensor_clone"] = make(map[int64]float64)
		canonicalSeries["sensor_inv"] = make(map[int64]float64)
		canonicalSeries["sensor_dead"] = make(map[int64]float64)

		for _, tick := range ticks {
			val := math.Sin(float64(tick) * 0.5)
			rawSeries["sensor_sin@PEER"][tick] = val
			rawSeries["sensor_clone@PEER"][tick] = val + 0.001
			rawSeries["sensor_inv@PEER"][tick] = -val
			rawSeries["sensor_dead@PEER"][tick] = 0.0

			canonicalSeries["sensor_sin"][tick] = val
			canonicalSeries["sensor_clone"][tick] = val + 0.001
			canonicalSeries["sensor_inv"][tick] = -val
			canonicalSeries["sensor_dead"][tick] = 0.0
		}

		Convey("When analyzing contract integrity (Stage 0)", func() {
			meas := data.NewMeasurement(1, "BTC/USD", "test", 1, 1)
			meas.At = time.Now().UTC()
			meas.From = meas.At
			meas = meas.Write(
				data.NewMetric("good_corr", 0.5, data.UnitCorrelation, data.TimescaleTick),
				data.NewMetric("bad_corr", 1.45, data.UnitCorrelation, data.TimescaleTick),
				// The label contains "correlation", but the declared coordinate is a
				// z-score and is therefore not bounded to [-1,1].
				data.NewMetric("correlation_zscore@PEER", 6.2, data.UnitZScore, data.TimescaleTick),
				// Dimensionless metric named signed_correlation violating [-1, 1]
				data.NewMetric("signed_correlation", 1.85, data.UnitDimensionless, data.TimescaleTick),
				// Dimensionless metric named absolute_correlation violating [0, 1]
				data.NewMetric("absolute_correlation", 1.2, data.UnitDimensionless, data.TimescaleTick),
				// Negative SNR violating [0, +inf)
				data.NewMetric("signal_to_noise", -3.5, data.UnitSNR, data.TimescaleTick),
				// Negative Entropy violating [0, +inf)
				data.NewMetric("shannon_entropy", -0.4, data.UnitEntropy, data.TimescaleTick),
				// A log-likelihood in nats and an observed-minus-expected count
				// residual are signed: negative values are not breaches.
				data.NewMetric("log_likelihood:hawkes", -12.5, data.UnitNat, data.TimescaleTick),
				data.NewMetric("count_innovation:buy", -3, data.UnitCountResidual, data.TimescaleTick),
			)

			contract := AnalyzeContract([]*data.Measurement{meas}, 0.05)
			So(contract.TotalMetricsChecked, ShouldEqual, 9)
			So(contract.BreachingMetricsCount, ShouldEqual, 5)
			So(contract.Passed, ShouldBeFalse)
		})

		Convey("When analyzing timing and clock synchronization (Stage 0.5)", func() {
			Convey("with well-synchronized monotonic timestamps", func() {
				now := time.Now().UnixNano()
				measurements := make([]*data.Measurement, 0, 10)
				for idx := int64(0); idx < 10; idx++ {
					meas := data.NewMeasurement(1, "BTC/USD", "test", idx+1, idx+1)
					meas.At = time.Unix(0, now+idx*1_000_000-25_000_000)
					meas.Timestamp = now + idx*1_000_000
					measurements = append(measurements, meas)
				}
				timing := AnalyzeTiming(measurements, DefaultThresholds())
				So(timing.TotalChecked, ShouldEqual, 10)
				So(timing.SequenceInversions, ShouldEqual, 0)
				So(timing.LatencySpikes, ShouldEqual, 0)
				So(timing.Status, ShouldEqual, VerdictValid)
				So(timing.Passed, ShouldBeTrue)
			})

			Convey("with latency jitter spikes and sequence inversions", func() {
				now := time.Now().UnixNano()
				measurements := make([]*data.Measurement, 0, 10)
				for idx := int64(0); idx < 6; idx++ {
					meas := data.NewMeasurement(1, "BTC/USD", "test", idx+1, idx+1)
					meas.At = time.Unix(0, now+idx*100_000_000)
					meas.Timestamp = now + idx*100_000_000 + 10_000_000
					measurements = append(measurements, meas)
				}

				// Latency spike: latency is 700ms
				spikeMeas := data.NewMeasurement(1, "BTC/USD", "test", 7, 7)
				spikeMeas.At = time.Unix(0, now+600_000_000)
				spikeMeas.Timestamp = now + 600_000_000 + 700_000_000
				measurements = append(measurements, spikeMeas)

				// Sequence inversion: At goes backwards
				invMeas := data.NewMeasurement(1, "BTC/USD", "test", 8, 8)
				invMeas.At = time.Unix(0, now+500_000_000)
				invMeas.Timestamp = now + 700_000_000
				measurements = append(measurements, invMeas)

				for idx := int64(8); idx < 10; idx++ {
					meas := data.NewMeasurement(1, "BTC/USD", "test", idx+1, idx+1)
					meas.At = time.Unix(0, now+idx*100_000_000)
					meas.Timestamp = now + idx*100_000_000 + 10_000_000
					measurements = append(measurements, meas)
				}

				timing := AnalyzeTiming(measurements, DefaultThresholds())
				So(timing.LatencySpikes, ShouldBeGreaterThanOrEqualTo, 1)
				So(timing.SequenceInversions, ShouldBeGreaterThanOrEqualTo, 1)
				So(timing.Passed, ShouldBeFalse)
			})
		})

		Convey("When analyzing metric vitality (Stage 1)", func() {
			vitality := AnalyzeVitality(ticks, rawSeries, canonicalSeries)

			So(vitality.RawProducerMetrics, ShouldEqual, 4)
			So(vitality.RawHealthyMetrics, ShouldEqual, 3)
			So(vitality.CanonicalGridCells, ShouldEqual, 4)
			So(vitality.CanonicalHealthyCells, ShouldEqual, 3)
			So(vitality.CanonicalDeadCells, ShouldEqual, 1)
			So(len(vitality.RedundantPairs), ShouldBeGreaterThanOrEqualTo, 1)
			So(math.Abs(vitality.RedundantPairs[0].Correlation), ShouldBeGreaterThan, 0.99)
		})

		Convey("When analyzing pair sympathy against shuffled null (Stage 2)", func() {
			vitality := AnalyzeVitality(ticks, rawSeries, canonicalSeries)
			sympathy := AnalyzeSympathy(ticks, canonicalSeries, vitality.CanonicalCells, 20, 0.05)

			So(sympathy.TotalPairs, ShouldBeGreaterThan, 0)
			So(sympathy.PositivePairs, ShouldBeGreaterThan, 0)
			So(sympathy.InversePairs, ShouldBeGreaterThan, 0)
			So(math.IsNaN(sympathy.NullDistribution.MeanConcordance), ShouldBeFalse)
		})

		Convey("When testing autocorrelation block size and circular block permutation (Stage 2)", func() {
			persistentSeries := make([]float64, 100)
			for idx := range persistentSeries {
				persistentSeries[idx] = math.Sin(float64(idx) * 0.1)
			}
			blockSize := empiricalAutocorrelationBlockSize(persistentSeries)
			So(blockSize, ShouldBeGreaterThan, 2)

			rng := rand.New(rand.NewSource(42))
			permuted := blockPermute(rng, persistentSeries, blockSize)
			So(len(permuted), ShouldEqual, len(persistentSeries))

			noiseSeries := make([]float64, 100)
			for idx := range noiseSeries {
				if idx%2 == 0 {
					noiseSeries[idx] = 1.0
				}
				if idx%2 != 0 {
					noiseSeries[idx] = -1.0
				}
			}
			noiseBlock := empiricalAutocorrelationBlockSize(noiseSeries)
			So(noiseBlock, ShouldEqual, 2)
		})

		Convey("When testing multi-symbol token dynamics isolation (Stage 4)", func() {
			tokensBySymbol := map[string][]string{
				"BTC/USD": {"R1", "R2", "R1", "R2", "R1"},
				"ETH/USD": {"R10", "R20", "R10", "R20", "R10"},
			}
			nullEntropies := computeBlockNullTransitionEntropies(tokensBySymbol, 2, 10)
			So(len(nullEntropies), ShouldEqual, 10)
			for _, entropyVal := range nullEntropies {
				So(entropyVal, ShouldBeGreaterThan, 0.0)
			}
		})

		Convey("When computing classification metrics under heavy class imbalance (Stage 6)", func() {
			actuals := []string{"wait", "wait", "wait", "wait", "enter"}
			preds := []string{"wait", "wait", "wait", "wait", "wait"}
			counts := map[string]int{"wait": 4, "enter": 1}

			balancedAcc, mcc, enterPrec, enterRec := computeClassificationMetrics(actuals, preds, counts)
			// Balanced Accuracy evaluates wait recall (1.0) and enter recall (0.0) = 50%
			So(balancedAcc, ShouldAlmostEqual, 0.5, 1e-6)
			// MCC is 0 because enter predictions are absent
			So(mcc, ShouldAlmostEqual, 0.0, 1e-6)
			// Enter precision and recall are zero
			So(enterPrec, ShouldAlmostEqual, 0.0, 1e-6)
			So(enterRec, ShouldAlmostEqual, 0.0, 1e-6)
		})

		Convey("When analyzing cognitive trie with insufficient evidence (Stage 6)", func() {
			cognitive := AnalyzeCognitiveTrie(t.Context(), nil, 1, "BTC/USD", nil, nil, nil, 10, 0.05, nil)

			So(cognitive.Status, ShouldEqual, "INSUFFICIENT_DATA")
			So(cognitive.Passed, ShouldBeFalse)
			So(cognitive.SummaryText, ShouldContainSubstring, "Cognitive Trie: Catalog or grid unavailable.")
		})

		Convey("When testing trie node metric traversal and token deduplication", func() {
			deduped := contextOfTokens([]string{"R1", "R1", "R5", "R12", "R12", "R12"})
			So(deduped, ShouldEqual, "R1/R5/R12")

			emptyDeduped := contextOfTokens(nil)
			So(emptyDeduped, ShouldEqual, "")
		})
	})
}
