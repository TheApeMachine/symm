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
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
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

			_ = (&types.Measurement{
				Symbol:     "BTC/EUR",
				Source:     types.SourceSentiment,
				Category:   types.CategoryRiskOnSurge,
				Strength:   0.8,
				Confidence: 0.6,
				SNR:        1.2,
				Last:       50_000,
			}).Send(pool)

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
		thoughts := []reasoning.Thought{{
			When: reasoning.Predicate{All: []reasoning.Predicate{
				{Subject: reasoning.SubjectPosition, Op: reasoning.ComparisonEquals, Lifecycle: types.ObservationNotHolding},
				{Subject: reasoning.SubjectSignal, Category: types.CategoryVerticalIgnition, Unit: reasoning.UnitSNR, Op: reasoning.ComparisonAtLeast, Value: 1.0},
			}},
			Do: reasoning.Act{Type: reasoning.ActionMarket},
		}}

		encoded, marshalErr := reasoning.MarshalThoughts(thoughts, 2)
		convey.So(marshalErr, convey.ShouldBeNil)

		path := filepath.Join(t.TempDir(), "perspectives.yaml")
		convey.So(os.WriteFile(path, encoded, 0o644), convey.ShouldBeNil)
		t.Setenv("SYMM_PERSPECTIVES_FILE", path)

		auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
		viper.Set("trading.audit.file", auditPath)
		defer viper.Set("trading.audit.file", "")

		story := NewStory(ctx, pool)
		subscriber := story.raw.Subscribe("test:story:raw", 4)

		done := make(chan struct{})

		go func() {
			_ = story.Tick()
			close(done)
		}()

		_ = (&types.Measurement{
			Symbol:     "BTC/EUR",
			Source:     types.SourcePumpDump,
			Category:   types.CategoryVerticalIgnition,
			Strength:   2.0,
			Confidence: 0.8,
			SNR:        1.5,
			Last:       50_000,
		}).Send(pool)

		convey.Convey("It should publish the Thought-driven Action on raw", func() {
			select {
			case frame := <-subscriber.Incoming:
				action, ok := frame.Value.(reasoning.Action)

				convey.So(ok, convey.ShouldBeTrue)
				convey.So(action.Symbol, convey.ShouldEqual, "BTC/EUR")
				convey.So(action.Type, convey.ShouldEqual, reasoning.ActionMarket)
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

func TestStoryPublishMarketRegime(t *testing.T) {
	convey.Convey("Given the story UI bus", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		viper.Set("market.anchor_symbol", "BTC/EUR")
		viper.Set("market.default_symbols", []string{"BTC/EUR", "ETH/EUR"})
		defer viper.Set("market.anchor_symbol", "")
		defer viper.Set("market.default_symbols", nil)

		pool := qpool.NewQ(ctx, 1, 4, nil)
		story := NewStory(ctx, pool)
		ui := bus.Group(pool, "ui", 10*time.Millisecond)
		subscriber := ui.Subscribe("test:story:regime", 8)

		done := make(chan struct{})

		go func() {
			_ = story.Tick()
			close(done)
		}()

		for range 20 {
			_ = (&types.Measurement{
				Symbol:     "ETH/EUR",
				Source:     types.SourceFluid,
				Category:   types.CategoryLaminar,
				Strength:   0.5,
				Confidence: 0.4,
				SNR:        1.0,
				Last:       3_000,
			}).Send(pool)
		}

		_ = (&types.Measurement{
			Symbol:     "BTC/EUR",
			Source:     types.SourceFluid,
			Category:   types.CategoryLaminar,
			Strength:   0.5,
			Confidence: 0.4,
			SNR:        1.0,
			Last:       50_000,
		}).Send(pool)

		convey.Convey("It should publish cross-section market regime radar frames", func() {
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
				convey.So(payload["symbol"], convey.ShouldEqual, "market")
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
		story := NewStory(ctx, pool)
		subscriber := story.raw.Subscribe("test:story:fixture:raw", 4)

		done := make(chan struct{})

		go func() {
			_ = story.Tick()
			close(done)
		}()

		for _, measurement := range perspectives.FixturePlaybookEntryMeasurements("BTC/EUR", 50_000) {
			convey.So(measurement.Send(pool), convey.ShouldBeNil)
		}

		convey.Convey("It should publish the fixture limit entry", func() {
			select {
			case frame := <-subscriber.Incoming:
				action, ok := frame.Value.(reasoning.Action)

				convey.So(ok, convey.ShouldBeTrue)
				convey.So(action.Symbol, convey.ShouldEqual, "BTC/EUR")
				convey.So(action.Type, convey.ShouldEqual, reasoning.ActionLimit)
			case <-time.After(500 * time.Millisecond):
				convey.So("fixture action", convey.ShouldBeBlank)
			}

			cancel()
			<-done
			convey.So(story.Close(), convey.ShouldBeNil)
		})
	})
}

func TestStoryPositionState(t *testing.T) {
	convey.Convey("Given a story wired to the trader inventory view", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		story := NewStory(ctx, pool)
		held := map[string]struct{}{"BTC/EUR": {}}
		story.SetPositionHeld(func(symbol string) bool {
			_, ok := held[symbol]

			return ok
		})

		now := time.Now().UTC()

		convey.Convey("It should treat a flat symbol as not holding even when chart focus would include it", func() {
			state := story.positionState(types.Measurement{
				Symbol: "ETH/EUR",
				Last:   3_000,
				At:     now,
			})

			convey.So(state.Holding, convey.ShouldBeFalse)
		})

		convey.Convey("It should track peak while the exchange reports an open position", func() {
			first := story.positionState(types.Measurement{
				Symbol: "BTC/EUR",
				Last:   50_000,
				At:     now,
			})
			second := story.positionState(types.Measurement{
				Symbol: "BTC/EUR",
				Last:   51_000,
				At:     now.Add(time.Second),
			})

			convey.So(first.Holding, convey.ShouldBeTrue)
			convey.So(second.Peak, convey.ShouldEqual, 51_000)
		})

		convey.Convey("It should drop lifecycle state when the position closes", func() {
			story.positionState(types.Measurement{Symbol: "BTC/EUR", Last: 50_000, At: now})
			delete(held, "BTC/EUR")

			flat := story.positionState(types.Measurement{
				Symbol: "BTC/EUR",
				Last:   49_000,
				At:     now.Add(2 * time.Second),
			})

			convey.So(flat.Holding, convey.ShouldBeFalse)
		})
	})
}

func TestStorySkipsShortActWithoutMargin(t *testing.T) {
	convey.Convey("Given margin is disabled", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		viper.Set("trading.margin_enabled", false)
		defer viper.Set("trading.margin_enabled", nil)

		thoughts := []reasoning.Thought{{
			When: reasoning.Predicate{All: []reasoning.Predicate{
				{Subject: reasoning.SubjectPosition, Op: reasoning.ComparisonEquals, Lifecycle: types.ObservationNotHolding},
				{Subject: reasoning.SubjectSignal, Category: types.CategoryMechanicalCollapse, Unit: reasoning.UnitSNR, Op: reasoning.ComparisonAtLeast, Value: 1.0},
			}},
			Do: reasoning.Act{Type: reasoning.ActionLimit, Side: trading.Sell},
		}}

		encoded, marshalErr := reasoning.MarshalThoughts(thoughts, 2)
		convey.So(marshalErr, convey.ShouldBeNil)

		path := filepath.Join(t.TempDir(), "perspectives.yaml")
		convey.So(os.WriteFile(path, encoded, 0o644), convey.ShouldBeNil)
		t.Setenv("SYMM_PERSPECTIVES_FILE", path)

		story := NewStory(ctx, pool)
		subscriber := story.raw.Subscribe("test:story:short", 1)

		done := make(chan struct{})

		go func() {
			_ = story.Tick()
			close(done)
		}()

		_ = (&types.Measurement{
			Symbol:   "BTC/EUR",
			Source:   types.SourceExhaustion,
			Category: types.CategoryMechanicalCollapse,
			SNR:      2.0,
			Last:     50_000,
		}).Send(pool)

		convey.Convey("It should not publish a short entry on raw", func() {
			select {
			case <-subscriber.Incoming:
				convey.So("short entry action", convey.ShouldBeBlank)
			case <-time.After(200 * time.Millisecond):
			}

			cancel()
			<-done
			convey.So(story.Close(), convey.ShouldBeNil)
		})
	})
}
