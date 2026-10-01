package strategy

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestFrozenPolicyEdgeAbstainIsZero(t *testing.T) {
	Convey("Abstain frozen decision records zero policy return — never ProfitFraction", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frameA := regionFrame("BTC/USD", 1, 10)
		frameB := regionFrame("BTC/USD", 2, 11)
		training.grid.Update(frameA)
		training.grid.Update(frameB)
		training.grid.Settle()

		training.supervise(heldEpisode{
			record: tables.ExcursionRecord{
				ID: "edge:abstain", Symbol: "BTC/USD", Direction: "up", ClearsFriction: true,
				PrecursorStartTick: 1, AnchorTick: 2, ExitTick: 4,
				EntryPrice: 100, ExitPrice: 102, ProfitFraction: 0.02,
			},
			frames: []*data.Measurement[float64]{frameA, frameB},
		}, true)

		So(training.returns.Count, ShouldEqual, 1)
		So(training.returns.Mean, ShouldEqual, 0)
		So(training.returnSamples[0], ShouldEqual, 0)
	})
}

func TestFrozenPolicyEdgeEnterWithoutExitIsIncomplete(t *testing.T) {
	Convey("ENTER predicted but EXIT never predicted must NOT receive oracle C return", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		// Peer-enriched B→C keeps exitCtx distinct; only ENTER is seeded (no EXIT).
		frames := []*data.Measurement[float64]{
			regionFrame("BTC/USD", 1, 2),
			regionFrame("BTC/USD", 2, 3),
			regionFrameWithPeer("BTC/USD", 4, 10, 3.5, "energy"),
			regionFrameWithPeer("BTC/USD", 5, 11, 2.5, "energy"),
			regionFrameWithPeer("BTC/USD", 7, 12, 1.5, "energy"),
		}
		for _, f := range frames {
			training.grid.Update(f)
		}
		training.grid.Settle()

		enterCtx := training.signatureOf(framesBefore(frames, 4))
		So(len(enterCtx), ShouldBeGreaterThan, 0)
		_, err := training.engine.Observe(cognition.Association{
			Context: append([]byte{}, enterCtx...), Class: []byte(cognition.ActionEnter),
			Feedback: 1, Graded: true,
		})
		So(err, ShouldBeNil)
		So(training.frozenAction(enterCtx), ShouldEqual, cognition.ActionEnter)

		exitCtx := training.signatureOf(framesRange(frames, 4, 8))
		So(len(exitCtx), ShouldBeGreaterThan, 0)
		So(string(exitCtx), ShouldNotEqual, string(enterCtx))
		So(training.frozenAction(exitCtx), ShouldNotEqual, cognition.ActionExit)

		training.supervise(heldEpisode{
			record: tables.ExcursionRecord{
				ID: "edge:enter-no-exit", Symbol: "BTC/USD", Direction: "up", ClearsFriction: true,
				PrecursorStartTick: 1, AnchorTick: 4, ExitTick: 8, PostEndTick: 10,
				EntryPrice: 100, ExitPrice: 101.5, PostEndPrice: 97.0, Fee: 0.001,
			},
			frames: frames,
		}, true)

		So(training.returns.Count, ShouldEqual, 1)
		// Missed exit: forced liquidation at C — NOT 0.0, and NOT oracle exit price.
		So(training.returns.Mean, ShouldBeLessThan, 0)
		So(training.returnSamples[0], ShouldBeLessThan, 0)
		So(training.returns.Mean, ShouldNotEqual, 0)
		So(training.returns.Mean, ShouldNotEqual, 0.015)
	})
}

func TestFrozenPolicyEdgeEnterExitUsesExecutableReturn(t *testing.T) {
	Convey("Frozen ENTER+EXIT scores (ExitPrice-EntryPrice)/EntryPrice as edge", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		// Bare mid on A→B; peer-enriched B→C so Enter/Exit associations do not collide.
		frames := []*data.Measurement[float64]{
			regionFrame("BTC/USD", 1, 2),
			regionFrame("BTC/USD", 2, 3),
			regionFrameWithPeer("BTC/USD", 4, 10, 3.5, "energy"),
			regionFrameWithPeer("BTC/USD", 5, 11, 2.5, "energy"),
			regionFrameWithPeer("BTC/USD", 7, 12, 1.5, "energy"),
		}
		for _, f := range frames {
			training.grid.Update(f)
		}
		training.grid.Settle()

		enterCtx := training.signatureOf(framesBefore(frames, 4))
		So(len(enterCtx), ShouldBeGreaterThan, 0)
		_, err := training.engine.Observe(cognition.Association{
			Context: append([]byte{}, enterCtx...), Class: []byte(cognition.ActionEnter),
			Feedback: 1, Graded: true,
		})
		So(err, ShouldBeNil)
		So(training.frozenAction(enterCtx), ShouldEqual, cognition.ActionEnter)

		exitCtx := training.signatureOf(framesRange(frames, 4, 8))
		So(len(exitCtx), ShouldBeGreaterThan, 0)
		So(string(exitCtx), ShouldNotEqual, string(enterCtx))
		_, err = training.engine.Observe(cognition.Association{
			Context: append([]byte{}, exitCtx...), Class: []byte(cognition.ActionExit),
			Feedback: 1, Graded: true,
		})
		So(err, ShouldBeNil)
		So(training.frozenAction(exitCtx), ShouldEqual, cognition.ActionExit)
		// Exit seed must not overwrite Enter on the distinct A→B context.
		So(training.frozenAction(enterCtx), ShouldEqual, cognition.ActionEnter)

		training.supervise(heldEpisode{
			record: tables.ExcursionRecord{
				ID: "edge:enter-exit", Symbol: "BTC/USD", Direction: "up", ClearsFriction: true,
				PrecursorStartTick: 1, AnchorTick: 4, ExitTick: 8,
				EntryPrice: 100, ExitPrice: 101.5, ProfitFraction: 0.999, // decoy
			},
			frames: frames,
		}, true)

		So(training.returns.Count, ShouldEqual, 1)
		So(training.returns.Mean, ShouldAlmostEqual, 0.015, 1e-12)
		// Must NOT use the decoy ProfitFraction.
		So(training.returns.Mean, ShouldNotEqual, 0.999)
	})
}

