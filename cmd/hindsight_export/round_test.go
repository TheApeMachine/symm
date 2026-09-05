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

