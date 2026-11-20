package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/system"
)

func TestObserveRushAndNet(t *testing.T) {
	Convey("Setup", t, func() {
		tape := NewTape()
		at := time.Unix(1_700_000_000, 0).UTC()

		Convey("When buy and sell market prints land", func() {
			tape.Observe("BTC/USD", market.SideBuy, market.OrderTypeMarket, 400, at)
			tape.Observe("BTC/USD", market.SideSell, market.OrderTypeMarket, 100, at)
			tape.Observe("BTC/USD", market.SideBuy, market.OrderTypeLimit, 50, at)

			Convey("Reading splits rush, net, and buy uptake", func() {
				flow := tape.Reading("BTC/USD", at, 0)
				So(flow.BuyRecent, ShouldEqual, 450)
				So(flow.SellRecent, ShouldEqual, 100)
				So(flow.RushBuy, ShouldEqual, 400)
				So(flow.RushSell, ShouldEqual, 100)
				So(flow.NetFraction, ShouldAlmostEqual, (450-100)/550.0, 0.001)
				recent, _ := tape.Uptake("BTC/USD", at)
				So(recent, ShouldEqual, 450)
			})
		})
	})
}

func TestObserveFlipChurn(t *testing.T) {
	Convey("Setup", t, func() {
		tape := NewTape()
		at := time.Unix(1_700_000_000, 0).UTC()

		Convey("When near-equal opposing prints flip repeatedly", func() {
			for i := 0; i < 120; i++ {
				side := market.SideBuy
				if i%2 == 1 {
					side = market.SideSell
				}
				tape.Observe("BOT/USD", side, market.OrderTypeLimit, 100, at.Add(time.Duration(i)*time.Second))
			}

			Convey("Flip churn exceeds the bump-bot threshold", func() {
				flow := tape.Reading("BOT/USD", at.Add(120*time.Second), 0)
				So(flow.FlipChurn, ShouldBeGreaterThan, system.Cfg.Risk.MaxFlipChurn)
			})
		})
	})
}

func TestConditionedRatioFloorsDormant(t *testing.T) {
	Convey("Setup", t, func() {
		tape := NewTape()
		now := time.Unix(1_700_000_600, 0).UTC()

		Convey("When a dormant tape wakes with a tiny print over near-zero prior", func() {
			tape.Note("SLEEP/USD", 1, now.Add(-10*time.Minute))
			tape.Note("SLEEP/USD", 251, now)

			Convey("Conditioned ratio stays below a raw 251× explosion", func() {
				flow := tape.Reading("SLEEP/USD", now, 10_000)
				So(flow.Ratio, ShouldBeGreaterThan, 100)
				So(flow.ConditionedRatio, ShouldBeLessThan, flow.Ratio)
				So(flow.ConditionedRatio, ShouldBeLessThanOrEqualTo,
					251/system.Cfg.Risk.MinTapeNotionalUSD+0.01)
			})
		})
	})
}
