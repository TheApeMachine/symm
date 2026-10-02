package strategy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestTrainingSupervision(t *testing.T) {
	Convey("Given a frozen grid and a downward episode", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), NewTrader(ctx, nil, nil, nil), nil, nil)
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
			frames: training.LitFrames(btcFrames),
		}
		training.supervise(episode, true)

		// Cold freeze abstains (not Enter) — correct for a downward episode, so
		// skill samples +1. Ground truth still teaches ENTER avoidance (-1).
		So(training.skill.Count, ShouldEqual, 1)
		So(training.skill.Mean, ShouldEqual, 1)
		So(classCount(training, "enter"), ShouldBeGreaterThan, int32(0))
		So(training.returns.Count, ShouldEqual, 1)
		So(training.returns.Mean, ShouldEqual, 0)
		So(training.signatureOf(btcFrames), ShouldNotResemble, training.signatureOf(append(append([]*data.Measurement[float64]{}, btcFrames...), eth)))
	})
}

func TestTrainingPaperWaitsForSkill(t *testing.T) {
	Convey("Given a checkpointed model whose skill is not positive", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), NewTrader(ctx, nil, nil, nil), nil, nil)
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

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frame := regionFrame("BTC/USD", 2, 1)
		training.grid.Update(frame)
		training.frames["BTC/USD"] = training.LitFrames([]*data.Measurement[float64]{frame})
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
	measurement.SetMetric("mid", data.Metric[float64]{Label: "mid", Raw: raw, Standardized: &raw})

	return measurement
}

// regionFrameWithPeer adds a source-keyed peer metric so LitRegions differs from a bare mid frame.
func regionFrameWithPeer(symbol string, seq int64, mid, peerRaw float64, peerMetric string) *data.Measurement[float64] {
	measurement := regionFrame(symbol, seq, mid)
	peer := data.NewMeasurement("resonance", map[string]data.Metric[float64]{
		peerMetric: {Label: peerMetric, Raw: peerRaw, Standardized: &peerRaw},
	})
	peer.Label = symbol
	peer.SeqIdx = seq
	measurement.Peers = []*data.Measurement[float64]{peer}
	return measurement
}

func classCount(training *Training, class string) int32 {
	return training.engine.Census()[class]
}

func TestCanonicalObservationsIngressPlusSourceKeyedPeers(t *testing.T) {
	Convey("Given ingress + producer + training overlay at one seq", t, func() {
		raw := data.NewMeasurement("websocket", map[string]data.Metric[float64]{
			"bid": {Raw: 1},
			"ask": {Raw: 2},
		})
		raw.Label = "BTC/USD"
		raw.SeqIdx = 7

		rich := data.NewMeasurement("resonance", map[string]data.Metric[float64]{
			"energy":   {Raw: 3},
			"surprise": {Raw: 4},
			"contrast": {Raw: 5},
		})
		rich.Label = "BTC/USD"
		rich.SeqIdx = 7

		hawkes := data.NewMeasurement("hawkes:trade", map[string]data.Metric[float64]{
			"intensity": {Raw: 9},
		})
		hawkes.Label = "BTC/USD"
		hawkes.SeqIdx = 7

		overlay := data.NewMeasurement[float64]("training:live", nil)
		overlay.Label = "BTC/USD"
		overlay.SeqIdx = 7
		overlay.WriteMetric("stage_code", 1)

		kept := canonicalObservations([]*data.Measurement[float64]{raw, overlay, rich, hawkes})
		So(len(kept), ShouldEqual, 1)
		So(kept[0].Source, ShouldEqual, "websocket")
		So(len(kept[0].Metrics), ShouldEqual, 2) // ingress bid/ask only
		So(len(kept[0].Peers), ShouldEqual, 2)
		So(kept[0].Peers[0].Source, ShouldEqual, "hawkes:trade")
		So(kept[0].Peers[1].Source, ShouldEqual, "resonance")
	})
}

