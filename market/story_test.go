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

func TestStoryPublishActionOnRaw(t *testing.T) {
	convey.Convey("Given a story with a Thought playbook", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)

		// A minimal playbook: when flat and an ignition fires, enter at market.
		thoughts := []perspectives.Thought{{
			When: perspectives.Predicate{All: []perspectives.Predicate{
				{Subject: perspectives.SubjectPosition, Op: perspectives.ComparisonEquals, Lifecycle: perspectives.ObservationNotHolding},
				{Subject: perspectives.SubjectSignal, Category: perspectives.CategoryVerticalIgnition, Unit: perspectives.UnitSNR, Op: perspectives.ComparisonAtLeast, Value: 1.0},
			}},
			Do: perspectives.Act{Type: perspectives.ActionMarket},
		}}

		encoded, marshalErr := perspectives.MarshalThoughts(thoughts, 2)
		convey.So(marshalErr, convey.ShouldBeNil)

		path := filepath.Join(t.TempDir(), "perspectives.yaml")
		convey.So(os.WriteFile(path, encoded, 0o644), convey.ShouldBeNil)
		t.Setenv("SYMM_PERSPECTIVES_FILE", path)

		auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
		viper.Set("trading.audit.file", auditPath)
		defer viper.Set("trading.audit.file", "")

		story := NewStory(ctx, pool, focus.NewSet())
		subscriber := story.raw.Subscribe("test:story:raw", 4)

		done := make(chan struct{})

		go func() {
			_ = story.Tick()
			close(done)
		}()

		story.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Category: perspectives.CategoryVerticalIgnition,
			SNR:      1.5,
			Last:     50_000,
		}})

		convey.Convey("It should publish the Thought-driven Action on raw", func() {
			select {
			case frame := <-subscriber.Incoming:
				action, ok := frame.Value.(perspectives.Action)

				convey.So(ok, convey.ShouldBeTrue)
				convey.So(action.Symbol, convey.ShouldEqual, "BTC/EUR")
				convey.So(action.Type, convey.ShouldEqual, perspectives.ActionMarket)
			case <-time.After(500 * time.Millisecond):
				convey.So("raw action", convey.ShouldBeBlank)
			}

			cancel()
			<-done
			convey.So(story.Close(), convey.ShouldBeNil)

			raw, readErr := os.ReadFile(auditPath)
			convey.So(readErr, convey.ShouldBeNil)
			convey.So(string(raw), convey.ShouldContainSubstring, `"audit_event":"playbook_walk"`)
			convey.So(string(raw), convey.ShouldContainSubstring, `"verdict":"market"`)
		})
	})
}

func TestStoryPublishRegimeAnchorOnly(t *testing.T) {
	convey.Convey("Given the story UI bus", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		viper.Set("market.anchor_symbol", "BTC/EUR")
		viper.Set("market.default_symbols", []string{"BTC/EUR", "ETH/EUR"})
		defer viper.Set("market.anchor_symbol", "")
		defer viper.Set("market.default_symbols", nil)

		pool := qpool.NewQ(ctx, 1, 4, nil)
		story := NewStory(ctx, pool, focus.NewSet())
		ui := pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
		subscriber := ui.Subscribe("test:story:regime", 8)

		done := make(chan struct{})

		go func() {
			_ = story.Tick()
			close(done)
		}()

		for range 20 {
			story.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: perspectives.Measurement{
				Symbol: "ETH/EUR",
				Last:   3_000,
			}})
		}

		story.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol: "BTC/EUR",
			Last:   50_000,
		}})

		convey.Convey("It should publish regime radar frames only for the anchor symbol", func() {
			var frames []map[string]any

			deadline := time.After(500 * time.Millisecond)

		collect:
			for {
				select {
				case frame := <-subscriber.Incoming:
					payload, ok := frame.Value.(map[string]any)

					convey.So(ok, convey.ShouldBeTrue)

					if payload["chart"] == "regime" {
						frames = append(frames, payload)
					}
				case <-deadline:
					break collect
				}
			}

			convey.So(len(frames), convey.ShouldBeGreaterThan, 0)

			for _, payload := range frames {
				convey.So(payload["symbol"], convey.ShouldEqual, "BTC/EUR")
			}

			cancel()
			<-done
			convey.So(story.Close(), convey.ShouldBeNil)
		})
	})
}

func TestStoryFixturePlaybook(t *testing.T) {
	convey.Convey("Given fixture playbook mode", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		viper.Set("market.perspectives.fixture_playbook", true)
		defer viper.Set("market.perspectives.fixture_playbook", false)

		pool := qpool.NewQ(ctx, 1, 4, nil)
		story := NewStory(ctx, pool, focus.NewSet())
		subscriber := story.raw.Subscribe("test:story:fixture:raw", 4)

		done := make(chan struct{})

		go func() {
			_ = story.Tick()
			close(done)
		}()

		for _, measurement := range perspectives.FixturePlaybookEntryMeasurements("BTC/EUR", 50_000) {
			story.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
		}

		convey.Convey("It should publish the fixture limit entry", func() {
			select {
			case frame := <-subscriber.Incoming:
				action, ok := frame.Value.(perspectives.Action)

				convey.So(ok, convey.ShouldBeTrue)
				convey.So(action.Symbol, convey.ShouldEqual, "BTC/EUR")
				convey.So(action.Type, convey.ShouldEqual, perspectives.ActionLimit)
			case <-time.After(500 * time.Millisecond):
				convey.So("fixture action", convey.ShouldBeBlank)
			}

			cancel()
			<-done
			convey.So(story.Close(), convey.ShouldBeNil)
		})
	})
}
