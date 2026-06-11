package market

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func TestStoryShouldPublishUI(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a story", t, func() {
		viper.Set("system.audit.enabled", false)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		subscriber := internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelUI},
			[]internal.Subscription{internal.Subscribe(internal.ChannelUI, "story-test")},
		)
		story, err := NewStory(ctx, pool)

		So(err, ShouldBeNil)
		So(story, ShouldNotBeNil)
		So(subscriber, ShouldNotBeNil)

		treeFrame, treeErr := subscriber.Receive(internal.ChannelUI)

		Convey("It should publish the embedded playbook tree on startup", func() {
			So(treeErr, ShouldBeNil)
			So(treeFrame, ShouldNotBeNil)
			So(treeFrame.Type, ShouldEqual, "decision_tree")

			tree, ok := treeFrame.Value.(*logic.Tree)

			So(ok, ShouldBeTrue)
			So(len(tree.Branches), ShouldBeGreaterThan, 0)
		})

		statusFrame, statusErr := subscriber.Receive(internal.ChannelUI)

		Convey("It should publish story counters on startup", func() {
			So(statusErr, ShouldBeNil)
			So(statusFrame, ShouldNotBeNil)
			So(statusFrame.Type, ShouldEqual, "story")

			statusPayload, statusOK := statusFrame.Value.(map[string]any)

			So(statusOK, ShouldBeTrue)
			So(statusPayload["story_ticks"], ShouldEqual, 0)
			So(statusPayload["playbook_evaluations"], ShouldEqual, 0)
		})
	})
}

func TestStoryIngestMeasurement(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a story waiting for a full measurement spectrum", t, func() {
		viper.Set("system.audit.enabled", false)
		viper.Set("story.measurements.buffer", 32)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 64, nil)
		subscriber := internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelUI},
			[]internal.Subscription{internal.Subscribe(internal.ChannelUI, "story-test")},
		)
		story, err := NewStory(ctx, pool)

		So(err, ShouldBeNil)
		So(story, ShouldNotBeNil)

		drainStartup := func() {
			for range 2 {
				_, receiveErr := subscriber.Receive(internal.ChannelUI)

				So(receiveErr, ShouldBeNil)
			}
		}

		drainStartup()

		observedAt := time.Now()
		marketRow, rowErr := market.NewSymbolRow(
			"BTC/USD",
			100,
			0.01,
			100,
			1,
			observedAt,
		)

		So(rowErr, ShouldBeNil)

		for sourceIndex, source := range logic.SpectrumSources {
			measurement := logic.Measurement{
				Source:     source,
				Symbol:     "BTC/USD",
				Price:      100,
				Strength:   1,
				Volume:     1,
				Spread:     1,
				Elapsed:    1,
				Confidence: 0.8,
				Surprise:   2,
				ObservedAt: observedAt,
				Market:     *marketRow,
			}

			ingestErr := story.ingestMeasurement(measurement)

			So(ingestErr, ShouldBeNil)

			if sourceIndex < logic.SourceCount-1 {
				continue
			}

			Convey("It should increment story ticks after a complete spectrum", func() {
				statusFrame, statusErr := subscriber.Receive(internal.ChannelUI)

				So(statusErr, ShouldBeNil)
				So(statusFrame.Type, ShouldEqual, "story")

				statusPayload, statusOK := statusFrame.Value.(map[string]any)

				So(statusOK, ShouldBeTrue)
				So(statusPayload["story_ticks"], ShouldEqual, 1)
				So(statusPayload["playbook_evaluations"], ShouldEqual, 1)
			})
		}
	})
}

func TestStoryTicksFromMeasurementBus(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a story subscribed to the measurements bus", t, func() {
		viper.Set("system.audit.enabled", false)
		viper.Set("story.measurements.buffer", 32)

		ctx, cancel := context.WithCancel(context.Background())
		pool := qpool.NewQ[any](ctx, 2, 64, nil)

		t.Cleanup(func() {
			cancel()
			pool.Close()
		})

		subscriber := internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelUI},
			[]internal.Subscription{internal.Subscribe(internal.ChannelUI, "story-test")},
		)
		story, err := NewStory(ctx, pool)

		So(err, ShouldBeNil)
		So(story, ShouldNotBeNil)
		So(subscriber, ShouldNotBeNil)

		t.Cleanup(func() {
			_ = story.Close()
			_ = subscriber.Close()
		})

		go func() {
			_ = story.Tick()
		}()

		drainStartup := func() {
			for range 2 {
				_, receiveErr := subscriber.Receive(internal.ChannelUI)

				So(receiveErr, ShouldBeNil)
			}
		}

		drainStartup()

		observedAt := time.Now()
		marketRow, rowErr := market.NewSymbolRow(
			"BTC/USD",
			100,
			0.01,
			100,
			1,
			observedAt,
		)

		So(rowErr, ShouldBeNil)

		for sourceIndex, source := range logic.SpectrumSources {
			measurement := logic.Measurement{
				Source:     source,
				Symbol:     "BTC/USD",
				Price:      100,
				Strength:   1,
				Volume:     1,
				Spread:     1,
				Elapsed:    1,
				Confidence: 0.8,
				Surprise:   2,
				ObservedAt: observedAt,
				Market:     *marketRow,
			}

			So(measurement.Publish(story.bus), ShouldBeNil)

			if sourceIndex < logic.SourceCount-1 {
				continue
			}

			Convey("It should increment story ticks after a complete spectrum arrives on the bus", func() {
				statusFrame, statusErr := subscriber.Receive(internal.ChannelUI)

				So(statusErr, ShouldBeNil)
				So(statusFrame.Type, ShouldEqual, "story")

				statusPayload, statusOK := statusFrame.Value.(map[string]any)

				So(statusOK, ShouldBeTrue)
				So(statusPayload["story_ticks"], ShouldEqual, 1)
				So(statusPayload["playbook_evaluations"], ShouldEqual, 1)
			})
		}
	})
}

