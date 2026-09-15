package strategy

import (
	"context"
	"iter"
	"testing"

	"github.com/theapemachine/symm/nomagique/runtime"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

func newTestFormedSpace(symbol string) *grid.Space {
	gridSpace := grid.NewSpace(4)
	for i := 1; i <= 8; i++ {
		sign := 1.0
		if i%2 == 0 {
			sign = -1.0
		}

		m := data.NewMeasurement("spot", map[string]data.Metric[float64]{
			"price":  {Label: "price", Raw: 100.0 + float64(i)*sign},
			"volume": {Label: "volume", Raw: 10.0 - float64(i)*sign},
		})
		m.Label = symbol
		m.SeqIdx = int64(i)
		m.At = time.Now()

		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(m))
		}

		for range gridSpace.Next(in) {
		}
	}

	return gridSpace
}

func TestTraining_Learn(t *testing.T) {
	Convey("Given a Training instance with a formed cognitive pipeline", t, func() {
		ctx := context.Background()

		Convey("When replaying a profitable upward excursion", func() {
			gridSpace := newTestFormedSpace("BTC/USD")
			So(gridSpace.Formed(), ShouldBeTrue)

			training := NewTraining(ctx, nil, gridSpace)
			training.Transition(runtime.READY)

			excursion := tables.ExcursionRecord{
				ID:                 "exc-1",
				Symbol:             "BTC/USD",
				Direction:          "upward",
				ClearsFriction:     true,
				PrecursorStartTick: 9,
				AnchorTick:         14,
				ExtremumTick:       17,
				ExitTick:           19,
				PostEndTick:        20,
			}

			ticks := []int64{9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
			measurements := make([]*data.Measurement[float64], len(ticks))

			for idx, tick := range ticks {
				measurement := data.NewMeasurement("spot", map[string]data.Metric[float64]{
					"price":  {Label: "price", Raw: 100.0 + float64(tick)},
					"volume": {Label: "volume", Raw: 20.0 + float64(tick%3)},
				})
				measurement.Label = "BTC/USD"
				measurement.SeqIdx = tick
				measurement.At = time.Now()
				measurements[idx] = measurement
			}

			fragments := func(yield func(iter.Seq[unsafe.Pointer]) bool) {
				inner := func(innerYield func(unsafe.Pointer) bool) {
					for _, measurement := range measurements {
						if !innerYield(unsafe.Pointer(measurement)) {
							return
						}
					}
				}
				yield(inner)
			}

			training.Learn([]tables.ExcursionRecord{excursion}, fragments)
			training.wg.Wait()

			Convey("The cognitive engine should have learned actual associations in the radix trie", func() {
				So(training.engine.Len(), ShouldBeGreaterThan, 0)

				census := training.engine.Census()
				So(census[string(ActionEnter)], ShouldBeGreaterThan, 0)
				So(census[string(ActionExit)], ShouldBeGreaterThan, 0)

				export := training.engine.ExportTree(nil, 10)
				So(len(export.Branches), ShouldBeGreaterThan, 1)

				held := training.Register()
				So(held.Metrics["steps"].Raw, ShouldBeGreaterThan, 0)
				So(held.Metrics["win_rate"].Raw, ShouldEqual, 1.0)
			})

			Convey("Subsequent live Step queries on precursor patterns should evaluate with confidence", func() {
				// Step through precursor frames up to the anchor tick
				var stepOutput *data.Measurement[float64]
				for _, m := range measurements[:6] { // ticks 9 to 14 (AnchorTick)
					stepOutput = training.Step(m)
				}

				So(stepOutput, ShouldNotBeNil)
				So(stepOutput.Metrics["action"].Raw, ShouldEqual, 1.0) // ActionEnter
				So(stepOutput.Metrics["support"].Raw, ShouldBeGreaterThan, 0.0)
				So(stepOutput.Metrics["confidence"].Raw, ShouldBeGreaterThan, 0.0)
			})
		})

		Convey("When replaying a downward excursion that does not clear friction", func() {
			gridSpace := newTestFormedSpace("ETH/USD")
			So(gridSpace.Formed(), ShouldBeTrue)

			training := NewTraining(ctx, nil, gridSpace)
			training.Transition(runtime.READY)

			excursion := tables.ExcursionRecord{
				ID:                 "exc-2",
				Symbol:             "ETH/USD",
				Direction:          "downward",
				ClearsFriction:     false,
				PrecursorStartTick: 9,
				AnchorTick:         14,
				ExtremumTick:       17,
				ExitTick:           19,
				PostEndTick:        20,
			}

			ticks := []int64{9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
			measurements := make([]*data.Measurement[float64], len(ticks))

			for idx, tick := range ticks {
				measurement := data.NewMeasurement("spot", map[string]data.Metric[float64]{
					"price":  {Label: "price", Raw: 50.0 - float64(tick)},
					"volume": {Label: "volume", Raw: 15.0 - float64(tick%3)},
				})
				measurement.Label = "ETH/USD"
				measurement.SeqIdx = tick
				measurement.At = time.Now()
				measurements[idx] = measurement
			}

			fragments := func(yield func(iter.Seq[unsafe.Pointer]) bool) {
				inner := func(innerYield func(unsafe.Pointer) bool) {
					for _, measurement := range measurements {
						if !innerYield(unsafe.Pointer(measurement)) {
							return
						}
					}
				}
				yield(inner)
			}

			training.Learn([]tables.ExcursionRecord{excursion}, fragments)
			training.wg.Wait()

			Convey("The cognitive engine should have learned the wait action", func() {
				So(training.engine.Len(), ShouldBeGreaterThan, 0)

				census := training.engine.Census()
				So(census[string(ActionWait)], ShouldBeGreaterThan, 0)

				export := training.engine.ExportTree(nil, 10)
				So(len(export.Branches), ShouldBeGreaterThan, 1)
			})

			Convey("Live Step queries on downward patterns should evaluate to ActionWait", func() {
				var stepOutput *data.Measurement[float64]
				for _, m := range measurements[:6] {
					stepOutput = training.Step(m)
				}

				So(stepOutput, ShouldNotBeNil)
				So(stepOutput.Metrics["action"].Raw, ShouldEqual, 0.0) // ActionWait
				So(stepOutput.Metrics["support"].Raw, ShouldBeGreaterThan, 0.0)
			})
		})
	})
}

func TestTraining_RegisterAndStep(t *testing.T) {
	Convey("Given a Training node with registered telemetry", t, func() {
		ctx := context.Background()
		training := NewTraining(ctx, nil)
		training.Transition(runtime.READY)

		Convey("Register should populate the complete metric schema", func() {
			measurement := training.Register()
			So(measurement, ShouldNotBeNil)
			So(measurement.Source, ShouldEqual, "training")
			So(measurement.Label, ShouldEqual, "learner")
			So(measurement.Metadata["peer-interest"], ShouldEqual, "*")

			requiredMetrics := []string{
				"steps", "decisions", "resolved", "confidence", "contrast",
				"surprisal", "ambiguity", "action", "win_rate", "edge",
				"progress", "accuracy", "support", "quality",
			}

			for _, key := range requiredMetrics {
				_, exists := measurement.Metrics[key]
				So(exists, ShouldBeTrue)
			}
		})

		Convey("Step should be inert while unconfident and return the telemetry measurement", func() {
			input := data.NewMeasurement("workspace", map[string]data.Metric[float64]{
				"price": {Label: "price", Raw: 100.0},
			})
			input.Label = "BTC/USD"
			input.SeqIdx = 1

			output := training.Step(input)
			So(output, ShouldNotBeNil)
			So(output.Source, ShouldEqual, "training")
			So(output.Metrics["confidence"].Raw, ShouldEqual, 0)
			So(output.Metrics["action"].Raw, ShouldEqual, 0)
		})
	})
}

func TestTrainingStepReadiness(t *testing.T) {
	Convey("An inactive pipeline node drops input before touching processing state", t, func() {
		node := &Training{System: runtime.NewSystem(t.Context(), "readiness-test")}
		measurement := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: 7}
		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			So(node.Step(measurement), ShouldEqual, measurement)
			So(node.Status(), ShouldEqual, stage)
			So(measurement.SeqIdx, ShouldEqual, 7)
		}
	})
}
