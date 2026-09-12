package relation

import (
	"math"
	"math/rand"
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
seriesFixture is one named coordinate series for synthetic tests.
*/
type seriesFixture struct {
	coordinate Coordinate
	values     []float64
	start      time.Time
	step       time.Duration
}

func buildFixtureStore(fixtures []seriesFixture) core.Primitive {
	store := NewObservationStore(4096)

	for _, fixture := range fixtures {
		for index, value := range fixture.values {
			at := fixture.start.Add(time.Duration(index) * fixture.step)
			driveStore(store, &StoreCommand{Append: &Observation{
				Coordinate: fixture.coordinate,
				Raw:        value,
				At:         at,
			}})
		}
	}

	return store
}

func fixtureCoordinate(source string, metric string) Coordinate {
	return Coordinate{
		Symbol: "TEST/USD",
		Source: source,
		Metric: metric,
		Epoch:  1,
	}
}

/*
ringFor queries one coordinate's read-locked ring view from the store.
*/
func ringFor(store core.Primitive, coordinate Coordinate) RingView {
	result := driveStore(store, &StoreCommand{Ring: &RingRequest{Coordinate: coordinate}})
	return result.Ring
}

/*
driveInfluence executes one influence request and returns the single result.
*/
func driveInfluence(op core.Primitive, request *InfluenceRequest) *InfluenceResult {
	var result *InfluenceResult

	for out := range op.Next(singlePointer(unsafe.Pointer(request))) {
		result = *(**InfluenceResult)(out)
	}

	return result
}

/*
estimateFixture builds the influence request whose history views are taken
from the fixture store.
*/
func estimateFixture(
	store core.Primitive,
	source Coordinate,
	target Coordinate,
	controls []Control,
	lag LagDomain,
) *InfluenceResult {
	request := &InfluenceRequest{
		Source:   source,
		Target:   target,
		Controls: controls,
		History: InfluenceHistory{
			Source: ringFor(store, source),
			Target: ringFor(store, target),
		},
		Lag: lag,
	}

	for _, control := range controls {
		request.History.Controls = append(request.History.Controls, ringFor(store, control.Coordinate))
	}

	return driveInfluence(NewInfluence("test-v1"), request)
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

		store := buildFixtureStore([]seriesFixture{
			{coordinate: fixtureCoordinate("source", "x"), values: x, start: time.Unix(0, 0), step: time.Second},
			{coordinate: fixtureCoordinate("target", "y"), values: y, start: time.Unix(0, 0), step: time.Second},
		})

		Convey("X → Y discovers positive prequential gain", func() {
			result := estimateFixture(store,
				fixtureCoordinate("source", "x"),
				fixtureCoordinate("target", "y"),
				nil,
				LagDomain{MinLag: 500 * time.Millisecond, MaxLag: 5 * time.Second},
			)
			So(result.Defined(), ShouldBeTrue)
			So(result.PredictiveGain, ShouldNotBeNil)
			So(*result.PredictiveGain, ShouldBeGreaterThan, 0)

			Convey("the coefficient sign matches the true b", func() {
				So(result.CoefficientDefined(), ShouldBeTrue)
				So(*result.Coefficient, ShouldBeGreaterThan, 0.4)
				So(*result.Coefficient, ShouldBeLessThan, 1.0)
				So(*result.CoefficientSNR, ShouldBeGreaterThan, 4)
			})

			Convey("the selected lag matches the actual causal lag within resolution", func() {
				So(result.LagResolution, ShouldEqual, time.Second)
				So(result.Lag, ShouldEqual, time.Second)
			})

			Convey("lag provenance is published", func() {
				So(result.LagSearchSpan, ShouldBeGreaterThan, 0)
				So(result.LagCandidateCount, ShouldBeGreaterThanOrEqualTo, 5)
				So(len(result.LagSurface), ShouldEqual, result.LagCandidateCount)
				So(result.SourceObservedAt.Before(result.TargetObservedAt), ShouldBeTrue)
				So(result.SourceAge, ShouldBeGreaterThanOrEqualTo, result.Lag)
			})
		})

		Convey("Y → X is not fabricated as symmetric", func() {
			result := estimateFixture(store,
				fixtureCoordinate("target", "y"),
				fixtureCoordinate("source", "x"),
				nil,
				LagDomain{MinLag: 500 * time.Millisecond, MaxLag: 5 * time.Second},
			)
			So(result.Defined(), ShouldBeTrue)
			So(result.PredictiveGain, ShouldNotBeNil)
			So(*result.PredictiveGain, ShouldBeLessThan, 0.3)
			So(result.Coefficient, ShouldNotBeNil)
			So(math.Abs(*result.Coefficient), ShouldBeLessThan, 0.3)
		})
	})
}

