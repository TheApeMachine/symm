package category

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func measurementWithMetric(
	source types.SourceType,
	symbol string,
	at time.Time,
	metric types.MetricType,
	raw float64,
	normalized *float64,
	horizon time.Duration,
) *types.Measurement {
	row := &types.Measurement{
		Source:       source,
		Symbol:       symbol,
		At:           at,
		ObservedFrom: at.Add(-time.Second),
		Horizon:      horizon,
		Validity: types.MeasurementValidity{
			State: types.ValidityValid, Readiness: types.ReadinessObservation,
		},
		Metrics: map[string]types.MetricSample{
			types.MetricKey(metric, types.SideNone): {
				Raw: raw, Normalized: normalized,
			},
		},
	}

	return row
}

/*
publishMetrics upserts one source×symbol row with every named metric sample.
*/
func publishMetrics(
	thesis *types.Thesis,
	source types.SourceType,
	symbol string,
	at time.Time,
	horizon time.Duration,
	samples map[types.MetricType]types.MetricSample,
) {
	row := &types.Measurement{
		Source:       source,
		Symbol:       symbol,
		At:           at,
		ObservedFrom: at.Add(-time.Second),
		Horizon:      horizon,
		Validity: types.MeasurementValidity{
			State: types.ValidityValid, Readiness: types.ReadinessObservation,
		},
		Metrics: map[string]types.MetricSample{},
	}

	for metric, sample := range samples {
		row.Metrics[types.MetricKey(metric, types.SideNone)] = sample
	}

	thesis.Publish(source, []*types.Measurement{row})
}

func TestGraphUpdate(t *testing.T) {
	Convey("Given a resident graph updated from composed thesis evidence", t, func() {
		graph := NewGraph()
		at := time.Unix(1, 0).UTC()
		thesis := types.NewThesis()
		thesis.At = at
		spoof := 0.9
		fill := 0.4
		thesis.Publish(types.SourceDepthFlow, []*types.Measurement{
			measurementWithMetric(
				types.SourceDepthFlow, "PENGU/USD", at,
				types.MetricSpoofScore, spoof, &spoof, 2*time.Second,
			),
		})
		thesis.Publish(types.SourceToxicity, []*types.Measurement{
			measurementWithMetric(
				types.SourceToxicity, "PENGU/USD", at,
				types.MetricFillVolume, fill, &fill, 2*time.Second,
			),
		})
		categories := Compose(thesis, "PENGU/USD")

		Convey("When Update runs twice on the same evidence", func() {
			graph.Update(at, thesis, categories)
			firstWeight := graph.Weight(
				"PENGU/USD", types.SpoofTrap, types.HardSupport, Contradicts,
			)
			graph.Update(at.Add(time.Second), thesis, categories)
			secondWeight := graph.Weight(
				"PENGU/USD", types.SpoofTrap, types.HardSupport, Contradicts,
			)

			Convey("It should keep the same graph and strengthen Contradicts", func() {
				So(firstWeight, ShouldBeGreaterThan, 0)
				So(secondWeight, ShouldBeGreaterThan, firstWeight)
			})

			Convey("It should report trap pressure from node masses", func() {
				share, _ := Report(graph).TrapPressure("PENGU/USD")
				So(share, ShouldBeGreaterThan, 0)
			})
		})

		Convey("When a category newly activates after a peer was already active", func() {
			early := types.NewThesis()
			early.At = at
			ignition := 0.9
			early.Publish(types.SourcePumpDump, []*types.Measurement{
				measurementWithMetric(
					types.SourcePumpDump, "PENGU/USD", at,
					types.MetricIgnition, ignition, &ignition, 2*time.Second,
				),
			})
			graph.Update(at, early, Compose(early, "PENGU/USD"))

			later := types.NewThesis()
			laterAt := at.Add(2 * time.Second)
			later.At = laterAt
			trend := 0.8
			publishMetrics(later, types.SourcePumpDump, "PENGU/USD", laterAt, 2*time.Second, map[types.MetricType]types.MetricSample{
				types.MetricIgnition: {Raw: ignition, Normalized: &ignition},
				types.MetricTrend:    {Raw: trend, Normalized: &trend},
			})
			graph.Update(laterAt, later, Compose(later, "PENGU/USD"))

			Convey("It should strengthen Leads from prior activation order", func() {
				So(graph.Weight(
					"PENGU/USD", types.VerticalIgnition, types.OrganicTrend, Leads,
				), ShouldBeGreaterThan, 0)
			})
		})
	})
}

func BenchmarkGraphUpdate(b *testing.B) {
	graph := NewGraph()
	at := time.Unix(1, 0).UTC()
	thesis := types.NewThesis()
	thesis.At = at
	value := 0.8
	thesis.Publish(types.SourcePumpDump, []*types.Measurement{
		measurementWithMetric(
			types.SourcePumpDump, "PENGU/USD", at,
			types.MetricIgnition, value, &value, time.Second,
		),
	})
	categories := Compose(thesis, "PENGU/USD")

	for b.Loop() {
		graph.Update(at, thesis, categories)
	}
}
