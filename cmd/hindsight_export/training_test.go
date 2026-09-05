package main

import (
	"testing"

	flatbuffers "github.com/google/flatbuffers/go"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

func TestTrainingObservationStreamObserve(t *testing.T) {
	Convey("Given metrics arriving before the Advisor clock advances", t, func() {
		stream := newTrainingObservationStream(
			"pumpdump/completed_volume_bar_ordinal",
		)
		builder := flatbuffers.NewBuilder(1024)
		first := (&telemetry.EnvelopeStateT{
			TypeId: byte(types.EnvelopeTrade),
			TradeData: &telemetry.EnvelopeTradeDataT{
				Symbol:      "FLOCK/USD",
				TimestampNs: 10,
			},
			Cvd: &telemetry.EnvelopeMeasurementT{
				Maturity: 1,
				Metrics: []*telemetry.EnvelopeMeasurementMetricT{{
					Key: "gross_notional",
					Value: &telemetry.EnvelopeMetricT{
						Label: "gross_notional",
						Raw:   20,
					},
				}},
			},
		}).Pack(builder)
		builder.Finish(first)
		firstState := telemetry.GetRootAsEnvelopeState(builder.FinishedBytes(), 0)

		_, observed, err := stream.Observe(
			"run-1", 1, 0, firstState, extractAllMetrics(firstState),
		)
		So(err, ShouldBeNil)
		So(observed, ShouldBeFalse)

		builder = flatbuffers.NewBuilder(1024)
		quote := (&telemetry.EnvelopeStateT{
			TypeId: byte(types.EnvelopeTicker),
			TickerData: &telemetry.EnvelopeTickerDataT{
				Symbol: "FLOCK/USD",
				HasBid: true,
				Bid:    99,
				HasAsk: true,
				Ask:    100,
			},
		}).Pack(builder)
		builder.Finish(quote)
		quoteState := telemetry.GetRootAsEnvelopeState(builder.FinishedBytes(), 0)

		_, observed, err = stream.Observe(
			"run-1", 2, 0, quoteState, extractAllMetrics(quoteState),
		)
		So(err, ShouldBeNil)
		So(observed, ShouldBeFalse)

		builder = flatbuffers.NewBuilder(1024)
		second := (&telemetry.EnvelopeStateT{
			TypeId: byte(types.EnvelopeTrade),
			TradeData: &telemetry.EnvelopeTradeDataT{
				Symbol:      "FLOCK/USD",
				TimestampNs: 20,
			},
			PumpDump: &telemetry.EnvelopeMeasurementT{
				Metrics: []*telemetry.EnvelopeMeasurementMetricT{{
					Key: "completed_volume_bar_ordinal",
					Value: &telemetry.EnvelopeMetricT{
						Label: "completed_volume_bar_ordinal",
						Raw:   1,
						Unit:  "count",
					},
				}},
			},
		}).Pack(builder)
		builder.Finish(second)
		secondState := telemetry.GetRootAsEnvelopeState(builder.FinishedBytes(), 0)

		observation, observed, err := stream.Observe(
			"run-1", 3, 0, secondState, extractAllMetrics(secondState),
		)

		Convey("the emitted observation contains the exact retained state", func() {
			So(err, ShouldBeNil)
			So(observed, ShouldBeTrue)
			So(observation.Symbol, ShouldEqual, "FLOCK/USD")
			So(observation.ObservedAt, ShouldEqual, int64(20))
			So(observation.Coordinate, ShouldEqual, uint64(1))
			So(observation.HasQuote, ShouldBeTrue)
			So(observation.Bid, ShouldEqual, 99.0)
			So(observation.Ask, ShouldEqual, 100.0)
			So(observation.Metrics["cvd/gross_notional"], ShouldEqual, 20.0)
			So(observation.Metrics["pumpdump/completed_volume_bar_ordinal"], ShouldEqual, 1.0)
		})
	})
}

func BenchmarkTrainingObservationStreamObserve(b *testing.B) {
	builder := flatbuffers.NewBuilder(1024)
	offset := (&telemetry.EnvelopeStateT{
		TypeId: byte(types.EnvelopeTrade),
		TradeData: &telemetry.EnvelopeTradeDataT{
			Symbol:      "FLOCK/USD",
			TimestampNs: 20,
		},
	}).Pack(builder)
	builder.Finish(offset)
	state := telemetry.GetRootAsEnvelopeState(builder.FinishedBytes(), 0)
	stream := newTrainingObservationStream(
		"pumpdump/completed_volume_bar_ordinal",
	)

	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		_, _, err := stream.Observe(
			"run-1",
			uint64(index+1),
			0,
			state,
			map[string]float64{
				"cvd/gross_notional":                    float64(index),
				"pumpdump/completed_volume_bar_ordinal": float64(index + 1),
			},
		)

		if err != nil {
			b.Fatal(err)
		}
	}
}
