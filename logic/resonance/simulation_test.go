package resonance_test

import (
	"math"
	"slices"
	"sync"
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
	simulationRestAlpha             = 0.03
	simulationMinAlpha              = 0.005
	simulationMaxAlpha              = 0.150
	simulationHorizonRetentionFloor = 1.0 / 3.0

	/*
		Mirrors maxHorizon on the solver, which is likewise unexported.
	*/
	simulationMaxHorizon = 20
)

/*
resonanceTrace is what the predictive coding stage published on one analyzer pass.
*/
type resonanceTrace struct {
	alpha      float64
	confidence float64
	surprise   float64
	energy     float64
	samples    uint64
	horizon    int
	curve      []float64
	retention  []float64
}

type simulationPhase struct {
	state  testtypes.MarketState
	passes int
}

type resonanceEvaluator interface {
	Update(thesis *types.Thesis) *types.Thesis
}

/*
resonanceRecorder snapshots the analyzed row on the analyzer goroutine before
the planner spends and resets that thesis, then delegates to the real planner.
The analyzer's ordinary subscription cannot provide this guarantee because it
publishes the same mutable thesis pointer that the planner resets immediately.
*/
type resonanceRecorder struct {
	mu        sync.Mutex
	symbol    string
	evaluator resonanceEvaluator
	traces    []resonanceTrace
}

func (recorder *resonanceRecorder) Update(thesis *types.Thesis) *types.Thesis {
	if trace, ok := readResonance(thesis, recorder.symbol); ok {
		recorder.mu.Lock()
		recorder.traces = append(recorder.traces, trace)
		recorder.mu.Unlock()
	}

	return recorder.evaluator.Update(thesis)
}

func (recorder *resonanceRecorder) Traces() []resonanceTrace {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()

	return slices.Clone(recorder.traces)
}

func (recorder *resonanceRecorder) Count() int {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()

	return len(recorder.traces)
}

/*
readResonance lifts the stage's published readings off the thesis.

ok is false while the stage is still in warmup, which is the ordinary state
until the feature schema has settled and the task head has a supervised sample.
*/
func readResonance(thesis *types.Thesis, symbol string) (resonanceTrace, bool) {
	if thesis == nil || thesis.Resonance == nil {
		return resonanceTrace{}, false
	}

	rowRaw, found := thesis.Resonance.Load(symbol)

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

	if raw, found := row["samples"]; found {
		if samples, ok := raw.(uint64); ok {
			trace.samples = samples
		}
	}

	if raw, found := row["activeHorizon"]; found {
		if horizon, ok := raw.(int); ok {
			trace.horizon = horizon
		}
	}

	if raw, found := row["forwardCurve"]; found {
		if curve, ok := raw.([]float64); ok {
			trace.curve = slices.Clone(curve)
		}
	}

	if raw, found := row["forwardRetention"]; found {
		if retention, ok := raw.([]float64); ok {
			trace.retention = slices.Clone(retention)
		}
	}

	return trace, true
}

