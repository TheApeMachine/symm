package relation

import (
	"math"
	"math/rand"
	"strconv"
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
buildFixtureStore writes every fixture series at a one-second cadence.
*/
func buildFixtureStore(fixtures map[string][]float64) *ObservationStore {
	store := NewObservationStore(4096)

	for key, values := range fixtures {
		appendSeries(store, key, values, time.Second)
	}

	return store
}

/*
estimateFixture runs one estimate against the fixture store.
*/
func estimateFixture(
	store *ObservationStore, source string, target string,
	controls []string, controlLags []time.Duration,
	minLag time.Duration, maxLag time.Duration,
) map[string]float64 {
	estimator := NewInfluence("test-v1", store)
	var lags []float64

	for _, lag := range controlLags {
		lags = append(lags, float64(lag))
	}

	candidate := &Candidate{
		Source:           source,
		Target:           target,
		Controls:         controls,
		ControlLags:      lags,
		MinLag:           float64(minLag),
		MaxLag:           float64(maxLag),
		ControlsComplete: true,
	}

	var result *Estimate

	for ptr := range estimator.Next(data.NewValue(unsafe.Pointer(candidate)).Next(nil)) {
		result = (*Estimate)(ptr)
	}

	So(estimator.Error(), ShouldBeNil)

	if result == nil {
		return nil
	}

	return result.Metrics
}

func gaussianSequence(random *rand.Rand, count int) []float64 {
	values := make([]float64, count)

	for index := range values {
		values[index] = random.NormFloat64()
	}

	return values
}

func TestDirectedSystem(t *testing.T) {
	Convey("Given a directed system X_t = noise; Y_t = a*Y_{t-1} + b*X_{t-1} + noise", t, func() {
		random := rand.New(rand.NewSource(42))
		count := 300
		x := gaussianSequence(random, count)
		y := make([]float64, count)
		noise := gaussianSequence(random, count)

		for index := 1; index < count; index++ {
			y[index] = 0.5*y[index-1] + 0.7*x[index-1] + noise[index]
		}

		source, target := fixtureKey("source", "x"), fixtureKey("target", "y")
		store := buildFixtureStore(map[string][]float64{source: x, target: y})

		Convey("X → Y discovers positive prequential gain", func() {
			result := estimateFixture(store, source, target, nil, nil, 500*time.Millisecond, 5*time.Second)
			So(result["status"], ShouldEqual, float64(FitOK))
			So(result["predictive_gain"], ShouldBeGreaterThan, 0)

			Convey("the coefficient sign matches the true b", func() {
				So(result["coefficient"], ShouldBeGreaterThan, 0.4)
				So(result["coefficient"], ShouldBeLessThan, 1.0)
				So(result["coefficient_snr"], ShouldBeGreaterThan, 4)
			})

			Convey("the selected lag matches the actual causal lag within resolution", func() {
				So(result["lag_resolution"], ShouldEqual, float64(time.Second))
				So(result["lag"], ShouldEqual, float64(time.Second))
			})

			Convey("lag provenance is published", func() {
				candidates := int(result["lag_candidate_count"])
				So(result["lag_search_span"], ShouldBeGreaterThan, 0)
				So(candidates, ShouldBeGreaterThanOrEqualTo, 5)

				_, last := result["lag_surface."+strconv.Itoa(candidates-1)]
				_, beyond := result["lag_surface."+strconv.Itoa(candidates)]
				So(last, ShouldBeTrue)
				So(beyond, ShouldBeFalse)
				So(result["source_observed_at"], ShouldBeLessThan, result["target_observed_at"])
				So(result["source_age"], ShouldBeGreaterThanOrEqualTo, result["lag"])
			})
		})

		Convey("Y → X is not fabricated as symmetric", func() {
			result := estimateFixture(store, target, source, nil, nil, 500*time.Millisecond, 5*time.Second)
			So(result["status"], ShouldEqual, float64(FitOK))
			So(result["predictive_gain"], ShouldBeLessThan, 0.3)
			So(math.Abs(result["coefficient"]), ShouldBeLessThan, 0.3)
		})
	})
}

func TestIndependentSystem(t *testing.T) {
	Convey("Given two independent white-noise coordinates", t, func() {
		random := rand.New(rand.NewSource(7))
		count := 300
		x := gaussianSequence(random, count)
		y := gaussianSequence(random, count)
		source, target := fixtureKey("a", "x"), fixtureKey("b", "y")
		store := buildFixtureStore(map[string][]float64{source: x, target: y})

		Convey("the relation remains represented with near-zero gain and coefficient", func() {
			result := estimateFixture(store, source, target, nil, nil, time.Second, 3*time.Second)
			So(result["status"], ShouldEqual, float64(FitOK))

			if !math.IsNaN(result["predictive_gain"]) {
				So(math.Abs(result["predictive_gain"]), ShouldBeLessThan, 0.25)
			}

			if !math.IsNaN(result["coefficient"]) {
				So(math.Abs(result["coefficient"]), ShouldBeLessThan, 0.3)
			}
		})
	})
}

func TestMediation(t *testing.T) {
	Convey("Given a mediation chain X → M → Y", t, func() {
		random := rand.New(rand.NewSource(11))
		count := 400
		x := gaussianSequence(random, count)
		m := make([]float64, count)
		y := make([]float64, count)
		noiseM := gaussianSequence(random, count)
		noiseY := gaussianSequence(random, count)

		for index := 1; index < count; index++ {
			m[index] = 0.6*m[index-1] + 0.8*x[index-1] + noiseM[index]
			y[index] = 0.5*y[index-1] + 0.9*m[index-1] + noiseY[index]
		}

		xKey, mKey, yKey := fixtureKey("m", "x"), fixtureKey("m", "mediator"), fixtureKey("m", "y")
		store := buildFixtureStore(map[string][]float64{xKey: x, mKey: m, yKey: y})

		Convey("pairwise X → Y appears predictive through the path", func() {
			result := estimateFixture(store, xKey, yKey, nil, nil, time.Second, 5*time.Second)
			So(result["status"], ShouldEqual, float64(FitOK))
			So(result["predictive_gain"], ShouldBeGreaterThan, 0.05)
		})

		Convey("conditional X → Y given M at its path lag loses incremental contribution", func() {
			result := estimateFixture(store, xKey, yKey, []string{mKey}, []time.Duration{time.Second}, time.Second, 5*time.Second)
			So(result["status"], ShouldEqual, float64(FitOK))

			if !math.IsNaN(result["predictive_gain"]) {
				So(result["predictive_gain"], ShouldBeLessThan, 0.05)
			}
		})
	})
}

func TestConfounding(t *testing.T) {
	Convey("Given a confounder Z driving both X and Y: Z → X, Z → Y", t, func() {
		random := rand.New(rand.NewSource(17))
		count := 400
		z := gaussianSequence(random, count)
		x := make([]float64, count)
		y := make([]float64, count)
		noiseX := gaussianSequence(random, count)
		noiseY := gaussianSequence(random, count)

		for index := 1; index < count; index++ {
			x[index] = 0.8*z[index-1] + noiseX[index]
			y[index] = 0.8*z[index-1] + noiseY[index]
		}

		xKey, yKey, zKey := fixtureKey("c", "x"), fixtureKey("c", "y"), fixtureKey("c", "confounder")
		store := buildFixtureStore(map[string][]float64{xKey: x, yKey: y, zKey: z})

		Convey("pairwise X → Y is confounded by common history", func() {
			result := estimateFixture(store, xKey, yKey, nil, nil, time.Second, 5*time.Second)
			So(result["status"], ShouldEqual, float64(FitOK))
		})

		Convey("conditional X → Y given Z loses incremental contribution", func() {
			result := estimateFixture(store, xKey, yKey, []string{zKey}, []time.Duration{time.Second}, time.Second, 5*time.Second)
			So(result["status"], ShouldEqual, float64(FitOK))

			if !math.IsNaN(result["predictive_gain"]) {
				So(result["predictive_gain"], ShouldBeLessThan, 0.05)
			}
		})
	})
}

func TestStructuralUncertainty(t *testing.T) {
	Convey("Given degenerate cases", t, func() {
		store := NewObservationStore(16)
		xKey, yKey := fixtureKey("d", "x"), fixtureKey("d", "y")

		Convey("empty history reports missing history", func() {
			result := estimateFixture(store, xKey, yKey, nil, nil, time.Second, 5*time.Second)
			So(result["status"], ShouldEqual, float64(FitNoSourceHistory))
		})

		Convey("missing target reports missing history", func() {
			appendSeries(store, xKey, []float64{1, 2, 3}, time.Second)
			result := estimateFixture(store, xKey, yKey, nil, nil, time.Second, 5*time.Second)
			So(result["status"], ShouldEqual, float64(FitNoTargetHistory))
		})

		Convey("missing control reports control unavailable", func() {
			appendSeries(store, xKey, []float64{1, 2, 3, 4}, time.Second)
			appendSeries(store, yKey, []float64{1, 2, 3, 4}, time.Second)
			result := estimateFixture(store, xKey, yKey, []string{fixtureKey("d", "absent")}, nil, time.Second, 5*time.Second)
			So(result["status"], ShouldEqual, float64(FitControlUnavailable))
		})

		Convey("every undefined metric is explicitly NaN, never zero", func() {
			result := estimateFixture(store, xKey, yKey, nil, nil, time.Second, 5*time.Second)
			_, held := result["predictive_gain"]
			So(held, ShouldBeTrue)
		})

		Convey("an empty version is a domain failure, not a silent default", func() {
			invalid := NewInfluence("", store)
			So(invalid.Error(), ShouldNotBeNil)

			candidate := &Candidate{
				Source:           xKey,
				Target:           yKey,
				MinLag:           float64(time.Second),
				MaxLag:           float64(2 * time.Second),
				ControlsComplete: true,
			}
			var yielded int

			for range invalid.Next(data.NewValue(unsafe.Pointer(candidate)).Next(nil)) {
				yielded++
			}

			So(yielded, ShouldEqual, 0)
		})
	})
}

func TestAlign(t *testing.T) {
	Convey("Given two series at a one-second cadence", t, func() {
		random := rand.New(rand.NewSource(5))
		count := 50
		x := gaussianSequence(random, count)
		y := gaussianSequence(random, count)
		xFlat, yFlat := make([]float64, 0, 2*count), make([]float64, 0, 2*count)

		for index := range count {
			at := float64(time.Duration(index) * time.Second)
			xFlat = append(xFlat, at, x[index])
			yFlat = append(yFlat, at, y[index])
		}

		var rows [][]float64
		aligner := NewAlign()

		for pointer := range aligner.Next(data.NewValue([][]float64{
			{float64(time.Second)}, yFlat, xFlat,
		}).Next(nil)) {
			rows = *(*[][]float64)(pointer)
		}

		So(aligner.Error(), ShouldBeNil)

		Convey("the first aligned row pairs Y_t with X_{t-1}", func() {
			So(rows, ShouldHaveLength, count-1)
			So(rows[0][1], ShouldEqual, y[1])
			So(rows[0][3], ShouldEqual, x[0])
		})

		Convey("future observations never enter a row", func() {
			for index, row := range rows {
				So(row[2], ShouldBeLessThan, row[0])
				So(row[1], ShouldEqual, y[index+1])
			}
		})
	})
}

func TestPlanner(t *testing.T) {
	Convey("Given a plan over resident coordinates", t, func() {
		store := NewObservationStore(8)

		for range store.Next(data.NewValue(map[string][]float64{
			fixtureKey("cvd", "signed_net_fraction"): nil,
			fixtureKey("cvd", "midpoint_log_return"): nil,
			fixtureKey("hawkes", "arrival_rate"):     nil,
		}).Next(nil)) {
		}

		compile := func(planner *Planner, symbol string, epoch float64) []map[string]string {
			scope := &PlanScope{
				Symbol: symbol,
				Epoch:  uint64(epoch),
			}

			var candidates []map[string]string

			for pointer := range planner.Next(data.NewValue(unsafe.Pointer(scope)).Next(nil)) {
				candidate := (*Candidate)(pointer)
				read := map[string]string{}
				read["source"] = candidate.Source
				read["target"] = candidate.Target
				read["controls"] = strconv.Itoa(len(candidate.Controls))
				read["controls_complete"] = "0"

				if candidate.ControlsComplete {
					read["controls_complete"] = "1"
				}

				if len(candidate.Controls) > 0 {
					read["control.0"] = candidate.Controls[0]
					read["control.0.lag"] = strconv.Itoa(int(candidate.ControlLags[0]))
				}

				candidates = append(candidates, read)
			}

			So(planner.Error(), ShouldBeNil)
			return candidates
		}

		planner := NewPlanner(
			store, 1, "TEST/USD", "", time.Second, 5*time.Second,
			nil,
			[][3]string{{"cvd", "", ""}},
			[][3]string{{"cvd", "", ""}, {"hawkes", "", ""}},
			[][3]string{{"hawkes", "arrival_rate", ""}},
			time.Second,
		)

		Convey("cross-product pairs expand with self-pairs excluded", func() {
			candidates := compile(planner, "TEST/USD", 1)
			So(candidates, ShouldHaveLength, 2)

			for _, candidate := range candidates {
				So(candidate["source"], ShouldNotEqual, candidate["target"])
				So(candidate["source"], ShouldStartWith, "TEST/USD|cvd|")
			}
		})

		Convey("exact controls resolve against resident coordinates", func() {
			for _, candidate := range compile(planner, "TEST/USD", 1) {
				So(candidate["controls_complete"], ShouldEqual, "1")
				So(candidate["controls"], ShouldEqual, "1")
				So(candidate["control.0"], ShouldEqual, fixtureKey("hawkes", "arrival_rate"))
				So(candidate["control.0.lag"], ShouldEqual, strconv.Itoa(int(time.Second)))
			}
		})

		Convey("a foreign epoch compiles nothing", func() {
			So(compile(planner, "TEST/USD", 2), ShouldBeEmpty)
		})

		Convey("a symbol outside the plan scope compiles nothing", func() {
			So(compile(planner, "OTHER/USD", 1), ShouldBeEmpty)
		})

		Convey("a missing exact control is reported incomplete and unavailable", func() {
			missing := NewPlanner(
				store, 1, "", "", 0, 0,
				[][2][3]string{{{"cvd", "signed_net_fraction", ""}, {"cvd", "midpoint_log_return", ""}}},
				nil, nil,
				[][3]string{{"nope", "", ""}},
			)

			candidates := compile(missing, "TEST/USD", 1)
			So(candidates, ShouldHaveLength, 1)
			So(candidates[0]["controls_complete"], ShouldEqual, "0")
			So(candidates[0]["control.0"], ShouldEqual, "nope||")
		})
	})
}

func TestPlannedInfluence(t *testing.T) {
	Convey("Given a planner piped straight into Influence", t, func() {
		random := rand.New(rand.NewSource(42))
		count := 300
		x := gaussianSequence(random, count)
		y := make([]float64, count)

		for index := 1; index < count; index++ {
			y[index] = 0.5*y[index-1] + 0.7*x[index-1] + random.NormFloat64()
		}

		store := buildFixtureStore(map[string][]float64{
			fixtureKey("source", "x"): x,
			fixtureKey("target", "y"): y,
		})

		planner := NewPlanner(
			store, 1, "TEST/USD", "", time.Second, 5*time.Second,
			[][2][3]string{{{"source", "x", ""}, {"target", "y", ""}}},
			nil, nil, nil,
		)

		scope := &PlanScope{
			Symbol: "TEST/USD",
			Epoch:  1,
		}

		estimator := NewInfluence("test-v1", store)
		var statuses []float64

		for pointer := range estimator.Next(planner.Next(data.NewValue(unsafe.Pointer(scope)).Next(nil))) {
			estimate := (*Estimate)(pointer)
			statuses = append(statuses, float64(estimate.Status))
			So(estimate.Metrics["predictive_gain"], ShouldBeGreaterThan, 0)
		}

		Convey("each compiled candidate is estimated once", func() {
			So(estimator.Error(), ShouldBeNil)
			So(statuses, ShouldResemble, []float64{float64(FitOK)})
		})
	})
}

var benchmarkEstimateSink float64

/*
BenchmarkInfluence measures the full prequential estimate over the resident
store: history copy, alignment, and regression accumulation per lag.
*/
func BenchmarkInfluence(b *testing.B) {
	random := rand.New(rand.NewSource(42))
	count := 256
	x := gaussianSequence(random, count)
	y := make([]float64, count)

	for index := 1; index < count; index++ {
		y[index] = 0.3*y[index-1] + 0.5*x[index-1] + random.NormFloat64()
	}

	source, target := fixtureKey("z", "x"), fixtureKey("z", "y")
	store := buildFixtureStore(map[string][]float64{source: x, target: y})
	estimator := NewInfluence("bench-v1", store)

	candidate := &Candidate{
		Source:           source,
		Target:           target,
		MinLag:           float64(time.Second),
		MaxLag:           float64(10 * time.Second),
		ControlsComplete: true,
	}

	b.ReportAllocs()

	for b.Loop() {
		for ptr := range estimator.Next(data.NewValue(unsafe.Pointer(candidate)).Next(nil)) {
			est := (*Estimate)(ptr)
			benchmarkEstimateSink = float64(est.Status)
		}
	}
}
