package main

import (
	"testing"

	flatbuffers "github.com/google/flatbuffers/go"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func TestExtractAllMetrics(t *testing.T) {
	Convey("Given a nil EnvelopeState", t, func() {
		metrics := extractAllMetrics(nil)

		Convey("extractAllMetrics returns nil without panicking", func() {
			So(metrics, ShouldBeNil)
		})
	})

	Convey("Given an empty valid EnvelopeState", t, func() {
		builder := flatbuffers.NewBuilder(1024)
		stateT := &telemetry.EnvelopeStateT{}
		offset := stateT.Pack(builder)
		builder.Finish(offset)
		state := telemetry.GetRootAsEnvelopeState(builder.FinishedBytes(), 0)

		metrics := extractAllMetrics(state)

		Convey("extractAllMetrics returns nil when no measurements exist", func() {
			So(metrics, ShouldBeNil)
		})
	})

	Convey("Given estimated metrics in a persisted measurement", t, func() {
		builder := flatbuffers.NewBuilder(1024)
		stateT := &telemetry.EnvelopeStateT{
			Cvd: &telemetry.EnvelopeMeasurementT{
				Maturity:   0.8,
				Snr:        3,
				SnrDefined: true,
				Metadata: []*telemetry.NamedNumberT{
					{Name: "support", Value: 10},
				},
				Metrics: []*telemetry.EnvelopeMeasurementMetricT{
					{
						Key: "rate",
						Value: &telemetry.EnvelopeMetricT{
							Label: "rate",
							Raw:   10,
							Unit:  "rate",
						},
					},
					{
						Key: "completed_ordinal",
						Value: &telemetry.EnvelopeMetricT{
							Label: "completed_ordinal",
							Raw:   11,
							Unit:  "count",
						},
					},
				},
			},
		}
		offset := stateT.Pack(builder)
		builder.Finish(offset)
		state := telemetry.GetRootAsEnvelopeState(builder.FinishedBytes(), 0)

		metrics := extractAllMetrics(state)

		Convey("extractAllMetrics mirrors the live authority-adjusted value", func() {
			So(metrics["cvd/rate"], ShouldAlmostEqual, 6.0)
			So(metrics["cvd/completed_ordinal"], ShouldEqual, 11.0)
		})
	})
}

func TestBuildPerspectiveRecord(t *testing.T) {
	Convey("Given a durable issued Perspective", t, func() {
		builder := flatbuffers.NewBuilder(1024)
		perspectiveT := &telemetry.EnvelopePerspectiveT{
			Symbol:    "BTC/USD",
			Advisor:   "momentum",
			Question:  "momentum",
			IssuedAt:  100,
			Sequence:  7,
			Round:     3,
			Lifecycle: "issued",
			Classes: []*telemetry.EnvelopePerspectiveClassT{
				{
					State:       "Building",
					Probability: 0.7,
					Evidence:    []string{"cvd/rate"},
				},
				{State: "Stalling", Probability: 0.3},
			},
			Predictions: []*telemetry.EnvelopePerspectivePredictionT{
				{Class: "Building", Event: "cvd/rate", Effect: "supports", Move: "INCREASE"},
				{Class: "Building", Event: "cvd/rate", Effect: "falsifies", Move: "DECREASE"},
			},
			Lease: &telemetry.EnvelopePerspectiveLeaseT{
				Clock: "pumpdump/completed_volume_bar_ordinal",
				From:  12,
				Until: 13,
			},
		}
		offset := perspectiveT.Pack(builder)
		builder.Finish(offset)
		perspective := telemetry.GetRootAsEnvelopePerspective(builder.FinishedBytes(), 0)

		record := buildPerspectiveRecord(
			"run-1", 21, 22, 23, perspective, map[string]float64{"cvd/rate": 4},
		)

		Convey("buildPerspectiveRecord preserves the event-time outcome contract", func() {
			So(record.Run, ShouldEqual, "run-1")
			So(record.Symbol, ShouldEqual, "BTC/USD")
			So(record.Advisor, ShouldEqual, "momentum")
			So(record.ClaimSequence, ShouldEqual, uint64(7))
			So(record.Class, ShouldEqual, "Building")
			So(record.Evidence["Building"], ShouldResemble, []string{"cvd/rate"})
			So(record.Lifecycle, ShouldEqual, "issued")
			So(record.Clock, ShouldEqual, "pumpdump/completed_volume_bar_ordinal")
			So(record.LeaseFrom, ShouldEqual, uint64(12))
			So(record.LeaseUntil, ShouldEqual, uint64(13))
			So(record.Predictions, ShouldHaveLength, 2)
			So(record.Predictions[0].Move, ShouldEqual, "INCREASE")
			So(record.Metrics["cvd/rate"], ShouldEqual, 4.0)
		})
	})
}
