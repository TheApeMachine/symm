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
	"github.com/theapemachine/symm/kraken/trading"
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

		story, storyErr := NewStory(ctx, pool, focus.NewSet())
		convey.So(storyErr, convey.ShouldBeNil)

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

func TestStoryPublishActionOnRaw(t *testing.T) {
	convey.Convey("Given a story wired to raw", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		recordPath := filepath.Join(t.TempDir(), "capture.jsonl")

		viper.Set("trading.record.file", recordPath)
		viper.Set("trading.paper.wallet_eur", 200.0)
		viper.Set("market.quote_currency", "EUR")
		trading.MarkDeskReady()
		defer viper.Set("trading.record.file", "")
		defer viper.Set("trading.paper.wallet_eur", 0)
		defer trading.ResetDeskReady()

		story, storyErr := NewStory(ctx, pool, focus.NewSet())
		convey.So(storyErr, convey.ShouldBeNil)
		subscriber := story.raw.Subscribe("test:story:raw", 4)

		done := make(chan struct{})

		go func() {
			_ = story.Tick()
			close(done)
		}()

		story.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:     "BTC/EUR",
			Category:   perspectives.CategoryVolumeStarvation,
			SNR:        1.0,
			Last:       50_000,
		}})
		story.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Category: perspectives.CategoryLiquidityVacuum,
			SNR:      1.011867,
			Last:     50_000,
		}})
		story.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Category: perspectives.CategoryLiquidityVacuum,
			SNR:      1.0,
			Last:     50_000,
		}})

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
		trading.ResetDeskReady()
		defer trading.ResetDeskReady()

		viper.Set("trading.paper.wallet_eur", 200.0)
		viper.Set("market.quote_currency", "EUR")
		defer viper.Set("trading.paper.wallet_eur", 0)
		defer viper.Set("market.quote_currency", "")

		story, storyErr := NewStory(ctx, pool, focus.NewSet())
		convey.So(storyErr, convey.ShouldBeNil)
		subscriber := story.raw.Subscribe("test:story:ready", 4)

		done := make(chan struct{})

		go func() {
			_ = story.Tick()
			close(done)
		}()

		measurements := story.broadcasts["measurements"]
		measurements.Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Category: perspectives.CategoryVolumeStarvation,
			SNR:      1.0,
			Last:     50_000,
		}})
		measurements.Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Category: perspectives.CategoryLiquidityVacuum,
			SNR:      1.011867,
			Last:     50_000,
		}})
		measurements.Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Category: perspectives.CategoryLiquidityVacuum,
			SNR:      1.0,
			Last:     50_000,
		}})

		time.Sleep(50 * time.Millisecond)

		convey.Convey("It should not publish an entry action yet", func() {
			select {
			case <-subscriber.Incoming:
				convey.So("entry action before desk ready", convey.ShouldBeBlank)
			default:
			}
		})

		trading.MarkDeskReady()

		measurements.Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:   "ETH/EUR",
			Category: perspectives.CategoryVolumeStarvation,
			SNR:      1.0,
			Last:     3_000,
		}})
		measurements.Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:   "ETH/EUR",
			Category: perspectives.CategoryLiquidityVacuum,
			SNR:      1.011867,
			Last:     3_000,
		}})
		measurements.Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:   "ETH/EUR",
			Category: perspectives.CategoryLiquidityVacuum,
			SNR:      1.0,
			Last:     3_000,
		}})

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