func TestSuperviseScoresFrozenPrediction(t *testing.T) {
	Convey("Given a trie that wrongly prefers Enter+Exit before a losing excursion", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), NewTrader(ctx, nil, nil, nil), nil, nil)

		// Bare mid on A→B; peer-enriched B→C so signatures do not collide/dedupe.
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
			Context:  append([]byte{}, enterCtx...),
			Class:    []byte(cognition.ActionEnter),
			Feedback: 1,
			Graded:   true,
		})
		So(err, ShouldBeNil)
		So(training.frozenAction(enterCtx), ShouldEqual, cognition.ActionEnter)

		exitCtx := training.signatureOf(framesRange(frames, 4, 8))
		So(len(exitCtx), ShouldBeGreaterThan, 0)
		So(string(exitCtx), ShouldNotEqual, string(enterCtx))
		_, err = training.engine.Observe(cognition.Association{
			Context:  append([]byte{}, exitCtx...),
			Class:    []byte(cognition.ActionExit),
			Feedback: 1,
			Graded:   true,
		})
		So(err, ShouldBeNil)
		So(training.frozenAction(exitCtx), ShouldEqual, cognition.ActionExit)
		So(training.frozenAction(enterCtx), ShouldEqual, cognition.ActionEnter)

		episode := heldEpisode{
			record: tables.ExcursionRecord{
				ID:                 "BTC/USD:pred:1",
				Symbol:             "BTC/USD",
				Direction:          "down",
				PrecursorStartTick: 1,
				AnchorTick:         4,
				ExitTick:           8,
				EntryPrice:         100.1,
				ExitPrice:          99.0,
			},
			frames: training.LitFrames(frames),
		}
		training.supervise(episode, true)

		So(training.skill.Count, ShouldEqual, 1)
		So(training.skill.Mean, ShouldEqual, -1)
		// Completed ENTER→EXIT hypo on a loser → negative policy return, not raw skip.
		So(training.returns.Count, ShouldEqual, 1)
		So(training.returns.Mean, ShouldBeLessThan, 0)
	})
}

func TestAugmentRetainsEpisodeIdentity(t *testing.T) {
	Convey("Given one episode with two eligible A offsets", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()

		episode := heldEpisode{
			record: tables.ExcursionRecord{
				ID:                 "BTC/USD:aug:1",
				Symbol:             "BTC/USD",
				Direction:          "up",
				ClearsFriction:     true,
				PrecursorStartTick: 1,
				AnchorTick:         4,
				ExitTick:           8,
			},
			frames: training.LitFrames([]*data.Measurement[float64]{
				regionFrame("BTC/USD", 1, 2),
				regionFrame("BTC/USD", 2, 2),
				regionFrame("BTC/USD", 3, 3),
			}),
		}

		before, err := training.engine.Snapshot()
		So(err, ShouldBeNil)
		_ = before

		training.replayVaried(episode)
		training.replayVaried(episode)
		training.replayVaried(episode)
		training.replayVaried(episode)

		So(len(training.augmented), ShouldEqual, 3)
		So(training.skill.Count, ShouldEqual, 0)
	})
}

func TestGradePaperClosedRequiresReconciledExit(t *testing.T) {
	Convey("Given an open regulator, gradePaperClosed is a no-op", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.gradePaperClosed("BTC/USD", []byte{1, 2, 3}, nil)
		So(training.paper.Count, ShouldEqual, 0)
		So(training.skill.Count, ShouldEqual, 0)
	})
}

func TestTeachMarksModelDirty(t *testing.T) {
	Convey("Given a graded teach after the grid is durable", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()
		token := training.grid.LitRegions(frame)
		So(len(token), ShouldBeGreaterThan, 0)

		training.mu.Lock()
		So(training.modelRevision, ShouldEqual, 0)
		training.mu.Unlock()

		training.teach(append(append([]byte{}, token...), 0), string(cognition.ActionEnter), 1)

		training.mu.Lock()
		dirty := training.modelRevision > 0
		training.mu.Unlock()
		So(dirty, ShouldBeTrue)
	})
}

