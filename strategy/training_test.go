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

		Convey("When multi-leg market replay tape arrives, the grid clusters metrics sympathetically and forms regions", func() {
			tape := market.TrainingTape(4)
			So(len(tape), ShouldBeGreaterThan, 0)

			for _, frame := range tape {
				output := training.Step(frame)
				So(output, ShouldNotBeNil)
			}

			So(len(training.grid.Metrics), ShouldBeGreaterThan, 1)
			So(training.grid.Settled, ShouldBeTrue)
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

		Convey("When cognitive engine is trained on precursor sequence, it evaluates action and exports tree", func() {
			training.Transition(runtime.READY)

			token := []byte{2}
			_, trainErr := training.engine.Train(token, []byte(ActionEnter), 1.0)
			So(trainErr, ShouldBeNil)

			frame := data.NewMeasurement[float64]("BTC/USD", nil)
			frame.Metrics = map[string]data.Metric[float64]{
				"price": {Label: "price", Raw: 50000.0, Region: 2},
			}

			output := training.Step(frame)
			So(output, ShouldNotBeNil)
			So(output.Metrics["action"].Raw, ShouldEqual, 1)

			tree := training.CognitionTree()
			So(tree.Root, ShouldNotBeNil)
		})
	})
}

func TestTrainingCheckpoint(t *testing.T) {
	Convey("Given a trained checkpoint", t, func() {
		defer os.Remove("grid_checkpoint.json")

		training := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)

		tape := market.TrainingTape(2)
		for _, frame := range tape {
			training.Step(frame)
		}

		err := training.SaveCheckpoint()
		So(err, ShouldBeNil)

		Convey("When loading checkpoint into a new instance", func() {
			restored := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)

			loadErr := restored.LoadCheckpoint()
			So(loadErr, ShouldBeNil)
			So(len(restored.grid.Metrics), ShouldBeGreaterThan, 0)
			So(restored.engine.Root(), ShouldNotBeNil)
		})
	})
}

func TestTrainingStateMetrics(t *testing.T) {
	Convey("Given a Training instance processing market tape", t, func() {
		training := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)

		tape := market.TrainingTape(4)
		var lastOutput *data.Measurement[float64]

		for _, frame := range tape {
			lastOutput = training.Step(frame)
		}

		So(lastOutput, ShouldNotBeNil)

		Convey("Step emits the training state metrics the dashboard reads", func() {
			So(lastOutput.Metrics["stage_code"].Raw, ShouldBeGreaterThanOrEqualTo, 0)
			So(lastOutput.Metrics["steps"].Raw, ShouldBeGreaterThan, 0)
			So(lastOutput.Metrics["decisions"].Raw, ShouldBeGreaterThanOrEqualTo, 0)
			So(lastOutput.Metrics["resolved"].Raw, ShouldBeGreaterThanOrEqualTo, 0)
			So(lastOutput.Metrics["evaluated"].Raw, ShouldBeGreaterThanOrEqualTo, 0)

			_, hasConfidence := lastOutput.Metrics["confidence"]
			So(hasConfidence, ShouldBeTrue)

			_, hasContrast := lastOutput.Metrics["contrast"]
			So(hasContrast, ShouldBeTrue)

			_, hasEdge := lastOutput.Metrics["edge"]
			So(hasEdge, ShouldBeTrue)

			_, hasWinRate := lastOutput.Metrics["win_rate"]
			So(hasWinRate, ShouldBeTrue)

			_, hasTrading := lastOutput.Metrics["trading"]
			So(hasTrading, ShouldBeTrue)
		})

		Convey("Step emits stage blocker in provenance", func() {
			_, hasBlocker := lastOutput.Provenance["stage_blocker"]
			So(hasBlocker, ShouldBeTrue)
		})

		Convey("Step count matches the number of frames processed", func() {
			So(training.steps.Load(), ShouldEqual, uint64(len(tape)))
		})
	})
}

func TestTrainingStageProgression(t *testing.T) {
	Convey("Given a Training instance that has developed its grid", t, func() {
		training := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)

		So(training.stageCode.Load(), ShouldEqual, StageModelDevelopment)

		tape := market.TrainingTape(4)
		for _, frame := range tape {
			training.Step(frame)
		}

		Convey("Once the grid settles and engine learns, stage advances past MODEL DEVELOPMENT", func() {
			So(training.grid.Settled, ShouldBeTrue)

			// Train the engine so it has at least one association.
			_, err := training.engine.Train([]byte{1}, []byte(ActionEnter), 1.0)
			So(err, ShouldBeNil)

			training.advanceStage()
			So(training.stageCode.Load(), ShouldBeGreaterThanOrEqualTo, StageHistoricalValidation)
		})

		Convey("RecordTradeResult updates win counters correctly", func() {
			training.RecordTradeResult(0.05) // win
			training.RecordTradeResult(-0.02) // loss
			training.RecordTradeResult(0.01) // win

			So(training.totalTrades.Load(), ShouldEqual, 3)
			So(training.wins.Load(), ShouldEqual, 2)
			So(training.resolved.Load(), ShouldEqual, 3)
		})
	})
}
