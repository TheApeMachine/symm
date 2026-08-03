package resonance_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/tests"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

/*
The pace bounds the solver is configured with. These mirror restAlpha, minAlpha
and maxAlpha in the resonance package, which are unexported, and this test is an
external consumer of the published readings rather than of the package internals.
A drift between the two shows up as a failure here rather than as a silently
weakened assertion, because every bound below is stated relative to them.
*/
const (
	simulationRestAlpha = 0.03
	simulationMinAlpha  = 0.005
	simulationMaxAlpha  = 0.150

	/*
		Mirrors maxHorizon on the solver, which is likewise unexported.
	*/
	simulationMaxHorizon = 20
)

/*
resonanceTrace is what the predictive coding stage published on one tick.
*/
type resonanceTrace struct {
	alpha      float64
	confidence float64
	surprise   float64
	energy     float64
	horizon    int
	curve      []float64
	retention  []float64
}

/*
readResonance lifts the stage's published readings off the thesis.

ok is false while the stage is still in warmup, which is the ordinary state
until the feature schema has settled and the task head has a supervised sample.
*/
func readResonance(thesis *types.Thesis) (resonanceTrace, bool) {
	if thesis == nil || thesis.Resonance == nil {
		return resonanceTrace{}, false
	}

	rowRaw, found := thesis.Resonance.Load(types.Focus())

	if !found {
		return resonanceTrace{}, false
	}

	row, ok := rowRaw.(map[string]any)

	if !ok {
		return resonanceTrace{}, false
	}

	read := func(key string) (float64, bool) {
		raw, found := row[key]

		if !found {
			return 0, false
		}

		value, ok := raw.(float64)

		return value, ok
	}

	alpha, hasAlpha := read("alpha")
	confidence, hasConfidence := read("confidence")
	surprise, hasSurprise := read("surprise")
	energy, hasEnergy := read("energy")

	if !hasAlpha || !hasConfidence || !hasSurprise || !hasEnergy {
		return resonanceTrace{}, false
	}

	trace := resonanceTrace{
		alpha:      alpha,
		confidence: confidence,
		surprise:   surprise,
		energy:     energy,
	}

	if raw, found := row["activeHorizon"]; found {
		if horizon, ok := raw.(int); ok {
			trace.horizon = horizon
		}
	}

	if raw, found := row["forwardCurve"]; found {
		if curve, ok := raw.([]float64); ok {
			trace.curve = curve
		}
	}

	if raw, found := row["forwardRetention"]; found {
		if retention, ok := raw.([]float64); ok {
			trace.retention = retention
		}
	}

	return trace, true
}

/*
simulate runs the full production ensemble across a scripted sequence of market
regimes and returns every reading the predictive coding stage published.
*/
func simulate(market *tests.Market, script []struct {
	state testtypes.MarketState
	ticks int
}) []resonanceTrace {
	traces := make([]resonanceTrace, 0, 4096)

	for _, phase := range script {
		market.Transition(phase.state)

		for range phase.ticks {
			market.Tick()
			market.Planner.Update(market.Thesis)

			if trace, ok := readResonance(market.Thesis); ok {
				traces = append(traces, trace)
			}
		}
	}

	return traces
}

/*
simulationSymbols is the cross-section the simulation runs.

Two symbols rather than one, because several signals are relative readings taken
against a peer and report nothing at all in a single-symbol market, which would
leave the feature schema unrepresentatively narrow. Two rather than more because
the full ensemble costs roughly eleven milliseconds per symbol per tick, and the
regime coverage this test needs matters more than the width of the cross-section
it runs over.
*/
func simulationSymbols() []*testtypes.Symbol {
	return []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
	}
}

