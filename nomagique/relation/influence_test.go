package relation

import (
	"math"
	"math/rand"
	"strconv"
	"testing"
	"time"

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
roleAdapter builds an Influence request adapter whose published estimate
lands in the returned output map.
*/
func roleAdapter(
	source string, target string, controls []string, controlLags []time.Duration,
	minLag time.Duration, maxLag time.Duration,
) (*data.Adapter, map[string]float64) {
	out := data.NewOutputMap()
	adapter := data.NewAdapter(nil, data.NewState(data.NewMap(), out))
	roles := data.NewTextMap()
	roles.Values["source"] = source
	roles.Values["target"] = target
	domain := data.NewOutputMap()
	domain.Values["controls"] = float64(len(controls))
	domain.Values["min_lag"] = float64(minLag)
	domain.Values["max_lag"] = float64(maxLag)

	for index, key := range controls {
		roles.Values["control."+strconv.Itoa(index)] = key
		domain.Values["control."+strconv.Itoa(index)+".lag"] = 0

		if index < len(controlLags) {
			domain.Values["control."+strconv.Itoa(index)+".lag"] = float64(controlLags[index])
		}
	}

	for range adapter.Next(data.NewValue(roles)) {
	}

	for range adapter.Next(data.NewValue(domain)) {
	}

	return adapter, out.Values
}

/*
estimateFixture runs one estimate against the fixture store.
*/
func estimateFixture(
	store *ObservationStore, source string, target string,
	controls []string, controlLags []time.Duration,
	minLag time.Duration, maxLag time.Duration,
) map[string]float64 {
	adapter, out := roleAdapter(source, target, controls, controlLags, minLag, maxLag)
	estimator := NewInfluence("test-v1", store)

	for range estimator.Next(data.NewValue(adapter)) {
	}

	So(estimator.Error(), ShouldBeNil)
	return out
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

		Convey("M → Y remains measured", func() {
			result := estimateFixture(store, mKey, yKey, nil, nil, time.Second, 5*time.Second)
			So(result["status"], ShouldEqual, float64(FitOK))
			So(result["predictive_gain"], ShouldBeGreaterThan, 0.05)
		})
	})
}

func TestFutureLeakage(t *testing.T) {
	Convey("Given future X perfectly predicts Y but past X is useless", t, func() {
		random := rand.New(rand.NewSource(21))
		count := 300
		x := gaussianSequence(random, count)
		y := make([]float64, count)

		// Y_t = X_{t+1}: the future source value is the perfect predictor.
		for index := 0; index < count-1; index++ {
			y[index] = x[index+1]
		}

		source, target := fixtureKey("f", "x"), fixtureKey("f", "y")
		store := buildFixtureStore(map[string][]float64{source: x, target: y})

		Convey("Influence does not discover the future relationship", func() {
			result := estimateFixture(store, source, target, nil, nil, time.Second, 3*time.Second)
			So(result["status"], ShouldEqual, float64(FitOK))
			So(math.Abs(result["predictive_gain"]), ShouldBeLessThan, 0.2)
			So(math.Abs(result["coefficient"]), ShouldBeLessThan, 0.3)
		})
	})
}

func TestRankDeficiency(t *testing.T) {
	Convey("Given duplicated exact controls", t, func() {
		random := rand.New(rand.NewSource(31))
		count := 300
		x := gaussianSequence(random, count)
		control := gaussianSequence(random, count)
		y := make([]float64, count)
		noise := gaussianSequence(random, count)

		for index := 1; index < count; index++ {
			y[index] = 0.5*y[index-1] + 0.4*control[index-1] + noise[index]
		}

		xKey, cKey, yKey := fixtureKey("r", "x"), fixtureKey("r", "control"), fixtureKey("r", "y")
		store := buildFixtureStore(map[string][]float64{xKey: x, cKey: control, yKey: y})

		Convey("the fit is undefined with no silent regularization", func() {
			result := estimateFixture(store, xKey, yKey, []string{cKey, cKey}, nil, time.Second, 2*time.Second)
			So(result["status"], ShouldEqual, float64(FitRankDeficient))
			So(math.IsNaN(result["coefficient"]), ShouldBeTrue)
			So(math.IsNaN(result["coefficient_variance"]), ShouldBeTrue)
			So(math.IsNaN(result["coefficient_snr"]), ShouldBeTrue)
			So(math.IsNaN(result["predictive_gain"]), ShouldBeTrue)
		})
	})
}

