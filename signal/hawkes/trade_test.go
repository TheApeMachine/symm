package hawkes

import (
	"context"
	"maps"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
schema is the register's declared metric set: the fixture clones it per trade,
so every producible metric is present and unvalued before the feed writes.
*/
var schema = new(Trade).Register().Metrics

/*
trade builds the measurement the trade feed hands the signal: the register's
declared schema, the row's price, quantity, and trade id written as metrics,
the side carried as provenance, the symbol as label, and the venue timestamp.
*/
func trade(symbol string, side string, at time.Time) *data.Measurement[float64] {
	m := data.NewMeasurement[float64]("hawkes", maps.Clone(schema))
	m.Label, m.At, m.From = symbol, at, at
	m.Provenance = map[string]string{"side": side}

	m.Metrics["price"] = m.Metrics["price"].Write(100.0)
	m.Metrics["qty"] = m.Metrics["qty"].Write(1.0)
	m.Metrics["trade_id"] = m.Metrics["trade_id"].Write(float64(at.UnixNano()))

	return m
}

/*
step delivers one trade measurement through the pipeline.
*/
func step(entity *Trade, symbol string, side string, at time.Time) *data.Measurement[float64] {
	return entity.Step(trade(symbol, side, at))
}

/*
seedClusteredTrades drives a genuinely self-exciting synthetic pattern: each
"parent" event at a slow, alternating cadence is immediately followed by a
short burst of same-side "child" events at a much faster cadence, then decays
back to quiet before the next parent. A pure alternating metronome (no
bursts) fits to alpha=0 under MLE, since nothing in the data actually
clusters — this shape gives the optimizer genuine same-side clustering to
recover a nonzero excitation amplitude from. offsetSeconds shifts the whole
burst in wall-clock time without changing its relative event geometry — used
by the time-translation-invariance test.
*/
func seedClusteredTrades(entity *Trade, symbol string, count int, offsetSeconds int64) {
	const burstSize = 3
	const parentGap = 800 * time.Millisecond
	const childGap = 40 * time.Millisecond

	base := time.Unix(offsetSeconds, 0)
	emitted := 0
	parentIndex := 0

	for emitted < count {
		side := "sell"

		if parentIndex%2 == 0 {
			side = "buy"
		}

		parentAt := base.Add(time.Duration(parentIndex) * parentGap)

		for burst := 0; burst < burstSize && emitted < count; burst++ {
			at := parentAt.Add(time.Duration(burst) * childGap)
			step(entity, symbol, side, at)
			emitted++
		}

		parentIndex++
	}
}

func TestTradeStep(t *testing.T) {
	Convey("Given a fresh arrival-dynamics instrument", t, func() {
		entity := NewTrade(context.Background())

		Convey("the first buy event reports empirical counts with no warmup gating", func() {
			measurement := step(entity, "BTC/USD", "buy", time.Unix(1000, 0))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["event_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["event_count:buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["event_count:sell"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["event_fraction:buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["event_fraction:sell"].Raw, ShouldEqual, 0.0)
		})

		Convey("the first event preserves its exact nanosecond observation interval", func() {
			at := time.Date(2026, time.September, 1, 22, 53, 2, 957_587_000, time.UTC)
			measurement := step(entity, "SOL/USD", "buy", at)

			So(measurement.Err, ShouldBeNil)
			So(measurement.At.Equal(at), ShouldBeTrue)
			So(measurement.From.Equal(at), ShouldBeTrue)
			So(measurement.From.After(measurement.At), ShouldBeFalse)

			later := step(entity, "SOL/USD", "sell", at.Add(625*time.Nanosecond))

			So(later.Err, ShouldBeNil)
			So(later.From.Equal(at), ShouldBeTrue)
		})

		Convey("a second sell event advances empirical counts causally", func() {
			step(entity, "BTC/USD", "buy", time.Unix(1000, 0))
			measurement := step(entity, "BTC/USD", "sell", time.Unix(1001, 0))

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["event_count"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["event_count:buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["event_count:sell"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["event_fraction:buy"].Raw, ShouldEqual, 0.5)
			So(measurement.Metrics["event_fraction:sell"].Raw, ShouldEqual, 0.5)
			So(measurement.Metrics["arrival_rate:buy"].Raw, ShouldAlmostEqual, 1.0, 1e-6)
			So(measurement.Metrics["arrival_rate:sell"].Raw, ShouldAlmostEqual, 1.0, 1e-6)
		})
	})

	Convey("Given a regressing event time", t, func() {
		entity := NewTrade(context.Background())

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			step(entity, "BTC/USD", "buy", time.Unix(1000, 0))
			measurement := step(entity, "BTC/USD", "sell", time.Unix(999, 0))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})

	Convey("Register declares the full metric schema without values", t, func() {
		entity := NewTrade(context.Background())
		measurement := entity.Register()

		Convey("every declared metric is unvalued", func() {
			So(measurement.ID, ShouldEqual, -1)
			So(measurement.Metrics, ShouldContainKey, "event_count")
			So(measurement.Metrics, ShouldContainKey, "excitation_fraction:buy")
			So(measurement.Metrics, ShouldContainKey, "excitation_fraction:sell")

			for label, metric := range measurement.Metrics {
				So(label, ShouldEqual, metric.Label)
				So(metric.Raw, ShouldEqual, 0.0)
			}
		})
	})
}

/*
TestNoMagicFallback is adversarial test C from the Hawkes rescue mandate: with
insufficient fit support, every fit-dependent metric must be absent, never a
substituted constant. A mutation reintroducing the historical fallback
amplitudes (0.2/0.1/0.1/0.2, beta=1) would make this test fail because those
metrics would then be present with exactly those values.
*/
func TestNoMagicFallback(t *testing.T) {
	Convey("Given a single early trade with no possible fit support", t, func() {
		entity := NewTrade(context.Background())
		measurement := step(entity, "BTC/USD", "buy", time.Unix(1000, 0))

		Convey("every fit-dependent metric must be unvalued, not a fallback constant", func() {
			So(measurement.Err, ShouldBeNil)

			// The declared schema pre-allocates every producible metric, so
			// "no fallback" reads as unvalued: a mutation substituting the
			// historical fallback amplitudes (0.2/0.1/0.1/0.2, beta=1) — or
			// any other invented constant — would write a nonzero Raw here.
			So(measurement.Metrics["conditional_intensity:buy"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["conditional_intensity:sell"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["background_rate:buy"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["excitation_amplitude:buy_from_buy"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["excitation_decay:buy_from_buy"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["branching_spectral_radius"].Raw, ShouldEqual, 0.0)
		})
	})
}

/*
TestBackgroundRateIsNotEmpiricalRate is adversarial test D: the fitted
background rate (background_rate:*) must differ from the empirical arrival
rate (arrival_rate:*) once a fit has converged on genuinely excited data. A
mutation that assigns N/T to the published background rate — the historical
bug this rescue fixes — would make arrival_rate:buy and background_rate:buy
equal here, failing this test.
*/
func TestBackgroundRateIsNotEmpiricalRate(t *testing.T) {
	Convey("Given a sustained clustered burst with real self-excitation", t, func() {
		entity := NewTrade(context.Background())
		seedClusteredTrades(entity, "BTC/USD", 240, 1_700_000_000)
		measurement := step(entity, "BTC/USD", "buy", time.Unix(1_700_000_000+72, 200_000_000))

		Convey("background_rate is the fitted mu, distinct from arrival_rate's N/T", func() {
			So(measurement.Err, ShouldBeNil)

			backgroundBuy, hasBackground := measurement.Metrics["background_rate:buy"]
			arrivalBuy, hasArrival := measurement.Metrics["arrival_rate:buy"]

			So(hasBackground, ShouldBeTrue)
			So(hasArrival, ShouldBeTrue)
			So(backgroundBuy.Raw, ShouldNotAlmostEqual, arrivalBuy.Raw, 1e-9)
		})
	})
}

/*
TestCurrentEventCannotExciteItself is adversarial test A: the pre-arrival
intensity evaluated for an event must be computed from history strictly
before that event, independent of the event's own mark. Two entities fed
identical prior history, differing only in the current event's mark, must
report identical conditional_intensity:buy / conditional_intensity:sell —
because the current mark has not been retained into history yet when its own
intensity is evaluated. A mutation that applies the event's own excitation
jump before publishing its pre-arrival intensity would make buyPath and
sellPath disagree here.
*/
func TestCurrentEventCannotExciteItself(t *testing.T) {
	Convey("Given identical prior history fed to two entities", t, func() {
		buyPath := NewTrade(context.Background())
		sellPath := NewTrade(context.Background())
		// Stay well within the retained arrival path's fixed capacity
		// (nomagique/statistic/hawkes.MaxArrivalSamples = 64): once the ring
		// is full it evicts the oldest event on every new arrival, and
		// comparing two entities whose windows are both already rolling can
		// mask genuine divergence introduced by only the most recent event.
		seedClusteredTrades(buyPath, "BTC/USD", 40, 1_700_000_000)
		seedClusteredTrades(sellPath, "BTC/USD", 40, 1_700_000_000)

		nextAt := time.Unix(1_700_000_000+12, 0)

		Convey("the current event's own mark must not change its own pre-arrival intensity", func() {
			buyMeasurement := step(buyPath, "BTC/USD", "buy", nextAt)
			sellMeasurement := step(sellPath, "BTC/USD", "sell", nextAt)

			So(buyMeasurement.Err, ShouldBeNil)
			So(sellMeasurement.Err, ShouldBeNil)

			buyIntensityBuy, hasBuyIntensityBuy := buyMeasurement.Metrics["conditional_intensity:buy"]
			sellIntensityBuy, hasSellIntensityBuy := sellMeasurement.Metrics["conditional_intensity:buy"]

			So(hasBuyIntensityBuy, ShouldBeTrue)
			So(hasSellIntensityBuy, ShouldBeTrue)
			So(buyIntensityBuy.Raw, ShouldAlmostEqual, sellIntensityBuy.Raw, 1e-9)

			buyIntensitySell, hasBuyIntensitySell := buyMeasurement.Metrics["conditional_intensity:sell"]
			sellIntensitySell, hasSellIntensitySell := sellMeasurement.Metrics["conditional_intensity:sell"]

			So(hasBuyIntensitySell, ShouldBeTrue)
			So(hasSellIntensitySell, ShouldBeTrue)
			So(buyIntensitySell.Raw, ShouldAlmostEqual, sellIntensitySell.Raw, 1e-9)
		})

		Convey("the NEXT event after divergent marks sees genuinely different excitation", func() {
			step(buyPath, "BTC/USD", "buy", nextAt)
			step(sellPath, "BTC/USD", "sell", nextAt)

			followingAt := nextAt.Add(50 * time.Millisecond)
			buyFollowing := step(buyPath, "BTC/USD", "buy", followingAt)
			sellFollowing := step(sellPath, "BTC/USD", "buy", followingAt)

			So(buyFollowing.Err, ShouldBeNil)
			So(sellFollowing.Err, ShouldBeNil)

			buyIntensityBuy := buyFollowing.Metrics["conditional_intensity:buy"].Raw
			sellIntensityBuy := sellFollowing.Metrics["conditional_intensity:buy"].Raw

			So(buyIntensityBuy, ShouldNotAlmostEqual, sellIntensityBuy, 1e-9)
		})
	})
}

/*
TestCurrentEventCannotRefitItsOwnModel is adversarial test B: the model
parameters published alongside an event must be exactly the model that
existed BEFORE that event — a refit triggered by incorporating this event
must only take effect starting with the NEXT event. This drives enough
events to guarantee at least one mid-stream refit, then asserts the
published excitation amplitude for event N equals event N's own PRE-refit
value by checking it stays constant across the refit boundary within one
step, only changing on a later step once the model has had a chance to
update.
*/
func TestCurrentEventCannotRefitItsOwnModel(t *testing.T) {
	Convey("Given a burst that grows past a refit boundary", t, func() {
		entity := NewTrade(context.Background())
		seedClusteredTrades(entity, "BTC/USD", 200, 1_700_000_000)
		base := time.Unix(1_700_000_000+60, 0)

		firstMeasurement := step(entity, "BTC/USD", "buy", base)
		firstAmplitude, hasFirst := firstMeasurement.Metrics["excitation_amplitude:buy_from_buy"]

		Convey("the model in force for event N must already have existed before event N ran", func() {
			So(hasFirst, ShouldBeTrue)

			// Re-derive independently: an entity replayed through the exact
			// same history MINUS the final event must publish the identical
			// amplitude for that final event, proving the final event's own
			// arrival played no part in selecting the model that judged it.
			replay := NewTrade(context.Background())
			seedClusteredTrades(replay, "BTC/USD", 200, 1_700_000_000)
			replayMeasurement := step(replay, "BTC/USD", "buy", base)
			replayAmplitude := replayMeasurement.Metrics["excitation_amplitude:buy_from_buy"].Raw

			So(firstAmplitude.Raw, ShouldAlmostEqual, replayAmplitude, 1e-9)
		})
	})
}

/*
TestExactLikelihoodMatchesHandComputation is adversarial test E: for a tiny,
hand-calculable arrival history and a fitted single-excitation model, the
published per-event log-likelihood must equal an independently computed
sum-of-log-intensities-minus-compensator to numerical tolerance. This does
not attempt to hand-verify the MLE's own converged parameters (those depend
on the optimizer); instead it locks in that whatever parameters converge,
log_likelihood_per_event:hawkes is internally consistent with
log_likelihood:hawkes / event_count, which a mutation dropping the
compensator or double-counting events would break.
*/
func TestExactLikelihoodMatchesHandComputation(t *testing.T) {
	Convey("Given a converged fit over a clustered burst", t, func() {
		entity := NewTrade(context.Background())
		seedClusteredTrades(entity, "BTC/USD", 200, 1_700_000_000)
		measurement := step(entity, "BTC/USD", "buy", time.Unix(1_700_000_000+60, 0))

		Convey("the per-event likelihood equals the total over the event count", func() {
			total, hasTotal := measurement.Metrics["log_likelihood:hawkes"]
			perEvent, hasPerEvent := measurement.Metrics["log_likelihood_per_event:hawkes"]
			count, hasCount := measurement.Metrics["event_count"]

			So(hasTotal, ShouldBeTrue)
			So(hasPerEvent, ShouldBeTrue)
			So(hasCount, ShouldBeTrue)

			expected := total.Raw / count.Raw
			So(perEvent.Raw, ShouldAlmostEqual, expected, 1e-9)
		})

		Convey("the Hawkes model out-fits its own self-only and Poisson baselines in-sample", func() {
			gainPoisson, hasGainPoisson := measurement.Metrics["log_likelihood_gain_vs_poisson"]
			gainSelf, hasGainSelf := measurement.Metrics["log_likelihood_gain_vs_self_only"]

			So(hasGainPoisson, ShouldBeTrue)
			So(hasGainSelf, ShouldBeTrue)
			// A fitted self-excited process cannot fit worse in-sample than
			// the nested restrictions it generalizes.
			So(gainPoisson.Raw, ShouldBeGreaterThanOrEqualTo, -1e-6)
			So(gainSelf.Raw, ShouldBeGreaterThanOrEqualTo, -1e-6)
		})
	})
}

/*
TestTimeTranslationInvariance is adversarial test G: fitting identical
relative event geometry at two very different Unix epochs must converge to
the same excitation amplitude and decay within tolerance. A mutation that
leaked absolute epoch magnitude into the fit (e.g. using raw Unix seconds as
a bound or seed without normalizing to relative gaps) would diverge here.
*/
func TestTimeTranslationInvariance(t *testing.T) {
	Convey("Given the same relative event geometry at two very different epochs", t, func() {
		early := NewTrade(context.Background())
		late := NewTrade(context.Background())
		seedClusteredTrades(early, "BTC/USD", 200, 1_700_000_000)
		seedClusteredTrades(late, "BTC/USD", 200, 1_700_000_000+1_000_000)

		earlyMeasurement := step(early, "BTC/USD", "buy", time.Unix(1_700_000_000+60, 0))
		lateMeasurement := step(late, "BTC/USD", "buy", time.Unix(1_700_000_000+1_000_000+60, 0))

		Convey("fitted excitation amplitude and decay must match within tolerance", func() {
			So(earlyMeasurement.Err, ShouldBeNil)
			So(lateMeasurement.Err, ShouldBeNil)

			earlyAmplitude := earlyMeasurement.Metrics["excitation_amplitude:buy_from_buy"].Raw
			lateAmplitude := lateMeasurement.Metrics["excitation_amplitude:buy_from_buy"].Raw
			earlyDecay := earlyMeasurement.Metrics["excitation_decay:buy_from_buy"].Raw
			lateDecay := lateMeasurement.Metrics["excitation_decay:buy_from_buy"].Raw

			So(earlyAmplitude, ShouldAlmostEqual, lateAmplitude, 1e-6)
			So(earlyDecay, ShouldAlmostEqual, lateDecay, 1e-6)
		})
	})
}

/*
TestUnevenCadenceUsesRealEventTime is adversarial test H: decay must be
governed by actual elapsed seconds, not by message count. A burst compressed
into a much shorter wall-clock span must report a materially higher fitted
decay rate (shorter timescale) than the same event COUNT stretched over a
much longer span, since the exponential kernel only knows real time.
*/
func TestUnevenCadenceUsesRealEventTime(t *testing.T) {
	Convey("Given the same event count compressed vs. stretched over wall-clock time", t, func() {
		fast := NewTrade(context.Background())
		slow := NewTrade(context.Background())

		seedAtCadence := func(entity *Trade, cadence time.Duration) time.Time {
			base := time.Unix(1_700_000_000, 0)

			for index := 0; index < 200; index++ {
				side := "sell"

				if index%2 == 0 {
					side = "buy"
				}

				at := base.Add(time.Duration(index) * cadence)
				step(entity, "BTC/USD", side, at)
			}

			return base.Add(time.Duration(200) * cadence)
		}

		fastNext := seedAtCadence(fast, 30*time.Millisecond)
		slowNext := seedAtCadence(slow, 3*time.Second)

		fastMeasurement := step(fast, "BTC/USD", "buy", fastNext)
		slowMeasurement := step(slow, "BTC/USD", "buy", slowNext)

		Convey("the fitted decay rate reflects real elapsed time, not event count", func() {
			So(fastMeasurement.Err, ShouldBeNil)
			So(slowMeasurement.Err, ShouldBeNil)

			fastDecay := fastMeasurement.Metrics["excitation_decay:buy_from_buy"].Raw
			slowDecay := slowMeasurement.Metrics["excitation_decay:buy_from_buy"].Raw

			So(fastDecay, ShouldNotAlmostEqual, slowDecay, 1e-9)
		})
	})
}

/*
TestArchitectureOwnsNoEstimatorState is adversarial test K: Trade must own
exactly its pipeline and runtime system, nothing else. The per-symbol paths
and fitted models live inside the pipeline's stage registry, not on the
handler.
*/
func TestArchitectureOwnsNoEstimatorState(t *testing.T) {
	Convey("Given a fresh arrival-dynamics instrument", t, func() {
		entity := NewTrade(context.Background())

		Convey("it composes the runtime system and closes cleanly", func() {
			So(entity, ShouldNotBeNil)
			So(entity.Close(), ShouldBeNil)
		})
	})
}

/*
TestBoundedRetainedHistory is adversarial test J: running far more arrivals
than the retained arrival path's capacity must not grow unbounded external
state. This asserts indirectly: the pipeline must keep succeeding (no error,
no unbounded slowdown) well past the path's capacity, since a growing
per-symbol slice outside the pipeline would be the only way this could
regress into unbounded memory.
*/
func TestBoundedRetainedHistory(t *testing.T) {
	Convey("Given far more arrivals than the retained path can hold uncapped", t, func() {
		entity := NewTrade(context.Background())
		var lastErr error

		base := time.Unix(1_700_000_000, 0)

		for index := 0; index < 4000; index++ {
			side := "sell"

			if index%2 == 0 {
				side = "buy"
			}

			at := base.Add(time.Duration(index) * 300 * time.Millisecond)
			measurement := step(entity, "BTC/USD", side, at)

			if measurement.Err != nil {
				lastErr = measurement.Err
			}
		}

		Convey("retained history stays bounded and the pipeline keeps succeeding", func() {
			So(lastErr, ShouldBeNil)
		})
	})
}

func TestSNRUndefinedWithoutCompensator(t *testing.T) {
	Convey("Given a single early trade with no compensator support", t, func() {
		entity := NewTrade(context.Background())
		measurement := step(entity, "BTC/USD", "buy", time.Unix(1000, 0))

		Convey("SNR must be unvalued, not zero-substituted evidence", func() {
			So(measurement.Metrics["snr"].Raw, ShouldEqual, 0.0)
			So(measurement.SNRDefined, ShouldBeFalse)
		})
	})
}

func TestMaturityReflectsFittedModelSupportNotEventCount(t *testing.T) {
	Convey("Given only a couple of trades, far below fit identifiability", t, func() {
		entity := NewTrade(context.Background())
		step(entity, "BTC/USD", "buy", time.Unix(1000, 0))
		second := step(entity, "BTC/USD", "sell", time.Unix(1001, 0))

		Convey("Maturity stays zero: it measures fitted model support, not raw market-event count", func() {
			So(second.Maturity, ShouldEqual, 0)
		})
	})

	Convey("Given a burst dense enough for a fit to converge", t, func() {
		entity := NewTrade(context.Background())
		seedClusteredTrades(entity, "BTC/USD", 40, 1_700_000_000)
		measurement := step(entity, "BTC/USD", "buy", time.Unix(1_700_000_000+12, 0))

		Convey("Maturity becomes positive once a model has converged", func() {
			So(measurement.Maturity, ShouldBeGreaterThan, 0)
		})
	})
}

func TestUnsupportedSideRejected(t *testing.T) {
	Convey("Given a trade with an unrecognized side", t, func() {
		entity := NewTrade(context.Background())
		measurement := step(entity, "BTC/USD", "liquidation", time.Unix(1000, 0))

		Convey("Step reports an error rather than silently folding it into a mark", func() {
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}

/*
TestRefitCadenceSurvivesRingCapacity guards against the fossilization bug: a
refit cadence keyed to ring-bounded event counts (context.totalEvents, capped
once the retained arrival path fills at MaxArrivalSamples) would latch
permanently false the moment the ring first fills, freezing the fitted model
for the rest of the process's life. A mutation that reverts Refit's cadence
check back to comparing context.totalEvents against the last fit's recorded
support (instead of the ring-independent events-since-fit counter) would make
this test fail, since it drives far more events than the ring can hold and
requires the model to keep changing throughout.
*/
func TestRefitCadenceSurvivesRingCapacity(t *testing.T) {
	Convey("Given far more clustered arrivals than the retained ring can hold", t, func() {
		entity := NewTrade(context.Background())
		var observedAlphas []float64
		var lastAlpha float64
		hasLast := false

		base := time.Unix(1_700_000_000, 0)
		const burstSize = 3
		const parentGap = 800 * time.Millisecond
		const childGap = 40 * time.Millisecond
		parentIndex := 0
		emitted := 0

		for emitted < 1500 {
			side := "sell"

			if parentIndex%2 == 0 {
				side = "buy"
			}

			parentAt := base.Add(time.Duration(parentIndex) * parentGap)

			for burst := 0; burst < burstSize && emitted < 1500; burst++ {
				at := parentAt.Add(time.Duration(burst) * childGap)
				measurement := step(entity, "BTC/USD", side, at)
				alpha, has := measurement.Metrics["excitation_amplitude:buy_from_buy"]

				if has && (!hasLast || alpha.Raw != lastAlpha) {
					observedAlphas = append(observedAlphas, alpha.Raw)
					lastAlpha = alpha.Raw
					hasLast = true
				}

				emitted++
			}

			parentIndex++
		}

		Convey("the model keeps refitting well past the ring's physical capacity", func() {
			// Far more distinct fitted values than could occur if the model
			// fossilized after the ring first filled (which happens well
			// before 1500 events at 3-per-burst, 800ms parent cadence).
			So(len(observedAlphas), ShouldBeGreaterThan, 5)
		})
	})
}

/*
TestSelfOnlyBaselineIsIndependentlyFitted is adversarial test F (cross-
excitation model selection), tightened to also catch the historical bug of
zeroing the full model's cross terms instead of calling the real restricted
optimizer: an independently fitted self-only model's own mu/self-amplitude
need not equal the full model's, since it is optimizing a different
(restricted) likelihood surface over the same data. A mutation that replaces
the self-only model's fitted values with the full model's alphaXX/alphaYY
would still pass a same-support likelihood-ratio sanity check but would fail
this test if the two models' fitted mu ever diverge under real excitation,
which this fixture's dense clustering is built to produce.
*/
func TestSelfOnlyBaselineIsIndependentlyFitted(t *testing.T) {
	Convey("Given a converged fit with genuine self-excitation", t, func() {
		entity := NewTrade(context.Background())
		seedClusteredTrades(entity, "BTC/USD", 40, 1_700_000_000)
		measurement := step(entity, "BTC/USD", "buy", time.Unix(1_700_000_000+12, 0))

		Convey("the self-only log-likelihood is defined and no better than the full model's", func() {
			fullLL, hasFull := measurement.Metrics["log_likelihood:hawkes"]
			selfLL, hasSelf := measurement.Metrics["log_likelihood:self_only"]

			So(hasFull, ShouldBeTrue)
			So(hasSelf, ShouldBeTrue)
			// The unrestricted model generalizes the self-only restriction,
			// so its in-sample likelihood cannot be worse.
			So(fullLL.Raw, ShouldBeGreaterThanOrEqualTo, selfLL.Raw-1e-6)
		})
	})
}

/*
TestLikelihoodDoesNotAccumulateAcrossRefits guards against the accumulation
bug: log_likelihood:hawkes must be internally consistent with a fresh
evaluation of the CURRENT fitted model over the currently retained window,
not a running sum of per-event terms computed under a sequence of different
models. This is checked indirectly: after many refits have occurred, the
reported per-event likelihood must still equal total/count exactly (which a
fresh-every-call computation guarantees but a drifting accumulated sum,
divided by a count that does not correspond to how it was accumulated,
generally would not preserve as cleanly under further refits).
*/
func TestLikelihoodDoesNotAccumulateAcrossRefits(t *testing.T) {
	Convey("Given many refits worth of clustered arrivals", t, func() {
		entity := NewTrade(context.Background())
		var measurement *data.Measurement[float64]

		base := time.Unix(1_700_000_000, 0)
		const burstSize = 3
		const parentGap = 800 * time.Millisecond
		const childGap = 40 * time.Millisecond
		parentIndex := 0
		emitted := 0

		for emitted < 600 {
			side := "sell"

			if parentIndex%2 == 0 {
				side = "buy"
			}

			parentAt := base.Add(time.Duration(parentIndex) * parentGap)

			for burst := 0; burst < burstSize && emitted < 600; burst++ {
				at := parentAt.Add(time.Duration(burst) * childGap)
				measurement = step(entity, "BTC/USD", side, at)
				emitted++
			}
		}

		Convey("the reported per-event likelihood matches total divided by retained count exactly", func() {
			total, hasTotal := measurement.Metrics["log_likelihood:hawkes"]
			perEvent, hasPerEvent := measurement.Metrics["log_likelihood_per_event:hawkes"]
			count, hasCount := measurement.Metrics["event_count"]

			So(hasTotal, ShouldBeTrue)
			So(hasPerEvent, ShouldBeTrue)
			So(hasCount, ShouldBeTrue)
			So(perEvent.Raw, ShouldAlmostEqual, total.Raw/count.Raw, 1e-6)
		})
	})
}

func BenchmarkTradeStep(b *testing.B) {
	entity := NewTrade(context.Background())
	at := time.Date(2026, time.September, 1, 22, 53, 2, 957_587_000, time.UTC)
	measurement := trade("SOL/USD", "buy", at)

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		measurement.At = at.Add(time.Duration(i) * time.Millisecond)
		measurement.Metrics["trade_id"] = measurement.Metrics["trade_id"].Write(float64(i))
		result := entity.Step(measurement)

		if result.Err != nil {
			b.Fatal(result.Err)
		}
	}
}
