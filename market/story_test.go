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
	"github.com/theapemachine/symm/focus"
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

		story := NewStory(ctx, pool, focus.NewSet())

		convey.So(story.subscribers["measurements"], convey.ShouldNotBeNil)
		convey.So(story.subscribers["measurements"].ID, convey.ShouldEqual, storyMeasurementsSubscriberID)

		convey.Convey("It should receive published measurements", func() {
			done := make(chan struct{})

			go func() {
				_ = story.Tick()
				close(done)
			}()

			story.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: perspectives.Measurement{
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

func TestStoryGaugeMeanConfidence(t *testing.T) {
	convey.Convey("Given a story publishing gauge frames", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		story := NewStory(ctx, pool, focus.NewSet())

		uiSubscriber := story.ui.Subscribe("test:story:gauge", 8)
		done := make(chan struct{})

		go func() {
			_ = story.Tick()
			close(done)
		}()

		measurements := story.broadcasts["measurements"]
		measurements.Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:     "BTC/EUR",
			Source:     perspectives.SourceHawkes,
			Confidence: 1.0,
		}})
		measurements.Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:     "ETH/EUR",
			Source:     perspectives.SourceHawkes,
			Confidence: 0.2,
		}})

		convey.Convey("It should flush gauge clarity for each ingested measurement", func() {
			var last map[string]any
			deadline := time.After(500 * time.Millisecond)

		drain:
			for {
				select {
				case row := <-uiSubscriber.Incoming:
					parsed, ok := row.Value.(map[string]any)

					if ok && parsed["source"] == "hawkes" {
						last = parsed
					}
				case <-deadline:
					break drain
				}
			}

			convey.So(last, convey.ShouldNotBeNil)
			convey.So(last["source"], convey.ShouldEqual, "hawkes")
			convey.So(last["confidence"], convey.ShouldAlmostEqual, 0.2, 1e-9)
			convey.So(last["count"], convey.ShouldEqual, 1)
		})

		cancel()
		<-done
		convey.So(story.Close(), convey.ShouldBeNil)
	})
}

func TestStoryGaugeTracksLatestMeasurement(t *testing.T) {
	convey.Convey("Given successive gauge readings for one source", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		story := NewStory(ctx, pool, focus.NewSet())

		uiSubscriber := story.ui.Subscribe("test:story:gauge-latest", 8)
		done := make(chan struct{})

		go func() {
			_ = story.Tick()
			close(done)
		}()

		measurements := story.broadcasts["measurements"]
		measurements.Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:     "BTC/EUR",
			Source:     perspectives.SourceHawkes,
			Confidence: 1.0,
		}})
		measurements.Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:     "BTC/EUR",
			Source:     perspectives.SourceHawkes,
			Confidence: 0.1,
		}})

		var last map[string]any
		deadline := time.After(500 * time.Millisecond)

	drain:
		for {
			select {
			case row := <-uiSubscriber.Incoming:
				parsed, ok := row.Value.(map[string]any)

				if ok && parsed["source"] == "hawkes" {
					last = parsed
				}
			case <-deadline:
				break drain
			}
		}

		convey.Convey("It should publish the latest reading instead of freezing on the first", func() {
			convey.So(last, convey.ShouldNotBeNil)
			convey.So(last["confidence"], convey.ShouldAlmostEqual, 0.1, 1e-9)
			convey.So(last["count"], convey.ShouldEqual, 1)
		})

		cancel()
		<-done
		convey.So(story.Close(), convey.ShouldBeNil)
	})
}

func TestStoryTickSurvivesBadMeasurement(t *testing.T) {
	convey.Convey("Given a bad measurement followed by a valid one", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		story := NewStory(ctx, pool, focus.NewSet())

		uiSubscriber := story.ui.Subscribe("test:story:bad-measurement", 8)
		done := make(chan struct{})

		go func() {
			_ = story.Tick()
			close(done)
		}()

		measurements := story.broadcasts["measurements"]
		measurements.Send(&qpool.QValue[any]{Value: "not-a-measurement"})
		measurements.Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:     "BTC/EUR",
			Source:     perspectives.SourceHawkes,
			Confidence: 0.42,
		}})

		var frame map[string]any

		select {
		case row := <-uiSubscriber.Incoming:
			parsed, ok := row.Value.(map[string]any)

			convey.So(ok, convey.ShouldBeTrue)
			frame = parsed
		case <-time.After(500 * time.Millisecond):
			convey.So("gauge frame after bad measurement", convey.ShouldBeBlank)
		}

		convey.Convey("It should keep ticking and publish the valid reading", func() {
			convey.So(frame["source"], convey.ShouldEqual, "hawkes")
			convey.So(frame["confidence"], convey.ShouldAlmostEqual, 0.42, 1e-9)
		})

		cancel()
		<-done
		convey.So(story.Close(), convey.ShouldBeNil)
	})
}

