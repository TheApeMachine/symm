package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

type stubMarket struct {
	snapshots map[string]engine.Snapshot
}

func (market stubMarket) Read(symbol string) engine.Snapshot {
	return market.snapshots[symbol]
}

func TestClassifyMarketRegime(t *testing.T) {
	Convey("Given directional two-sided flow", t, func() {
		regime := ClassifyMarketRegime(stubMarket{snapshots: map[string]engine.Snapshot{
			"A/EUR": {LastOK: true, PressureOK: true, BuyPressure: 0.6, BatchOK: true, BatchVolume: 10},
			"B/EUR": {LastOK: true, PressureOK: true, BuyPressure: 0.55, BatchOK: true, BatchVolume: 12},
		}}, []string{"A/EUR", "B/EUR"})

		Convey("It should classify trending", func() {
			So(regime, ShouldEqual, RegimeTrending)
		})
	})

	Convey("Given quiet low-activity symbols", t, func() {
		regime := ClassifyMarketRegime(stubMarket{snapshots: map[string]engine.Snapshot{
			"A/EUR": {LastOK: true, PressureOK: true, BuyPressure: 0.01},
			"B/EUR": {LastOK: true, PressureOK: true, BuyPressure: -0.01},
		}}, []string{"A/EUR", "B/EUR"})

		Convey("It should classify dead", func() {
			So(regime, ShouldEqual, RegimeDead)
		})
	})
}

func TestRegimeWeight(t *testing.T) {
	Convey("Given a trending regime", t, func() {
		Convey("It should favor hawkes over pumpdump", func() {
			So(RegimeWeight(RegimeTrending, "hawkes"), ShouldBeGreaterThan, RegimeWeight(RegimeTrending, "pumpdump"))
		})
	})
}

func TestRequiredEdgeReturnOmitsSpreadDoubleCount(t *testing.T) {
	Convey("Given default fee config and a tight quote", t, func() {
		edge := requiredEdgeReturn(stubPrices{"PUMP/EUR": 100}, "PUMP/EUR")

		Convey("It should not add spread on top of round-trip fees", func() {
			So(edge, ShouldBeLessThan, 0.007)
		})
	})
}
