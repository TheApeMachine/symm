package logic

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func addLeadLagStrength(graph *types.Graph, symbol string, at time.Time) {
	strength := 0.6
	So(graph.AddNode(&types.Measurement{
		Source: types.SourceLeadLag, Stream: types.LeadLag,
		Metric: types.MetricStrength, Symbol: symbol, At: at,
		Unit: types.UnitDimensionless, Normalized: &strength,
		Validity: types.MeasurementValidity{State: types.ValidityValid},
	}), ShouldBeNil)
}

func TestRelateLeadLag(t *testing.T) {
	Convey("Given an anchor and a follower whose direction names the anchor", t, func() {
		analyzer := &Analyzer{}
		thesis := types.NewThesis(nil)
		at := time.Unix(200, 0)

		anchorGraph := types.NewGraph("BTC/USD")
		addLeadLagStrength(anchorGraph, "BTC/USD", at)
		thesis.Graphs.Store("BTC/USD", anchorGraph)

		followerGraph := types.NewGraph("ETH/USD")
		leads := 1.0
		So(followerGraph.AddNode(&types.Measurement{
			Source: types.SourceLeadLag, Stream: types.LeadLag,
			Metric: types.MetricSignedLagDirection, Symbol: "ETH/USD",
			Peer: "BTC/USD", At: at, Unit: types.UnitDimensionless,
			Raw: 1, Normalized: &leads,
			Validity: types.MeasurementValidity{State: types.ValidityValid},
		}), ShouldBeNil)
		thesis.Graphs.Store("ETH/USD", followerGraph)

		analyzer.relateLeadLag(thesis)

		Convey("Then the follower graph carries directed Leads and Lags edges", func() {
			var leads, lags int
			anchorReferenced := false
			anchorKey := ""

			for _, node := range followerGraph.Nodes() {
				if node.Measurement.Symbol == "BTC/USD" {
					anchorReferenced = true
					anchorKey = node.Key
				}
			}

			So(anchorReferenced, ShouldBeTrue)

			for _, edge := range followerGraph.Edges() {
				switch edge.Type {
				case types.Leads:
					leads++
					So(edge.From, ShouldEqual, anchorKey) // anchor leads follower
				case types.Lags:
					lags++
				}
			}

			So(leads, ShouldEqual, 1)
			So(lags, ShouldEqual, 1)
		})
	})

	Convey("Given a follower with no signed lag direction", t, func() {
		analyzer := &Analyzer{}
		thesis := types.NewThesis(nil)
		at := time.Unix(200, 0)
		anchorGraph := types.NewGraph("BTC/USD")
		addLeadLagStrength(anchorGraph, "BTC/USD", at)
		thesis.Graphs.Store("BTC/USD", anchorGraph)

		followerGraph := types.NewGraph("ETH/USD")
		zero := 0.0
		So(followerGraph.AddNode(&types.Measurement{
			Source:     types.SourceLeadLag,
			Stream:     types.LeadLag,
			Metric:     types.MetricSignedLagDirection,
			Symbol:     "ETH/USD",
			Peer:       "BTC/USD",
			At:         at,
			Unit:       types.UnitDimensionless,
			Normalized: &zero,
			Validity:   types.MeasurementValidity{State: types.ValidityValid},
		}), ShouldBeNil)
		thesis.Graphs.Store("ETH/USD", followerGraph)

		analyzer.relateLeadLag(thesis)

		Convey("Then no lead or lag relationship is invented", func() {
			So(followerGraph.Edges(), ShouldBeEmpty)
		})
	})
}

func TestRelateCausal(t *testing.T) {
	Convey("Given a ready causal hypothesis with a finite effect", t, func() {
		analyzer := &Analyzer{}
		thesis := types.NewThesis(nil)
		at := time.Unix(300, 0)

		graph := types.NewGraph("BTC/USD")
		strength := 0.5
		So(graph.AddNode(&types.Measurement{
			Source: types.SourceHawkes, Stream: types.Hawkes,
			Metric: types.MetricStrength, Symbol: "BTC/USD", At: at,
			Unit: types.UnitDimensionless, Normalized: &strength,
			Validity: types.MeasurementValidity{State: types.ValidityValid},
		}), ShouldBeNil)
		thesis.Graphs.Store("BTC/USD", graph)

		thesis.Hypotheses = append(thesis.Hypotheses, types.Hypothesis{
			Source: types.SourceCausal, Symbol: "BTC/USD", At: at, Ready: true,
			Treatment: "buy_sell_arrival_intensity_imbalance",
			Outcome:   "next_l3_epoch_mid_log_return", DoExpectation: 0.002,
		})

		analyzer.relateCausal(thesis)

		Convey("Then a directed Conditions edge links treatment to outcome", func() {
			var conditions int

			for _, edge := range graph.Edges() {
				if edge.Type == types.Conditions {
					conditions++
					So(edge.From, ShouldEqual,
						types.ConceptKey("buy_sell_arrival_intensity_imbalance"))
					So(edge.To, ShouldEqual,
						types.ConceptKey("next_l3_epoch_mid_log_return"))
				}
			}

			So(conditions, ShouldEqual, 1)
		})

		Convey("Then a hypothesis with no effect draws no edge", func() {
			flat := types.NewThesis(nil)
			flatGraph := types.NewGraph("BTC/USD")
			flat.Graphs.Store("BTC/USD", flatGraph)
			flat.Hypotheses = append(flat.Hypotheses, types.Hypothesis{
				Source: types.SourceCausal, Symbol: "BTC/USD", At: at, Ready: true,
				Treatment:     "buy_sell_arrival_intensity_imbalance",
				Outcome:       "next_l3_epoch_mid_log_return",
				DoExpectation: 0, Uplift: 0,
			})

			analyzer.relateCausal(flat)
			So(flatGraph.Edges(), ShouldBeEmpty)
		})
	})
}
