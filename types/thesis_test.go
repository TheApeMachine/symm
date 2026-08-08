package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestThesisAppendMeasurements(t *testing.T) {
	Convey("Given multiple measurements from one signal", t, func() {
		thesis := NewThesis(nil)
		measurements := []*Measurement{
			{Source: SourceLeadLag, Symbol: "BTC/USD", At: time.Unix(1, 0)},
			{Source: SourceLeadLag, Symbol: "ETH/USD", At: time.Unix(2, 0)},
			{Source: SourceLeadLag, Symbol: "SOL/USD", At: time.Unix(3, 0)},
		}

		thesis.AppendMeasurements(measurements, true)

		Convey("Then it should retain each measurement exactly once in source order", func() {
			stored, found := thesis.Measurements.Load(SourceLeadLag)
			So(found, ShouldBeTrue)
			actual := stored.([]*Measurement)
			So(actual, ShouldResemble, measurements)
			So(thesis.Readiness.LeadLag, ShouldBeTrue)
		})
	})

	Convey("Given a new measurement for one previously observed symbol", t, func() {
		thesis := NewThesis(nil)
		bitcoin := &Measurement{
			Source: SourceHawkes,
			Symbol: "BTC/USD",
			At:     time.Unix(1, 0),
		}
		ether := &Measurement{
			Source: SourceHawkes,
			Symbol: "ETH/USD",
			At:     time.Unix(1, 0),
		}
		updated := &Measurement{
			Source: SourceHawkes,
			Symbol: "BTC/USD",
			At:     time.Unix(2, 0),
		}
		thesis.AppendMeasurements([]*Measurement{bitcoin, ether}, false)
		thesis.AppendMeasurements([]*Measurement{updated}, false)

		Convey("Then it should replace that identity and retain the other symbol", func() {
			stored, found := thesis.Measurements.Load(SourceHawkes)
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, []*Measurement{ether, updated})
		})
	})

	Convey("Given multiple rows sharing one measurement identity", t, func() {
		thesis := NewThesis(nil)
		previous := []*Measurement{
			{Source: SourceToxicity, Symbol: "BTC/USD", At: time.Unix(1, 0)},
			{Source: SourceToxicity, Symbol: "BTC/USD", At: time.Unix(2, 0)},
		}
		updated := []*Measurement{
			{Source: SourceToxicity, Symbol: "BTC/USD", At: time.Unix(3, 0)},
			{Source: SourceToxicity, Symbol: "BTC/USD", At: time.Unix(4, 0)},
		}
		thesis.AppendMeasurements(previous, false)
		thesis.AppendMeasurements(updated, false)

		Convey("Then the new identity group should replace the previous group intact", func() {
			stored, found := thesis.Measurements.Load(SourceToxicity)
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, updated)
		})
	})
}

func TestThesisReset(t *testing.T) {
	Convey("Given a completed Thesis with ready measurement evidence", t, func() {
		thesis := NewThesis(nil)
		measurements := []*Measurement{{
			Source: SourceToxicity,
			Symbol: "BTC/USD",
			At:     time.Unix(1, 0),
		}}
		consumed := kraken.TradeData{
			Symbol: "BTC/USD", TradeID: 1, Timestamp: time.Unix(1, 0),
		}
		thesis.AppendTrade(consumed)
		So(thesis.MarketTrades(SourceToxicity), ShouldHaveLength, 1)
		thesis.AppendMeasurements(measurements, true)
		thesis.Categories.Store("BTC/USD", []Category{{Symbol: "BTC/USD"}})

		for _, source := range []SourceType{
			SourceCorrelation,
			SourceCVD,
			SourceDepthFlow,
			SourceExhaustion,
			SourceHawkes,
			SourceLeadLag,
			SourceLiquidity,
			SourcePumpDump,
			SourceSentiment,
			SourceCategories,
			SourceCognition,
			SourceManifold,
			SourceResonance,
			SourceCausal,
			SourceGraph,
			SourcePlanner,
		} {
			thesis.Stamp(source)
		}

		So(thesis.Readiness.Complete(), ShouldBeTrue)
		thesis.Reset()

		Convey("Then the next epoch should retain only the prior measurements", func() {
			stored, found := thesis.Measurements.Load(SourceToxicity)
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, measurements)
			So(thesis.Readiness.Complete(), ShouldBeFalse)
			_, found = thesis.Categories.Load("BTC/USD")
			So(found, ShouldBeFalse)

			thesis.AppendTrade(consumed)
			thesis.AppendTrade(kraken.TradeData{
				Symbol: "BTC/USD", TradeID: 2, Timestamp: time.Unix(2, 0),
			})
			unseen := thesis.MarketTrades(SourceToxicity)
			So(unseen, ShouldHaveLength, 1)
			So(unseen[0].TradeID, ShouldEqual, 2)
		})
	})
}
