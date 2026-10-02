package strategy

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestWriteQuotePrefersBidAskMid(t *testing.T) {
	Convey("writeQuote publishes mid from bid/ask", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		src := quoteAt("BTC/USD", 7, 101)

		clone := data.NewMeasurement[float64]("training", nil)
		clone.Label = src.Label
		clone.SeqIdx = src.SeqIdx
		training.writeQuote(clone, src)

		price, ok := clone.LookupMetric("price")
		So(ok, ShouldBeTrue)
		So(price.Raw, ShouldEqual, 101)
	})
}

func TestWriteQuoteFallsBackToLast(t *testing.T) {
	Convey("writeQuote uses last when bid/ask missing", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		src := data.NewMeasurement[float64]("measurements", nil)
		src.Label = "ETH/USD"
		src.SeqIdx = 3
		src.SetMetric("last", data.Metric[float64]{Label: "last", Raw: 55.5})

		clone := data.NewMeasurement[float64]("training", nil)
		clone.Label = src.Label
		clone.SeqIdx = src.SeqIdx
		training.writeQuote(clone, src)

		price, ok := clone.LookupMetric("price")
		So(ok, ShouldBeTrue)
		So(price.Raw, ShouldEqual, 55.5)
	})
}

func TestWriteQuoteRefusesInventedSingleSide(t *testing.T) {
	Convey("single-sided bid alone is not published as price", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		src := data.NewMeasurement[float64]("spot_ticker", nil)
		src.Label = "BTC/USD"
		src.SeqIdx = 1
		src.SetMetric("bid", data.Metric[float64]{
			Label: "bid",
			Raw:   100,
			Exact: decimal.NewFromFloat64(100),
		})

		clone := data.NewMeasurement[float64]("training", nil)
		clone.Label = src.Label
		clone.SeqIdx = src.SeqIdx
		training.writeQuote(clone, src)

		_, ok := clone.LookupMetric("price")
		So(ok, ShouldBeFalse)
	})
}

func TestWriteSkillPublishesFragmentAndEnterGrades(t *testing.T) {
	Convey("writeSkill surfaces graded fragment and enter counters", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.mu.Lock()
		training.fragmentsUp = 3
		training.fragmentsDown = 2
		training.fragmentsChop = 1
		training.histCorrectEnter = 4
		training.histMissedEnter = 1
		training.histFalseEnter = 2
		training.mu.Unlock()

		clone := data.NewMeasurement[float64]("training:historical", nil)
		training.writeSkill(clone)

		up, _ := clone.LookupMetric("fragments_up")
		So(up.Raw, ShouldEqual, 3)
		down, _ := clone.LookupMetric("fragments_down")
		So(down.Raw, ShouldEqual, 2)
		correct, _ := clone.LookupMetric("hist_correct_enter")
		So(correct.Raw, ShouldEqual, 4)
		falseEnter, _ := clone.LookupMetric("hist_false_enter")
		So(falseEnter.Raw, ShouldEqual, 2)
	})
}

func TestSuperviseCountsFragmentOnce(t *testing.T) {
	Convey("first scored supervise tallies direction; second does not", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()

		episode := heldEpisode{
			record: tables.ExcursionRecord{
				ID:                 "exc-1",
				Symbol:             "BTC/USD",
				Direction:          "up",
				ClearsFriction:     true,
				PrecursorStartTick: 1,
				AnchorTick:         2,
				ExitTick:           3,
			},
			frames: training.LitFrames([]*data.Measurement[float64]{frame}),
		}

		training.supervise(episode, true)
		So(training.fragmentsUp, ShouldEqual, 1)

		training.supervise(episode, true)
		So(training.fragmentsUp, ShouldEqual, 1)
	})
}

func TestWriteSkillPublishesEdgeSamples(t *testing.T) {
	Convey("writeSkill stamps frozen policy-return samples for the edge panel", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.mu.Lock()
		training.recordPolicyReturnLocked(0.0025, tables.ExcursionRecord{
			Direction: "up", ClearsFriction: true,
			AnchorTick: 1, ExitTick: 40,
		})
		training.recordPolicyReturnLocked(-0.001, tables.ExcursionRecord{
			Direction: "up", ClearsFriction: false,
			AnchorTick: 1, ExitTick: 5,
		})
		training.mu.Unlock()

		clone := data.NewMeasurement[float64]("training:historical", nil)
		training.writeSkill(clone)

		samples, ok := clone.GetMetadata("edge_samples")
		So(ok, ShouldBeTrue)
		So(samples, ShouldEqual, "0.0025,-0.001")
		count, ok := clone.LookupMetric("edge_sample_count")
		So(ok, ShouldBeTrue)
		So(count.Raw, ShouldEqual, 2)
		edge, ok := clone.LookupMetric("edge")
		So(ok, ShouldBeTrue)
		So(edge.Raw, ShouldAlmostEqual, (0.0025-0.001)/2, 1e-12)
		// UI basis: 0.00075 * 10000 = 7.5 bp — not skill±1 * 10000.
		So(edge.Raw*10000, ShouldAlmostEqual, 7.5, 1e-9)
	})
}