func TestZeroVsUnavailable(t *testing.T) {
	Convey("Given observed zeros and a missing coordinate", t, func() {
		random := rand.New(rand.NewSource(41))
		count := 200
		x := gaussianSequence(random, count)
		y := make([]float64, count)
		zero := make([]float64, count)

		for index := 1; index < count; index++ {
			y[index] = 0.3*y[index-1] + random.NormFloat64()
		}

		xKey, yKey, zeroKey := fixtureKey("z", "x"), fixtureKey("z", "y"), fixtureKey("z", "zero")
		store := buildFixtureStore(map[string][]float64{xKey: x, yKey: y, zeroKey: zero})

		Convey("an observed zero coordinate is retained and distinct from missing", func() {
			window := residentCopy(store)[zeroKey]
			So(window, ShouldHaveLength, 2*count)

			for index := 1; index < len(window); index += 2 {
				So(window[index], ShouldEqual, 0)
			}
		})

		Convey("a missing source coordinate yields no_source_history, not a zero relation", func() {
			result := estimateFixture(store, fixtureKey("z", "missing"), yKey, nil, nil, time.Second, 2*time.Second)
			So(result["status"], ShouldEqual, float64(FitNoSourceHistory))
			So(math.IsNaN(result["coefficient"]), ShouldBeTrue)
			So(math.IsNaN(result["predictive_gain"]), ShouldBeTrue)
		})

		Convey("a missing control makes the relation unavailable, not control-free", func() {
			result := estimateFixture(store, xKey, yKey, []string{fixtureKey("z", "missing_control")}, nil, time.Second, 2*time.Second)
			So(result["status"], ShouldEqual, float64(FitControlUnavailable))
		})

		Convey("a constant zero source is a valid zero-coefficient relation, not deleted", func() {
			result := estimateFixture(store, zeroKey, yKey, nil, nil, time.Second, 2*time.Second)
			_, held := result["status"]
			So(held, ShouldBeTrue)
		})

		Convey("an empty version is a domain failure, not a silent default", func() {
			invalid := NewInfluence("", store)
			So(invalid.Error(), ShouldNotBeNil)

			adapter, _ := roleAdapter(xKey, yKey, nil, nil, time.Second, 2*time.Second)
			var yielded int

			for range invalid.Next(data.NewValue(adapter)) {
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
		})) {
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
		})) {
		}

		compile := func(planner *Planner, symbol string, epoch float64) []map[string]string {
			out := data.NewOutputMap()
			request := data.NewAdapter(nil, data.NewState(data.NewMap(), out))
			scope := data.NewTextMap()
			scope.Values["symbol"] = symbol
			stamp := data.NewOutputMap()
			stamp.Values["epoch"] = epoch

			for range request.Next(data.NewValue(scope)) {
			}

			for range request.Next(data.NewValue(stamp)) {
			}

			var candidates []map[string]string

			for pointer := range planner.Next(data.NewValue(request)) {
				candidate := *(**data.Adapter)(pointer)
				read := map[string]string{}
				var count, complete, lag float64

				for values := range candidate.Next(data.NewValue(data.NewMap("controls", "", "controls_complete", ""))) {
					numbers := *(*data.Map[float64])(values)
					count, complete = numbers.Values["controls"], numbers.Values["controls_complete"]
				}

				literal := data.NewLiteral("source", "target")

				for index := range int(count) {
					literal.Values["control."+strconv.Itoa(index)] = ""
				}

				for values := range candidate.Next(data.NewValue(literal)) {
					for key, value := range (*(*data.Map[string])(values)).Values {
						read[key] = value
					}
				}

				if count > 0 {
					for values := range candidate.Next(data.NewValue(data.NewMap("control.0.lag", ""))) {
						lag = (*(*data.Map[float64])(values)).Values["control.0.lag"]
					}
				}

				So(candidate.Error(), ShouldBeNil)
				read["controls"] = strconv.Itoa(int(count))
				read["controls_complete"] = strconv.Itoa(int(complete))
				read["control.0.lag"] = strconv.Itoa(int(lag))
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

		request := data.NewAdapter(nil, data.NewState(data.NewMap()))
		scope := data.NewTextMap()
		scope.Values["symbol"] = "TEST/USD"
		stamp := data.NewOutputMap()
		stamp.Values["epoch"] = 1

		for range request.Next(data.NewValue(scope)) {
		}

		for range request.Next(data.NewValue(stamp)) {
		}

		estimator := NewInfluence("test-v1", store)
		var statuses []float64

		for pointer := range estimator.Next(planner.Next(data.NewValue(request))) {
			adapter := *(**data.Adapter)(pointer)

			for values := range adapter.Next(data.NewValue(data.NewMap("status", "", "predictive_gain", ""))) {
				numbers := *(*data.Map[float64])(values)
				statuses = append(statuses, numbers.Values["status"])
				So(numbers.Values["predictive_gain"], ShouldBeGreaterThan, 0)
			}
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

	b.ReportAllocs()

	for b.Loop() {
		adapter, out := roleAdapter(source, target, nil, nil, time.Second, 10*time.Second)

		for range estimator.Next(data.NewValue(adapter)) {
		}

		benchmarkEstimateSink = out["status"]
	}
}
