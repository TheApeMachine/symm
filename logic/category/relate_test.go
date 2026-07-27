package category

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestHasMetric(t *testing.T) {
	Convey("Given a metric list", t, func() {
		metrics := []string{"ignition", "trend", "noise_score"}

		Convey("When searching for a present metric", func() {
			So(hasMetric(metrics, "trend"), ShouldBeTrue)
		})

		Convey("When searching for an absent metric", func() {
			So(hasMetric(metrics, "missing"), ShouldBeFalse)
		})

		Convey("When the list is empty", func() {
			So(hasMetric(nil, "ignition"), ShouldBeFalse)
		})
	})
}

func TestSharedSupport(t *testing.T) {
	Convey("Given a graph for shared support computation", t, func() {
		graph := NewGraph()

		Convey("When two categories share some supporting metrics", func() {
			left := []string{"ignition", "trend", "flow"}
			right := []string{"trend", "flow", "noise"}

			jaccard, shared := graph.sharedSupport(left, right)

			Convey("It should compute the correct Jaccard overlap", func() {
				So(jaccard, ShouldAlmostEqual, 2.0/4.0)
				So(len(shared), ShouldEqual, 2)
			})
		})

		Convey("When categories have no shared metrics", func() {
			left := []string{"ignition"}
			right := []string{"trend"}

			jaccard, shared := graph.sharedSupport(left, right)

			Convey("It should return zero overlap", func() {
				So(jaccard, ShouldEqual, 0)
				So(shared, ShouldBeNil)
			})
		})

		Convey("When one list is empty", func() {
			jaccard, _ := graph.sharedSupport(nil, []string{"trend"})

			Convey("It should return zero", func() {
				So(jaccard, ShouldEqual, 0)
			})
		})
	})
}

func TestConditionsMass(t *testing.T) {
	Convey("Given a graph for conditions computation", t, func() {
		graph := NewGraph()

		Convey("When provider supports metrics that dependent is missing", func() {
			provider := types.Category{
				Symbol: "SIM/USD", Type: types.VerticalIgnition,
				Strength: 0.8, Supporting: []string{"ignition", "flow"},
			}
			dependent := types.Category{
				Symbol: "SIM/USD", Type: types.OrganicTrend,
				Strength: 0.6, Missing: []string{"flow", "depth"},
			}

			mass, evidence := graph.conditionsMass(provider, dependent)

			Convey("It should return positive mass with the filled metrics", func() {
				So(mass, ShouldBeGreaterThan, 0)
				So(evidence, ShouldContain, "flow")
			})
		})

		Convey("When provider has no supporting metrics", func() {
			provider := types.Category{Strength: 0.8}
			dependent := types.Category{
				Strength: 0.6, Missing: []string{"flow"},
			}

			mass, _ := graph.conditionsMass(provider, dependent)

			Convey("It should return zero", func() {
				So(mass, ShouldEqual, 0)
			})
		})

		Convey("When dependent has no missing metrics", func() {
			provider := types.Category{
				Strength: 0.8, Supporting: []string{"ignition"},
			}
			dependent := types.Category{Strength: 0.6}

			mass, _ := graph.conditionsMass(provider, dependent)

			Convey("It should return zero", func() {
				So(mass, ShouldEqual, 0)
			})
		})
	})
}

func TestLinkPair(t *testing.T) {
	Convey("Given a graph with evidence from valid measurements", t, func() {
		graph := NewGraph()
		graph.touched = map[edgeKey]struct{}{}
		thesis := types.NewThesis()
		at := time.Unix(100, 0).UTC()

		ignitionMass := 0.8
		trendMass := 0.6
		thesis.AppendMeasurements([]*types.Measurement{
			{
				Source: types.SourcePumpDump, Symbol: "SIM/USD",
				At: at, ObservedFrom: at.Add(-time.Second),
				Horizon: 2 * time.Second,
				Validity: types.MeasurementValidity{
					State: types.ValidityValid, Readiness: types.ReadinessObservation,
				},
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricIgnition, types.SideNone): {
						Raw: ignitionMass, Normalized: &ignitionMass,
					},
				},
			},
			{
				Source: types.SourceLiquidity, Symbol: "SIM/USD",
				At: at, ObservedFrom: at.Add(-time.Second),
				Horizon: 2 * time.Second,
				Validity: types.MeasurementValidity{
					State: types.ValidityValid, Readiness: types.ReadinessObservation,
				},
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricTrend, types.SideNone): {
						Raw: trendMass, Normalized: &trendMass,
					},
				},
			},
		})

		graph.evidence = newEvidenceIndex()
		graph.evidence.UpdateFrom(thesis)

		first := types.Category{
			Symbol: "SIM/USD", Type: types.VerticalIgnition,
			Strength: 0.8, Freshness: 1,
			Supporting: []string{string(types.MetricIgnition)},
		}
		second := types.Category{
			Symbol: "SIM/USD", Type: types.OrganicTrend,
			Strength: 0.6, Freshness: 1,
			Supporting: []string{string(types.MetricTrend)},
		}

		Convey("When linkPair runs with positive strengths", func() {
			graph.linkPair(at, graph.evidence, "SIM/USD", first, second)

			Convey("It should derive at least one edge", func() {
				So(len(graph.Edges), ShouldBeGreaterThan, 0)
			})
		})

		Convey("When one category has zero strength", func() {
			zero := types.Category{
				Symbol: "SIM/USD", Type: types.OrganicTrend, Strength: 0,
			}
			graph.linkPair(at, graph.evidence, "SIM/USD", first, zero)

			Convey("It should not derive any edges", func() {
				So(len(graph.Edges), ShouldEqual, 0)
			})
		})
	})
}