/*
TestSimulatedRegimeSequence drives the predictive coding stage through a full
market cycle and holds every published reading to what it claims to mean.

This is the test the unit assertions cannot replace. Each defect being pinned
here was invisible on a short synthetic stream and only appeared once the stage
ran against a live-shaped feature schema over many regime changes.
*/
func TestSimulatedRegimeSequence(t *testing.T) {
	Convey("Given the full ensemble driven across a market cycle", t, func() {
		/*
			The baseline stretches are sized to carry the calibration window,
			which needs errorCalibratorWindow ticks of history before it reports
			a rank at all. The regime stretches are sized to outlast that window
			so a sustained condition is visible as sustained rather than as a
			transient inside it.
		*/
		script := []struct {
			state testtypes.MarketState
			ticks int
		}{
			{testtypes.Baseline, 300},
			{testtypes.SlowPump, 120},
			{testtypes.FastPump, 120},
			{testtypes.SidewaysChop, 120},
			{testtypes.FastDump, 120},
			{testtypes.FlashCrash, 80},
			{testtypes.Baseline, 200},
		}

		var traces []resonanceTrace

		tests.WithMarket(t, simulationSymbols(), func(market *tests.Market) {
			traces = simulate(market, script)
		})()

		So(len(traces), ShouldBeGreaterThan, 500)

		Convey("Then the learning pace never pins to a bound", func() {
			/*
				The previous controller reached its ceiling within roughly forty
				ticks of any spiky stream and stayed there for the rest of the
				run. A cycle containing two pumps, a dump and a flash crash is
				the strongest available version of that stream.
			*/
			atCeiling := 0
			atFloor := 0

			for _, trace := range traces {
				if trace.alpha >= simulationMaxAlpha-1e-9 {
					atCeiling++
				}

				if trace.alpha <= simulationMinAlpha+1e-9 {
					atFloor++
				}
			}

			ceilingShare := float64(atCeiling) / float64(len(traces))
			floorShare := float64(atFloor) / float64(len(traces))
			pinnedShare := ceilingShare + floorShare

			t.Logf("alpha at ceiling %.1f%%, at floor %.1f%%, pinned %.1f%%",
				ceilingShare*100, floorShare*100, pinnedShare*100)

			/*
				A share below a half on each bound separately would still admit
				a pace pinned to one bound or the other for virtually the whole
				run, which is the very condition being excluded. The bound is
				therefore on the combined share, and set where a controller with
				a working fixed point should sit: the extremes are reserved for
				genuine excursions, so the pace belongs in its dynamic range for
				the large majority of a cycle that is mostly not in crisis.
			*/
			So(pinnedShare, ShouldBeLessThan, 0.20)
		})

		Convey("Then the pace returns toward rest after the crash", func() {
			/*
				A controller with a fixed point relaxes once the market quiets.
				A ratchet does not: whatever the crash did to the pace would
				still be there at the end of the closing baseline.

				The assertion is a strict decay from the peak, not a comparison
				against it. A pace still locked at whatever the crash drove it
				to satisfies "no greater than the peak" exactly, so that form
				would pass on the very behaviour it claims to exclude.
			*/
			crashPeak := 0.0

			for _, trace := range traces[:len(traces)-200] {
				crashPeak = math.Max(crashPeak, trace.alpha)
			}

			settled := traces[len(traces)-1].alpha
			decayed := 1.0 - settled/crashPeak

			t.Logf("peak alpha %.5f, settled alpha %.5f, decayed %.1f%% toward rest",
				crashPeak, settled, decayed*100)

			/*
				Rest is where the pace belongs once the evidence that moved it
				has passed, so the settled pace is held to a neighbourhood of
				rest rather than merely to some distance below the peak. A
				factor of two either side allows for the closing baseline still
				carrying ordinary variation without admitting a pace left
				anywhere near a bound.
			*/
			So(settled, ShouldBeLessThan, crashPeak*0.7)
			So(settled, ShouldBeBetween, simulationRestAlpha/2, simulationRestAlpha*2)
		})

		Convey("Then confidence is a calibrated probability rather than zero", func() {
			/*
				exp(-surprise) returned approximately zero on every tick once the
				schema grew past a handful of features, which read downstream as
				a permanent no-confidence. A calibrated reading has to actually
				use its range.
			*/
			minimum := math.Inf(1)
			maximum := math.Inf(-1)
			total := 0.0

			/*
				Extremes alone would be a weak reading: a single tick above a
				half satisfies a maximum bound while every other tick sits at
				zero, which is the collapsed behaviour being excluded. A
				quantile against a stable distribution is uniform by
				construction, so the test is on how the mass is spread.
			*/
			var buckets [4]int

			for _, trace := range traces {
				minimum = math.Min(minimum, trace.confidence)
				maximum = math.Max(maximum, trace.confidence)
				total += trace.confidence

				bucket := int(trace.confidence * 4)

				if bucket > 3 {
					bucket = 3
				}

				buckets[bucket]++
			}

			mean := total / float64(len(traces))

			t.Logf("confidence min %.4f mean %.4f max %.4f, quartile counts %v",
				minimum, mean, maximum, buckets)

			So(minimum, ShouldBeGreaterThanOrEqualTo, 0)
			So(maximum, ShouldBeLessThanOrEqualTo, 1)

			/*
				Every quartile occupied, and the mean near the middle of the
				range. exp(-surprise) put essentially all its mass in the lowest
				bucket with a mean indistinguishable from zero.
			*/
			for _, count := range buckets {
				So(count, ShouldBeGreaterThan, 0)
			}

			So(mean, ShouldBeBetween, 0.25, 0.75)
		})

		Convey("Then every published reading stays finite", func() {
			/*
				Downstream reads these straight into a trade candidate, where a
				NaN silently disqualifies a symbol and an infinity sizes an
				order against a meaningless edge.
			*/
			for _, trace := range traces {
				So(math.IsNaN(trace.surprise), ShouldBeFalse)
				So(math.IsInf(trace.surprise, 0), ShouldBeFalse)
				So(math.IsNaN(trace.energy), ShouldBeFalse)
				So(math.IsInf(trace.energy, 0), ShouldBeFalse)
				So(math.IsNaN(trace.confidence), ShouldBeFalse)
				So(math.IsNaN(trace.alpha), ShouldBeFalse)

				for _, value := range trace.curve {
					So(math.IsNaN(value), ShouldBeFalse)
					So(math.IsInf(value, 0), ShouldBeFalse)
				}
			}
		})

		Convey("Then the forward curve is paired with an honest retention envelope", func() {
			/*
				The rollout is a contraction, so later curve entries decay toward
				zero whatever the market does. Retention is what lets a consumer
				tell a forecast from the fade, so wherever a curve is published
				its envelope must be published with it and must agree in length.
			*/
			withCurve := 0

			for _, trace := range traces {
				if len(trace.curve) == 0 {
					continue
				}

				withCurve++

				So(len(trace.retention), ShouldEqual, len(trace.curve))

				for _, surviving := range trace.retention {
					So(surviving, ShouldBeGreaterThanOrEqualTo, 0)
					So(math.IsNaN(surviving), ShouldBeFalse)
				}
			}

			t.Logf("ticks publishing a forward curve: %d of %d", withCurve, len(traces))
			So(withCurve, ShouldBeGreaterThan, 0)
		})

		Convey("Then the horizon is bounded by the retention floor", func() {
			for _, trace := range traces {
				So(trace.horizon, ShouldBeGreaterThanOrEqualTo, 1)
				So(trace.horizon, ShouldBeLessThanOrEqualTo, 20)
			}
		})
	})
}