/*
simulate runs the full production ensemble across a scripted sequence of market
regimes and returns every reading the predictive coding stage published.
*/
func simulate(
	market *tests.Market,
	script []simulationPhase,
) []resonanceTrace {
	recorder := &resonanceRecorder{
		symbol:    market.Symbols[0].Pair,
		evaluator: market.Planner,
		traces:    make([]resonanceTrace, 0, 4096),
	}
	market.Analyzer.AttachEvaluator(recorder)

	for _, phase := range script {
		for _, symbol := range market.Symbols {
			So(market.Transition(symbol.Pair, phase.state), ShouldBeNil)
		}

		phaseStart := recorder.Count()

		for recorder.Count()-phaseStart < phase.passes {
			market.Tick()
		}
	}

	return recorder.Traces()
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

func resonanceInput(market *tests.Market) *types.Thesis {
	thesis := types.NewThesis()

	for source, measurements := range market.Measurements() {
		thesis.Measurements.Store(types.SourceType(source), measurements)
	}

	market.Thesis.Tickers.Range(func(symbol, ticker any) bool {
		thesis.Tickers.Store(symbol, ticker)
		return true
	})

	return thesis
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
			which needs errorCalibratorWindow analyzer passes before it reports
			a rank at all. The regime stretches are sized to outlast that window
			so a sustained condition is visible as sustained rather than as a
			transient inside it.
		*/
		const closingBaselinePasses = 100

		script := []simulationPhase{
			{testtypes.Baseline, 300},
			{testtypes.SlowPump, 80},
			{testtypes.FastPump, 80},
			{testtypes.SidewaysChop, 80},
			{testtypes.FastDump, 80},
			{testtypes.FlashCrash, 60},
			{testtypes.Baseline, closingBaselinePasses},
		}

		var traces []resonanceTrace

		tests.WithMarket(t, simulationSymbols(), func(market *tests.Market) {
			traces = simulate(market, script)
		})()

		expectedPasses := 0

		for _, phase := range script {
			expectedPasses += phase.passes
		}

		So(len(traces), ShouldBeGreaterThanOrEqualTo, expectedPasses)

		/*
			The previous controller reached its ceiling within roughly forty
			analyzer passes of any spiky stream and stayed there for the rest of the
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

		/*
			A controller with a fixed point relaxes once the market quiets.
			A ratchet does not: whatever the crash did to the pace would
			still be there at the end of the closing baseline.
		*/
		crashPeak := 0.0

		for _, trace := range traces[:len(traces)-closingBaselinePasses] {
			crashPeak = math.Max(crashPeak, trace.alpha)
		}

		settled := traces[len(traces)-1].alpha
		peakDistance := math.Abs(crashPeak - simulationRestAlpha)
		settledDistance := math.Abs(settled - simulationRestAlpha)

		t.Logf("peak alpha %.5f, settled alpha %.5f, distance to rest %.5f -> %.5f",
			crashPeak, settled, peakDistance, settledDistance)

		So(settledDistance, ShouldBeLessThan, peakDistance)
		So(settled, ShouldBeBetween, simulationRestAlpha/2, simulationRestAlpha*2)

		minimum := math.Inf(1)
		maximum := math.Inf(-1)
		total := 0.0
		var buckets [4]int

		for _, trace := range traces {
			minimum = math.Min(minimum, trace.confidence)
			maximum = math.Max(maximum, trace.confidence)
			total += trace.confidence

			bucket := min(3, int(trace.confidence*4))
			buckets[bucket]++
		}

		mean := total / float64(len(traces))

		t.Logf("confidence min %.4f mean %.4f max %.4f, quartile counts %v",
			minimum, mean, maximum, buckets)

		So(minimum, ShouldBeGreaterThanOrEqualTo, 0)
		So(maximum, ShouldBeLessThanOrEqualTo, 1)

		for _, count := range buckets {
			So(count, ShouldBeGreaterThan, 0)
		}

		So(mean, ShouldBeBetween, 0.25, 0.75)

		withCurve := 0

		for _, trace := range traces {
			So(math.IsNaN(trace.surprise), ShouldBeFalse)
			So(math.IsInf(trace.surprise, 0), ShouldBeFalse)
			So(math.IsNaN(trace.energy), ShouldBeFalse)
			So(math.IsInf(trace.energy, 0), ShouldBeFalse)
			So(math.IsNaN(trace.confidence), ShouldBeFalse)
			So(math.IsNaN(trace.alpha), ShouldBeFalse)
			So(trace.horizon, ShouldBeGreaterThanOrEqualTo, 1)
			So(trace.horizon, ShouldBeLessThanOrEqualTo, simulationMaxHorizon)

			if len(trace.curve) == 0 {
				continue
			}

			withCurve++
			So(len(trace.retention), ShouldEqual, len(trace.curve))

			for _, value := range trace.curve {
				So(math.IsNaN(value), ShouldBeFalse)
				So(math.IsInf(value, 0), ShouldBeFalse)
			}

			for _, surviving := range trace.retention {
				So(surviving, ShouldBeGreaterThanOrEqualTo, 0)
				So(math.IsNaN(surviving), ShouldBeFalse)
			}
		}

		t.Logf("analyzer passes publishing a forward curve: %d of %d",
			withCurve, len(traces))
		So(withCurve, ShouldBeGreaterThan, 0)
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
		traces := make([]resonanceTrace, 0, 512)

		tests.WithMarket(t, simulationSymbols(), func(market *tests.Market) {
			for range 500 {
				market.Tick()
				thesis := resonanceInput(market)

				if err := solver.Update(thesis); err != nil {
					continue
				}

				if trace, ok := readResonance(thesis, market.Symbols[0].Pair); ok {
					traces = append(traces, trace)
				}
			}
		})()

		So(len(traces), ShouldBeGreaterThan, 100)

		first := traces[0].horizon
		widest := 0
		maximumConfidence := 0.0
		widestRetention := []float64(nil)

		for _, trace := range traces {
			maximumConfidence = math.Max(maximumConfidence, trace.confidence)

			if trace.horizon > widest {
				widest = trace.horizon
				widestRetention = trace.retention
			}
		}

		settled := traces[len(traces)-1].horizon

		t.Logf(
			"horizon first %d, widest %d, settled %d, confidence max %.4f, retention %v, over %d passes",
			first,
			widest,
			settled,
			maximumConfidence,
			widestRetention,
			len(traces),
		)

		So(first, ShouldEqual, 1)
		So(widest, ShouldBeGreaterThan, first)
		So(widest, ShouldBeLessThanOrEqualTo, simulationMaxHorizon)
		So(len(widestRetention), ShouldEqual, widest)
		So(widestRetention[0], ShouldBeGreaterThan, 0)
		So(
			widestRetention[len(widestRetention)-1]/widestRetention[0],
			ShouldBeGreaterThanOrEqualTo,
			simulationHorizonRetentionFloor,
		)

		distinct := map[int]struct{}{}

		for _, trace := range traces {
			distinct[trace.horizon] = struct{}{}
		}

		So(len(distinct), ShouldBeGreaterThan, 1)
	})
}