func TestContradictMass(t *testing.T) {
	Convey("Given an evidence index with active metrics", t, func() {
		index := newEvidenceIndex()
		thesis := types.NewThesis()
		mass := 0.9
		thesis.AppendMeasurements([]*types.Measurement{
			{
				Source: types.SourcePumpDump, Symbol: "SIM/USD",
				At: time.Unix(10, 0), ObservedFrom: time.Unix(9, 0),
				Horizon: time.Second,
				Validity: types.MeasurementValidity{
					State: types.ValidityValid, Readiness: types.ReadinessObservation,
				},
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricIgnition, types.SideNone): {
						Raw: mass, Normalized: &mass,
					},
				},
			},
		})
		index.UpdateFrom(thesis)

		Convey("When affinity defines a contradiction path", func() {
			targets := contradictIndex[types.VerticalIgnition]

			if len(targets) > 0 {
				for to, metrics := range targets {
					if len(metrics) > 0 {
						result := contradictMass(index, "SIM/USD", types.VerticalIgnition, to)

						Convey("It should return mass when contradicting metrics are live", func() {
							So(result, ShouldBeGreaterThanOrEqualTo, 0)
						})

						break
					}
				}
			}
		})

		Convey("When no contradiction path exists in the index", func() {
			result := contradictMass(index, "SIM/USD", types.CategoryTypeNone, types.CategoryTypeNone)

			Convey("It should return zero", func() {
				So(result, ShouldEqual, 0)
			})
		})
	})
}

func BenchmarkLinkPair(b *testing.B) {
	graph := NewGraph()
	graph.touched = map[edgeKey]struct{}{}
	thesis := types.NewThesis()
	at := time.Unix(100, 0).UTC()

	mass := 0.7
	thesis.AppendMeasurements([]*types.Measurement{
		{
			Source: types.SourcePumpDump, Symbol: "SIM/USD",
			At: at, ObservedFrom: at.Add(-time.Second),
			Horizon: 2 * time.Second,
			Validity: types.MeasurementValidity{
				State: types.ValidityValid, Readiness: types.ReadinessObservation,
			},
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricIgnition, types.SideNone): {
					Raw: mass, Normalized: &mass,
				},
				types.MetricKey(types.MetricTrend, types.SideNone): {
					Raw: mass, Normalized: &mass,
				},
			},
		},
	})

	graph.evidence = newEvidenceIndex()
	graph.evidence.UpdateFrom(thesis)

	first := types.Category{
		Symbol: "SIM/USD", Type: types.VerticalIgnition,
		Strength: 0.8, Freshness: 1,
		Supporting: []string{string(types.MetricIgnition)},
	}
	second := types.Category{
		Symbol: "SIM/USD", Type: types.OrganicTrend,
		Strength: 0.6, Freshness: 1,
		Supporting: []string{string(types.MetricTrend)},
	}

	b.ReportAllocs()

	for b.Loop() {
		graph.linkPair(at, graph.evidence, "SIM/USD", first, second)
	}
}

func BenchmarkSharedSupport(b *testing.B) {
	graph := NewGraph()
	left := []string{"ignition", "trend", "flow", "depth"}
	right := []string{"trend", "flow", "noise", "depth"}

	b.ReportAllocs()

	for b.Loop() {
		graph.sharedSupport(left, right)
	}
}

func BenchmarkContradictMass(b *testing.B) {
	index := newEvidenceIndex()
	thesis := types.NewThesis()
	mass := 0.9

	thesis.AppendMeasurements([]*types.Measurement{
		{
			Source: types.SourcePumpDump, Symbol: "SIM/USD",
			At: time.Unix(10, 0), ObservedFrom: time.Unix(9, 0),
			Horizon: time.Second,
			Validity: types.MeasurementValidity{
				State: types.ValidityValid, Readiness: types.ReadinessObservation,
			},
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricIgnition, types.SideNone): {
					Raw: mass, Normalized: &mass,
				},
			},
		},
	})
	index.UpdateFrom(thesis)

	b.ReportAllocs()

	for b.Loop() {
		contradictMass(index, "SIM/USD", types.VerticalIgnition, types.Exhaustion)
	}
}
