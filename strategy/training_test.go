package strategy

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestTrainingSupervision(t *testing.T) {
	Convey("Given a frozen grid and a downward episode", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.001), NewTrader(ctx, nil, nil, nil), nil, nil)
		btc := regionFrame("BTC/USD", 1, 2)
		eth := regionFrame("ETH/USD", 1, 4)
		training.grid.Update(btc)
		training.grid.Update(eth)
		training.grid.Settle()

		btcFrames := []*data.Measurement[float64]{
			regionFrame("BTC/USD", 2, 2),
			regionFrame("BTC/USD", 3, 3),
		}
		episode := heldEpisode{
			record: tables.ExcursionRecord{
				ID:                 "BTC/USD:4:8",
				Symbol:             "BTC/USD",
				Direction:          "down",
				PrecursorStartTick: 1,
				AnchorTick:         4,
				ExitTick:           8,
			},
			frames: btcFrames,
		}
		training.supervise(episode, true)

		So(training.skill.Count, ShouldEqual, 1)
		So(training.skill.Mean, ShouldEqual, -1)
		So(classCount(training, "wait"), ShouldEqual, 0)
		So(classCount(training, "enter"), ShouldBeGreaterThan, 0)
		So(training.signatureOf(btcFrames), ShouldNotResemble, training.signatureOf(append(append([]*data.Measurement[float64]{}, btcFrames...), eth)))
	})
}

func TestTrainingPaperWaitsForSkill(t *testing.T) {
	Convey("Given a checkpointed model whose skill is not positive", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.001), NewTrader(ctx, nil, nil, nil), nil, nil)
		frame := regionFrame("BTC/USD", 1, 3)
		training.grid.Update(frame)
		training.grid.Settle()
		token := training.grid.LitRegions(frame)
		_, err := training.engine.Observe(cognition.Association{
			Context:  append(append([]byte{}, token...), 0),
			Class:    []byte(cognition.ActionEnter),
			Feedback: 1,
			Graded:   true,
		})
		So(err, ShouldBeNil)

		training.mu.Lock()
		training.checkpointed = true
		training.mu.Unlock()
		training.Step(frame)

		So(training.entries["BTC/USD"], ShouldBeNil)
		So(training.paperTrades, ShouldEqual, 0)

		training.mu.Lock()
		training.skill.Update(1)
		training.skill.Update(1)
		training.signatures["BTC/USD"] = nil
		training.mu.Unlock()
		frame.SeqIdx = 2
		training.Step(frame)

		So(len(training.entries["BTC/USD"]), ShouldBeGreaterThan, 0)
	})
}

func TestTrainingCheckpointBlocksSupervision(t *testing.T) {
	Convey("Given no catalog to store the episode", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.001), nil, nil, nil)
		frame := regionFrame("BTC/USD", 2, 1)
		training.grid.Update(frame)
		training.frames["BTC/USD"] = []*data.Measurement[float64]{frame}
		record := &tables.ExcursionRecord{
			ID:                 "BTC/USD:1:2",
			Symbol:             "BTC/USD",
			Direction:          "up",
			ClearsFriction:     true,
			PrecursorStartTick: 1,
			AnchorTick:         1,
			ExitTick:           2,
		}
		training.resolve("BTC/USD", record)

		So(training.graded[record.ID], ShouldBeFalse)
		So(training.skill.Count, ShouldEqual, 0)
		So(training.Error(), ShouldNotBeNil)
	})
}

func regionFrame(symbol string, seq int64, raw float64) *data.Measurement[float64] {
	measurement := data.NewMeasurement[float64]("websocket", nil)
	measurement.Label = symbol
	measurement.SeqIdx = seq
	measurement.SetMetric("mid", data.Metric[float64]{Label: "mid", Raw: raw})

	return measurement
}

func classCount(training *Training, class string) int32 {
	return training.engine.Census()[class]
}
