package trader

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

type stubMarket struct {
	snapshots map[string]engine.Snapshot
}

func (market stubMarket) Read(symbol string) engine.Snapshot {
	return market.snapshots[symbol]
}

func (market stubMarket) ReadFresh(
	symbol string,
	_ time.Time,
	_ time.Duration,
) engine.Snapshot {
	return market.Read(symbol)
}

func TestClassifyMarketRegime(t *testing.T) {
	now := time.Now()

	Convey("Given directional two-sided flow", t, func() {
		regime := ClassifyMarketRegime(stubMarket{snapshots: map[string]engine.Snapshot{
			"A/EUR": {LastOK: true, PressureOK: true, BuyPressure: 0.6, BatchOK: true, BatchVolume: 10, SpreadOK: true},
			"B/EUR": {LastOK: true, PressureOK: true, BuyPressure: 0.55, BatchOK: true, BatchVolume: 12, SpreadOK: true},
		}}, []string{"A/EUR", "B/EUR"}, now)

		Convey("It should classify trending", func() {
			So(regime, ShouldEqual, RegimeTrending)
		})
	})

	Convey("Given quiet low-activity symbols", t, func() {
		regime := ClassifyMarketRegime(stubMarket{snapshots: map[string]engine.Snapshot{
			"A/EUR": {LastOK: true, PressureOK: true, BuyPressure: 0.01, SpreadOK: true, BatchOK: true},
			"B/EUR": {LastOK: true, PressureOK: true, BuyPressure: -0.01, SpreadOK: true, BatchOK: true},
		}}, []string{"A/EUR", "B/EUR"}, now)

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

func TestRequiredEdgeReturnIncludesDynamicCosts(t *testing.T) {
	Convey("Given default fee config and a tight quote", t, func() {
		edge := requiredEdgeReturn(
			stubPrices{"PUMP/EUR": 100},
			stubMarket{snapshots: map[string]engine.Snapshot{
				"PUMP/EUR": {LastOK: true, SpreadOK: true, BatchOK: true},
			}},
			"PUMP/EUR",
			10,
			time.Now(),
		)

		Convey("It should include spread, fees, and min edge", func() {
			So(edge, ShouldBeGreaterThan, 0.005)
			So(edge, ShouldBeLessThan, 0.02)
		})
	})
}
