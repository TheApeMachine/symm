package strategy

import (
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/tests/market"
)

func TestTrainingRegister(t *testing.T) {
	Convey("Training owns its telemetry and declares all upstream producers", t, func() {
		training := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)
		So(training.Register(), ShouldEqual, training.Register())
	})
}

func TestTrainingStep(t *testing.T) {
	Convey("Given a Training instance in INIT stage", t, func() {
		training := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)
		So(training.Status(), ShouldEqual, runtime.INIT)

		Convey("When live market signals arrive, the grid develops coordinates and regions", func() {
			frame := data.NewMeasurement[float64]("BTC/USD", nil)
			frame.Metrics = map[string]data.Metric[float64]{
				"price":  {Label: "price", Raw: 50000.0},
				"volume": {Label: "volume", Raw: 12.5},
			}

			output := training.Step(frame)
			So(output, ShouldNotBeNil)
			So(output.Metrics["price"].Region, ShouldBeGreaterThan, 0)
			So(output.Metrics["volume"].Region, ShouldBeGreaterThan, 0)

			Convey("When grid settles, it transitions to BUSY and runs fragment training", func() {
				for tick := 0; tick < 15; tick++ {
					repeated := data.NewMeasurement[float64]("BTC/USD", nil)
					repeated.Metrics = map[string]data.Metric[float64]{
						"price":  {Label: "price", Raw: 50000.0},
						"volume": {Label: "volume", Raw: 12.5},
					}
					training.Step(repeated)
				}

				So(training.grid.Settled, ShouldBeTrue)
			})
		})

		Convey("When model transitions to READY, paper trading predicts actions", func() {
			training.Transition(runtime.READY)

			token := []byte{1}
			training.trie.Insert(token, []byte(ActionEnter))

			frame := data.NewMeasurement[float64]("BTC/USD", nil)
			frame.Metrics = map[string]data.Metric[float64]{
				"price": {Label: "price", Raw: 50000.0, Region: 1},
			}

			output := training.Step(frame)
			So(output, ShouldNotBeNil)
			So(output.Metrics["action"].Raw, ShouldEqual, 1)
		})
	})
}

func TestTrainingCheckpoint(t *testing.T) {
	Convey("Given a trained checkpoint", t, func() {
		defer os.Remove("grid_checkpoint.json")

		training := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)

		frame := data.NewMeasurement[float64]("BTC/USD", nil)
		frame.Metrics = map[string]data.Metric[float64]{
			"price": {Label: "price", Raw: 50000.0},
		}
		training.Step(frame)

		err := training.SaveCheckpoint()
		So(err, ShouldBeNil)

		Convey("When loading checkpoint into a new instance", func() {
			restored := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)

			loadErr := restored.LoadCheckpoint()
			So(loadErr, ShouldBeNil)
			So(len(restored.grid.Metrics), ShouldBeGreaterThan, 0)
		})
	})
}