func TestSuperviseCausalEdge(t *testing.T) {
	Convey("Given a cold engine with an ENTER prediction but no EXIT prediction", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		price := priced(t, "BTC/USD", 0.1)
		training := NewTraining(ctx, data.NewArenaOwner(32), 1, price, nil, nil, nil)

		enterFrame := data.NewMeasurement[float64]("source", nil)
		enterFrame.Label = "BTC/USD"
		enterFrame.SeqIdx = 0
		val := 1.0
		enterFrame.SetMetric("x", data.Metric[float64]{Label: "x", Raw: val, Standardized: &val})

		exitFrame := data.NewMeasurement[float64]("source", nil)
		exitFrame.Label = "BTC/USD"
		exitFrame.SeqIdx = 1
		val2 := 2.0
		exitFrame.SetMetric("y", data.Metric[float64]{Label: "y", Raw: val2, Standardized: &val2})

		training.grid.Update(enterFrame)
		training.grid.Update(exitFrame)
		training.grid.Settle()

		// Manually teach ENTER so it predicts ENTER
		enterToken := append(training.grid.LitRegions(enterFrame), 0)
		training.teach(enterToken, string(cognition.ActionEnter), 1.0)

		episode := heldEpisode{
			record: tables.ExcursionRecord{
				ID:                 "BTC/USD:1",
				Symbol:             "BTC/USD",
				Direction:          "up",
				ClearsFriction:     true,
				EntryPrice:         100.0,
				ExitPrice:          110.0,
				PostEndPrice:       95.0,
				Fee:                0.001,
				PrecursorStartTick: 0,
				AnchorTick:         1,
				ExitTick:           2,
				PostEndTick:        3,
			},
			frames: training.LitFrames([]*data.Measurement[float64]{
				enterFrame,
				exitFrame,
			}),
		}

		training.supervise(episode, true)

		// Did not know EXIT yet -> missed exit -> forced liquidation at C (not 0.0!)
		So(training.returns.Count, ShouldEqual, 1)
		firstReturn := training.returnSamples[0]
		So(firstReturn, ShouldBeLessThan, 0)

		// But supervise taught the exit frame as ActionExit
		episode2 := episode
		episode2.record.ID = "BTC/USD:2"
		training.supervise(episode2, true)

		// Now cognition knows EXIT: predicts EXIT and earns full positive return
		So(training.returns.Count, ShouldEqual, 2)
		secondReturn := training.returnSamples[1]
		So(secondReturn, ShouldBeGreaterThan, 0)
		So(secondReturn, ShouldBeGreaterThan, firstReturn)
	})
}

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

func TestFramesRangeExitIsBToC(t *testing.T) {
	Convey("framesRange keeps [B, C) for exit learning", t, func() {
		frames := []*data.Measurement[float64]{
			{SeqIdx: 1}, // before B
			{SeqIdx: 4}, // B
			{SeqIdx: 5},
			{SeqIdx: 7}, // last before C
			{SeqIdx: 8}, // C excluded
		}
		got := framesRange(frames, 4, 8)
		So(len(got), ShouldEqual, 3)
		So(got[0].SeqIdx, ShouldEqual, 4)
		So(got[2].SeqIdx, ShouldEqual, 7)

		beforeB := framesBefore(frames, 4)
		So(len(beforeB), ShouldEqual, 1)
		So(beforeB[0].SeqIdx, ShouldEqual, 1)
	})
}

