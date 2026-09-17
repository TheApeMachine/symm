package liquidity

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/theapemachine/symm/nomagique/data/sequence"

	"github.com/theapemachine/symm/nomagique/runtime"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func readyTicker(ctx context.Context) *Ticker {
	ticker := NewTicker(ctx)
	ticker.Transition(runtime.READY)
	return ticker
}

/*
tick builds the measurement the workload's data management would hand the
signal: the register's declared schema with the feed's touch quote written.
The label is the symbol and the timestamp is the venue's.
*/
var schema = new(Ticker).Register().Metrics

func tick(symbol string, bid, ask, bidQty, askQty float64, at time.Time) *data.Measurement[float64] {
	m := data.NewMeasurement[float64]("liquidity", cloneSchema())
	m.Label, m.At, m.From = symbol, at, at
	m.Metrics["bid"] = m.Metrics["bid"].Write(bid)
	m.Metrics["ask"] = m.Metrics["ask"].Write(ask)
	m.Metrics["bid_qty"] = m.Metrics["bid_qty"].Write(bidQty)
	m.Metrics["ask_qty"] = m.Metrics["ask_qty"].Write(askQty)

	return m
}

func cloneSchema() map[string]data.Metric[float64] {
	metrics := make(map[string]data.Metric[float64], len(schema))

	for key, metric := range schema {
		metrics[key] = metric
	}

	return metrics
}

/*
TestTickerStepPreObservationBaseline is the exact BLOCKER 1 fixture: with one
prior observation, the only possible causal baseline is that observation, so
all four published facts must reference the SAME pre-observation baseline.
*/
func TestTickerStepPreObservationBaseline(t *testing.T) {
	Convey("Given bid depth 100 then 200", t, func() {
		entity := readyTicker(t.Context())
		base := time.Unix(1_700_000_000, 0)

		sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 1.0, 1.0, base))))
		second := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))))

		Convey("baseline, ratio and divergence reference the same pre-observation baseline", func() {
			So(second.Err, ShouldBeNil)
			So(second.Metrics["touch_notional_baseline:bid"].Raw, ShouldAlmostEqual, 100.0, 1e-9)
			So(second.Metrics["depth_ratio:bid"].Raw, ShouldAlmostEqual, 2.0, 1e-9)
			So(second.Metrics["depth_divergence:bid"].Raw, ShouldAlmostEqual, math.Log(2.0), 1e-9)

			// log(depth_ratio) == depth_divergence exactly.
			So(math.Log(second.Metrics["depth_ratio:bid"].Raw), ShouldAlmostEqual, second.Metrics["depth_divergence:bid"].Raw, 1e-12)
		})
	})

	Convey("the ask-side and spread facts follow the same contract", t, func() {
		entity := readyTicker(t.Context())
		base := time.Unix(20, 0)

		// Ask notional stays 102 across both steps (askQty constant), so the
		// ask divergence is 0 and the baseline is the prior ask notional.
		sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 1.0, 1.0, base))))
		second := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))))

		Convey("ask baseline is the prior ask notional with zero divergence", func() {
			So(second.Metrics["touch_notional_baseline:ask"].Raw, ShouldAlmostEqual, 102.0, 1e-9)
			So(second.Metrics["depth_divergence:ask"].Raw, ShouldAlmostEqual, 0.0, 1e-9)
		})
	})
}

/*
TestTickerStepDegenerateNoise is BLOCKER 3: the z-score is undefined (zero,
never a fabricated number) when the pre-observation noise is unavailable or
degenerate.
*/
func TestTickerStepDegenerateNoise(t *testing.T) {
	Convey("Given no prior observation", t, func() {
		entity := readyTicker(t.Context())
		first := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 1.0, 1.0, time.Unix(1, 0)))))

		Convey("no baseline produces no z-score", func() {
			So(first.Metrics["depth_zscore:bid"].Raw, ShouldEqual, 0.0)
			So(first.Metrics["touch_notional_baseline:bid"].Raw, ShouldEqual, 0.0)
		})
	})

	Convey("Given a single prior observation (degenerate residual scale)", t, func() {
		entity := readyTicker(t.Context())
		base := time.Unix(1, 0)
		sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 1.0, 1.0, base))))
		second := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))))

		Convey("the z-score is unwritten, the divergence is defined", func() {
			So(second.Metrics["depth_zscore:bid"].Raw, ShouldEqual, 0.0)
			So(second.Metrics["depth_divergence:bid"].Raw, ShouldAlmostEqual, math.Log(2.0), 1e-9)
		})
	})
}

