package strategy

import (
	"context"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/tests/market"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestRehearsalResolve(t *testing.T) {
	Convey("Delayed labels update composed counts only after grading the frozen prediction", t, func() {
		training := NewTraining(t.Context(), 1, market.TrainingPrice(t.Context()))
		var context grid.Impulse

		// Build the address from actual owner observations over rises and reversals.
		for _, frame := range market.ImpulseTape("BTC/USD", 6) {
			So(training.Grid.Step(frame), ShouldBeNil)
			reading := training.Grid.Markets["BTC/USD"].Impulse

			if len(reading.Regions) > 0 {
				context = reading
			}
		}
		So(len(context.Regions), ShouldBeGreaterThan, 0)
		learner := training.Rehearsal
		So(learner.capture(&context), ShouldBeNil)
		So(learner.anchors[context.Label].prediction, ShouldEqual, "")

		// These labels specify successive independent resolutions of the same context.
		for index, clears := range []bool{true, false, false, true} {
			context.SeqIdx++
			record := tables.ExcursionRecord{Symbol: context.Label,
				AnchorTick: context.SeqIdx - 1, ExitTick: context.SeqIdx,
				ClearsFriction: clears, ProfitFraction: -0.01}

			if clears {
				record.ProfitFraction = 0.01
			}
			So(learner.resolve(record, &context), ShouldBeNil)
			So(learner.reading.Learned, ShouldEqual, index+1)
			So(learner.capture(&context), ShouldBeNil)
			expected := []Action{ActionEnter, ActionWait, ActionWait, ActionWait}[index]
			So(learner.anchors[context.Label].prediction, ShouldEqual, string(expected))
		}

		So(learner.reading.Predicted, ShouldEqual, 3)
		So(learner.reading.Correct, ShouldEqual, 1)
		So(learner.reading.Entered, ShouldEqual, 1)
		So(learner.reading.Profitable, ShouldEqual, 0)
		So(learner.reading.Return, ShouldEqual, -0.01)

		Convey("An invalid exit does not change learned evidence", func() {
			before := learner.reading
			So(learner.resolve(tables.ExcursionRecord{Symbol: context.Label}, &context), ShouldNotBeNil)
			So(learner.reading, ShouldResemble, before)
		})

		Convey("Another market cannot borrow this market's labels", func() {
			context.Label = "ETH/USD"
			So(learner.capture(&context), ShouldBeNil)
			So(learner.anchors[context.Label].prediction, ShouldEqual, "")
		})
	})
}

func TestRehearsalStep(t *testing.T) {
	Convey("Complete Tee boundaries resolve outcomes and train the same trie used by live inference", t, func() {
		training := NewTraining(t.Context(), 1, market.TrainingPrice(t.Context()))
		training.Transition(runtime.READY)
		frames := market.TrainingTape(6)
		var records []tables.ExcursionRecord
		for _, frame := range frames {
			current := training.Step(frame)
			So(current.Err, ShouldBeNil)
			closed, err := training.Rehearsal.Step(current)
			So(err, ShouldBeNil)
			records = append(records, closed...)
		}
		So(len(records), ShouldBeGreaterThan, 1)
		So(training.Rehearsal.reading.Learned, ShouldBeGreaterThan, 0)
		So(training.measurement.Metrics["decisions"].Raw, ShouldBeGreaterThan, 0)
		key := training.Rehearsal.target
		evidence := nomagique.NewNumber(store.NewKeyQuery[float64](&key, data.ActionRead), training.model)
		So(sequence.Read[float64](evidence.Next(nil)), ShouldBeGreaterThan, 0)
		So(training.Grid.Markets["BTC/USD"].Volume.String(), ShouldEqual,
			training.Rehearsal.space.Markets["BTC/USD"].Volume.String())
		Convey("Duplicate delivery is rejected without another learning update", func() {
			before := training.Rehearsal.reading.Learned
			_, err := training.Rehearsal.Step(frames[len(frames)-1])
			So(err, ShouldNotBeNil)
			So(training.Rehearsal.reading.Learned, ShouldEqual, before)
		})
	})

	Convey("Concurrent inference and cold supervision have independent state", t, func() {
		training := NewTraining(t.Context(), 1, market.TrainingPrice(t.Context()))
		training.Transition(runtime.READY)
		live, cold := market.TrainingTape(6), market.TrainingTape(6)
		var workers sync.WaitGroup
		workers.Go(func() {
			for _, frame := range cold {
				if _, err := training.Rehearsal.Step(frame); err != nil {
					t.Error(err)
					return
				}
			}
		})
		for _, frame := range live {
			So(training.Step(frame).Err, ShouldBeNil)
		}
		workers.Wait()
		So(training.Rehearsal.reading.Learned, ShouldBeGreaterThan, 0)
	})
}