/*
TestSimulatedHorizonExtends pins the behaviour the predictive coding stage was
built for, driven by a real market rather than a synthetic stream: the forecast
window grows as far ahead as the head's precision supports, and gives way when
it does not.

The solver is driven directly rather than read off the analyzer's own instance,
because what is under test is the horizon controller and the temporal dynamics
that feed it. Reading the analyzer's solver would make this a test of how many
measurements the whole signal pipeline happened to deliver.
*/
func TestSimulatedHorizonExtends(t *testing.T) {
	Convey("Given a solver driven through a real market", t, func() {
		solver := resonance.NewSolver(make(chan []byte, 1), nil)

		horizons := make([]int, 0, 512)

		tests.WithMarket(t, simulationSymbols(), func(market *tests.Market) {
			for range 500 {
				market.Tick()

				if err := solver.Update(market.Thesis); err != nil {
					continue
				}

				rowRaw, found := market.Thesis.Resonance.Load(types.Focus())

				if !found {
					continue
				}

				row, ok := rowRaw.(map[string]any)

				if !ok {
					continue
				}

				if horizon, ok := row["activeHorizon"].(int); ok {
					horizons = append(horizons, horizon)
				}
			}
		})()

		So(len(horizons), ShouldBeGreaterThan, 100)

		first := horizons[0]
		widest := 0

		for _, horizon := range horizons {
			if horizon > widest {
				widest = horizon
			}
		}

		settled := horizons[len(horizons)-1]

		t.Logf("horizon first %d, widest %d, settled %d, over %d ticks",
			first, widest, settled, len(horizons))

		Convey("Then it starts at the shortest reach", func() {
			/*
				Reach is earned. A head that has resolved nothing has no basis
				for claiming to see any distance at all.
			*/
			So(first, ShouldEqual, 1)
		})

		Convey("Then it extends well past a single step as precision holds", func() {
			/*
				A window stuck at one step makes the entire rollout dead weight,
				which is what the uncalibrated confidence produced: the ceiling
				was multiplied by a number that evaluated to approximately zero
				on every tick, so the horizon was one however well the network
				was predicting.
			*/
			So(widest, ShouldBeGreaterThan, 5)
			So(widest, ShouldBeLessThanOrEqualTo, simulationMaxHorizon)
		})

		Convey("Then it stays responsive rather than pinning at the ceiling", func() {
			/*
				A horizon that reached the cap and stayed there would be a
				ratchet in the reach, the same defect the pace controller had.
				The published window has to keep moving with retention and
				confidence.
			*/
			distinct := map[int]struct{}{}

			for _, horizon := range horizons {
				distinct[horizon] = struct{}{}
			}

			So(len(distinct), ShouldBeGreaterThan, 1)
		})
	})
}

