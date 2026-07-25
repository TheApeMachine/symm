package category

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestLinkPairRelations(t *testing.T) {
	Convey("Given Thesis measurements that justify typed category edges", t, func() {
		graph := NewGraph()
		at := time.Unix(100, 0).UTC()

		Convey("When two categories share supporting metrics", func() {
			thesis := types.NewThesis()
			thesis.At = at
			drive := 0.8
			thesis.Publish(types.SourceCVD, []*types.Measurement{
				measurementWithMetric(
					types.SourceCVD, "SIM/USD", at,
					types.MetricDrive, drive, &drive, 2*time.Second,
				),
			})
			categories := Compose(thesis, "SIM/USD")
			graph.Update(at, thesis, categories)

			Convey("It strengthens RedundantWith from Jaccard overlap", func() {
				So(graph.Weight(
					"SIM/USD", types.AggressiveDrive, types.RiskOnSurge, RedundantWith,
				), ShouldBeGreaterThan, 0)
			})
		})

		Convey("When affinity Supports A and Opposes B with live mass", func() {
			thesis := types.NewThesis()
			thesis.At = at
			spoof := 0.9
			fill := 0.7
			thesis.Publish(types.SourceDepthFlow, []*types.Measurement{
				measurementWithMetric(
					types.SourceDepthFlow, "SIM/USD", at,
					types.MetricSpoofScore, spoof, &spoof, 2*time.Second,
				),
			})
			thesis.Publish(types.SourceToxicity, []*types.Measurement{
				measurementWithMetric(
					types.SourceToxicity, "SIM/USD", at,
					types.MetricFillVolume, fill, &fill, 2*time.Second,
				),
			})
			categories := Compose(thesis, "SIM/USD")
			graph.Update(at, thesis, categories)

			Convey("It strengthens Contradicts from CategoryAffinity", func() {
				So(graph.Weight(
					"SIM/USD", types.SpoofTrap, types.HardSupport, Contradicts,
				), ShouldBeGreaterThan, 0)
				So(graph.Weight(
					"SIM/USD", types.HardSupport, types.SpoofTrap, Contradicts,
				), ShouldBeGreaterThan, 0)
			})
		})

		Convey("When supporting evidence clocks are ordered", func() {
			thesis := types.NewThesis()
			thesis.At = at
			ignition := 0.8
			trend := 0.7
			early := at.Add(-3 * time.Second)
			late := at.Add(-time.Second)
			thesis.Measurements = append(thesis.Measurements,
				measurementWithMetric(
					types.SourcePumpDump, "SIM/USD", early,
					types.MetricIgnition, ignition, &ignition, 5*time.Second,
				),
				measurementWithMetric(
					types.SourcePumpDump, "SIM/USD", late,
					types.MetricTrend, trend, &trend, 5*time.Second,
				),
			)
			categories := Compose(thesis, "SIM/USD")
			graph.Update(at, thesis, categories)

			Convey("It strengthens Leads/Lags from evidence envelopes", func() {
				So(graph.Weight(
					"SIM/USD", types.VerticalIgnition, types.OrganicTrend, Leads,
				), ShouldBeGreaterThan, 0)
				So(graph.Weight(
					"SIM/USD", types.OrganicTrend, types.VerticalIgnition, Lags,
				), ShouldBeGreaterThan, 0)
			})
		})

		Convey("When provider supporting fills dependent missing", func() {
			first := types.Category{
				Symbol: "SIM/USD", Type: types.VerticalIgnition,
				Strength: 0.8, Supporting: []string{string(types.MetricIgnition)},
			}
			second := types.Category{
				Symbol: "SIM/USD", Type: types.RiskOnSurge,
				Strength: 0.5, Supporting: []string{string(types.MetricDrive)},
				Missing: []string{string(types.MetricIgnition)},
			}
			mass, filled := conditionsMass(first, second)

			Convey("It reports Conditions mass from filled missing evidence", func() {
				So(mass, ShouldBeGreaterThan, 0)
				So(filled, ShouldContain, string(types.MetricIgnition))
			})
		})
	})
}

func BenchmarkLinkPair(b *testing.B) {
	graph := NewGraph()
	at := time.Unix(100, 0).UTC()
	thesis := types.NewThesis()
	thesis.At = at
	value := 0.7
	thesis.Measurements = append(thesis.Measurements,
		measurementWithMetric(
			types.SourcePumpDump, "SIM/USD", at.Add(-2*time.Second),
			types.MetricIgnition, value, &value, 5*time.Second,
		),
		measurementWithMetric(
			types.SourcePumpDump, "SIM/USD", at.Add(-time.Second),
			types.MetricTrend, value, &value, 5*time.Second,
		),
	)
	categories := Compose(thesis, "SIM/USD")

	for b.Loop() {
		graph.Update(at, thesis, categories)
	}
}