func TestRehearsalReplay(t *testing.T) {
	Convey("Persisted outcomes restore the same model without future labels in inputs", t, func() {
		catalog := tablestest.New(t)
		training := NewTraining(t.Context(), 1, market.TrainingPrice(t.Context()))
		frames := market.TrainingTape(6)
		tee := hindsight.NewStoreTee(t.Context(), "training-test", len(frames)*5)
		tee.Transition(runtime.READY)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		for _, frame := range frames {
			for _, peer := range frame.Peers {
				tee.Push(peer)
			}
			tee.Push(frame)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		So(catalog.Drain(ctx, 1, tee, training.Rehearsal.Step), ShouldBeNil)
		records, replay, err := catalog.Replay(t.Context(), 1)
		So(err, ShouldBeNil)
		So(len(records), ShouldBeGreaterThan, 0)
		restored := NewTraining(t.Context(), 2, market.TrainingPrice(t.Context()))
		So(restored.Rehearsal.Replay(records, replay), ShouldBeNil)
		for _, key := range training.Rehearsal.keys {
			source := nomagique.NewNumber(store.NewKeyQuery[float64](&key, data.ActionRead), training.model)
			restoredRead := nomagique.NewNumber(store.NewKeyQuery[float64](&key, data.ActionRead), restored.model)
			So(tests.CollectSeq[float64](restoredRead.Next(nil)), ShouldResemble, tests.CollectSeq[float64](source.Next(nil)))
		}
		So(restored.Rehearsal.reading, ShouldResemble, training.Rehearsal.reading)
		Convey("Startup rebuilds only compatible completed training intervals", func() {
			So(catalog.RecordRun(t.Context(), tables.Run{Epoch: 1, BuildID: TrainingFormat}), ShouldBeNil)
			restarted := NewTraining(t.Context(), 3, market.TrainingPrice(t.Context()))
			So(restarted.Rehearsal.Restore(catalog), ShouldBeNil)
			So(restarted.Rehearsal.reading, ShouldResemble, training.Rehearsal.reading)
		})
		Convey("Repeated outcome identities fail rather than doubling evidence", func() {
			duplicate := append(records, records[0])
			fresh := NewTraining(t.Context(), 4, market.TrainingPrice(t.Context()))
			So(fresh.Rehearsal.Replay(duplicate, trainingTape(frames)), ShouldNotBeNil)
			So(fresh.Rehearsal.reading.Learned, ShouldEqual, 0)
		})
	})
}

func BenchmarkRehearsalStep(b *testing.B) {
	training := NewTraining(b.Context(), 1, market.TrainingPrice(b.Context()))
	frames := market.TrainingTape(6)
	b.ReportAllocs()
	for index := 0; b.Loop(); index++ {
		frame := frames[index%len(frames)]
		frame.SeqIdx = int64(index + 1)
		metric := frame.Metrics["previous_input"]
		metric.Raw = float64(index)
		frame.Metrics["previous_input"] = metric
		for _, peer := range frame.Peers {
			peer.SeqIdx = frame.SeqIdx
		}
		if _, err := training.Rehearsal.Step(frame); err != nil {
			b.Fatal(err)
		}
	}
}
