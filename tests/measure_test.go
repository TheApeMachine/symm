package tests

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func measurementWithSamples(
	source types.SourceType,
	symbol string,
	at time.Time,
	samples map[types.MetricType]float64,
) *types.Measurement {
	row := &types.Measurement{
		Source:  source,
		Symbol:  symbol,
		At:      at,
		Metrics: map[string]types.MetricSample{},
	}

	for metric, raw := range samples {
		row.PutMetric(metric, types.SideNone, types.MetricSample{Raw: raw})
	}

	return row
}

func TestPeakMeasurements(t *testing.T) {
	Convey("PeakMeasurements indexes Metrics map samples", t, func() {
		base := time.Unix(10, 0).UTC()
		rows := []*types.Measurement{
			measurementWithSamples(types.SourceLeadLag, "A/USD", base, map[types.MetricType]float64{
				types.MetricSync: 0.2,
			}),
			measurementWithSamples(types.SourceLeadLag, "A/USD", base.Add(time.Second), map[types.MetricType]float64{
				types.MetricSync: 0.9,
			}),
			measurementWithSamples(types.SourceLeadLag, "B/USD", base, map[types.MetricType]float64{
				types.MetricSync: 0.5,
			}),
		}
		peak := PeakMeasurements(rows, types.SourceLeadLag, []types.MetricType{types.MetricSync})

		So(peak[types.MetricSync]["A/USD"], ShouldEqual, 0.9)
		So(peak[types.MetricSync]["B/USD"], ShouldEqual, 0.5)
	})
}

func TestLatestMeasurements(t *testing.T) {
	Convey("LatestMeasurements keeps newest At per metric", t, func() {
		base := time.Unix(10, 0).UTC()
		rows := []*types.Measurement{
			measurementWithSamples(types.SourceCorrelation, "A/USD", base, map[types.MetricType]float64{
				types.MetricHerdScore: 0.1,
			}),
			measurementWithSamples(types.SourceCorrelation, "A/USD", base.Add(2*time.Second), map[types.MetricType]float64{
				types.MetricHerdScore: 0.8,
			}),
		}
		latest := LatestMeasurements(
			rows, types.SourceCorrelation, []types.MetricType{types.MetricHerdScore},
		)

		So(latest[types.MetricHerdScore]["A/USD"], ShouldEqual, 0.8)
	})
}

func TestPeakMagnitudeMeasurements(t *testing.T) {
	Convey("PeakMagnitudeMeasurements tracks largest absolute Raw", t, func() {
		base := time.Unix(10, 0).UTC()
		rows := []*types.Measurement{
			measurementWithSamples(types.SourceSentiment, "A/USD", base, map[types.MetricType]float64{
				types.MetricChange: -0.4,
			}),
			measurementWithSamples(types.SourceSentiment, "A/USD", base.Add(time.Second), map[types.MetricType]float64{
				types.MetricChange: 0.2,
			}),
		}
		peak := PeakMagnitudeMeasurements(
			rows, types.SourceSentiment, []types.MetricType{types.MetricChange},
		)

		So(peak[types.MetricChange]["A/USD"], ShouldEqual, -0.4)
	})
}
