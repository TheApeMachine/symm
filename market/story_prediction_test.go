package market

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestForwardFeedbackObserve(t *testing.T) {
	convey.Convey("Given a forward feedback learner", t, func() {
		types.ResetSourceFeedback()
		defer types.ResetSourceFeedback()

		base := time.Unix(0, 0).UTC()

		convey.Convey("It should sharpen a source that undercalled realized movement", func() {
			feedback := newForwardFeedback(time.Second, 1)

			err := feedback.Observe(types.Measurement{
				At:         base,
				Symbol:     "BTC/EUR",
				Source:     types.SourceDepthFlow,
				Category:   types.CategoryLoadedImbalance,
				Confidence: 0.25,
				Last:       100,
			})
			convey.So(err, convey.ShouldBeNil)

			err = feedback.Observe(types.Measurement{
				At:         base.Add(2 * time.Second),
				Symbol:     "BTC/EUR",
				Source:     types.SourceDepthFlow,
				Category:   types.CategoryLoadedImbalance,
				Confidence: 0.25,
				Last:       200,
			})
			convey.So(err, convey.ShouldBeNil)

			sourceFeedback := types.CurrentSourceFeedback(types.SourceDepthFlow)
			convey.So(sourceFeedback.Samples, convey.ShouldEqual, 1)
			convey.So(sourceFeedback.Scale, convey.ShouldAlmostEqual, 2, 1e-9)
		})

		convey.Convey("It should soften a source that overcalled a flat horizon", func() {
			feedback := newForwardFeedback(time.Second, 1)

			err := feedback.Observe(types.Measurement{
				At:         base,
				Symbol:     "ETH/EUR",
				Source:     types.SourceExhaustion,
				Category:   types.CategoryMechanicalCollapse,
				Confidence: 0.8,
				Last:       100,
			})
			convey.So(err, convey.ShouldBeNil)

			err = feedback.Observe(types.Measurement{
				At:         base.Add(2 * time.Second),
				Symbol:     "ETH/EUR",
				Source:     types.SourceExhaustion,
				Category:   types.CategoryMechanicalCollapse,
				Confidence: 0.8,
				Last:       100,
			})
			convey.So(err, convey.ShouldBeNil)

			sourceFeedback := types.CurrentSourceFeedback(types.SourceExhaustion)
			convey.So(sourceFeedback.Samples, convey.ShouldEqual, 1)
			convey.So(sourceFeedback.Scale, convey.ShouldEqual, 0)
		})
	})
}

func TestStoryObservePredictionFeedback(t *testing.T) {
	convey.Convey("Given a story with prediction feedback enabled", t, func() {
		testconfig.Load(t)
		types.ResetSourceFeedback()
		defer types.ResetSourceFeedback()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		tempDir := t.TempDir()
		viper.Set("trading.record.file", filepath.Join(tempDir, "capture.jsonl"))
		viper.Set("trading.audit.file", filepath.Join(tempDir, "audit.jsonl"))
		defer viper.Set("trading.record.file", "")
		defer viper.Set("trading.audit.file", "")

		t.Setenv("SYMM_PERSPECTIVES_FILE", filepath.Join(tempDir, "missing.yaml"))

		pool := qpool.NewQ(ctx, 1, 4, nil)
		story := NewStory(ctx, pool)
		convey.So(story, convey.ShouldNotBeNil)
		defer func() {
			convey.So(story.Close(), convey.ShouldBeNil)
		}()

		subscriber := story.ui.Subscribe("test:prediction:gauge", 8)
		now := time.Now().UTC()

		err := story.ingestMeasurement(types.Measurement{
			At:         now,
			Symbol:     "BTC/EUR",
			Source:     types.SourceDepthFlow,
			Category:   types.CategoryLoadedImbalance,
			Strength:   1,
			Confidence: 0.5,
			SNR:        1,
			Last:       100,
		})
		convey.So(err, convey.ShouldBeNil)

		convey.Convey("It should publish a prediction gauge frame and record the measurement", func() {
			select {
			case frame := <-subscriber.Incoming:
				payload, ok := frame.Value.(map[string]any)

				convey.So(ok, convey.ShouldBeTrue)
				convey.So(payload["chart"], convey.ShouldEqual, "gauge")
				convey.So(payload["source"], convey.ShouldEqual, "prediction")
				convey.So(payload["confidence"], convey.ShouldBeGreaterThan, 0)
			case <-time.After(500 * time.Millisecond):
				convey.So("prediction gauge", convey.ShouldBeBlank)
			}

			convey.So(story.recorder.Flush(), convey.ShouldBeNil)
			raw, readErr := os.ReadFile(filepath.Join(tempDir, "capture.jsonl"))

			convey.So(readErr, convey.ShouldBeNil)
			convey.So(strings.Count(string(raw), "\n"), convey.ShouldBeGreaterThanOrEqualTo, 2)
		})
	})
}

func BenchmarkForwardFeedbackObserve(b *testing.B) {
	types.ResetSourceFeedback()
	defer types.ResetSourceFeedback()

	feedback := newForwardFeedback(time.Nanosecond, 0.1)
	at := time.Unix(0, 0).UTC()
	index := 0

	for b.Loop() {
		at = at.Add(2 * time.Nanosecond)
		index++
		price := 100 + float64(index%1000)*0.001

		_ = feedback.Observe(types.Measurement{
			At:         at,
			Symbol:     "BTC/EUR",
			Source:     types.SourceDepthFlow,
			Category:   types.CategoryLoadedImbalance,
			Confidence: 0.5,
			Last:       price,
		})
	}
}
