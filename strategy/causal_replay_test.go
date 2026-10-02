package strategy

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestReplayCausalSupervisesInExitOrder(t *testing.T) {
	Convey("Unsorted excursions still supervise by ExitTick chronology", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.grid.Settle()
		training.mu.Lock()
		training.checkpointed = true
		training.mu.Unlock()

		early := tables.ExcursionRecord{
			ID: "early", Symbol: "BTC/USD", Direction: "down",
			PrecursorStartTick: 10, AnchorTick: 20, ExitTick: 30,
			Epoch: 1,
		}
		late := tables.ExcursionRecord{
			ID: "late", Symbol: "BTC/USD", Direction: "up", ClearsFriction: true,
			PrecursorStartTick: 100, AnchorTick: 200, ExitTick: 1000,
			Epoch: 1,
		}

		tape := make([]*data.Measurement[float64], 0)
		for _, seq := range []int64{10, 15, 20, 25, 30, 100, 150, 200, 500, 1000} {
			tape = append(tape, regionFrame("BTC/USD", seq, float64(seq)))
		}

		So(training.replayCausal(
			[]tables.ExcursionRecord{late, early},
			map[string][]*data.Measurement[float64]{"BTC/USD": tape},
			map[string]bool{},
		), ShouldBeNil)

		So(training.graded["early"], ShouldBeTrue)
		So(training.graded["late"], ShouldBeTrue)
	})
}

func TestReplayDurableDoesNotEnqueue(t *testing.T) {
	Convey("Durable Iceberg records are accepted without persist queue growth", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.grid.Settle()
		training.mu.Lock()
		training.checkpointed = true
		training.mu.Unlock()

		record := tables.ExcursionRecord{
			ID: "durable-1", Symbol: "BTC/USD", Direction: "chop",
			PrecursorStartTick: 1, AnchorTick: 2, ExitTick: 3, Epoch: 1,
		}
		tape := []*data.Measurement[float64]{
			regionFrame("BTC/USD", 1, 1),
			regionFrame("BTC/USD", 2, 2),
			regionFrame("BTC/USD", 3, 3),
		}

		So(training.replayExcursion(&record, tape, false), ShouldBeNil)
		training.mu.Lock()
		So(len(training.queue), ShouldEqual, 0)
		So(training.graded["durable-1"], ShouldBeTrue)
		training.mu.Unlock()
	})
}

func TestGradePaperClosedDoesNotForgetOpen(t *testing.T) {
	Convey("entryCtx retained when exit is not reconciled closed", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.remember("BTC/USD", []byte{1, 2, 3})

		training.gradePaperClosed("BTC/USD", []byte{1, 2, 3}, nil)
		// Mimic predictFrom EXIT gate: only forget when closed.
		closed := training.position("BTC/USD")
		if closed == nil || !closed.IsClosed() {
			// do not forget
		} else {
			training.forget("BTC/USD")
		}

		training.mu.Lock()
		So(len(training.entries["BTC/USD"]), ShouldEqual, 3)
		training.mu.Unlock()
	})
}
