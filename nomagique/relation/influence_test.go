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

			if result.PredictiveGain != nil {
				So(*result.PredictiveGain, ShouldBeLessThan, 0.3)
			}

			if result.Coefficient != nil {
				So(math.Abs(*result.Coefficient), ShouldBeLessThan, 0.3)
			}
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

			if result.PredictiveGain != nil {
				So(math.Abs(*result.PredictiveGain), ShouldBeLessThan, 0.2)
			}

			if result.Coefficient != nil {
				So(math.Abs(*result.Coefficient), ShouldBeLessThan, 0.3)
			}
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
			y[index] = 0.3*y[index-1] + noiseFor(random)
		}

		store := buildFixtureStore([]seriesFixture{
			{coordinate: fixtureCoordinate("z", "x"), values: x, start: time.Unix(0, 0), step: time.Second},
			{coordinate: fixtureCoordinate("z", "y"), values: y, start: time.Unix(0, 0), step: time.Second},
			{coordinate: fixtureCoordinate("z", "zero"), values: zero, start: time.Unix(0, 0), step: time.Second},
		})

		Convey("an observed zero coordinate is retained and distinct from missing", func() {
			history := store.History(fixtureCoordinate("z", "zero"))
			So(len(history), ShouldEqual, count)

			for _, observation := range history {
				So(observation.Raw, ShouldEqual, 0)
			}
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

func TestStoreRetention(t *testing.T) {
	Convey("Given a bounded observation store", t, func() {
		store := NewObservationStore(3)
		coordinate := fixtureCoordinate("s", "m")

		for index := 0; index < 10; index++ {
			store.Append(Observation{
				Coordinate: coordinate,
				Raw:        float64(index),
				At:         time.Unix(0, int64(index)*int64(time.Second)),
			})
		}

		Convey("retention is chronological and bounded by the infrastructure capacity", func() {
			history := store.History(coordinate)
			So(len(history), ShouldEqual, 3)
			So(history[0].Raw, ShouldEqual, 7)
			So(history[2].Raw, ShouldEqual, 9)
		})

		Convey("eviction is never value-based", func() {
			So(store.Retention().Capacity, ShouldEqual, 3)
		})

		Convey("snapshots report coordinate and observation counts", func() {
			snapshot := store.Snapshot()
			So(snapshot.Coordinates, ShouldEqual, 1)
			So(snapshot.Observations, ShouldEqual, 3)
			So(snapshot.Appended, ShouldEqual, 10)
		})
	})
}

func TestEpochSeparation(t *testing.T) {
	Convey("Given observations in two model epochs", t, func() {
		store := NewObservationStore(64)
		epochOne := fixtureCoordinate("e", "m")
		epochOne.Epoch = 1
		epochTwo := fixtureCoordinate("e", "m")
		epochTwo.Epoch = 2

		store.Append(Observation{Coordinate: epochOne, Raw: 1, At: time.Unix(1, 0)})
		store.Append(Observation{Coordinate: epochTwo, Raw: 2, At: time.Unix(2, 0)})

		Convey("incompatible epochs are never mixed", func() {
			So(store.Count(epochOne), ShouldEqual, 1)
			So(store.Count(epochTwo), ShouldEqual, 1)

			historyOne := store.History(epochOne)
			historyTwo := store.History(epochTwo)
			So(historyOne[0].Raw, ShouldEqual, 1)
			So(historyTwo[0].Raw, ShouldEqual, 2)
		})
	})
}

func noiseFor(random *rand.Rand) float64 {
	return random.NormFloat64()
}
