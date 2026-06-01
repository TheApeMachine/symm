package market

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/signal/sentiment"
)

func TestNewStory(t *testing.T) {
	convey.Convey("Given signals already registered on measurements", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		recordPath := filepath.Join(t.TempDir(), "capture.jsonl")

		viper.Set("trading.record.file", recordPath)
		viper.Set("trading.model", "record")
		defer viper.Set("trading.record.file", "")
		defer viper.Set("trading.model", "")

		sentiment.NewSignal(ctx, pool)

		story := NewStory(ctx, pool)

		convey.So(story.subscribers["measurements"], convey.ShouldNotBeNil)
		convey.So(story.subscribers["measurements"].ID, convey.ShouldEqual, storyMeasurementsSubscriberID)

		convey.Convey("It should receive published measurements", func() {
			done := make(chan struct{})

			go func() {
				_ = story.Tick()
				close(done)
			}()

			measurements := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
			measurements.Send(&qpool.QValue[any]{Value: perspectives.Measurement{
				Symbol: "BTC/EUR",
			}})

			time.Sleep(50 * time.Millisecond)

			cancel()
			<-done
			convey.So(story.Close(), convey.ShouldBeNil)

			raw, readErr := os.ReadFile(recordPath)
			convey.So(readErr, convey.ShouldBeNil)
			convey.So(string(raw), convey.ShouldContainSubstring, "BTC/EUR")
		})
	})
}
