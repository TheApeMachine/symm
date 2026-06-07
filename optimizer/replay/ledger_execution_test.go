package replay

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestExecutionFillMeasurement(t *testing.T) {
	Convey("Given a pending signal row and a later market row", t, func() {
		signalRow := types.Measurement{
			Symbol:    "BTC/EUR",
			Last:      100,
			SpreadBPS: 10,
			At:        time.Unix(1, 0),
		}
		currentRow := types.Measurement{
			Symbol:    "BTC/EUR",
			Last:      101,
			SpreadBPS: 12,
			At:        time.Unix(2, 0),
		}

		fillRow := executionFillMeasurement(signalRow, currentRow)

		Convey("It should carry the current price and spread into the fill", func() {
			So(fillRow.Last, ShouldEqual, 101)
			So(fillRow.SpreadBPS, ShouldEqual, 12)
			So(fillRow.At, ShouldEqual, currentRow.At)
		})

		Convey("It should fall back to the signal row without a usable current price", func() {
			fillRow = executionFillMeasurement(signalRow, types.Measurement{Symbol: "ETH/EUR", Last: 50})

			So(fillRow.Last, ShouldEqual, 100)
		})
	})
}

func TestExecutionReady(t *testing.T) {
	Convey("Given a replay ledger with execution latency", t, func() {
		ledger := newReplayLedger(triggerTestCosts())
		ledger.configureExecutionStress(100*time.Millisecond, 50*time.Millisecond)

		Convey("It should release actions once executeAt is reached", func() {
			item := pendingReplayAction{
				executeAt: time.Unix(2, 0),
			}

			So(ledger.executionReady(item, time.Unix(1, 0)), ShouldBeFalse)
			So(ledger.executionReady(item, time.Unix(2, 0)), ShouldBeTrue)
		})

		Convey("It should fall back to tick-index scheduling without timestamps", func() {
			ledger.tickIndex = 5
			item := pendingReplayAction{executeTick: 7}

			So(ledger.executionReady(item, time.Time{}), ShouldBeFalse)

			ledger.tickIndex = 7

			So(ledger.executionReady(item, time.Time{}), ShouldBeTrue)
		})
	})
}

func TestFlushPending(t *testing.T) {
	Convey("Given a queued market entry under execution latency", t, func() {
		testconfig.Load(t)
		ledger := newReplayLedger(triggerTestCosts())
		ledger.configureExecutionStress(100*time.Millisecond, 50*time.Millisecond)

		base := time.Unix(1_700_000_000, 0)
		signalRow := QuotedMeasurement(types.Measurement{
			Symbol: "BTC/EUR",
			Last:   100,
			At:     base,
		})

		ledger.queueAction(
			reasoning.Act{Type: reasoning.ActionMarket},
			signalRow,
			nil,
		)
		ledger.flushEntryBatch(base.Add(replayEntryBatchWindow()))

		Convey("It should keep the action pending before the latency window elapses", func() {
			ledger.flushPending(base.Add(50*time.Millisecond), signalRow)

			So(ledger.holding("BTC/EUR"), ShouldBeFalse)
			So(len(ledger.pending), ShouldEqual, 1)
		})

		Convey("It should apply the action once execution is ready", func() {
			fillRow := types.Measurement{
				Symbol: "BTC/EUR",
				Last:   100,
				At:     base.Add(100 * time.Millisecond),
			}

			ledger.flushPending(fillRow.At, fillRow)

			So(ledger.holding("BTC/EUR"), ShouldBeTrue)
			So(len(ledger.pending), ShouldEqual, 0)
		})
	})
}

func TestExecutionSlippagePct(t *testing.T) {
	Convey("Given turbulent snapshot readings", t, func() {
		costs := triggerTestCosts()
		snapshots := []types.Measurement{
			{Category: types.CategoryTurbulent, SNR: 2},
		}

		slippage := executionSlippagePct(costs, 20, snapshots)

		Convey("It should expand slippage above the half-spread baseline", func() {
			So(slippage, ShouldBeGreaterThan, halfSpreadSlippagePct(costs, 20))
		})
	})
}

func TestDeriveExecutionLatency(t *testing.T) {
	Convey("Given measurement timestamps", t, func() {
		base := time.Unix(1_700_000_000, 0)
		rows := []types.Measurement{
			{At: base},
			{At: base.Add(100 * time.Millisecond)},
			{At: base.Add(200 * time.Millisecond)},
		}

		Convey("It should derive a bounded latency from the median interval", func() {
			latency := deriveExecutionLatency(rows)

			So(latency, ShouldBeGreaterThanOrEqualTo, 50*time.Millisecond)
			So(latency, ShouldBeLessThanOrEqualTo, 200*time.Millisecond)
		})

		Convey("It should return zero without enough timestamps", func() {
			So(deriveExecutionLatency(nil), ShouldEqual, 0)
		})
	})
}

func TestExecutionLatencyTicks(t *testing.T) {
	Convey("Given latency and median interval", t, func() {
		Convey("It should convert duration into tick counts", func() {
			So(
				executionLatencyTicks(150*time.Millisecond, 50*time.Millisecond),
				ShouldEqual,
				3,
			)
		})

		Convey("It should return zero without latency", func() {
			So(executionLatencyTicks(0, 50*time.Millisecond), ShouldEqual, 0)
		})
	})
}

func TestDeriveExecutionLatencyFromTape(t *testing.T) {
	Convey("Given a precompiled tape", t, func() {
		base := time.Unix(1_700_000_000, 0)
		tape := ReplayTape{
			Ticks: []PrecompiledTick{
				{Row: types.Measurement{At: base}},
				{Row: types.Measurement{At: base.Add(100 * time.Millisecond)}},
			},
		}

		Convey("It should derive latency from tape rows", func() {
			latency := deriveExecutionLatencyFromTape(tape)

			So(latency, ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkExecutionSlippagePct(b *testing.B) {
	costs := triggerTestCosts()
	snapshots := []types.Measurement{
		{Category: types.CategoryTurbulent, SNR: 2},
	}

	for b.Loop() {
		_ = executionSlippagePct(costs, 20, snapshots)
	}
}

func BenchmarkDeriveExecutionLatency(b *testing.B) {
	base := time.Unix(1_700_000_000, 0)
	rows := []types.Measurement{
		{At: base},
		{At: base.Add(100 * time.Millisecond)},
		{At: base.Add(200 * time.Millisecond)},
	}

	for b.Loop() {
		_ = deriveExecutionLatency(rows)
	}
}
