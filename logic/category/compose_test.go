package category

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestCompose(t *testing.T) {
	Convey("Given affinity-bearing measurements on one symbol", t, func() {
		thesis := types.NewThesis()
		thesis.At = time.Unix(10, 0).UTC()
		ignition := 0.8
		spoof := 0.6
		maturity := 0.9

		thesis.Publish(types.SourcePumpDump, []*types.Measurement{{
			Source:   types.SourcePumpDump,
			Symbol:   "PENGU/USD",
			At:       thesis.At,
			Maturity: maturity,
			Horizon:  time.Second,
			Validity: types.MeasurementValidity{
				State: types.ValidityValid, Readiness: types.ReadinessObservation,
			},
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricIgnition, types.SideNone): {
					Raw: 0.8, Normalized: &ignition,
				},
			},
		}})
		thesis.Publish(types.SourceDepthFlow, []*types.Measurement{{
			Source:   types.SourceDepthFlow,
			Symbol:   "PENGU/USD",
			At:       thesis.At,
			Maturity: maturity,
			Horizon:  time.Second,
			Validity: types.MeasurementValidity{
				State: types.ValidityValid, Readiness: types.ReadinessObservation,
			},
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricSpoofScore, types.SideNone): {
					Raw: 0.6, Normalized: &spoof,
				},
			},
		}})

		Convey("When Compose runs", func() {
			categories := Compose(thesis, "PENGU/USD")

			Convey("It should emit supported categories with evidence lists", func() {
				So(len(categories), ShouldBeGreaterThan, 0)

				var ignitionCat, spoofCat *types.Category

				for index := range categories {
					row := &categories[index]

					if row.Type == types.VerticalIgnition {
						ignitionCat = row
					}

					if row.Type == types.SpoofTrap {
						spoofCat = row
					}
				}

				So(ignitionCat, ShouldNotBeNil)
				So(ignitionCat.Strength, ShouldBeGreaterThan, 0)
				So(ignitionCat.Supporting, ShouldContain, string(types.MetricIgnition))
				So(len(ignitionCat.Missing), ShouldBeGreaterThan, 0)

				So(spoofCat, ShouldNotBeNil)
				So(spoofCat.Supporting, ShouldContain, string(types.MetricSpoofScore))
				So(spoofCat.Opposing, ShouldBeNil)
			})
		})
	})
}

func BenchmarkCompose(b *testing.B) {
	thesis := types.NewThesis()
	thesis.At = time.Unix(10, 0).UTC()
	value := 0.5
	thesis.Publish(types.SourcePumpDump, []*types.Measurement{{
		Source: types.SourcePumpDump,
		Symbol: "PENGU/USD", At: thesis.At,
		Maturity: 0.5, Horizon: time.Second,
		Validity: types.MeasurementValidity{
			State: types.ValidityValid, Readiness: types.ReadinessObservation,
		},
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricIgnition, types.SideNone): {
				Raw: 0.5, Normalized: &value,
			},
		},
	}})
	thesis.Publish(types.SourceDepthFlow, []*types.Measurement{{
		Source: types.SourceDepthFlow,
		Symbol: "PENGU/USD", At: thesis.At,
		Maturity: 0.5, Horizon: time.Second,
		Validity: types.MeasurementValidity{
			State: types.ValidityValid, Readiness: types.ReadinessObservation,
		},
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricSpoofScore, types.SideNone): {
				Raw: 0.5, Normalized: &value,
			},
		},
	}})

	for b.Loop() {
		_ = Compose(thesis, "PENGU/USD")
	}
}

/*
BenchmarkComposeAllFrom measures the allocation reduction from passing
a pre-snapshotted slice versus calling ComposeAll on a thesis each tick.
*/
func BenchmarkComposeAllFrom(b *testing.B) {
	at := time.Unix(10, 0).UTC()
	value := 0.5
	thesis := types.NewThesis()
	thesis.At = at
	thesis.Publish(types.SourcePumpDump, []*types.Measurement{{
		Source: types.SourcePumpDump,
		Symbol: "PENGU/USD", At: at,
		Maturity: 0.5, Horizon: time.Second,
		Validity: types.MeasurementValidity{
			State: types.ValidityValid, Readiness: types.ReadinessObservation,
		},
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricIgnition, types.SideNone): {
				Raw: value, Normalized: &value,
			},
		},
	}})

	measurements := thesis.SnapshotMeasurements()

	b.ReportAllocs()

	for b.Loop() {
		_ = ComposeAllFrom(measurements, at)
	}
}
