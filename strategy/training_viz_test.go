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

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		src := quoteAt("BTC/USD", 7, 101)

		clone := src.Clone()
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

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		src := data.NewMeasurement[float64]("measurements", nil)
		src.Label = "ETH/USD"
		src.SeqIdx = 3
		src.SetMetric("last", data.Metric[float64]{Label: "last", Raw: 55.5})

		clone := src.Clone()
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

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		src := data.NewMeasurement[float64]("spot_ticker", nil)
		src.Label = "BTC/USD"
		src.SeqIdx = 1
		src.SetMetric("bid", data.Metric[float64]{
			Label: "bid",
			Raw:   100,
			Exact: decimal.NewFromFloat64(100),
		})

		clone := src.Clone()
		training.writeQuote(clone, src)

		_, ok := clone.LookupMetric("price")
		So(ok, ShouldBeFalse)
	})
}

func TestWriteSkillPublishesFragmentAndEnterGrades(t *testing.T) {
	Convey("writeSkill surfaces graded fragment and enter counters", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
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

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
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
			frames: []*data.Measurement[float64]{frame},
		}

		training.supervise(episode, true)
		So(training.fragmentsUp, ShouldEqual, 1)

		training.supervise(episode, true)
		So(training.fragmentsUp, ShouldEqual, 1)
	})
}

func TestWriteSkillPublishesEdgeSamples(t *testing.T) {
	Convey("writeSkill stamps ProfitFraction samples for the edge panel", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.mu.Lock()
		training.recordReturnLocked(tables.ExcursionRecord{
			Direction: "up", ClearsFriction: true, ProfitFraction: 0.0025,
			AnchorTick: 1, ExitTick: 40,
		})
		training.recordReturnLocked(tables.ExcursionRecord{
			Direction: "up", ClearsFriction: false, ProfitFraction: -0.001,
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
	Convey("supervise teaches EXIT on B→C only when up+clearsFriction", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
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

		before := training.engine.Census()
		enterBefore := before["enter"]
		exitBefore := before["exit"]

		training.supervise(heldEpisode{
			record: tables.ExcursionRecord{
				ID:                 "BTC/USD:up:1",
				Symbol:             "BTC/USD",
				Direction:          "up",
				ClearsFriction:     true,
				PrecursorStartTick: 1,
				AnchorTick:         2,
				ExitTick:           4,
			},
			frames: []*data.Measurement[float64]{frameA, frameB, frameC},
		}, true)

		after := training.engine.Census()
		So(after["enter"], ShouldBeGreaterThan, enterBefore)
		So(after["exit"], ShouldBeGreaterThan, exitBefore)
	})
}

func TestHistoricalPredictSurfacesExitWithoutHolding(t *testing.T) {
	Convey("historical predictFrom returns EXIT markers without trader inventory", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
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
	Convey("writeSkill stamps win_rate from skill±1 and edge from ProfitFraction", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.mu.Lock()
		training.skill.Update(1)
		training.skill.Update(1)
		training.skill.Update(-1)
		mean := training.skill.Mean
		training.recordReturnLocked(tables.ExcursionRecord{
			Direction: "up", ClearsFriction: true, ProfitFraction: 0.01,
			AnchorTick: 10, ExitTick: 100,
		})
		training.recordReturnLocked(tables.ExcursionRecord{
			Direction: "down", ClearsFriction: false, ProfitFraction: -0.002,
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

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
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

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
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

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
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
	Convey("fee-failing ups grade skill but do not teach ENTER -1", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
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
			frames: frames,
		}, true)

		So(classCount(training, "enter"), ShouldEqual, before)
		So(training.skill.Count, ShouldEqual, 1)
		So(training.returns.Count, ShouldEqual, 1)
		So(training.returns.Mean, ShouldEqual, -0.0015)
		So(training.histFeeFailUp, ShouldEqual, 1)
		So(training.histClears, ShouldEqual, 0)
	})
}
