package learning

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"gonum.org/v1/gonum/mat"
)

func TestNewResonanceManifold(testingTB *testing.T) {
	Convey("Given a valid architecture and learning pace", testingTB, func() {
		manifold := NewResonanceManifold([]int{4, 8, 4}, 2, 0.01)

		Convey("It should construct vector state and matrix operators", func() {
			So(manifold, ShouldNotBeNil)
			So(manifold.latentStates, ShouldHaveLength, 3)
			So(manifold.latentStates[0].Len(), ShouldEqual, 4)
			So(manifold.latentStates[1].Len(), ShouldEqual, 8)
			So(manifold.latentStates[2].Len(), ShouldEqual, 4)
			So(manifold.taskWeights, ShouldNotBeNil)
		})
	})

	Convey("Given an invalid architecture or learning pace", testingTB, func() {
		Convey("It should reject construction", func() {
			So(NewResonanceManifold([]int{4}, 1, 0.01), ShouldBeNil)
			So(NewResonanceManifold([]int{4, 2}, 1, 0), ShouldBeNil)
		})
	})
}

func TestResonanceManifoldSettle(testingTB *testing.T) {
	Convey("Given a manifold settling a sequence of inputs", testingTB, func() {
		manifold := NewResonanceManifold([]int{4, 8, 4}, 0, 0.03)
		inputs := [][]float64{
			{0.8, -0.2, 0.4, 0.1},
			{-0.3, 0.6, -0.1, 0.2},
			{0.1, 0.2, -0.4, 0.7},
		}

		for _, input := range inputs {
			So(manifold.Settle(input, true), ShouldBeNil)
		}

		Convey("It should keep every reported quantity finite", func() {
			So(finite(manifold.Energy()), ShouldBeTrue)
			So(finite(manifold.PredictionEnergy()), ShouldBeTrue)
			So(finite(manifold.ReconstructionError()), ShouldBeTrue)

			for _, latent := range manifold.latentStates {
				for _, value := range latent.RawVector().Data {
					So(finite(value), ShouldBeTrue)
				}
			}
		})

		Convey("It should reuse its inference workspace", func() {
			var settleErr error
			allocations := testing.AllocsPerRun(100, func() {
				settleErr = manifold.Settle(inputs[0], true)
			})

			So(settleErr, ShouldBeNil)
			So(allocations, ShouldEqual, 0)
		})
	})
}

func TestResonanceManifoldWireSnapshot(testingTB *testing.T) {
	Convey("Given a settled manifold with active latent regularization", testingTB, func() {
		manifold := NewResonanceManifold([]int{4, 8, 4}, 0, 0.03)
		So(manifold.Settle([]float64{0.8, -0.2, 0.4, 0.1}, true), ShouldBeNil)
		manifold.cfg.LatentDecay = 100
		manifold.cfg.Sparsity = 100

		_, surprise, energy := manifold.WireSnapshot()

		Convey("It should report per-residual prediction diagnostics", func() {
			predictionDimensions := float64(manifold.arch[0] + manifold.arch[1])
			So(surprise, ShouldAlmostEqual,
				manifold.ReconstructionError()/math.Sqrt(float64(manifold.arch[0])))
			So(energy, ShouldAlmostEqual,
				manifold.PredictionEnergy()/predictionDimensions)
			So(energy, ShouldBeLessThan, manifold.Energy())
		})
	})
}

