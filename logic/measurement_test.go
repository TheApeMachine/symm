package logic

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestMeasurementPublishable(t *testing.T) {
	Convey("Given measurement publishability rules", t, func() {
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		row, rowErr := krakenmarket.NewSymbolRow(
			"BTC/USD",
			42000,
			0.01,
			42000,
			1,
			eventAt,
		)

		So(rowErr, ShouldBeNil)

		complete := Measurement{
			Source:     SourceFluid,
			Symbol:     "BTC/USD",
			Price:      42000,
			Strength:   0.5,
			Volume:     100,
			Spread:     1,
			Elapsed:    1,
			Category:   CategoryOrganic,
			Confidence: 0.8,
			Surprise:   1.2,
			ObservedAt: eventAt,
			Market:     *row,
		}

		Convey("Publishable should accept complete measurements", func() {
			So(complete.Publishable(), ShouldBeTrue)
			So(complete.Publish(internal.NewBus(context.Background(), qpool.NewQ[any](context.Background(), 2, 8, nil), []internal.Channel{internal.ChannelMeasurements}, []internal.Subscription{internal.Subscribe(internal.ChannelMeasurements, "test-measurements")})), ShouldBeNil)
		})

		Convey("DecisionEligible should reject stale, neutral, and best-effort evidence", func() {
			So(complete.DecisionEligible(eventAt.Add(time.Second), 2*time.Second), ShouldBeTrue)

			stale := complete
			stale.ObservedAt = eventAt.Add(-3 * time.Second)

			So(stale.DecisionEligible(eventAt, 2*time.Second), ShouldBeFalse)

			neutral := complete
			neutral.Category = CategoryTypeNone

			So(neutral.DecisionEligible(eventAt, 2*time.Second), ShouldBeFalse)

			bestEffort := complete
			bestEffort.BestEffort = true

			So(bestEffort.DecisionEligible(eventAt, 2*time.Second), ShouldBeFalse)
		})

		Convey("Publishable should reject incomplete measurements", func() {
			incomplete := Measurement{Symbol: "BTC/USD"}

			So(incomplete.Publishable(), ShouldBeFalse)
			So(incomplete.Publish(internal.NewBus(context.Background(), qpool.NewQ[any](context.Background(), 2, 8, nil), []internal.Channel{internal.ChannelMeasurements}, []internal.Subscription{internal.Subscribe(internal.ChannelMeasurements, "test-measurements")})), ShouldNotBeNil)
		})

		Convey("PublishGap should list missing fields", func() {
			incomplete := Measurement{Symbol: "BTC/USD"}

			So(incomplete.Publish(internal.NewBus(context.Background(), qpool.NewQ[any](context.Background(), 2, 8, nil), []internal.Channel{internal.ChannelMeasurements}, []internal.Subscription{internal.Subscribe(internal.ChannelMeasurements, "test-measurements")})), ShouldNotBeNil)
		})

		Convey("Publishable should reject non-finite strength", func() {
			nonFinite := complete
			nonFinite.Strength = math.NaN()

			So(nonFinite.Publish(internal.NewBus(context.Background(), qpool.NewQ[any](context.Background(), 2, 8, nil), []internal.Channel{internal.ChannelMeasurements}, []internal.Subscription{internal.Subscribe(internal.ChannelMeasurements, "test-measurements")})), ShouldNotBeNil)
		})

		Convey("Publish should reject non-finite floats", func() {
			ctx := context.Background()
			pool := qpool.NewQ[any](ctx, 2, 8, nil)
			bus := internal.NewBus(
				ctx,
				pool,
				[]internal.Channel{internal.ChannelMeasurements},
				[]internal.Subscription{
					internal.Subscribe(internal.ChannelMeasurements, "test-measurements"),
				},
			)

			invalid := Measurement{
				Source:     SourceDepthFlow,
				Symbol:     "ETH/USD",
				Price:      1600,
				Strength:   math.NaN(),
				Volume:     100,
				Spread:     1,
				Elapsed:    1,
				Confidence: 0.8,
				Surprise:   1.2,
				ObservedAt: eventAt,
				Market:     *row,
			}

			So(invalid.Publish(bus), ShouldNotBeNil)

			received, receiveErr := awaitMeasurement(bus, 20*time.Millisecond)

			So(receiveErr, ShouldBeNil)
			So(received, ShouldBeNil)
		})

		Convey("Publish should reject incomplete measurements", func() {
			ctx := context.Background()
			pool := qpool.NewQ[any](ctx, 2, 8, nil)
			bus := internal.NewBus(
				ctx,
				pool,
				[]internal.Channel{internal.ChannelMeasurements},
				[]internal.Subscription{
					internal.Subscribe(internal.ChannelMeasurements, "test-measurements"),
				},
			)

			So(complete.Publish(bus), ShouldBeNil)

			received, receiveErr := bus.Receive(internal.ChannelMeasurements)

			So(receiveErr, ShouldBeNil)
			So(received, ShouldNotBeNil)

			incomplete := Measurement{Symbol: "ETH/USD"}

			So(incomplete.Publish(bus), ShouldNotBeNil)

			secondRow, secondReceiveErr := awaitMeasurement(bus, 20*time.Millisecond)

			So(secondReceiveErr, ShouldBeNil)
			So(secondRow, ShouldBeNil)
		})
	})
}