func TestSuperviseTeachesExitOnlyForWantEnter(t *testing.T) {
	Convey("supervise always teaches EXIT from resolved C on clearing up (ground truth)", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frameA := regionFrame("BTC/USD", 1, 10)
		frameB := regionFrame("BTC/USD", 2, 11)
		frameC := regionFrame("BTC/USD", 3, 12)
		training.grid.Update(frameA)
		training.grid.Update(frameB)
		training.grid.Update(frameC)
		training.grid.Settle()
		training.mu.Lock()
		training.checkpointed = true
		training.mu.Unlock()

		exitCtx := training.signatureOf(framesRange([]*data.Measurement[float64]{frameA, frameB, frameC}, 2, 4))
		So(len(exitCtx), ShouldBeGreaterThan, 0)

		before := training.engine.Census()
		So(before["exit"], ShouldEqual, int32(0))

		training.supervise(heldEpisode{
			record: tables.ExcursionRecord{
				ID:                 "BTC/USD:up:1",
				Symbol:             "BTC/USD",
				Direction:          "up",
				ClearsFriction:     true,
				PrecursorStartTick: 1,
				AnchorTick:         2,
				ExitTick:           4,
				EntryPrice:         100,
				ExitPrice:          101,
			},
			frames: training.LitFrames([]*data.Measurement[float64]{frameA, frameB, frameC}),
		}, true)

		after := training.engine.Census()
		So(after["exit"], ShouldBeGreaterThan, int32(0))
		So(training.frozenAction(exitCtx), ShouldEqual, cognition.ActionExit)
		So(training.returns.Count, ShouldEqual, 1)
		So(training.returns.Mean, ShouldEqual, 0)   // abstain on enter → incomplete/zero edge
		So(training.histMissedExit, ShouldEqual, 1) // cold freeze missed EXIT
	})
}

func TestSuperviseTeachesEnterFromGroundTruth(t *testing.T) {
	Convey("supervise always teaches ENTER from resolved C on clearing up (ground truth)", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		// Bare A→B; peer-enriched B→C so Enter/Exit associations do not collide.
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
		training.mu.Lock()
		training.checkpointed = true
		training.mu.Unlock()

		enterCtx := training.signatureOf(framesBefore(frames, 4))
		So(len(enterCtx), ShouldBeGreaterThan, 0)
		exitCtx := training.signatureOf(framesRange(frames, 4, 8))
		So(len(exitCtx), ShouldBeGreaterThan, 0)
		So(string(exitCtx), ShouldNotEqual, string(enterCtx))
		// Cold freeze must not already be Enter — association comes from teach.
		So(training.frozenAction(enterCtx), ShouldNotEqual, cognition.ActionEnter)

		before := training.engine.Census()
		So(before["enter"], ShouldEqual, int32(0))

		training.supervise(heldEpisode{
			record: tables.ExcursionRecord{
				ID:                 "BTC/USD:up:enter-gt",
				Symbol:             "BTC/USD",
				Direction:          "up",
				ClearsFriction:     true,
				PrecursorStartTick: 1,
				AnchorTick:         4,
				ExitTick:           8,
				EntryPrice:         100,
				ExitPrice:          101,
			},
			frames: training.LitFrames(frames),
		}, true)

		after := training.engine.Census()
		So(after["enter"], ShouldBeGreaterThan, int32(0))
		So(training.frozenAction(enterCtx), ShouldEqual, cognition.ActionEnter)
		// EXIT teach must not overwrite the distinct A→B ENTER association.
		So(training.frozenAction(enterCtx), ShouldNotEqual, cognition.ActionExit)
		So(training.returns.Count, ShouldEqual, 1)
		So(training.returns.Mean, ShouldEqual, 0)    // cold abstain → incomplete/zero edge
		So(training.histMissedEnter, ShouldEqual, 1) // cold freeze missed ENTER
	})
}

func TestHistoricalPredictSurfacesExitWithoutHolding(t *testing.T) {
	Convey("historical predictFrom returns EXIT markers without trader inventory", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()
		training.mu.Lock()
		training.checkpointed = true
		training.mu.Unlock()

		// Seed an exit association so Evaluate can win exit.
		token := training.grid.LitRegions(frame)
		So(len(token), ShouldBeGreaterThan, 0)
		_, err := training.engine.Observe(cognition.Association{
			Context:  append([]byte{}, token...),
			Class:    []byte(cognition.ActionExit),
			Feedback: 1,
			Graded:   true,
		})
		So(err, ShouldBeNil)

		reading := training.predictFrom(token, "BTC/USD", 9, true)
		So(reading.action, ShouldEqual, string(cognition.ActionExit))
		So(reading.exit, ShouldEqual, int64(9))
	})
}