func TestSuperviseExitUsesBToCNotFullPath(t *testing.T) {
	Convey("Cold engine: one eligible B→C episode teaches EXIT without seed prediction", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)

		frames := []*data.Measurement[float64]{
			regionFrame("BTC/USD", 1, 2),
			regionFrame("BTC/USD", 2, 3),
			regionFrame("BTC/USD", 4, 10),
			regionFrame("BTC/USD", 5, 11),
			regionFrame("BTC/USD", 7, 12),
		}
		for _, f := range frames {
			training.grid.Update(f)
		}
		training.grid.Settle()

		bToC := framesRange(frames, 4, 8)
		aToC := framesBefore(frames, 8)
		So(len(bToC), ShouldBeLessThan, len(aToC))
		So(len(bToC), ShouldEqual, 3)

		exitCtx := training.signatureOf(bToC)
		So(len(exitCtx), ShouldBeGreaterThan, 0)

		// Cold engine: zero EXIT associations — freeze must not already predict EXIT.
		before := training.engine.Census()
		So(before["exit"], ShouldEqual, int32(0))
		So(training.frozenAction(exitCtx), ShouldNotEqual, cognition.ActionExit)

		episode := heldEpisode{
			record: tables.ExcursionRecord{
				ID:                 "BTC/USD:exit:1",
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
		}
		training.supervise(episode, true)

		// Ground-truth teach from resolved C installs EXIT even when freeze missed.
		after := training.engine.Census()
		So(after["exit"], ShouldBeGreaterThan, int32(0))
		So(training.frozenAction(exitCtx), ShouldEqual, cognition.ActionExit)
		// Cold freeze missed EXIT → grade as miss; skill still recorded.
		So(training.histMissedExit, ShouldEqual, 1)
		So(training.histCorrectExit, ShouldEqual, 0)
		So(training.skill.Count, ShouldBeGreaterThan, 0)
	})
}

func TestWriteEpisodeStampsMarksBeforeExitTick(t *testing.T) {
	Convey("Historical developing frames carry A/B/C marks", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		clone := regionFrame("BTC/USD", 5, 2)
		record := &tables.ExcursionRecord{
			PrecursorStartTick: 1,
			AnchorTick:         4,
			ExitTick:           8,
			Direction:          "up",
			ClearsFriction:     true,
			ProfitFraction:     0.01,
		}

		training.writeEpisode(clone, record)
		event, _ := clone.GetMetadata("excursion_event")
		So(event, ShouldEqual, "developing")
		markA := clone.GetMetric("mark_a")
		So(markA.Raw, ShouldEqual, 1)
		markB := clone.GetMetric("mark_b")
		So(markB.Raw, ShouldEqual, 4)
		markC := clone.GetMetric("mark_c")
		So(markC.Raw, ShouldEqual, 8)

		done := regionFrame("BTC/USD", 8, 2)
		training.writeEpisode(done, record)
		doneEvent, _ := done.GetMetadata("excursion_event")
		So(doneEvent, ShouldEqual, "completed")
	})
}

func TestStageBlocksPaperDuringHistoricalReplay(t *testing.T) {
	Convey("replaying keeps HISTORICAL VALIDATION even with positive skill", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()

		training.recordSkill(true)
		training.recordSkill(true)

		training.mu.Lock()
		training.checkpointed = true
		training.replaying = true
		training.mu.Unlock()

		So(training.paperOpen(), ShouldBeFalse)
		_, stage, detail := training.stage()
		So(stage, ShouldEqual, "HISTORICAL VALIDATION")
		So(detail, ShouldEqual, "historical replay in progress")

		training.mu.Lock()
		training.replaying = false
		training.mu.Unlock()
		So(training.paperOpen(), ShouldBeTrue)
	})
}

func TestStageSurfacesPersistError(t *testing.T) {
	Convey("Given a checkpointed model with a failed Iceberg persist", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()

		training.mu.Lock()
		training.checkpointed = true
		training.blocked = true
		training.persistErr = errors.New("iceberg unavailable")
		training.mu.Unlock()

		_, stage, detail := training.stage()
		So(stage, ShouldEqual, "HISTORICAL VALIDATION")
		So(detail, ShouldContainSubstring, "durability blocked")
		So(detail, ShouldContainSubstring, "iceberg unavailable")
	})
}

func TestFinishWriteRequeuesWithoutClearingBlocked(t *testing.T) {
	Convey("Given CommitReady failure, the batch is requeued and stays blocked", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, data.NewArenaOwner(32), 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		batch := []heldEpisode{{
			record: tables.ExcursionRecord{
				ID:         "BTC/USD:1:2",
				Symbol:     "BTC/USD",
				AnchorTick: 1,
				ExitTick:   2,
			},
		}}

		training.mu.Lock()
		training.writing = true
		training.blocked = true
		training.mu.Unlock()

		training.finishWrite(batch, errors.New("append failed"))

		training.mu.Lock()
		defer training.mu.Unlock()
		So(training.writing, ShouldBeFalse)
		So(training.blocked, ShouldBeTrue)
		So(len(training.queue), ShouldEqual, 1)
		So(training.persistErr, ShouldNotBeNil)
	})
}

