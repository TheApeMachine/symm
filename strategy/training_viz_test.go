package strategy

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
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
	Convey("writeSkill stamps observed skill samples for the edge panel", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		training.recordSkill(true)
		training.recordSkill(false)
		training.recordSkill(true)

		clone := data.NewMeasurement[float64]("training:historical", nil)
		training.writeSkill(clone)

		samples, ok := clone.GetMetadata("edge_samples")
		So(ok, ShouldBeTrue)
		So(samples, ShouldEqual, "1,-1,1")
		count, ok := clone.LookupMetric("edge_sample_count")
		So(ok, ShouldBeTrue)
		So(count.Raw, ShouldEqual, 3)
	})
}
