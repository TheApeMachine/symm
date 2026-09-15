package strategy

import (
	"context"
	"iter"
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestTraining_Learn(t *testing.T) {
	Convey("Given a Training instance with a cognitive radix trie", t, func() {
		ctx := context.Background()
		training := NewTraining(ctx, nil)

		Convey("When replaying a profitable upward excursion", func() {
			excursion := tables.ExcursionRecord{
				ID:                 "exc-1",
				Symbol:             "BTC/USD",
				Direction:          "upward",
				ClearsFriction:     true,
				PrecursorStartTick: 1,
				AnchorTick:         5,
				ExtremumTick:       8,
				ExitTick:           10,
				PostEndTick:        12,
			}

			ticks := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
			measurements := make([]*data.Measurement[float64], len(ticks))

			for idx, tick := range ticks {
				measurement := data.NewMeasurement("spot", map[string]data.Metric[float64]{
					"price": {Label: "price", Raw: 100.0 + float64(tick)},
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

			Convey("The cognitive engine should have learned associations in the radix trie", func() {
				export := training.engine.ExportTree(nil, 10)
				So(len(export.Branches), ShouldBeGreaterThan, 0)
			})
		})

		Convey("When replaying a downward excursion that does not clear friction", func() {
			excursion := tables.ExcursionRecord{
				ID:                 "exc-2",
				Symbol:             "ETH/USD",
				Direction:          "downward",
				ClearsFriction:     false,
				PrecursorStartTick: 1,
				AnchorTick:         5,
				ExtremumTick:       8,
				ExitTick:           10,
				PostEndTick:        12,
			}

			ticks := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
			measurements := make([]*data.Measurement[float64], len(ticks))

			for idx, tick := range ticks {
				measurement := data.NewMeasurement("spot", map[string]data.Metric[float64]{
					"price": {Label: "price", Raw: 50.0 - float64(tick)},
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
				export := training.engine.ExportTree(nil, 10)
				So(len(export.Branches), ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestTraining_RegisterAndStep(t *testing.T) {
	Convey("Given a Training node with registered telemetry", t, func() {
		ctx := context.Background()
		training := NewTraining(ctx, nil)

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
		})
	})
}