func TestPersistLoopDrainsWithCatalog(t *testing.T) {
	Convey("Given a real catalog, enqueue clears durability blocked after CommitReady", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tablestest.New(t)
		training := NewTraining(ctx, data.NewArenaOwner(32), 7, priced(t, "BTC/USD", 0.1), nil, catalog, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()
		training.mu.Lock()
		training.checkpointed = true
		training.mu.Unlock()

		err := training.enqueue(heldEpisode{
			record: tables.ExcursionRecord{
				ID:                 "BTC/USD:drain:1",
				Symbol:             "BTC/USD",
				Direction:          "up",
				PrecursorStartTick: 1,
				AnchorTick:         2,
				ExitTick:           3,
				Epoch:              7,
			},
		})
		So(err, ShouldBeNil)

		deadline := time.Now().Add(5 * time.Second)
		for {
			training.mu.Lock()
			blocked := training.blocked
			writing := training.writing
			qlen := len(training.queue)
			perr := training.persistErr
			training.mu.Unlock()

			if !blocked && !writing && qlen == 0 && perr == nil {
				break
			}

			if time.Now().After(deadline) {
				t.Fatalf("still blocked writing=%v queue=%d err=%v", writing, qlen, perr)
			}

			time.Sleep(20 * time.Millisecond)
		}

		_, _, detail := training.stage()
		So(detail, ShouldNotContainSubstring, "durability blocked")

		rows, err := catalog.Excursions(ctx, 7, nil)
		So(err, ShouldBeNil)
		So(len(rows), ShouldBeGreaterThan, 0)
	})
}

func TestWaitDrainedDoesNotBailOnTransientPersistErr(t *testing.T) {
	Convey("Given a requeued failed batch that later succeeds, waitDrained returns nil", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tablestest.New(t)
		training := NewTraining(ctx, data.NewArenaOwner(32), 9, priced(t, "BTC/USD", 0.1), nil, catalog, nil)

		batch := []heldEpisode{{
			record: tables.ExcursionRecord{
				ID:         "BTC/USD:wait:1",
				Symbol:     "BTC/USD",
				AnchorTick: 1,
				ExitTick:   2,
				Epoch:      9,
			},
		}}

		// Simulate a failed write that requeues (the old waitDrained bailed here).
		training.finishWrite(batch, errors.New("transient"))

		training.mu.Lock()
		So(training.persistErr, ShouldNotBeNil)
		So(len(training.queue), ShouldEqual, 1)
		training.mu.Unlock()

		// Wake persistLoop to retry against a healthy catalog.
		select {
		case training.wake <- struct{}{}:
		default:
		}

		done := make(chan error, 1)
		go func() { done <- training.waitDrained() }()

		select {
		case err := <-done:
			So(err, ShouldBeNil)
		case <-time.After(5 * time.Second):
			t.Fatal("waitDrained timed out")
		}
	})
}