func TestStoryPublishActionOnRaw(t *testing.T) {
	convey.Convey("Given a story wired to raw", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		recordPath := filepath.Join(t.TempDir(), "capture.jsonl")

		viper.Set("trading.record.file", recordPath)
		viper.Set("trading.paper.wallet_eur", 200.0)
		viper.Set("market.perspectives.fixture_playbook", true)
		viper.Set("market.quote_currency", "EUR")
		defer viper.Set("trading.record.file", "")
		defer viper.Set("trading.paper.wallet_eur", 0)
		defer viper.Set("market.perspectives.fixture_playbook", false)
		defer viper.Set("market.quote_currency", "")

		story := NewStory(ctx, pool, focus.NewSet())
		subscriber := story.raw.Subscribe("test:story:raw", 4)

		done := make(chan struct{})

		go func() {
			_ = story.Tick()
			close(done)
		}()

		for _, row := range perspectives.FixturePlaybookEntryMeasurements("BTC/EUR", 50_000) {
			story.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: row})
		}

		convey.Convey("It should publish a perspectives.Action on raw", func() {
			select {
			case frame := <-subscriber.Incoming:
				action, ok := frame.Value.(perspectives.Action)

				convey.So(ok, convey.ShouldBeTrue)
				convey.So(action.Symbol, convey.ShouldEqual, "BTC/EUR")
				convey.So(action.Type, convey.ShouldEqual, perspectives.ActionLimit)
			case <-time.After(500 * time.Millisecond):
				convey.So("raw action", convey.ShouldBeBlank)
			}

			cancel()
			<-done
			convey.So(story.Close(), convey.ShouldBeNil)
		})
	})
}

func TestStoryEntryWaitsForDeskReady(t *testing.T) {
	convey.Convey("Given a story before the desk is ready", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)

		viper.Set("trading.paper.wallet_eur", 200.0)
		viper.Set("market.perspectives.fixture_playbook", true)
		viper.Set("market.quote_currency", "EUR")
		defer viper.Set("trading.paper.wallet_eur", 0)
		defer viper.Set("market.perspectives.fixture_playbook", false)
		defer viper.Set("market.quote_currency", "")

		story := NewStory(ctx, pool, focus.NewSet())
		subscriber := story.raw.Subscribe("test:story:ready", 4)

		done := make(chan struct{})

		go func() {
			_ = story.Tick()
			close(done)
		}()

		measurements := story.broadcasts["measurements"]

		for _, row := range perspectives.FixturePlaybookEntryMeasurements("BTC/EUR", 50_000) {
			measurements.Send(&qpool.QValue[any]{Value: row})
		}

		time.Sleep(50 * time.Millisecond)

		convey.Convey("It should not publish an entry action yet", func() {
			select {
			case <-subscriber.Incoming:
				convey.So("entry action before desk ready", convey.ShouldBeBlank)
			default:
			}
		})

		for _, row := range perspectives.FixturePlaybookEntryMeasurements("ETH/EUR", 3_000) {
			measurements.Send(&qpool.QValue[any]{Value: row})
		}

		convey.Convey("It should publish once the desk is ready", func() {
			select {
			case frame := <-subscriber.Incoming:
				action, ok := frame.Value.(perspectives.Action)

				convey.So(ok, convey.ShouldBeTrue)
				convey.So(action.Symbol, convey.ShouldEqual, "ETH/EUR")
			case <-time.After(500 * time.Millisecond):
				convey.So("raw action after desk ready", convey.ShouldBeBlank)
			}

			cancel()
			<-done
			convey.So(story.Close(), convey.ShouldBeNil)
		})
	})
}