func TestStoryPlaybookEvaluationWithoutAction(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a complete spectrum that does not match any playbook action", t, func() {
		viper.Set("system.audit.enabled", false)
		viper.Set("story.measurements.buffer", 32)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 64, nil)
		subscriber := internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelUI},
			[]internal.Subscription{internal.Subscribe(internal.ChannelUI, "story-test")},
		)
		story, err := NewStory(ctx, pool)

		So(err, ShouldBeNil)
		So(story, ShouldNotBeNil)

		for range 2 {
			_, receiveErr := subscriber.Receive(internal.ChannelUI)
			So(receiveErr, ShouldBeNil)
		}

		observedAt := time.Now()
		marketRow, rowErr := market.NewSymbolRow(
			"BTC/USD",
			100,
			0.01,
			100,
			1,
			observedAt,
		)

		So(rowErr, ShouldBeNil)

		for sourceIndex, source := range logic.SpectrumSources {
			measurement := logic.Measurement{
				Source:     source,
				Symbol:     "BTC/USD",
				Price:      100,
				Strength:   0.25,
				Volume:     1,
				Spread:     1,
				Elapsed:    1,
				Confidence: 0.25,
				Surprise:   0.25,
				ObservedAt: observedAt,
				Market:     *marketRow,
			}

			ingestErr := story.ingestMeasurement(measurement)

			So(ingestErr, ShouldBeNil)

			if sourceIndex < logic.SourceCount-1 {
				continue
			}

			Convey("It should still count a playbook evaluation", func() {
				statusFrame, statusErr := subscriber.Receive(internal.ChannelUI)

				So(statusErr, ShouldBeNil)

				statusPayload, statusOK := statusFrame.Value.(map[string]any)

				So(statusOK, ShouldBeTrue)
				So(statusPayload["story_ticks"], ShouldEqual, 1)
				So(statusPayload["playbook_evaluations"], ShouldEqual, 1)

				walkFrame, walkErr := subscriber.Receive(internal.ChannelUI)

				So(walkErr, ShouldBeNil)
				So(walkFrame.Type, ShouldEqual, "decision_walk")

				walkTrace, walkOK := walkFrame.Value.(*logic.WalkTrace)

				So(walkOK, ShouldBeTrue)
				So(walkTrace.Symbol, ShouldEqual, "BTC/USD")
				So(len(walkTrace.Steps), ShouldBeGreaterThan, 0)
			})
		}
	})
}

func TestStoryPlaybookNoActionAudit(t *testing.T) {
	testconfig.Load(t)

	Convey("Given audit recording enabled", t, func() {
		auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
		viper.Set("system.audit.enabled", true)
		viper.Set("system.audit.file", auditPath)
		viper.Set("story.measurements.buffer", 32)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 64, nil)
		story, err := NewStory(ctx, pool)

		So(err, ShouldBeNil)
		So(story, ShouldNotBeNil)

		observedAt := time.Now()
		marketRow, rowErr := market.NewSymbolRow(
			"BTC/USD",
			100,
			0.01,
			100,
			1,
			observedAt,
		)

		So(rowErr, ShouldBeNil)

		for sourceIndex, source := range logic.SpectrumSources {
			measurement := logic.Measurement{
				Source:     source,
				Symbol:     "BTC/USD",
				Price:      100,
				Strength:   0.25,
				Volume:     1,
				Spread:     1,
				Elapsed:    1,
				Confidence: 0.25,
				Surprise:   0.25,
				ObservedAt: observedAt,
				Market:     *marketRow,
			}

			ingestErr := story.ingestMeasurement(measurement)

			So(ingestErr, ShouldBeNil)

			if sourceIndex < logic.SourceCount-1 {
				continue
			}
		}

		Convey("It should append a playbook_no_action diagnostic row", func() {
			file, openErr := os.Open(auditPath)
			So(openErr, ShouldBeNil)

			found := false

			scanner := bufio.NewScanner(file)

			for scanner.Scan() {
				var decoded map[string]any
				So(json.Unmarshal(scanner.Bytes(), &decoded), ShouldBeNil)

				if decoded["channel"] == "diagnostic" && decoded["type"] == "playbook_no_action" {
					found = true
				}
			}

			So(scanner.Err(), ShouldBeNil)
			So(found, ShouldBeTrue)
			So(file.Close(), ShouldBeNil)
		})
	})
}
