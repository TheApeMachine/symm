package relation

import (
	"math"
	"math/rand"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
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

func buildFixtureStore(fixtures []seriesFixture) *ObservationStore {
	store := NewObservationStore(4096)

	for _, fixture := range fixtures {
		for index, value := range fixture.values {
			at := fixture.start.Add(time.Duration(index) * fixture.step)
			store.Append(Observation{
				Coordinate: fixture.coordinate,
				Raw:        value,
				At:         at,
			})
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

		estimator := NewInfluenceEstimator("test-v1")

		Convey("X → Y discovers positive prequential gain", func() {
			result, err := estimator.Estimate(store, InfluenceRequest{
				Source: fixtureCoordinate("source", "x"),
				Target: fixtureCoordinate("target", "y"),
				Lag:    LagDomain{MinLag: 500 * time.Millisecond, MaxLag: 5 * time.Second},
			})
			So(err, ShouldBeNil)
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
			result, err := estimator.Estimate(store, InfluenceRequest{
				Source: fixtureCoordinate("target", "y"),
				Target: fixtureCoordinate("source", "x"),
				Lag:    LagDomain{MinLag: 500 * time.Millisecond, MaxLag: 5 * time.Second},
			})
			So(err, ShouldBeNil)
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

		estimator := NewInfluenceEstimator("test-v1")

		Convey("the relation remains represented with near-zero gain and coefficient", func() {
			result, err := estimator.Estimate(store, InfluenceRequest{
				Source: fixtureCoordinate("a", "x"),
				Target: fixtureCoordinate("b", "y"),
				Lag:    LagDomain{MinLag: time.Second, MaxLag: 3 * time.Second},
			})
			So(err, ShouldBeNil)
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

		estimator := NewInfluenceEstimator("test-v1")

		Convey("pairwise X → Y appears predictive through the path", func() {
			result, err := estimator.Estimate(store, InfluenceRequest{
				Source: fixtureCoordinate("m", "x"),
				Target: fixtureCoordinate("m", "y"),
				Lag:    LagDomain{MinLag: time.Second, MaxLag: 5 * time.Second},
			})
			So(err, ShouldBeNil)
			So(result.Defined(), ShouldBeTrue)
			So(result.PredictiveGain, ShouldNotBeNil)
			So(*result.PredictiveGain, ShouldBeGreaterThan, 0.05)
		})

		Convey("conditional X → Y given M at its path lag loses incremental contribution", func() {
			result, err := estimator.Estimate(store, InfluenceRequest{
				Source: fixtureCoordinate("m", "x"),
				Target: fixtureCoordinate("m", "y"),
				Controls: []Control{{
					Coordinate: fixtureCoordinate("m", "mediator"),
					Lag:        time.Second,
				}},
				Lag: LagDomain{MinLag: time.Second, MaxLag: 5 * time.Second},
			})
			So(err, ShouldBeNil)
			So(result.Defined(), ShouldBeTrue)

			if result.PredictiveGain != nil {
				So(*result.PredictiveGain, ShouldBeLessThan, 0.05)
			}
		})

		Convey("M → Y remains measured", func() {
			result, err := estimator.Estimate(store, InfluenceRequest{
				Source: fixtureCoordinate("m", "mediator"),
				Target: fixtureCoordinate("m", "y"),
				Lag:    LagDomain{MinLag: time.Second, MaxLag: 5 * time.Second},
			})
			So(err, ShouldBeNil)
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

		estimator := NewInfluenceEstimator("test-v1")

		Convey("Influence does not discover the future relationship", func() {
			result, err := estimator.Estimate(store, InfluenceRequest{
				Source: fixtureCoordinate("f", "x"),
				Target: fixtureCoordinate("f", "y"),
				Lag:    LagDomain{MinLag: time.Second, MaxLag: 3 * time.Second},
			})
			So(err, ShouldBeNil)
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

		estimator := NewInfluenceEstimator("test-v1")

		Convey("the fit is undefined with no silent regularization", func() {
			result, err := estimator.Estimate(store, InfluenceRequest{
				Source: fixtureCoordinate("r", "x"),
				Target: fixtureCoordinate("r", "y"),
				Controls: []Control{
					{Coordinate: fixtureCoordinate("r", "control")},
					{Coordinate: fixtureCoordinate("r", "control")},
				},
				Lag: LagDomain{MinLag: time.Second, MaxLag: 2 * time.Second},
			})
			So(err, ShouldBeNil)
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
			visited := 0

			store.RangeHistory(fixtureCoordinate("z", "zero"), func(observation Observation) bool {
				So(observation.Raw, ShouldEqual, 0)
				visited++
				return true
			})

			So(visited, ShouldEqual, count)
		})

		Convey("a missing source coordinate yields no_source_history, not a zero relation", func() {
			estimator := NewInfluenceEstimator("test-v1")
			result, err := estimator.Estimate(store, InfluenceRequest{
				Source: fixtureCoordinate("z", "missing"),
				Target: fixtureCoordinate("z", "y"),
				Lag:    LagDomain{MinLag: time.Second, MaxLag: 2 * time.Second},
			})
			So(err, ShouldBeNil)
			So(result.Status, ShouldEqual, FitNoSourceHistory)
			So(result.Coefficient, ShouldBeNil)
			So(result.PredictiveGain, ShouldBeNil)
		})

		Convey("a missing control makes the relation unavailable, not control-free", func() {
			estimator := NewInfluenceEstimator("test-v1")
			result, err := estimator.Estimate(store, InfluenceRequest{
				Source: fixtureCoordinate("z", "x"),
				Target: fixtureCoordinate("z", "y"),
				Controls: []Control{{
					Coordinate: fixtureCoordinate("z", "missing_control"),
				}},
				Lag: LagDomain{MinLag: time.Second, MaxLag: 2 * time.Second},
			})
			So(err, ShouldBeNil)
			So(result.Status, ShouldEqual, FitControlUnavailable)
		})

		Convey("a constant zero source is a valid zero-coefficient relation, not deleted", func() {
			estimator := NewInfluenceEstimator("test-v1")
			result, err := estimator.Estimate(store, InfluenceRequest{
				Source: fixtureCoordinate("z", "zero"),
				Target: fixtureCoordinate("z", "y"),
				Lag:    LagDomain{MinLag: time.Second, MaxLag: 2 * time.Second},
			})
			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
		})
	})
}

var benchmarkEstimateSink FitStatus

/*
BenchmarkInfluenceEstimate measures the full prequential Estimate path over
resident ring views: alignment and regression accumulation are fused into a
single per-lag walk, so no history copy and no aligned-row materialization
remains in the loop.
*/
func BenchmarkInfluenceEstimate(b *testing.B) {
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

	estimator := NewInfluenceEstimator("bench-v1")
	request := InfluenceRequest{
		Source: fixtureCoordinate("z", "x"),
		Target: fixtureCoordinate("z", "y"),
		Lag:    LagDomain{MinLag: time.Second, MaxLag: 10 * time.Second},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		result, err := estimator.Estimate(store, request)

		if err != nil {
			b.Fatal(err)
		}

		benchmarkEstimateSink = result.Status
	}
}