func awaitMeasurement(
	bus *internal.Bus,
	wait time.Duration,
) (*qpool.QValue[any], error) {
	received := make(chan struct {
		message *qpool.QValue[any]
		err     error
	}, 1)

	go func() {
		message, err := bus.Receive(internal.ChannelMeasurements)

		received <- struct {
			message *qpool.QValue[any]
			err     error
		}{message, err}
	}()

	select {
	case result := <-received:
		return result.message, result.err
	case <-time.After(wait):
		return nil, nil
	}
}

func TestMeasurementJSONEncoding(t *testing.T) {
	Convey("Given a complete measurement", t, func() {
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		row, rowErr := krakenmarket.NewSymbolRow(
			"BTC/USD",
			42000,
			0.01,
			42000,
			1,
			eventAt,
		)

		So(rowErr, ShouldBeNil)

		measurement := Measurement{
			Source:     SourceLeadLag,
			Symbol:     "BTC/USD",
			Price:      42000,
			Strength:   0.5,
			Volume:     100,
			Spread:     1,
			Elapsed:    1,
			Confidence: 0.8,
			Surprise:   1.2,
			ObservedAt: eventAt,
			Market:     *row,
		}

		frame, err := encodedWireFrame("state", measurement)

		Convey("It should encode dashboard wire fields in lowercase", func() {
			So(err, ShouldBeNil)
			So(frame["source"], ShouldEqual, "leadlag")
			So(frame["confidence"], ShouldEqual, 0.8)
			So(frame["surprise"], ShouldEqual, 1.2)
		})
	})
}

func BenchmarkMeasurementDecisionEligible(benchmark *testing.B) {
	eventAt := time.Unix(100, 0)
	row, rowErr := krakenmarket.NewSymbolRow(
		"BTC/USD",
		42000,
		0.01,
		42000,
		1,
		eventAt,
	)

	if rowErr != nil {
		benchmark.Fatal(rowErr)
	}

	measurement := Measurement{
		Source:     SourceFluid,
		Symbol:     "BTC/USD",
		Price:      42000,
		Strength:   0.5,
		Volume:     100,
		Spread:     1,
		Elapsed:    1,
		Category:   CategoryOrganic,
		Confidence: 0.8,
		Surprise:   1.2,
		ObservedAt: eventAt,
		Market:     *row,
	}

	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for benchmark.Loop() {
		_ = measurement.DecisionEligible(eventAt, time.Second)
	}
}

func encodedWireFrame(messageType string, value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)

	if err != nil {
		return nil, err
	}

	frame := map[string]any{}

	if err = json.Unmarshal(encoded, &frame); err != nil {
		return nil, err
	}

	frame["type"] = messageType

	return frame, nil
}
