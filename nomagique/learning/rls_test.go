package learning

import (
	"math"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewRLS(testingTB *testing.T) {
	Convey("Given valid config", testingTB, func() {
		stage, err := NewRLS(RLSConfig{Dimension: 2, InitialVariance: 1000})

		Convey("It should return a usable learner", func() {
			So(err, ShouldBeNil)
			So(stage, ShouldNotBeNil)
		})
	})
}

func TestRLSMeasure(testingTB *testing.T) {
	Convey("Given concurrent forecasts and observations", testingTB, func() {
		stage, err := NewRLS(RLSConfig{Dimension: 1, InitialVariance: 1000})
		So(err, ShouldBeNil)
		errors := make(chan error, 128)
		wait := sync.WaitGroup{}
		wait.Add(2)

		go func() {
			defer wait.Done()

			for observation := 1; observation <= 64; observation++ {
				_, err := stage.Measure(RLSSample{
					Features: []float64{float64(observation)},
					Target:   float64(observation * 2),
				})
				errors <- err
			}
		}()

		go func() {
			defer wait.Done()

			for forecast := 1; forecast <= 64; forecast++ {
				_, err := stage.Predict([]float64{float64(forecast)})
				errors <- err
			}
		}()

		wait.Wait()
		close(errors)

		Convey("It should protect updates while read-only forecasts share the model", func() {
			for err := range errors {
				So(err, ShouldBeNil)
			}
		})
	})

	Convey("Given invalid dimension", testingTB, func() {
		_, err := NewRLS(RLSConfig{Dimension: 0, InitialVariance: 1000})

		Convey("It should reject the config", func() {
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Given a one-dimensional linear stream", testingTB, func() {
		stage, err := NewRLS(RLSConfig{Dimension: 1, InitialVariance: 1000})
		So(err, ShouldBeNil)

		for _, feature := range []float64{1, 2, 3, 4, 5} {
			prior, measureErr := stage.Predict([]float64{feature})
			So(measureErr, ShouldBeNil)

			output, measureErr := stage.Measure(RLSSample{
				Features: []float64{feature},
				Target:   2*feature + 1,
			})
			So(measureErr, ShouldBeNil)
			So(output.Value, ShouldEqual, prior.Value)
		}

		before, err := stage.Predict([]float64{0.5})
		So(err, ShouldBeNil)
		output, err := stage.Measure(RLSSample{
			Features: []float64{0.5},
			Target:   2,
		})
		snapshot, snapErr := stage.Snapshot()

		Convey("It should forecast before observing and retain finite state", func() {
			So(err, ShouldBeNil)
			So(snapErr, ShouldBeNil)
			So(output.Value, ShouldEqual, before.Value)
			So(math.IsNaN(output.Value), ShouldBeFalse)
			So(len(snapshot.Beta), ShouldEqual, 2)
			So(len(snapshot.CovarianceDiagonal), ShouldEqual, 2)
		})
	})
}

func TestRLSPredict(testingTB *testing.T) {
	Convey("Given a learner without resolved residual scale", testingTB, func() {
		stage, err := NewRLS(RLSConfig{Dimension: 1, InitialVariance: 1})
		So(err, ShouldBeNil)

		output, err := stage.Predict([]float64{1})

		Convey("It should forecast without inventing predictive uncertainty", func() {
			So(err, ShouldBeNil)
			So(output.Value, ShouldEqual, 0)
			So(output.Ready, ShouldBeFalse)
			So(output.Scale, ShouldEqual, 0)
			So(output.DegreesOfFreedom, ShouldEqual, 0)
		})
	})

	Convey("Given a trained one-dimensional learner", testingTB, func() {
		stage, err := NewRLS(RLSConfig{Dimension: 1, InitialVariance: 1000})
		So(err, ShouldBeNil)

		for _, feature := range []float64{1, 2, 3, 4, 5} {
			_, err = stage.Observe(RLSSample{
				Features: []float64{feature},
				Target:   2*feature + 1,
			})
			So(err, ShouldBeNil)
		}

		before, err := stage.Predict([]float64{6})
		So(err, ShouldBeNil)
		after, err := stage.Predict([]float64{6})
		beforeSnap, err := stage.Snapshot()
		So(err, ShouldBeNil)
		afterSnap, err := stage.Snapshot()

		Convey("It should predict without changing retained state", func() {
			So(err, ShouldBeNil)
			So(after.Value, ShouldEqual, before.Value)
			So(after.Ready, ShouldBeTrue)
			So(after.Scale, ShouldBeGreaterThan, 0)
			So(after.DegreesOfFreedom, ShouldEqual, 5)
			So(afterSnap.Beta, ShouldResemble, beforeSnap.Beta)
			So(afterSnap.Covariance, ShouldResemble, beforeSnap.Covariance)
		})
	})

	Convey("Given observations concentrated in one part of the design", testingTB, func() {
		stage, err := NewRLS(RLSConfig{Dimension: 1, InitialVariance: 1})
		So(err, ShouldBeNil)

		for _, target := range []float64{0.9, 1.1, 0.8, 1.2} {
			_, err = stage.Observe(RLSSample{
				Features: []float64{0},
				Target:   target,
			})
			So(err, ShouldBeNil)
		}

		familiar, err := stage.Predict([]float64{0})
		So(err, ShouldBeNil)
		novel, err := stage.Predict([]float64{10})

		Convey("It should widen uncertainty for a high-leverage forecast", func() {
			So(err, ShouldBeNil)
			So(familiar.Ready, ShouldBeTrue)
			So(novel.Ready, ShouldBeTrue)
			So(novel.Scale, ShouldBeGreaterThan, familiar.Scale)
		})
	})
}

func TestRLSPredictSum(testingTB *testing.T) {
	Convey("Given an affine learner with a resolved noise posterior", testingTB, func() {
		stage, err := NewRLS(RLSConfig{
			Dimension:       1,
			InitialVariance: 1e6,
		})
		So(err, ShouldBeNil)

		for repetition := range 8 {
			for feature := -8; feature <= 8; feature++ {
				noise := float64((feature+repetition)%3-1) * 0.01
				_, err = stage.Observe(RLSSample{
					Features: []float64{float64(feature)},
					Target:   -2 + 3*float64(feature) + noise,
				})
				So(err, ShouldBeNil)
			}
		}

		rows := [][]float64{{1}, {2}, {4}}
		aggregate, err := stage.PredictSum(rows)
		So(err, ShouldBeNil)
		snapshot, err := stage.Snapshot()
		So(err, ShouldBeNil)
		individualSum := 0.0

		for _, row := range rows {
			individual, predictErr := stage.Predict(row)
			So(predictErr, ShouldBeNil)
			individualSum += individual.Value
		}

		design := []float64{float64(len(rows)), 7}
		coefficientLeverage := 0.0

		for row := range design {
			for col := range design {
				coefficientLeverage += design[row] *
					snapshot.Covariance[row*len(design)+col] * design[col]
			}
		}

		noiseVariance := stage.session.noiseScale / stage.session.noiseShape
		expectedVariance := noiseVariance *
			(float64(len(rows)) + coefficientLeverage)

		Convey("It should sum means and preserve shared coefficient covariance", func() {
			So(aggregate.Ready, ShouldBeTrue)
			So(aggregate.Value, ShouldAlmostEqual, individualSum, 1e-12)
			So(aggregate.Scale*aggregate.Scale,
				ShouldAlmostEqual, expectedVariance, 1e-12)
			So(aggregate.DegreesOfFreedom,
				ShouldEqual, 2*stage.session.noiseShape)
		})
	})

	Convey("Given one future design row", testingTB, func() {
		stage, err := NewRLS(RLSConfig{Dimension: 1, InitialVariance: 1000})
		So(err, ShouldBeNil)

		for feature := -4; feature <= 4; feature++ {
			_, err = stage.Observe(RLSSample{
				Features: []float64{float64(feature)},
				Target:   1.5 * float64(feature),
			})
			So(err, ShouldBeNil)
		}

		individual, err := stage.Predict([]float64{3})
		So(err, ShouldBeNil)
		aggregate, err := stage.PredictSum([][]float64{{3}})

		Convey("It should equal the ordinary predictive distribution", func() {
			So(err, ShouldBeNil)
			So(aggregate, ShouldResemble, individual)
		})
	})

	Convey("Given malformed aggregate designs", testingTB, func() {
		stage, err := NewRLS(RLSConfig{Dimension: 1, InitialVariance: 1})
		So(err, ShouldBeNil)

		_, emptyErr := stage.PredictSum(nil)
		_, dimensionErr := stage.PredictSum([][]float64{{1, 2}})
		_, finiteErr := stage.PredictSum([][]float64{{math.Inf(1)}})

		Convey("It should reject every design that cannot describe a forecast", func() {
			So(emptyErr, ShouldNotBeNil)
			So(dimensionErr, ShouldNotBeNil)
			So(finiteErr, ShouldNotBeNil)
		})
	})
}

func TestRLSObserve(testingTB *testing.T) {
	Convey("Given a repeated adversarially ordered affine process", testingTB, func() {
		stage, err := NewRLS(RLSConfig{
			Dimension:       1,
			InitialVariance: 1e9,
		})
		So(err, ShouldBeNil)

		for repetition := range 16 {
			for index := -16; index <= 16; index++ {
				feature := index

				if repetition%2 == 1 {
					feature = -index
				}

				_, err = stage.Observe(RLSSample{
					Features: []float64{float64(feature)},
					Target:   -7.25 + 2.75*float64(feature),
				})
				So(err, ShouldBeNil)
			}
		}

		snapshot, err := stage.Snapshot()
		forecast, forecastErr := stage.Predict([]float64{123.5})

		Convey("It should recover the exact intercept and slope without order bias", func() {
			So(err, ShouldBeNil)
			So(forecastErr, ShouldBeNil)
			So(snapshot.Beta[0], ShouldAlmostEqual, -7.25, 1e-8)
			So(snapshot.Beta[1], ShouldAlmostEqual, 2.75, 1e-10)
			So(forecast.Value, ShouldAlmostEqual, -7.25+2.75*123.5, 1e-8)
		})
	})
}

func TestRLSObserveResetsTogether(testingTB *testing.T) {
	Convey("Given a learner forced through an unrecoverable update", testingTB, func() {
		stage, err := NewRLS(RLSConfig{Dimension: 1, InitialVariance: 1})
		So(err, ShouldBeNil)

		_, err = stage.Observe(RLSSample{
			Features: []float64{math.Inf(1)},
			Target:   1,
		})

		Convey("It should reject non-finite features without retaining half-reset state", func() {
			So(err, ShouldNotBeNil)
			snapshot, snapErr := stage.Snapshot()
			So(snapErr, ShouldBeNil)
			So(snapshot.Beta, ShouldResemble, []float64{0, 0})
			So(snapshot.CovarianceDiagonal[0], ShouldEqual, 1)
			So(snapshot.CovarianceDiagonal[1], ShouldEqual, 1)
		})
	})
}

func BenchmarkRLSMeasure(b *testing.B) {
	stage, err := NewRLS(RLSConfig{
		Dimension:        3,
		InitialVariance:  1000,
		ForgettingFactor: 0.99,
	})

	if err != nil {
		b.Fatal(err)
	}

	sample := RLSSample{
		Features: []float64{1, 2, 3},
		Target:   4,
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = stage.Measure(sample)
	}
}

func BenchmarkRLSPredictSum(b *testing.B) {
	stage, err := NewRLS(RLSConfig{Dimension: 3, InitialVariance: 1000})

	if err != nil {
		b.Fatal(err)
	}

	rows := [][]float64{
		{1, 2, 3},
		{2, 3, 4},
		{3, 4, 5},
		{4, 5, 6},
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = stage.PredictSum(rows)
	}
}
