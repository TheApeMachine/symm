package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/user"
)

func TestPositionMonitorBalanceOwnership(t *testing.T) {
	Convey("Given a backend balance snapshot without P&L economics", t, func() {
		monitor := NewPositionMonitor()
		changed := monitor.ApplyBalance(user.Balances{
			Currency:  "USD",
			Balance:   100,
			Inventory: map[string]float64{"BTC": 0.01},
			AvgEntry:  map[string]float64{"BTC": 50_000},
			Marks:     map[string]float64{"BTC/USD": 50_500},
		})

		frame := monitor.Snapshot()

		Convey("It should expose the position but not invent P&L", func() {
			So(changed, ShouldBeTrue)
			So(frame.OpenPositions, ShouldEqual, 1)
			So(frame.PricedPositions, ShouldEqual, 0)
			So(frame.Positions[0].Mark, ShouldEqual, 50_500)
			So(frame.Positions[0].Priced, ShouldBeFalse)
			So(frame.ExitBalance, ShouldEqual, 0)
		})
	})
}

func TestPositionMonitorStopTicker(t *testing.T) {
	Convey("Given a trailing stop monitoring a live position", t, func() {
		monitor := NewPositionMonitor()
		stopLoss, stopErr := NewStopLoss(
			"BTC/USD",
			0.01,
			50_000,
			0,
			config.ExitConfig{TrailDefault: 0.015, StopFloor: 0.012},
		)
		ticker := &market.TickerUpdate{
			Symbol:    "BTC/USD",
			Last:      50_630,
			Timestamp: time.Unix(1710000000, 0).UTC(),
		}

		_, ratchetErr := stopLoss.Ratchet(ticker)
		changed := monitor.ApplyStopTicker(stopLoss, ticker)
		frame := monitor.Snapshot()

		Convey("It should publish the same mark used by the stop monitor", func() {
			So(stopErr, ShouldBeNil)
			So(ratchetErr, ShouldBeNil)
			So(changed, ShouldBeTrue)
			So(frame.OpenPositions, ShouldEqual, 1)
			So(frame.PricedPositions, ShouldEqual, 1)
			So(frame.ExitValue, ShouldAlmostEqual, 506.3, 1e-9)
			So(frame.ExitBalance, ShouldAlmostEqual, 6.3, 1e-9)
			So(frame.Positions[0].Mark, ShouldEqual, 50_630)
			So(frame.Positions[0].PeakPrice, ShouldEqual, 50_630)
			So(frame.Positions[0].StopPrice, ShouldAlmostEqual, 49_870.55, 1e-9)
			So(frame.Positions[0].MarkSource, ShouldEqual, "stop_monitor")
		})
	})
}

func TestPositionMonitorUsesBidSideExitValue(t *testing.T) {
	Convey("Given a monitor position with an exit fee", t, func() {
		monitor := NewPositionMonitor()
		monitor.ApplyBalance(user.Balances{
			Currency:    "USD",
			Balance:     50,
			Inventory:   map[string]float64{"BTC": 0.5},
			AvgEntry:    map[string]float64{"BTC": 100},
			ExitFeeRate: map[string]float64{"BTC": 0.002},
		})

		stopLoss, stopErr := NewStopLoss(
			"BTC/USD",
			0.5,
			100,
			0,
			config.ExitConfig{TrailDefault: 0.015, StopFloor: 0.012},
		)
		ticker := &market.TickerUpdate{
			Symbol:    "BTC/USD",
			Last:      105,
			Bid:       103,
			Ask:       103.2,
			Timestamp: time.Unix(1710000001, 0).UTC(),
		}

		changed := monitor.ApplyStopTicker(stopLoss, ticker)
		frame := monitor.Snapshot()

		Convey("It should price liquidation from bid after exit fee", func() {
			So(stopErr, ShouldBeNil)
			So(changed, ShouldBeTrue)
			So(frame.PricedPositions, ShouldEqual, 1)
			So(frame.Positions[0].Mark, ShouldEqual, 103)
			So(frame.Positions[0].ExitValue, ShouldAlmostEqual, 51.397, 1e-9)
			So(frame.Positions[0].Unrealized, ShouldAlmostEqual, 1.397, 1e-9)
			So(frame.ExitBalance, ShouldAlmostEqual, 1.397, 1e-9)
		})
	})
}

func BenchmarkPositionMonitorApplyStopTicker(benchmark *testing.B) {
	monitor := NewPositionMonitor()
	stopLoss, stopErr := NewStopLoss(
		"BTC/USD",
		0.01,
		50_000,
		0,
		config.ExitConfig{TrailDefault: 0.015, StopFloor: 0.012},
	)

	if stopErr != nil {
		benchmark.Fatal(stopErr)
	}

	ticker := &market.TickerUpdate{
		Symbol: "BTC/USD",
		Bid:    50_630,
		Ask:    50_640,
	}

	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for benchmark.Loop() {
		monitor.ApplyStopTicker(stopLoss, ticker)
	}
}