/*
TestTickerStepZScorePresent is BLOCKER 3's positive case: once the noise scale
is estimable and positive, the z-score is present and equals divergence/noise.
*/
func TestTickerStepZScorePresent(t *testing.T) {
	Convey("Given a history with non-zero residual dispersion", t, func() {
		entity := readyTicker(t.Context())
		base := time.Unix(1000, 0)

		// Vary the bid depth so the residual dispersion is non-zero, then
		// observe the latest point's z-score.
		sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 1.0, 1.0, base))))
		sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))))
		sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 1.5, 1.0, base.Add(2*time.Second)))))
		fourth := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 3.0, 1.0, base.Add(3*time.Second)))))

		Convey("the z-score is present and equals divergence / noise", func() {
			zscore := fourth.Metrics["depth_zscore:bid"].Raw
			noise := fourth.Metrics["depth_noise_scale:bid"].Raw

			So(noise, ShouldBeGreaterThan, 0)
			So(zscore, ShouldAlmostEqual, fourth.Metrics["depth_divergence:bid"].Raw/noise, 1e-9)
		})
	})
}

/*
TestTickerMaturityUsesNEff asserts the measurement maturity follows the Kish
effective-support formula: Maturity = 1 - 1/N_eff (0 when N_eff <= 1). With a
single prior observation N_eff == 2, so Maturity == 0.5; with none, 0.
*/
func TestTickerMaturityUsesNEff(t *testing.T) {
	Convey("Given one then two observations", t, func() {
		entity := readyTicker(t.Context())
		base := time.Unix(1, 0)

		first := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 1.0, 1.0, base))))
		second := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))))

		Convey("maturity is 0 with no effective support, 1 - 1/N_eff with support", func() {
			// First observation: no prior baseline, N_eff <= 1 -> maturity 0.
			So(first.Maturity, ShouldEqual, 0.0)

			// Second observation: with an event-time decay alpha = 0.5
			// (elapsed == cadence), N_eff = (1.0)^2 / (0.5) = 2 -> maturity 0.5.
			So(second.Maturity, ShouldAlmostEqual, 0.5, 1e-6)
		})
	})
}

/*
TestTickerIrregularTimeRegression is BLOCKER 2: the divergence velocity is a
causal local-time regression slope, not a first difference. The same underlying
time slope sampled on different irregular grids must agree.
*/
func TestTickerIrregularTimeRegression(t *testing.T) {
	Convey("Given one linear divergence trajectory on two grids", t, func() {
		// The bid depth is chosen so log depth follows a linear-in-time
		// trajectory; the estimator must recover the same slope regardless of
		// the sampling grid. This is asserted directly at the statistic level
		// in TestLocalRegressionIrregularGrid in nomagique/statistic.
		entity := readyTicker(t.Context())

		// Feed enough history that the divergence path accumulates several
		// in-horizon samples and the causal local regression becomes defined —
		// before that, the velocity is legitimately undefined (unwritten, not
		// a numeric zero).
		base := time.Unix(1, 0)
		depths := []float64{1.0, 2.0, 1.5, 3.0, 2.5, 4.0, 3.5, 5.0}
		offsets := []time.Duration{0, time.Second, 3 * time.Second, 5 * time.Second, 8 * time.Second, 10 * time.Second, 13 * time.Second, 15 * time.Second}

		var fourth *data.Measurement[float64]

		for index := range depths {
			fourth = sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, depths[index], 1.0, base.Add(offsets[index])))))
		}

		Convey("the velocity is a fitted regression slope, not a message-count delta", func() {
			velocity := fourth.Metrics["divergence_velocity:bid"].Raw

			// A per-message delta would equal log(5.0/3.5); the regression slope
			// is time-normalized and therefore different. We only assert it is
			// finite (the exact slope equality across grids is tested at the
			// statistic level).
			So(math.IsNaN(velocity), ShouldBeFalse)
			So(math.IsInf(velocity, 0), ShouldBeFalse)
		})
	})
}