func TestWriteSkillPublishesEdgeWinRate(t *testing.T) {
	Convey("writeSkill stamps win_rate from skill±1 and edge from policy returns", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.mu.Lock()
		training.skill.Update(1)
		training.skill.Update(1)
		training.skill.Update(-1)
		mean := training.skill.Mean
		training.recordPolicyReturnLocked(0.01, tables.ExcursionRecord{
			Direction: "up", ClearsFriction: true,
			AnchorTick: 10, ExitTick: 100,
		})
		training.recordPolicyReturnLocked(-0.002, tables.ExcursionRecord{
			Direction: "down", ClearsFriction: false,
			AnchorTick: 10, ExitTick: 20,
		})
		retMean := training.returns.Mean
		training.mu.Unlock()

		clone := data.NewMeasurement[float64]("training:historical", nil)
		training.writeSkill(clone)

		resolved, ok := clone.LookupMetric("resolved")
		So(ok, ShouldBeTrue)
		So(resolved.Raw, ShouldEqual, 3)

		winRate, ok := clone.LookupMetric("win_rate")
		So(ok, ShouldBeTrue)
		So(winRate.Raw, ShouldAlmostEqual, (mean+1)/2, 0.001)

		edge, ok := clone.LookupMetric("edge")
		So(ok, ShouldBeTrue)
		So(edge.Raw, ShouldEqual, retMean)
		So(edge.Raw, ShouldNotEqual, mean)

		hist, ok := clone.LookupMetric("hist_mean_return")
		So(ok, ShouldBeTrue)
		So(hist.Raw, ShouldEqual, retMean)

		clears, ok := clone.LookupMetric("fragments_clears")
		So(ok, ShouldBeTrue)
		So(clears.Raw, ShouldEqual, 1)
		feeFail, ok := clone.LookupMetric("fragments_fee_fail_up")
		So(ok, ShouldBeTrue)
		So(feeFail.Raw, ShouldEqual, 0)

		steps, ok := clone.LookupMetric("steps")
		So(ok, ShouldBeTrue)
		So(steps.Raw >= 0, ShouldBeTrue)

		decisions, ok := clone.LookupMetric("decisions")
		So(ok, ShouldBeTrue)
		So(decisions.Raw >= 0, ShouldBeTrue)
	})
}

func TestWriteSkillOmitsEdgeWhenNoReturns(t *testing.T) {
	Convey("skill grades alone must not publish edge as ±1 mean", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.recordSkill(false)
		training.recordSkill(false)

		clone := data.NewMeasurement[float64]("training:historical", nil)
		training.writeSkill(clone)

		winRate, ok := clone.LookupMetric("win_rate")
		So(ok, ShouldBeTrue)
		So(winRate.Raw, ShouldEqual, 0)

		_, ok = clone.LookupMetric("edge")
		So(ok, ShouldBeFalse)
		_, ok = clone.LookupMetric("hist_mean_return")
		So(ok, ShouldBeFalse)
	})
}

func TestWriteSkillOmitsEdgeWhenNoOutcomes(t *testing.T) {
	Convey("writeSkill leaves edge/win_rate unset when skill.Count is 0", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		clone := data.NewMeasurement[float64]("training:historical", nil)
		training.writeSkill(clone)

		resolved, ok := clone.LookupMetric("resolved")
		So(ok, ShouldBeTrue)
		So(resolved.Raw, ShouldEqual, 0)

		_, ok = clone.LookupMetric("edge")
		So(ok, ShouldBeFalse)
		_, ok = clone.LookupMetric("win_rate")
		So(ok, ShouldBeFalse)

		_, stage, detail := training.stage()
		So(stage, ShouldEqual, "MODEL DEVELOPMENT")
		_ = detail
	})
}

func TestStageReportsNoGradedOutcomes(t *testing.T) {
	Convey("stage explains empty skill honestly after checkpoint", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()
		training.mu.Lock()
		training.checkpointed = true
		training.mu.Unlock()

		_, stage, detail := training.stage()
		So(stage, ShouldEqual, "HISTORICAL VALIDATION")
		So(detail, ShouldEqual, "no graded skill outcomes yet")
	})
}

func TestSuperviseSilentOnFeeFailingUp(t *testing.T) {
	Convey("fee-failing ups grade skill; abstain edge is 0; no ENTER teach", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()

		frames := []*data.Measurement[float64]{
			regionFrame("BTC/USD", 2, 2),
			regionFrame("BTC/USD", 3, 3),
		}
		before := classCount(training, "enter")
		training.supervise(heldEpisode{
			record: tables.ExcursionRecord{
				ID:                 "BTC/USD:feefail:1",
				Symbol:             "BTC/USD",
				Direction:          "up",
				ClearsFriction:     false,
				ProfitFraction:     -0.0015,
				PrecursorStartTick: 1,
				AnchorTick:         4,
				ExitTick:           6,
			},
			frames: training.LitFrames(frames),
		}, true)

		So(classCount(training, "enter"), ShouldEqual, before)
		So(training.skill.Count, ShouldEqual, 1)
		So(training.returns.Count, ShouldEqual, 1)
		// Abstain (no Enter prediction) → policy return 0, never raw ProfitFraction.
		So(training.returns.Mean, ShouldEqual, 0)
		So(training.histFeeFailUp, ShouldEqual, 1)
		So(training.histClears, ShouldEqual, 0)
	})
}
