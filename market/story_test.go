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
			_, receiveErr := subscriber.Receive(internal.ChannelUI)

			So(receiveErr, ShouldBeNil)
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
				So(story.storyTicks.Load(), ShouldEqual, 1)
				So(story.playbookEvaluations.Load(), ShouldEqual, logic.SourceCount)
			})
		}
	})
}

func TestStoryGaugeReadingsCarryDiagnosticsEvidence(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a publishable measurement in the story batch", t, func() {
		viper.Set("system.audit.enabled", false)
		viper.Set("story.measurements.buffer", 32)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 64, nil)
		subscriber := internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelUI},
			[]internal.Subscription{internal.Subscribe(internal.ChannelUI, "story-gauge-test")},
		)
		story, err := NewStory(ctx, pool)

		So(err, ShouldBeNil)
		So(story, ShouldNotBeNil)

		_, receiveErr := subscriber.Receive(internal.ChannelUI)
		So(receiveErr, ShouldBeNil)

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

		measurement := logic.Measurement{
			Source:     logic.SourceFluid,
			Symbol:     "BTC/USD",
			Price:      100,
			Strength:   0.5,
			Volume:     1,
			Spread:     1,
			Elapsed:    3,
			Category:   logic.CategoryOrganic,
			Confidence: 0.5,
			Surprise:   2,
			ObservedAt: observedAt,
			Market:     *marketRow,
		}

		ingestErr := story.ingestMeasurement(measurement)

		Convey("It should publish diagnostics-ready gauge readings in one state frame", func() {
			So(ingestErr, ShouldBeNil)

			frame, stateErr := subscriber.Receive(internal.ChannelUI)

			So(stateErr, ShouldBeNil)
			So(frame.Type, ShouldEqual, "state")

			payload, ok := frame.Value.(map[string]any)

			So(ok, ShouldBeTrue)

			readings, ok := payload["gauge_readings"].([]GaugeReading)

			So(ok, ShouldBeTrue)
			So(len(readings), ShouldEqual, 1)
			So(readings[0].Source, ShouldEqual, logic.SourceFluid)
			So(readings[0].Calibrated, ShouldBeTrue)
			So(readings[0].ActiveReadings, ShouldEqual, 1)
			So(readings[0].Strength, ShouldEqual, measurement.Strength)
			So(readings[0].Elapsed, ShouldEqual, measurement.Elapsed)
			So(readings[0].ObservedAt, ShouldEqual, observedAt)
		})
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
			_, receiveErr := subscriber.Receive(internal.ChannelUI)

			So(receiveErr, ShouldBeNil)
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
				deadline := time.Now().Add(2 * time.Second)

				for time.Now().Before(deadline) && story.storyTicks.Load() < 1 {
					time.Sleep(10 * time.Millisecond)
				}

				So(story.storyTicks.Load(), ShouldEqual, 1)
				So(story.playbookEvaluations.Load(), ShouldEqual, logic.SourceCount)
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

		_, receiveErr := subscriber.Receive(internal.ChannelUI)
		So(receiveErr, ShouldBeNil)

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
				So(story.storyTicks.Load(), ShouldEqual, 1)
				So(story.playbookEvaluations.Load(), ShouldEqual, logic.SourceCount)
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

		Convey("It should append a playbook trace row", func() {
			file, openErr := os.Open(auditPath)
			So(openErr, ShouldBeNil)

			found := false

			scanner := bufio.NewScanner(file)

			for scanner.Scan() {
				var decoded map[string]any
				So(json.Unmarshal(scanner.Bytes(), &decoded), ShouldBeNil)

				if decoded["trace"] != nil {
					found = true
				}
			}

			So(scanner.Err(), ShouldBeNil)
			So(found, ShouldBeTrue)
			So(file.Close(), ShouldBeNil)
		})
	})
}

func BenchmarkStoryGaugeWireReadings(b *testing.B) {
	observedAt := time.Now()
	marketRow, rowErr := market.NewSymbolRow(
		"BTC/USD",
		100,
		0.01,
		100,
		1,
		observedAt,
	)

	if rowErr != nil {
		b.Fatal(rowErr)
	}

	measurements := make([]logic.Measurement, 0, len(logic.SpectrumSources))

	for _, source := range logic.SpectrumSources {
		measurements = append(measurements, logic.Measurement{
			Source:     source,
			Symbol:     "BTC/USD",
			Price:      100,
			Strength:   0.5,
			Volume:     1,
			Spread:     1,
			Elapsed:    3,
			Category:   logic.CategoryOrganic,
			Confidence: 0.5,
			Surprise:   2,
			ObservedAt: observedAt,
			Market:     *marketRow,
		})
	}

	story := &Story{}

	b.ReportAllocs()

	for b.Loop() {
		readings := story.gaugeWireReadings(measurements)

		if len(readings) != len(measurements) {
			b.Fatal("missing gauge readings")
		}
	}
}
