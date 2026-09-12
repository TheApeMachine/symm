package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestOvercompleteMultiTimescaleManifold(t *testing.T) {
	Convey("Given an overcomplete multi-timescale architecture [2, 8, 3]", t, func() {
		manifold := NewResonanceManifold([]int{2, 8, 3}, 1, 1, 0.03, ReadoutAll).(*ResonanceManifold)

		Convey("The overcomplete layer should have higher sparsity penalty", func() {
			So(manifold.cfg.Sparsity[0], ShouldBeGreaterThan, manifold.cfg.Sparsity[1])
			So(len(manifold.temporalOperators), ShouldEqual, 2)
		})

		Convey("Settling should compute multi-layer readouts with innovations", func() {
			reading, err := manifold.execute(&ManifoldCommand{
				Settle: &SettleIntent{Input: []float64{0.5, -0.5}, AdvanceTemporal: true},
			})
			So(err, ShouldBeNil)

			// [z1(8) + z2(3)] + [e0(2) + e1(8)] = 21 dimensions
			So(reading.ReadoutDimension, ShouldEqual, 21)
			So(len(reading.Readout), ShouldEqual, 21)
		})

		Convey("Learn should update all multi-timescale temporal matrices and the RLS head", func() {
			_, err := manifold.execute(&ManifoldCommand{
				Settle: &SettleIntent{Input: []float64{0.5, -0.5}},
			})
			So(err, ShouldBeNil)

			reading, learnErr := manifold.execute(&ManifoldCommand{
				Learn: &LearnIntent{Target: []float64{0.02}},
			})
			So(learnErr, ShouldBeNil)

			So(reading.TaskPrediction, ShouldHaveLength, 1)
		})
	})
}

func TestPerHorizonTaskHead(t *testing.T) {
	Convey("Given a per-horizon task head over architecture [2, 8, 3]", t, func() {
		manifold := NewResonanceManifold([]int{2, 8, 3}, 1, 4, 0.03, ReadoutAll).(*ResonanceManifold)

		Convey("The task head holds one row per horizon", func() {
			So(manifold.taskRows, ShouldEqual, 4)

			reading := manifold.snapshot()
			So(reading.TaskPrediction, ShouldHaveLength, 4)
		})

		Convey("ObserveTask trains only the addressed horizon row", func() {
			reading, err := manifold.execute(&ManifoldCommand{
				Settle: &SettleIntent{Input: []float64{0.5, -0.5}},
			})
			So(err, ShouldBeNil)

			_, err = manifold.execute(&ManifoldCommand{
				ObserveTask: &TaskIntent{
					Horizon:    4,
					Features:   reading.Readout,
					Prediction: 0.1,
					Target:     1.0,
				},
			})
			So(err, ShouldBeNil)

			snapshot := manifold.snapshot()
			So(snapshot.SkillReady[3], ShouldBeTrue)
			So(snapshot.Skill[3], ShouldBeGreaterThan, 0)
			So(snapshot.SkillReady[0], ShouldBeFalse)
		})

		Convey("RolloutTaskForecast returns one cumulative forecast per horizon from the current readout", func() {
			_, err := manifold.execute(&ManifoldCommand{
				Settle: &SettleIntent{Input: []float64{0.5, -0.5}},
			})
			So(err, ShouldBeNil)

			reading, err := manifold.execute(&ManifoldCommand{
				Forecast: &ForecastIntent{Steps: 4},
			})
			So(err, ShouldBeNil)
			So(reading.Forecast, ShouldHaveLength, 4)

			// Clamping: a request beyond the head's rows yields the head's rows.
			clamped, err := manifold.execute(&ManifoldCommand{
				Forecast: &ForecastIntent{Steps: 9},
			})
			So(err, ShouldBeNil)
			So(clamped.Forecast, ShouldHaveLength, 4)
		})

		Convey("An out-of-range task horizon is rejected", func() {
			_, err := manifold.execute(&ManifoldCommand{
				ObserveTask: &TaskIntent{
					Horizon:  5,
					Features: make([]float64, 21),
					Target:   1,
				},
			})
			So(err, ShouldNotBeNil)
		})
	})
}

/*
TestManifoldPrimitiveWire proves the manifold answers through its command wire
as a core.Primitive, not only through in-package execution.
*/
func TestManifoldPrimitiveWire(t *testing.T) {
	Convey("Given a manifold primitive on the wire", t, func() {
		var manifold core.Primitive = NewResonanceManifold([]int{2, 4, 2}, 1, 2, 0.05, ReadoutAll)

		Convey("A settle command yields exactly one reading", func() {
			evaluation := transport.NewEvaluate(manifold)
			var reading ManifoldReading

			for out := range evaluation.Next(transport.NewValues(ManifoldCommand{
				Settle: &SettleIntent{Input: []float64{0.5, -0.5}},
			}).Next(nil)) {
				reading = *(*ManifoldReading)(out)
			}

			So(evaluation.Error(), ShouldBeNil)
			So(reading.ReadoutDimension, ShouldEqual, 12)
			So(reading.Layers, ShouldHaveLength, 3)
		})

		Convey("A rejected architecture yields nothing and records its error", func() {
			rejected := NewResonanceManifold([]int{2}, 1, 2, 0.05, ReadoutAll)

			for range rejected.Next(transport.NewValues(ManifoldCommand{
				Settle: &SettleIntent{Input: []float64{0.5}},
			}).Next(nil)) {
				t.Fatal("rejected manifold must yield nothing")
			}

			So(rejected.Error(), ShouldNotBeNil)
		})
	})
}
