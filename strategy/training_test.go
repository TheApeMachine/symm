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

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), NewTrader(ctx, nil, nil, nil), nil, nil)
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

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), NewTrader(ctx, nil, nil, nil), nil, nil)
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

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
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

// regionFrameWithPeer adds a source-keyed peer metric so LitRegions differs from a bare mid frame.
func regionFrameWithPeer(symbol string, seq int64, mid, peerRaw float64, peerMetric string) *data.Measurement[float64] {
	measurement := regionFrame(symbol, seq, mid)
	peer := data.NewMeasurement[float64]("resonance", map[string]data.Metric[float64]{
		peerMetric: {Label: peerMetric, Raw: peerRaw},
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
		raw := data.NewMeasurement[float64]("websocket", map[string]data.Metric[float64]{
			"bid": {Raw: 1},
			"ask": {Raw: 2},
		})
		raw.Label = "BTC/USD"
		raw.SeqIdx = 7

		rich := data.NewMeasurement[float64]("resonance", map[string]data.Metric[float64]{
			"energy":   {Raw: 3},
			"surprise": {Raw: 4},
			"contrast": {Raw: 5},
		})
		rich.Label = "BTC/USD"
		rich.SeqIdx = 7

		hawkes := data.NewMeasurement[float64]("hawkes:trade", map[string]data.Metric[float64]{
			"intensity": {Raw: 9},
		})
		hawkes.Label = "BTC/USD"
		hawkes.SeqIdx = 7

		overlay := rich.Clone()
		overlay.Source = "training:live"
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

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), NewTrader(ctx, nil, nil, nil), nil, nil)

		// Bare mid on A→B; peer-enriched B→C so signatures do not collide/dedupe.
		frames := []*data.Measurement[float64]{
			regionFrame("BTC/USD", 1, 2),
			regionFrame("BTC/USD", 2, 3),
			regionFrameWithPeer("BTC/USD", 4, 10, 1.5, "energy"),
			regionFrameWithPeer("BTC/USD", 5, 11, 2.5, "energy"),
			regionFrameWithPeer("BTC/USD", 7, 12, 3.5, "energy"),
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
			frames: frames,
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

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
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
			frames: []*data.Measurement[float64]{
				regionFrame("BTC/USD", 1, 2),
				regionFrame("BTC/USD", 2, 2),
				regionFrame("BTC/USD", 3, 3),
			},
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

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.gradePaperClosed("BTC/USD", []byte{1, 2, 3}, nil)
		So(training.paper.Count, ShouldEqual, 0)
		So(training.skill.Count, ShouldEqual, 0)
	})
}


func TestTeachMarksModelDirty(t *testing.T) {
	Convey("Given a graded teach after the grid is durable", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
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