func TestResonanceManifoldObserveTask(testingTB *testing.T) {
	Convey("Given identical features and targets with different issued forecasts", testingTB, func() {
		accurate := NewResonanceManifold([]int{1, 1}, 1, 0.05)
		wrong := NewResonanceManifold([]int{1, 1}, 1, 0.05)
		features := []float64{0.25}
		target := []float64{0.01}

		accurateErr := accurate.ObserveTask(
			features,
			[]float64{0.009},
			target,
		)
		wrongErr := wrong.ObserveTask(
			features,
			[]float64{-0.01},
			target,
		)
		accurateSkill, accurateReady := accurate.TaskSkill()
		wrongSkill, wrongReady := wrong.TaskSkill()

		Convey("It should score and learn from the strict-prior forecast that was shown", func() {
			So(accurateErr, ShouldBeNil)
			So(wrongErr, ShouldBeNil)
			So(accurateReady, ShouldBeTrue)
			So(wrongReady, ShouldBeTrue)
			So(accurateSkill, ShouldBeGreaterThan, wrongSkill)
			So(accurate.TaskPrediction()[0], ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given malformed retained task evidence", testingTB, func() {
		manifold := NewResonanceManifold([]int{1, 1}, 1, 0.05)

		Convey("It should reject it before changing the task learner", func() {
			before := manifold.TaskPrediction()
			err := manifold.ObserveTask(
				[]float64{0.25},
				[]float64{math.Inf(1)},
				[]float64{0.01},
			)

			So(err, ShouldNotBeNil)
			So(manifold.TaskPrediction(), ShouldResemble, before)
		})
	})
}

func TestResonanceManifoldRolloutTaskAggregateForecast(testingTB *testing.T) {
	Convey("Given a trained task head and a settled temporal state", testingTB, func() {
		manifold := NewResonanceManifold([]int{2, 2}, 1, 0.05)
		features := []float64{0.25, -0.5}

		for sample := range 32 {
			shift := float64(sample%5) * 0.01
			observed := []float64{features[0] + shift, features[1] - shift}
			prediction := []float64{0}
			target := []float64{0.2 + observed[0] - 0.5*observed[1]}
			So(manifold.ObserveTask(observed, prediction, target), ShouldBeNil)
		}

		manifold.latentStates[len(manifold.latentStates)-1].CopyVec(mat.NewVecDense(2, features))
		steps := 4
		pointForecasts, err := manifold.RolloutTaskForecast(steps)
		So(err, ShouldBeNil)
		aggregate, err := manifold.RolloutTaskAggregateForecast(steps)
		pointSum := 0.0

		for _, point := range pointForecasts {
			pointSum += point.Value
		}

		Convey("It should report the distribution of the complete cumulative return", func() {
			So(err, ShouldBeNil)
			So(aggregate, ShouldHaveLength, 1)
			So(aggregate[0].Value, ShouldAlmostEqual, pointSum, 1e-12)
			So(aggregate[0].Ready, ShouldBeTrue)
			So(aggregate[0].Scale, ShouldBeGreaterThan, 0)
		})
	})
}

func TestResonanceManifoldStateGradients(testingTB *testing.T) {
	Convey("Given an unregularized manifold with fixed latent state", testingTB, func() {
		manifold := NewResonanceManifold([]int{2, 3, 2}, 0, 0.03)
		manifold.cfg.UsePrecision = false
		manifold.cfg.LatentDecay = 0
		manifold.cfg.Sparsity = 0
		manifold.cfg.GradClip = math.Inf(1)
		manifold.temporalPriorReady = false
		manifold.latentStates[0].CopyVec(mat.NewVecDense(2, []float64{0.3, -0.4}))
		manifold.latentStates[1].CopyVec(mat.NewVecDense(3, []float64{0.2, -0.1, 0.5}))
		manifold.latentStates[2].CopyVec(mat.NewVecDense(2, []float64{-0.3, 0.6}))

		predictions, layerErrors := manifold.predictAdjacentLayers()
		gradients := manifold.stateGradients(predictions, layerErrors)
		gradientCopies := make([][]float64, len(gradients))

		for layerIndex := 1; layerIndex < len(gradients); layerIndex++ {
			gradientCopies[layerIndex] = append(
				[]float64(nil),
				gradients[layerIndex].RawVector().Data...,
			)
		}

		Convey("It should match the central difference of prediction energy", func() {
			finiteDifferenceStep := math.Cbrt(math.Nextafter(1, 2) - 1)

			for layerIndex := 1; layerIndex < len(manifold.latentStates); layerIndex++ {
				latent := manifold.latentStates[layerIndex]

				for valueIndex, analytical := range gradientCopies[layerIndex] {
					original := latent.AtVec(valueIndex)
					latent.SetVec(valueIndex, original+finiteDifferenceStep)
					positiveEnergy := manifold.PredictionEnergy()
					latent.SetVec(valueIndex, original-finiteDifferenceStep)
					negativeEnergy := manifold.PredictionEnergy()
					latent.SetVec(valueIndex, original)

					numerical := (positiveEnergy - negativeEnergy) /
						(2 * finiteDifferenceStep)
					So(analytical, ShouldAlmostEqual, numerical, 1e-8)
				}
			}
		})
	})
}

func TestResonanceManifoldProjectTemporalOperatorNorm(testingTB *testing.T) {
	Convey("Given a temporal operator above its contraction limit", testingTB, func() {
		manifold := NewResonanceManifold([]int{2, 2}, 0, 0.05)
		manifold.temporalOperator.Copy(mat.NewDense(2, 2, []float64{10, 4, -3, 8}))
		valuesBuffer := &manifold.workspace.svdValues[0]

		err := manifold.projectTemporalOperatorNorm()

		var decomposition mat.SVD
		So(decomposition.Factorize(manifold.temporalOperator, mat.SVDNone), ShouldBeTrue)
		operatorNorm := decomposition.Values(make([]float64, 2))[0]

		Convey("It should reuse the workspace and restore contraction", func() {
			So(err, ShouldBeNil)
			So(&manifold.workspace.svdValues[0], ShouldEqual, valuesBuffer)
			So(operatorNorm, ShouldBeLessThanOrEqualTo,
				manifold.cfg.TemporalNormMax+1e-12)
		})
	})
}

func BenchmarkResonanceManifoldSettle(testingTB *testing.B) {
	manifold := NewResonanceManifold([]int{8, 16, 8}, 0, 0.01)
	input := []float64{0.1, -0.2, 0.3, -0.4, 0.5, -0.6, 0.7, -0.8}

	testingTB.ReportAllocs()

	for testingTB.Loop() {
		if err := manifold.Settle(input, true); err != nil {
			testingTB.Fatal(err)
		}
	}
}

func BenchmarkResonanceManifoldWireSnapshot(testingTB *testing.B) {
	manifold := NewResonanceManifold([]int{8, 16, 8}, 0, 0.01)
	input := []float64{0.1, -0.2, 0.3, -0.4, 0.5, -0.6, 0.7, -0.8}

	if err := manifold.Settle(input, true); err != nil {
		testingTB.Fatal(err)
	}

	testingTB.ReportAllocs()

	for testingTB.Loop() {
		manifold.WireSnapshot()
	}
}

func BenchmarkResonanceManifoldObserveTask(testingTB *testing.B) {
	manifold := NewResonanceManifold([]int{8, 16, 8}, 1, 0.01)
	features := make([]float64, 8)
	prediction := []float64{0.001}
	target := []float64{0.002}

	testingTB.ReportAllocs()

	for testingTB.Loop() {
		if err := manifold.ObserveTask(features, prediction, target); err != nil {
			testingTB.Fatal(err)
		}
	}
}

func BenchmarkResonanceManifoldRolloutTaskAggregateForecast(testingTB *testing.B) {
	manifold := NewResonanceManifold([]int{8, 16, 8}, 1, 0.01)
	features := make([]float64, 8)

	for sample := range 16 {
		features[0] = float64(sample)

		if err := manifold.ObserveTask(
			features,
			[]float64{0},
			[]float64{float64(sample) * 0.001},
		); err != nil {
			testingTB.Fatal(err)
		}
	}

	testingTB.ReportAllocs()

	for testingTB.Loop() {
		if _, err := manifold.RolloutTaskAggregateForecast(16); err != nil {
			testingTB.Fatal(err)
		}
	}
}

func BenchmarkResonanceManifoldLearn(testingTB *testing.B) {
	manifold := NewResonanceManifold([]int{8, 16, 8}, 1, 0.01)
	input := []float64{0.1, -0.2, 0.3, -0.4, 0.5, -0.6, 0.7, -0.8}
	target := []float64{0.01}

	if err := manifold.Settle(input, false); err != nil {
		testingTB.Fatal(err)
	}

	if err := manifold.Learn(target); err != nil {
		testingTB.Fatal(err)
	}

	if err := manifold.Settle(input, false); err != nil {
		testingTB.Fatal(err)
	}

	testingTB.ReportAllocs()

	for testingTB.Loop() {
		if err := manifold.Learn(target); err != nil {
			testingTB.Fatal(err)
		}
	}
}

func BenchmarkResonanceManifoldProjectTemporalOperatorNorm(testingTB *testing.B) {
	manifold := NewResonanceManifold([]int{8, 8}, 0, 0.01)

	testingTB.ReportAllocs()

	for testingTB.Loop() {
		if err := manifold.projectTemporalOperatorNorm(); err != nil {
			testingTB.Fatal(err)
		}
	}
}
