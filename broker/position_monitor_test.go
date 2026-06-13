package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/user"
)

const positionMonitorTrailSpreadBps = 75

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
			positionMonitorTrailSpreadBps,
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
			So(frame.Positions[0].StopPrice, ShouldAlmostEqual, stopLoss.StopPrice, 1e-9)
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
			positionMonitorTrailSpreadBps,
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

func TestPositionMonitorApplyStopTickerPreservesBalanceEntry(t *testing.T) {
	Convey("Given a balance-owned position with fee-inclusive entry", t, func() {
		monitor := NewPositionMonitor()
		monitor.ApplyBalance(user.Balances{
			Currency:    "USD",
			Balance:     187.76,
			Inventory:   map[string]float64{"AVAX": 0.38140717},
			AvgEntry:    map[string]float64{"AVAX": 6.668},
			Expected:    map[string]float64{"AVAX": 2.5323},
			Unrealized:  map[string]float64{"AVAX": -0.0109},
			ExitFeeRate: map[string]float64{"AVAX": 0.0026},
		})

		stopLoss, stopErr := NewStopLoss(
			"AVAX/USD",
			0.38140717,
			6.658,
			positionMonitorTrailSpreadBps,
		)
		ticker := &market.TickerUpdate{
			Symbol:    "AVAX/USD",
			Bid:       6.658,
			Timestamp: time.Unix(1710000002, 0).UTC(),
		}

		changed := monitor.ApplyStopTicker(stopLoss, ticker)
		frame := monitor.Snapshot()

		Convey("It should keep the balance entry and conserve equity", func() {
			So(stopErr, ShouldBeNil)
			So(changed, ShouldBeTrue)
			So(frame.Positions[0].AverageEntry, ShouldAlmostEqual, 6.668, 1e-9)
			So(frame.LiquidationBalance, ShouldAlmostEqual,
				frame.Cash+frame.ExitValue, 1e-9)
			So(frame.LiquidationBalance, ShouldAlmostEqual,
				frame.Cash+frame.Positions[0].Quantity*frame.Positions[0].AverageEntry+
					frame.ExitBalance, 1e-6)
		})
	})
}

func TestPositionMonitorConservesEquity(t *testing.T) {
	Convey("Given multiple priced positions", t, func() {
		monitor := NewPositionMonitor()
		monitor.ApplyBalance(user.Balances{
			Currency: "USD",
			Balance:  187.76,
			Inventory: map[string]float64{
				"AVAX":  0.38140717,
				"LINK":  0.15228332,
				"STRK":  130.43285015,
				"TREMP": 914.44047378,
			},
			AvgEntry: map[string]float64{
				"AVAX":  6.668,
				"LINK":  7.9494,
				"STRK":  0.03468820,
				"TREMP": 0.00420676,
			},
			Expected: map[string]float64{
				"AVAX":  2.5323,
				"LINK":  1.2053,
				"STRK":  4.4843,
				"TREMP": 3.5428,
			},
			Unrealized: map[string]float64{
				"AVAX":  -0.0109,
				"LINK":  -0.0053,
				"STRK":  -0.0412,
				"TREMP": -0.3040,
			},
			ExitFeeRate: map[string]float64{
				"AVAX":  0.0026,
				"LINK":  0.0026,
				"STRK":  0.0026,
				"TREMP": 0.0026,
			},
		})

		frame := monitor.Snapshot()
		costBasis := 0.0

		for _, position := range frame.Positions {
			costBasis += position.Quantity * position.AverageEntry
		}

		Convey("It should keep cash plus exit value equal to cash plus cost plus open P&L", func() {
			So(frame.ExitBalance, ShouldAlmostEqual, -0.3614, 1e-3)
			So(frame.LiquidationBalance, ShouldAlmostEqual,
				frame.Cash+frame.ExitValue, 1e-9)
			So(frame.LiquidationBalance, ShouldAlmostEqual,
				frame.Cash+costBasis+frame.ExitBalance, 1e-2)
			So(frame.LiquidationBalance, ShouldBeLessThan, 200)
		})
	})
}

func BenchmarkPositionMonitorApplyStopTicker(benchmark *testing.B) {
	monitor := NewPositionMonitor()
	stopLoss, stopErr := NewStopLoss(
		"BTC/USD",
		0.01,
		50_000,
		positionMonitorTrailSpreadBps,
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