func TestCommitExcursionsTimesOutWhenGateHeld(t *testing.T) {
	Convey("Given a held commit gate, CommitExcursions returns ctx error and finishWrite clears writing", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tablestest.New(t)
		held := make(chan struct{})
		release := make(chan struct{})

		go func() {
			_ = catalog.WithCommit(context.Background(), func() error {
				close(held)
				<-release
				return nil
			})
		}()
		<-held

		training := NewTraining(ctx, data.NewArenaOwner(32), 11, priced(t, "BTC/USD", 0.1), nil, catalog, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()
		training.mu.Lock()
		training.checkpointed = true
		training.writing = true
		training.blocked = true
		training.mu.Unlock()

		writer := tables.NewWriter(catalog, 11)
		writer.AddExcursion(tables.ExcursionRecord{
			ID: "BTC/USD:timeout:1", Symbol: "BTC/USD", AnchorTick: 1, ExitTick: 2, Epoch: 11,
		})
		commitCtx, commitCancel := context.WithTimeout(ctx, 80*time.Millisecond)
		err := writer.CommitExcursions(commitCtx)
		commitCancel()
		So(err, ShouldNotBeNil)

		training.finishWrite([]heldEpisode{{
			record: tables.ExcursionRecord{ID: "BTC/USD:timeout:1", Symbol: "BTC/USD"},
		}}, err)

		training.mu.Lock()
		writing := training.writing
		blocked := training.blocked
		perr := training.persistErr
		training.mu.Unlock()
		So(writing, ShouldBeFalse)
		So(blocked, ShouldBeTrue)
		So(perr, ShouldNotBeNil)

		_, _, detail := training.stage()
		So(detail, ShouldContainSubstring, "durability blocked")
		So(detail, ShouldNotEqual, "durability blocked: writing")

		close(release)
	})
}

func TestCommitExcursionsBoundedSurfacesTimeout(t *testing.T) {
	Convey("Given a held commit gate, bounded commit sets persistErr before Append returns", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tablestest.New(t)
		held := make(chan struct{})
		release := make(chan struct{})

		go func() {
			_ = catalog.WithCommit(context.Background(), func() error {
				close(held)
				<-release
				return nil
			})
		}()
		<-held

		training := NewTraining(ctx, data.NewArenaOwner(32), 13, priced(t, "BTC/USD", 0.1), nil, catalog, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()
		training.mu.Lock()
		training.checkpointed = true
		training.writing = true
		training.blocked = true
		training.mu.Unlock()

		writer := tables.NewWriter(catalog, 13)
		writer.AddExcursion(tables.ExcursionRecord{
			ID: "BTC/USD:bounded:1", Symbol: "BTC/USD", AnchorTick: 1, ExitTick: 2, Epoch: 13,
		})

		commitCtx, commitCancel := context.WithTimeout(ctx, 80*time.Millisecond)
		errCh := make(chan error, 1)
		go func() {
			errCh <- training.commitExcursionsBounded(commitCtx, writer)
		}()

		deadline := time.Now().Add(2 * time.Second)
		for {
			training.mu.Lock()
			perr := training.persistErr
			training.mu.Unlock()
			if perr != nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("persistErr not set while Append blocked on gate")
			}
			time.Sleep(10 * time.Millisecond)
		}

		_, _, detail := training.stage()
		So(detail, ShouldContainSubstring, "durability blocked")
		So(detail, ShouldNotEqual, "durability blocked: writing")

		close(release)
		commitCancel()
		<-errCh
	})
}

func TestEnqueuePreservesPersistErrWhileWriting(t *testing.T) {
	Convey("enqueue must not clear persistErr while orphan Append still writing", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tablestest.New(t)
		training := NewTraining(ctx, data.NewArenaOwner(32), 17, priced(t, "BTC/USD", 0.1), nil, catalog, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()

		training.mu.Lock()
		training.checkpointed = true
		training.writing = true
		training.blocked = true
		training.persistErr = errors.New("training: excursion commit exceeded 45s")
		training.mu.Unlock()

		err := training.enqueue(heldEpisode{
			record: tables.ExcursionRecord{
				ID: "BTC/USD:enqueue:1", Symbol: "BTC/USD", AnchorTick: 1, ExitTick: 2, Epoch: 17,
			},
		})
		So(err, ShouldBeNil)

		training.mu.Lock()
		perr := training.persistErr
		writing := training.writing
		training.mu.Unlock()
		So(writing, ShouldBeTrue)
		So(perr, ShouldNotBeNil)
		So(perr.Error(), ShouldContainSubstring, "exceeded")

		_, _, detail := training.stage()
		So(detail, ShouldContainSubstring, "durability blocked")
		So(detail, ShouldNotEqual, "durability blocked: writing")
	})
}