/*
TestSimulatedTaskHeadLearns pins that the full pipeline supplies resolvable
supervised targets and publishes finite task-head forecasts.

Whether the head's update moves its weights correctly is deterministic learning
math and is tested in nomagique. This asynchronous market replay verifies the
integration contract: successful target samples accumulate instead of the head
remaining permanently unlearned.
*/
func TestSimulatedTaskHeadLearns(t *testing.T) {
	Convey("Given a long run through directional regimes", t, func() {
		script := []simulationPhase{
			{testtypes.Baseline, 300},
			{testtypes.SlowPump, 200},
			{testtypes.FastPump, 150},
			{testtypes.SlowDump, 200},
		}

		var traces []resonanceTrace

		tests.WithMarket(t, simulationSymbols(), func(market *tests.Market) {
			traces = simulate(market, script)
		})()

		firstSamples := uint64(0)
		lastSamples := uint64(0)
		forecasts := 0

		for _, trace := range traces {
			if trace.samples > 0 && firstSamples == 0 {
				firstSamples = trace.samples
			}

			lastSamples = max(lastSamples, trace.samples)

			for _, forecast := range trace.curve {
				forecasts++
				So(math.IsNaN(forecast), ShouldBeFalse)
				So(math.IsInf(forecast, 0), ShouldBeFalse)
				So(math.Abs(forecast), ShouldBeLessThan, 0.5)
			}
		}

		t.Logf("supervised samples %d -> %d, finite forecast entries %d",
			firstSamples, lastSamples, forecasts)

		So(firstSamples, ShouldBeGreaterThan, 0)
		So(lastSamples, ShouldBeGreaterThan, firstSamples)
		So(forecasts, ShouldBeGreaterThan, 0)
	})
}
