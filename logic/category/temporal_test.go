package category

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestStaleAndIncomparable(t *testing.T) {
	Convey("Given evidence clocks with horizon semantics", t, func() {
		Convey("When left evidence is older than right by more than left horizon", func() {
			left := evidenceClock{
				from: time.Unix(1, 0), through: time.Unix(2, 0),
				horizon: time.Second, ok: true,
			}
			right := evidenceClock{
				from: time.Unix(10, 0), through: time.Unix(12, 0),
				horizon: 5 * time.Second, ok: true,
			}

			Convey("It reports stale mass and refuses alignable comparison", func() {
				So(staleMass(left, right, 0.8, 0.7), ShouldBeGreaterThan, 0)
				So(alignable(left, right), ShouldBeFalse)
			})
		})

		Convey("When intervals overlap within a shared horizon", func() {
			left := evidenceClock{
				from: time.Unix(1, 0), through: time.Unix(4, 0),
				horizon: 3 * time.Second, ok: true,
			}
			right := evidenceClock{
				from: time.Unix(3, 0), through: time.Unix(5, 0),
				horizon: 3 * time.Second, ok: true,
			}

			Convey("It is alignable and not stale", func() {
				So(alignable(left, right), ShouldBeTrue)
				So(staleMass(left, right, 0.8, 0.7), ShouldEqual, 0)
			})
		})

		Convey("When IncomparableWith is observed on the graph", func() {
			graph := NewGraph()
			at := time.Unix(100, 0).UTC()
			thesis := types.NewThesis()
			thesis.At = at
			ignition := 0.8
			trend := 0.7
			thesis.Measurements = append(thesis.Measurements,
				&types.Measurement{
					Source:       types.SourcePumpDump,
					Symbol:       "SIM/USD",
					At:           time.Unix(1, 0),
					ObservedFrom: time.Unix(0, 0),
					Horizon:      time.Second,
					Validity: types.MeasurementValidity{
						State: types.ValidityValid, Readiness: types.ReadinessObservation,
					},
					Metrics: map[string]types.MetricSample{
						types.MetricKey(types.MetricIgnition, types.SideNone): {
							Raw: ignition, Normalized: &ignition,
						},
					},
				},
				&types.Measurement{
					Source:       types.SourcePumpDump,
					Symbol:       "SIM/USD",
					At:           time.Unix(100, 0),
					ObservedFrom: time.Unix(99, 0),
					Horizon:      time.Second,
					Validity: types.MeasurementValidity{
						State: types.ValidityValid, Readiness: types.ReadinessObservation,
					},
					Metrics: map[string]types.MetricSample{
						types.MetricKey(types.MetricTrend, types.SideNone): {
							Raw: trend, Normalized: &trend,
						},
					},
				},
			)
			thesis.Categories["SIM/USD"] = []types.Category{
				{
					Symbol:     "SIM/USD",
					Type:       types.VerticalIgnition,
					Strength:   ignition,
					Freshness:  1,
					Supporting: []string{string(types.MetricIgnition)},
				},
				{
					Symbol:     "SIM/USD",
					Type:       types.OrganicTrend,
					Strength:   trend,
					Freshness:  1,
					Supporting: []string{string(types.MetricTrend)},
				},
			}
			graph.UpdateFrom(thesis)

			Convey("It strengthens IncomparableWith instead of Leads", func() {
				So(graph.Weight(
					"SIM/USD", types.VerticalIgnition, types.OrganicTrend, IncomparableWith,
				), ShouldBeGreaterThan, 0)
				So(graph.Weight(
					"SIM/USD", types.VerticalIgnition, types.OrganicTrend, Leads,
				), ShouldEqual, 0)
			})
		})
	})
}