/*
TestSimulatedTaskHeadLearns pins that the supervised head actually fits the
market rather than decaying to zero.

The tanh head this replaced was aimed at a log-return target of order 1e-4,
which sits so deep in tanh's linear region that the head learned almost nothing
per sample while weight decay pulled it steadily toward zero. A head at zero
forecasts zero, which downstream reads as a confident no-edge rather than as an
unlearned one, so the failure was silent.
*/
func TestSimulatedTaskHeadLearns(t *testing.T) {
	Convey("Given a long run through directional regimes", t, func() {
		script := []struct {
			state testtypes.MarketState
			ticks int
		}{
			{testtypes.Baseline, 300},
			{testtypes.SlowPump, 200},
			{testtypes.FastPump, 150},
			{testtypes.SlowDump, 200},
		}

		var traces []resonanceTrace

		tests.WithMarket(t, simulationSymbols(), func(market *tests.Market) {
			traces = simulate(market, script)
		})()

		Convey("Then the forecast carries signal on the scale of a real return", func() {
			/*
				A count of non-zero entries is not evidence of learning: one
				entry anywhere in the run at the last bit of a float would
				satisfy it while the head had learned nothing. What has to hold
				is that most published entries carry a forecast, and that the
				magnitudes reach the scale a per-tick return actually moves on.

				A tenth of a basis point is the floor for meaningful here. The
				decayed head this replaced sat orders of magnitude below it,
				pulled toward zero by weight decay faster than it could learn.
			*/
			const meaningfulReturn = 1e-5

			entries := 0
			meaningful := 0
			largest := 0.0

			for _, trace := range traces {
				for _, value := range trace.curve {
					entries++

					if math.Abs(value) >= meaningfulReturn {
						meaningful++
					}

					largest = math.Max(largest, math.Abs(value))
				}
			}

			So(entries, ShouldBeGreaterThan, 0)

			meaningfulShare := float64(meaningful) / float64(entries)

			t.Logf("forecast entries %d, meaningful share %.1f%%, largest %.3e",
				entries, meaningfulShare*100, largest)

			So(meaningfulShare, ShouldBeGreaterThan, 0.5)
			So(largest, ShouldBeGreaterThan, meaningfulReturn)
		})

		Convey("Then the forecast stays on the scale of a log return", func() {
			/*
				A linear head cannot saturate, which is the point, but it also
				means nothing bounds a diverging head except the learning
				dynamics. A per-tick log return far outside a few percent is not
				a forecast the market could produce, so this is the guard that
				the head is fitting rather than diverging.
			*/
			for _, trace := range traces {
				for _, value := range trace.curve {
					So(math.Abs(value), ShouldBeLessThan, 0.5)
				}
			}
		})
	})
}