func TestSuperviseTeachesOncePerEpisodeID(t *testing.T) {
	Convey("Second supervise of same ID is a no-op (A-offset is separate)", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frames := []*data.Measurement[float64]{regionFrame("BTC/USD", 1, 2), regionFrame("BTC/USD", 2, 3)}
		for _, f := range frames {
			training.grid.Update(f)
		}
		training.grid.Settle()

		ep := heldEpisode{
			record: tables.ExcursionRecord{
				ID: "once:1", Symbol: "BTC/USD", Direction: "down",
				PrecursorStartTick: 1, AnchorTick: 2, ExitTick: 3,
				EntryPrice: 100, ExitPrice: 99,
			},
			frames: frames,
		}
		training.supervise(ep, true)
		firstSkill := training.skill.Count
		firstRet := training.returns.Count
		training.supervise(ep, true)
		So(training.skill.Count, ShouldEqual, firstSkill)
		So(training.returns.Count, ShouldEqual, firstRet)
	})
}

func TestReplayIsolationDefersLiveLearn(t *testing.T) {
	Convey("noteDurable during replaying queues learning without teaching", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()
		training.mu.Lock()
		training.checkpointed = true
		training.replaying = true
		training.mu.Unlock()

		ep := heldEpisode{
			record: tables.ExcursionRecord{
				ID: "live:deferred", Symbol: "BTC/USD", Direction: "up", ClearsFriction: true,
				PrecursorStartTick: 1, AnchorTick: 2, ExitTick: 3,
				EntryPrice: 100, ExitPrice: 101,
			},
			frames: []*data.Measurement[float64]{frame},
		}
		training.noteDurable(ep)
		So(training.skill.Count, ShouldEqual, 0)
		So(training.returns.Count, ShouldEqual, 0)
		training.mu.Lock()
		So(len(training.deferredLearn), ShouldEqual, 1)
		training.replaying = false
		deferred := training.deferredLearn
		training.deferredLearn = nil
		training.mu.Unlock()

		for _, episode := range deferred {
			training.supervise(episode, true)
		}
		So(training.returns.Count, ShouldEqual, 1)
	})
}

func TestPaperCloseOwnsFeedbackFraction(t *testing.T) {
	Convey("gradePaperClosed uses Realized/ClosedCost fraction once", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()
		token := training.grid.LitRegions(frame)
		entryCtx := append(append([]byte{}, token...), 0)

		reg := position.NewRegulator("BTC/USD")
		// Simulate closed economics: realized 2 on basis 100 → +0.02 fraction.
		reg.Realized = decimal.NewFromFloat64(2)
		reg.ClosedBasis = decimal.NewFromFloat64(100)
		reg.Quantity = decimal.NewFromFloat64(0)
		// Force closed state via exported path if available — IsClosed checks quantity.
		So(reg.IsClosed(), ShouldBeTrue)

		training.gradePaperClosed("BTC/USD", entryCtx, reg)
		So(training.paper.Count, ShouldEqual, 1)
		So(training.paper.Mean, ShouldAlmostEqual, 0.02, 1e-12)

		// Second call with same entryCtx must not double-teach.
		training.gradePaperClosed("BTC/USD", entryCtx, reg)
		So(training.paper.Count, ShouldEqual, 1)
	})
}

func TestExecutableEnterReturnFromBidAskFees(t *testing.T) {
	Convey("executableEnterReturn is (exit-entry)/entry from stored executable prices", t, func() {
		So(executableEnterReturn(tables.ExcursionRecord{EntryPrice: 100, ExitPrice: 101}), ShouldAlmostEqual, 0.01, 1e-12)
		So(executableEnterReturn(tables.ExcursionRecord{EntryPrice: 0, ExitPrice: 101}), ShouldEqual, 0)
		So(executableEnterReturn(tables.ExcursionRecord{EntryPrice: 100, ExitPrice: 99}), ShouldAlmostEqual, -0.01, 1e-12)
	})
}