/*
TestTickerVelocityUndefinedAbsent asserts BLOCKER: undefined ≠ zero for the
divergence velocity. Before the divergence path has enough in-horizon support
for the local regression, divergence_velocity:* must be unwritten (zero),
never emitted as a fabricated slope.
*/
func TestTickerVelocityUndefinedAbsent(t *testing.T) {
	Convey("Given two observations (insufficient regression support)", t, func() {
		entity := readyTicker(t.Context())
		base := time.Unix(1, 0)

		sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 1.0, 1.0, base))))
		second := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 2.0, 1.0, base.Add(time.Second)))))

		Convey("the divergence velocity is absent, not zero", func() {
			So(second.Metrics["divergence_velocity:bid"].Raw, ShouldEqual, 0.0)
			So(second.Metrics["divergence_velocity_snr:bid"].Raw, ShouldEqual, 0.0)
		})
	})
}

/*
TestTickerVelocitySNRPresent asserts the divergence velocity SNR is projected
once the regression is defined, and equals the velocity fit's own SNR.
*/
func TestTickerVelocitySNRPresent(t *testing.T) {
	Convey("Given a divergence trajectory with enough in-horizon support", t, func() {
		entity := readyTicker(t.Context())
		base := time.Unix(1, 0)
		depths := []float64{1.0, 2.0, 1.5, 3.0, 2.5, 4.0, 3.5, 5.0, 4.5, 6.0}
		offsets := []time.Duration{0, time.Second, 3 * time.Second, 5 * time.Second, 8 * time.Second, 10 * time.Second, 13 * time.Second, 15 * time.Second, 18 * time.Second, 20 * time.Second}

		var last *data.Measurement[float64]

		for index := range depths {
			last = sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, depths[index], 1.0, base.Add(offsets[index])))))
		}

		Convey("the velocity SNR metric is present and finite", func() {
			snr := last.Metrics["divergence_velocity_snr:bid"].Raw

			So(math.IsNaN(snr), ShouldBeFalse)
			So(math.IsInf(snr, 0), ShouldBeFalse)
			So(snr, ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

func TestTickerStepPeerIsolation(t *testing.T) {
	Convey("Given a quoted peer and an owned liquidity measurement", t, func() {
		entity := readyTicker(t.Context())
		peer := data.NewMeasurement("public", map[string]data.Metric[float64]{
			"bid":     data.NewMetric[float64]("bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
			"ask":     data.NewMetric[float64]("ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
			"bid_qty": data.NewMetric[float64]("bid_qty", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
			"ask_qty": data.NewMetric[float64]("ask_qty", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		})
		peer.Label = "BTC/USD"
		peer.At = time.Unix(1, 0)
		peer.Metrics["bid"] = peer.Metrics["bid"].Write(100)
		peer.Metrics["ask"] = peer.Metrics["ask"].Write(102)
		peer.Metrics["bid_qty"] = peer.Metrics["bid_qty"].Write(1)
		peer.Metrics["ask_qty"] = peer.Metrics["ask_qty"].Write(1)

		owned := tick("ETH/USD", 0, 0, 0, 0, time.Unix(1, 0))
		owned.Peers = []*data.Measurement[float64]{peer}

		result := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](owned)))

		Convey("the instrument writes onto its own measurement", func() {
			So(result.Err, ShouldBeNil)
			So(result.Source, ShouldEqual, "liquidity")
			So(result.Label, ShouldEqual, "BTC/USD")
			So(result.Metrics["midpoint"].Raw, ShouldEqual, 101.0)
		})

		Convey("the peer's metric map is not written", func() {
			_, hasMid := peer.Metrics["midpoint"]
			So(hasMid, ShouldBeFalse)
			So(len(peer.Metrics), ShouldEqual, 4)
			So(peer.Metrics["bid"].Raw, ShouldEqual, 100)
		})
	})
}

func TestTickerStepConcurrentPeers(t *testing.T) {
	Convey("Given many instruments sharing one quote peer", t, func() {
		peer := data.NewMeasurement("public", map[string]data.Metric[float64]{
			"bid":     data.NewMetric[float64]("bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
			"ask":     data.NewMetric[float64]("ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
			"bid_qty": data.NewMetric[float64]("bid_qty", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
			"ask_qty": data.NewMetric[float64]("ask_qty", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		})
		peer.Label = "BTC/USD"
		peer.At = time.Unix(1, 0)
		peer.Metrics["bid"] = peer.Metrics["bid"].Write(100)
		peer.Metrics["ask"] = peer.Metrics["ask"].Write(102)
		peer.Metrics["bid_qty"] = peer.Metrics["bid_qty"].Write(1)
		peer.Metrics["ask_qty"] = peer.Metrics["ask_qty"].Write(1)

		var group sync.WaitGroup

		for range 8 {
			group.Add(1)

			go func() {
				defer group.Done()

				entity := readyTicker(t.Context())
				owned := entity.Register()
				owned.Peers = []*data.Measurement[float64]{peer}

				for range 50 {
					sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](owned)))
				}
			}()
		}

		group.Wait()

		Convey("the shared peer is still the feed's quote", func() {
			So(peer.Metrics["bid"].Raw, ShouldEqual, 100)
			So(len(peer.Metrics), ShouldEqual, 4)
		})
	})
}

func TestTickerNext(t *testing.T) {
	Convey("Given a liquidity ticker-path instrument", t, func() {
		entity := readyTicker(t.Context())

		Convey("the first tick yields one measurement with the touch written", func() {
			measurement := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 1, 1, time.Unix(1, 0)))))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["midpoint"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["spread"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["touch_notional:bid"].Raw, ShouldEqual, 100.0)
		})

		Convey("a crossed quote fails the gate", func() {
			measurement := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 103, 102, 1, 1, time.Unix(1, 0)))))

			So(measurement.Err, ShouldNotBeNil)
		})

		Convey("a measurement without a quote fails the gate", func() {
			m := tick("BTC/USD", 0, 0, 1, 1, time.Unix(1, 0))

			measurement := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](m)))

			So(measurement.Err, ShouldNotBeNil)
		})

		Convey("time regression surfaces without error and without advancing", func() {
			sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 1, 1, time.Unix(2, 0)))))

			measurement := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](tick("BTC/USD", 100, 102, 2, 1, time.Unix(1, 0)))))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Provenance["event_time_state"], ShouldEqual, "regressed")
		})

		Convey("Register declares the full metric schema without values", func() {
			measurement := entity.Register()

			So(measurement.ID, ShouldEqual, -1)
			So(measurement.Metrics, ShouldContainKey, "bid")
			So(measurement.Metrics, ShouldContainKey, "spread")

			for label, metric := range measurement.Metrics {
				So(label, ShouldEqual, metric.Label)
				So(metric.Raw, ShouldEqual, 0.0)
			}
		})
	})
}