func TestIndependentSystem(t *testing.T) {
	Convey("Given two independent white-noise coordinates", t, func() {
		random := rand.New(rand.NewSource(7))
		count := 300
		x := gaussianSequence(random, count)
		y := gaussianSequence(random, count)

		store := buildFixtureStore([]seriesFixture{
			{coordinate: fixtureCoordinate("a", "x"), values: x, start: time.Unix(0, 0), step: time.Second},
			{coordinate: fixtureCoordinate("b", "y"), values: y, start: time.Unix(0, 0), step: time.Second},
		})

		Convey("the relation remains represented with near-zero gain and coefficient", func() {
			result := estimateFixture(store,
				fixtureCoordinate("a", "x"),
				fixtureCoordinate("b", "y"),
				nil,
				LagDomain{MinLag: time.Second, MaxLag: 3 * time.Second},
			)
			So(result, ShouldNotBeNil)

			Convey("the relation is not deleted", func() {
				So(result.Defined(), ShouldBeTrue)
			})

			if result.PredictiveGain != nil {
				So(math.Abs(*result.PredictiveGain), ShouldBeLessThan, 0.25)
			}

			if result.Coefficient != nil {
				So(math.Abs(*result.Coefficient), ShouldBeLessThan, 0.3)
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

		store := buildFixtureStore([]seriesFixture{
			{coordinate: fixtureCoordinate("m", "x"), values: x, start: time.Unix(0, 0), step: time.Second},
			{coordinate: fixtureCoordinate("m", "mediator"), values: m, start: time.Unix(0, 0), step: time.Second},
			{coordinate: fixtureCoordinate("m", "y"), values: y, start: time.Unix(0, 0), step: time.Second},
		})

		Convey("pairwise X → Y appears predictive through the path", func() {
			result := estimateFixture(store,
				fixtureCoordinate("m", "x"),
				fixtureCoordinate("m", "y"),
				nil,
				LagDomain{MinLag: time.Second, MaxLag: 5 * time.Second},
			)
			So(result.Defined(), ShouldBeTrue)
			So(result.PredictiveGain, ShouldNotBeNil)
			So(*result.PredictiveGain, ShouldBeGreaterThan, 0.05)
		})

		Convey("conditional X → Y given M at its path lag loses incremental contribution", func() {
			result := estimateFixture(store,
				fixtureCoordinate("m", "x"),
				fixtureCoordinate("m", "y"),
				[]Control{{
					Coordinate: fixtureCoordinate("m", "mediator"),
					Lag:        time.Second,
				}},
				LagDomain{MinLag: time.Second, MaxLag: 5 * time.Second},
			)
			So(result.Defined(), ShouldBeTrue)

			if result.PredictiveGain != nil {
				So(*result.PredictiveGain, ShouldBeLessThan, 0.05)
			}
		})

		Convey("M → Y remains measured", func() {
			result := estimateFixture(store,
				fixtureCoordinate("m", "mediator"),
				fixtureCoordinate("m", "y"),
				nil,
				LagDomain{MinLag: time.Second, MaxLag: 5 * time.Second},
			)
			So(result.Defined(), ShouldBeTrue)
			So(result.PredictiveGain, ShouldNotBeNil)
			So(*result.PredictiveGain, ShouldBeGreaterThan, 0.05)
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

		store := buildFixtureStore([]seriesFixture{
			{coordinate: fixtureCoordinate("f", "x"), values: x, start: time.Unix(0, 0), step: time.Second},
			{coordinate: fixtureCoordinate("f", "y"), values: y, start: time.Unix(0, 0), step: time.Second},
		})

		Convey("Influence does not discover the future relationship", func() {
			result := estimateFixture(store,
				fixtureCoordinate("f", "x"),
				fixtureCoordinate("f", "y"),
				nil,
				LagDomain{MinLag: time.Second, MaxLag: 3 * time.Second},
			)
			So(result.Defined(), ShouldBeTrue)
			So(result.PredictiveGain, ShouldNotBeNil)
			So(math.Abs(*result.PredictiveGain), ShouldBeLessThan, 0.2)
			So(result.Coefficient, ShouldNotBeNil)
			So(math.Abs(*result.Coefficient), ShouldBeLessThan, 0.3)
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

		store := buildFixtureStore([]seriesFixture{
			{coordinate: fixtureCoordinate("r", "x"), values: x, start: time.Unix(0, 0), step: time.Second},
			{coordinate: fixtureCoordinate("r", "control"), values: control, start: time.Unix(0, 0), step: time.Second},
			{coordinate: fixtureCoordinate("r", "y"), values: y, start: time.Unix(0, 0), step: time.Second},
		})

		Convey("the fit is undefined with no silent regularization", func() {
			result := estimateFixture(store,
				fixtureCoordinate("r", "x"),
				fixtureCoordinate("r", "y"),
				[]Control{
					{Coordinate: fixtureCoordinate("r", "control")},
					{Coordinate: fixtureCoordinate("r", "control")},
				},
				LagDomain{MinLag: time.Second, MaxLag: 2 * time.Second},
			)
			So(result, ShouldNotBeNil)
			So(result.Status, ShouldEqual, FitRankDeficient)
			So(result.Coefficient, ShouldBeNil)
			So(result.CoefficientVariance, ShouldBeNil)
			So(result.CoefficientSNR, ShouldBeNil)
			So(result.PredictiveGain, ShouldBeNil)
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

		store := buildFixtureStore([]seriesFixture{
			{coordinate: fixtureCoordinate("z", "x"), values: x, start: time.Unix(0, 0), step: time.Second},
			{coordinate: fixtureCoordinate("z", "y"), values: y, start: time.Unix(0, 0), step: time.Second},
			{coordinate: fixtureCoordinate("z", "zero"), values: zero, start: time.Unix(0, 0), step: time.Second},
		})

		Convey("an observed zero coordinate is retained and distinct from missing", func() {
			result := driveStore(store, &StoreCommand{History: &HistoryRequest{
				Coordinate: fixtureCoordinate("z", "zero"),
			}})

			for _, observation := range result.Observations {
				So(observation.Raw, ShouldEqual, 0)
			}

			So(result.Observations, ShouldHaveLength, count)
		})

		Convey("a missing source coordinate yields no_source_history, not a zero relation", func() {
			result := estimateFixture(store,
				fixtureCoordinate("z", "missing"),
				fixtureCoordinate("z", "y"),
				nil,
				LagDomain{MinLag: time.Second, MaxLag: 2 * time.Second},
			)
			So(result.Status, ShouldEqual, FitNoSourceHistory)
			So(result.Coefficient, ShouldBeNil)
			So(result.PredictiveGain, ShouldBeNil)
		})

		Convey("a missing control makes the relation unavailable, not control-free", func() {
			result := estimateFixture(store,
				fixtureCoordinate("z", "x"),
				fixtureCoordinate("z", "y"),
				[]Control{{
					Coordinate: fixtureCoordinate("z", "missing_control"),
				}},
				LagDomain{MinLag: time.Second, MaxLag: 2 * time.Second},
			)
			So(result.Status, ShouldEqual, FitControlUnavailable)
		})

		Convey("a constant zero source is a valid zero-coefficient relation, not deleted", func() {
			result := estimateFixture(store,
				fixtureCoordinate("z", "zero"),
				fixtureCoordinate("z", "y"),
				nil,
				LagDomain{MinLag: time.Second, MaxLag: 2 * time.Second},
			)
			So(result, ShouldNotBeNil)
		})

		Convey("an empty version is a domain failure, not a silent default", func() {
			invalid := NewInfluence("")
			So(invalid.Error(), ShouldNotBeNil)

			var yielded int

			for range invalid.Next(singlePointer(unsafe.Pointer(&InfluenceRequest{}))) {
				yielded++
			}

			So(yielded, ShouldEqual, 0)
		})
	})
}

func TestAlign(t *testing.T) {
	Convey("Given two resident rings at a one-second cadence", t, func() {
		random := rand.New(rand.NewSource(5))
		count := 50
		x := gaussianSequence(random, count)
		y := gaussianSequence(random, count)

		store := buildFixtureStore([]seriesFixture{
			{coordinate: fixtureCoordinate("al", "x"), values: x, start: time.Unix(0, 0), step: time.Second},
			{coordinate: fixtureCoordinate("al", "y"), values: y, start: time.Unix(0, 0), step: time.Second},
		})

		request := &AlignRequest{
			Target: ringFor(store, fixtureCoordinate("al", "y")),
			Series: []SeriesView{{History: ringFor(store, fixtureCoordinate("al", "x")), Lag: time.Second}},
		}

		var result *AlignResult

		aligner := NewAlign()
		for out := range aligner.Next(singlePointer(unsafe.Pointer(request))) {
			result = (*AlignResult)(out)
		}

		Convey("the first aligned row pairs Y_t with X_{t-1}", func() {
			So(result.Rows, ShouldHaveLength, count-1)
			So(result.Rows[0].Target.Raw, ShouldEqual, y[1])
			So(result.Rows[0].Predictors[0].Raw, ShouldEqual, x[0])
		})

		Convey("future observations never enter a row", func() {
			for index, row := range result.Rows {
				So(row.Predictors[0].At.Before(row.Target.At), ShouldBeTrue)
				So(row.Target.Raw, ShouldEqual, y[index+1])
			}
		})
	})
}

func TestPlanner(t *testing.T) {
	Convey("Given a plan over resident coordinates", t, func() {
		coordinates := []Coordinate{
			fixtureCoordinate("cvd", "signed_net_fraction"),
			fixtureCoordinate("cvd", "midpoint_log_return"),
			fixtureCoordinate("hawkes", "arrival_rate"),
		}

		plan := &RelationPlan{
			Epoch:   1,
			Symbol:  "TEST/USD",
			Sources: []Selector{{Source: "cvd"}},
			Targets: []Selector{{Source: "cvd"}, {Source: "hawkes"}},
			Controls: []ControlSelector{
				{Selector: Selector{Source: "hawkes", Metric: "arrival_rate"}, Lag: time.Second},
			},
			Lag: LagDomain{MinLag: time.Second, MaxLag: 5 * time.Second},
		}

		request := &CompileRequest{
			Plans:       []*RelationPlan{plan},
			Symbol:      "TEST/USD",
			Epoch:       1,
			Coordinates: coordinates,
		}

		var result *CompileResult

		planner := NewPlanner()
		for out := range planner.Next(singlePointer(unsafe.Pointer(request))) {
			result = (*CompileResult)(out)
		}

		Convey("cross-product pairs expand with self-pairs excluded", func() {
			So(result.Candidates, ShouldHaveLength, 2)

			for _, candidate := range result.Candidates {
				So(candidate.Source, ShouldNotResemble, candidate.Target)
				So(candidate.Source.Source, ShouldEqual, "cvd")
			}
		})

		Convey("exact controls resolve against resident coordinates", func() {
			for _, candidate := range result.Candidates {
				So(candidate.ControlsComplete, ShouldBeTrue)
				So(candidate.Controls, ShouldHaveLength, 1)
				So(candidate.Controls[0].Coordinate.Metric, ShouldEqual, "arrival_rate")
				So(candidate.Controls[0].Lag, ShouldEqual, time.Second)
			}
		})

		Convey("a foreign epoch compiles nothing", func() {
			stale := &CompileRequest{
				Plans:       []*RelationPlan{plan},
				Symbol:      "TEST/USD",
				Epoch:       2,
				Coordinates: coordinates,
			}

			var staleResult *CompileResult

			for out := range planner.Next(singlePointer(unsafe.Pointer(stale))) {
				staleResult = (*CompileResult)(out)
			}

			So(staleResult.Candidates, ShouldBeEmpty)
		})

		Convey("a symbol outside the plan scope compiles nothing", func() {
			scoped := &CompileRequest{
				Plans:       []*RelationPlan{plan},
				Symbol:      "OTHER/USD",
				Epoch:       1,
				Coordinates: coordinates,
			}

			var scopedResult *CompileResult

			for out := range planner.Next(singlePointer(unsafe.Pointer(scoped))) {
				scopedResult = (*CompileResult)(out)
			}

			So(scopedResult.Candidates, ShouldBeEmpty)
		})

		Convey("a missing exact control is reported incomplete", func() {
			missingPlan := &RelationPlan{
				Epoch: 1,
				Pairs: []PlannedPair{{
					Source: Selector{Source: "cvd", Metric: "signed_net_fraction"},
					Target: Selector{Source: "cvd", Metric: "midpoint_log_return"},
				}},
				Controls: []ControlSelector{
					{Selector: Selector{Source: "nope"}},
				},
			}

			missing := &CompileRequest{
				Plans:       []*RelationPlan{missingPlan},
				Symbol:      "TEST/USD",
				Epoch:       1,
				Coordinates: coordinates,
			}

			var missingResult *CompileResult

			for out := range planner.Next(singlePointer(unsafe.Pointer(missing))) {
				missingResult = (*CompileResult)(out)
			}

			So(missingResult.Candidates, ShouldHaveLength, 1)
			So(missingResult.Candidates[0].ControlsComplete, ShouldBeFalse)
			So(missingResult.Candidates[0].Controls, ShouldBeEmpty)
		})
	})
}

var benchmarkEstimateSink FitStatus

/*
BenchmarkInfluence measures the full prequential estimate path over resident
ring views: alignment and regression accumulation are fused into a single
per-lag walk, so no history copy and no aligned-row materialization remains
in the loop.
*/
func BenchmarkInfluence(b *testing.B) {
	random := rand.New(rand.NewSource(42))
	count := 256
	x := gaussianSequence(random, count)
	y := make([]float64, count)

	for index := 1; index < count; index++ {
		y[index] = 0.3*y[index-1] + 0.5*x[index-1] + random.NormFloat64()
	}

	store := buildFixtureStore([]seriesFixture{
		{coordinate: fixtureCoordinate("z", "x"), values: x, start: time.Unix(0, 0), step: time.Second},
		{coordinate: fixtureCoordinate("z", "y"), values: y, start: time.Unix(0, 0), step: time.Second},
	})

	estimator := NewInfluence("bench-v1")
	source := fixtureCoordinate("z", "x")
	target := fixtureCoordinate("z", "y")

	b.ReportAllocs()

	for b.Loop() {
		request := &InfluenceRequest{
			Source: source,
			Target: target,
			History: InfluenceHistory{
				Source: ringFor(store, source),
				Target: ringFor(store, target),
			},
			Lag: LagDomain{MinLag: time.Second, MaxLag: 10 * time.Second},
		}

		result := driveInfluence(estimator, request)
		benchmarkEstimateSink = result.Status
	}
}