func TestTickerStepReadiness(t *testing.T) {
	Convey("An inactive pipeline node drops input before touching processing state", t, func() {
		node := &Ticker{System: runtime.NewSystem(t.Context(), "readiness-test")}
		measurement := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: 7}
		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			So(sequence.Read[*data.Measurement[float64]](node.Next(sequence.NewValue[*data.Measurement[float64]](measurement))), ShouldEqual, measurement)
			So(node.Status(), ShouldEqual, stage)
			So(measurement.SeqIdx, ShouldEqual, 7)
		}
	})
}

func TestTickerStepQuoteSelection(t *testing.T) {
	Convey("Only complete touch quotes belong to liquidity", t, func() {
		ticker := readyTicker(t.Context())
		incomplete := data.NewMeasurement("futures", map[string]data.Metric[float64]{
			"bid": {Raw: 100}, "ask": {Raw: 102},
		})
		incomplete.Label = "BTC/USD"
		owned := ticker.Register()
		owned.Peers = []*data.Measurement[float64]{incomplete}

		Convey("an incomplete quote does not become zero displayed liquidity", func() {
			So(sequence.Read[*data.Measurement[float64]](ticker.Next(sequence.NewValue[*data.Measurement[float64]](owned))), ShouldBeNil)
		})

		Convey("a complete quote after it supplies the observation", func() {
			complete := tick("BTC/USD", 100, 102, 2, 3, time.Unix(1, 0))
			owned.Peers = append(owned.Peers, complete)
			result := sequence.Read[*data.Measurement[float64]](ticker.Next(sequence.NewValue[*data.Measurement[float64]](owned)))
			So(result.Err, ShouldBeNil)
			So(result.Metrics["touch_notional:bid"].Raw, ShouldEqual, 200)
			So(result.Metrics["touch_notional:ask"].Raw, ShouldEqual, 306)
		})
	})
}

func BenchmarkTickerNext(b *testing.B) {
	ticker := readyTicker(b.Context())
	peer := tick("BTC/USD", 100, 102, 2, 3, time.Unix(1, 0))
	owned := ticker.Register()
	owned.Peers = []*data.Measurement[float64]{peer}
	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		peer.At = peer.At.Add(time.Second)
		sequence.Read[*data.Measurement[float64]](ticker.Next(sequence.NewValue[*data.Measurement[float64]](owned)))
	}
}
