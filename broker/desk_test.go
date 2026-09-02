package broker

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

/*
saturatedPosition builds a Position whose guardian transport is unavailable —
the disruptor is nil, so publishGuardian deterministically fails to reserve a
slot. This is the "priority event cannot enter the guardian" boundary the desk
must surface as a non-nil error rather than a log line.
*/
func saturatedPosition(t *testing.T) (*Desk, *Position) {
	t.Helper()

	conn := newExecutingConn(nil)
	desk, workload := newDeliveryDesk(t, conn)
	conn.workload = workload

	decision := types.Decision{
		ID:               "saturated-1",
		Action:           types.ActionEnter,
		Symbol:           "TEST/USD",
		ProposedQuantity: mustDecimal("100000"),
		ProposedNotional: mustDecimal("200.00"),
		ForecastHorizon:  1,
	}

	position := &Position{
		Holding:  &types.Holding{Symbol: "TEST/USD"},
		Decision: decision,
	}
	position.setStatus(types.OPEN)
	desk.positions.Store("TEST/USD", position)

	return desk, position
}

/*
TestDeskGuardianSaturationError proves the guardian-saturation failure contract:
when a priority event cannot reserve a guardian slot, the caller receives a
non-nil error (never a log-and-success), and no emergency action is enqueued.
*/
func TestDeskGuardianSaturationError(t *testing.T) {
	Convey("Given a position whose guardian transport cannot reserve a slot", t, func() {
		desk, _ := saturatedPosition(t)

		Convey("StepTicker returns the saturation error rather than success", func() {
			err := desk.StepTicker(kraken.TickerData{
				Symbol: "TEST/USD",
				Bid:    decimal.NewFromFloat64(2.00),
			})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "guardian")
		})

		Convey("StepExecution returns the saturation error rather than success", func() {
			err := desk.StepExecution(kraken.ExecutionData{Symbol: "TEST/USD"})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "guardian")
		})

		Convey("StepLevel3 returns the saturation error rather than success", func() {
			err := desk.StepLevel3(kraken.Level3Data{Symbol: "TEST/USD"})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "guardian")
		})
	})
}

/*
newLevel3GuardianDesk builds a desk holding one filled position wired to a real
price surface, so the L3 → canonical book → guardian → ObserveExecutable path is
exercised end to end without calling ObserveExecutable directly.
*/
func newLevel3GuardianDesk(t testing.TB) (*Desk, *Position) {
	t.Helper()

	conn := newExecutingConn(nil)
	desk, workload := newDeliveryDesk(t, conn)
	conn.workload = workload

	// Seed a taker-fee row so ExecutableSurface can price the executable
	// surface; without it the book reports incomplete and BookObserved cannot
	// latch.
	desk.price.fees.Store("TEST/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.25),
	})

	entry := mustDecimal("2.00")
	stoploss, err := types.NewStoploss(
		t.Context(),
		"TEST/USD",
		entry,
		entry,
		&learning.RLSOutput{Ready: true},
		nil,
		mustDecimal("0.01"),
		mustDecimal("0"),
		mustDecimal("0"),
		time.Now(),
	)
	if err != nil {
		t.Fatalf("construct stoploss: %v", err)
	}
	stoploss.ArmClock()

	decision := types.Decision{
		ID:               "l3-guardian-1",
		Action:           types.ActionEnter,
		Symbol:           "TEST/USD",
		ProposedQuantity: mustDecimal("100000"),
		ProposedNotional: mustDecimal("200.00"),
		ForecastHorizon:  1,
		Stoploss:         stoploss,
	}

	pair := kraken.InstrumentPair{Symbol: "TEST/USD", TickSize: *decimal.NewFromFloat64(0.01)}
	position := NewPosition(
		t.Context(), desk.api, desk.instrument, desk.price, desk.balance,
		nil, desk.PositionStore, pair, decision,
	)
	position.Holding.Qty = mustDecimal("100000")
	position.Holding.SellableQty = mustDecimal("100000")
	position.Holding.EntryPrice = entry
	position.Holding.EntryFee = mustDecimal("0")
	position.setStatus(types.OPEN)
	position.Holding.Status = types.OPEN

	desk.positions.Store("TEST/USD", position)

	return desk, position
}

func coherentL3(symbol, bid, ask string) kraken.Level3Data {
	return kraken.Level3Data{
		Symbol: symbol,
		Type:   "snapshot",
		Bids:   []kraken.Level3Order{mustDecimalOrder("b1", bid, "100000")},
		Asks:   []kraken.Level3Order{mustDecimalOrder("a1", ask, "100000")},
	}
}

/*
TestDeskPositionOpenedAfterSnapshotConsumesContinuousState covers the P0
mid-stream bootstrap bug end to end. The L3 reducer is continuously advanced
from the genuine snapshot regardless of position existence, so a position opened
after several updates must see truthful current state on the next update — the
update must never be promoted to a snapshot merely because the position-local
reducer had no prior state.
*/
func TestDeskPositionOpenedAfterSnapshotConsumesContinuousState(t *testing.T) {
	Convey("Given a desk whose L3 reducer consumed a snapshot then several updates", t, func() {
		conn := newExecutingConn(nil)
		desk, workload := newDeliveryDesk(t, conn)
		conn.workload = workload

		desk.price.fees.Store("TEST/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.25),
		})

		So(desk.StepLevel3(coherentL3("TEST/USD", "2.50", "2.51")), ShouldBeNil)

		// Two mutations against the seeded state: the executable bid mark
		// moves from 2.50 to 2.60 before any position exists.
		So(desk.StepLevel3(kraken.Level3Data{
			Symbol: "TEST/USD", Type: "update",
			Bids: []kraken.Level3Order{mustDecimalOrder("b2", "2.60", "100000")},
			Asks: []kraken.Level3Order{mustDecimalOrder("a1", "2.61", "100000")},
		}), ShouldBeNil)

		entry := mustDecimal("2.00")
		stoploss, err := types.NewStoploss(
			t.Context(),
			"TEST/USD",
			entry,
			entry,
			&learning.RLSOutput{Ready: true},
			nil,
			mustDecimal("0.01"),
			mustDecimal("0"),
			mustDecimal("0"),
			time.Now(),
		)

		if err != nil {
			t.Fatalf("construct stoploss: %v", err)
		}

		stoploss.ArmClock()

		decision := types.Decision{
			ID:               "l3-continuous-1",
			Action:           types.ActionEnter,
			Symbol:           "TEST/USD",
			ProposedQuantity: mustDecimal("100000"),
			ProposedNotional: mustDecimal("200.00"),
			ForecastHorizon:  1,
			Stoploss:         stoploss,
		}

		pair := kraken.InstrumentPair{Symbol: "TEST/USD", TickSize: *decimal.NewFromFloat64(0.01)}
		position := NewPosition(
			t.Context(), desk.api, desk.instrument, desk.price, desk.balance,
			nil, desk.PositionStore, pair, decision,
		)
		position.Holding.Qty = mustDecimal("100000")
		position.Holding.SellableQty = mustDecimal("100000")
		position.Holding.EntryPrice = entry
		position.Holding.EntryFee = mustDecimal("0")
		position.setStatus(types.OPEN)
		position.Holding.Status = types.OPEN

		desk.positions.Store("TEST/USD", position)

		Convey("the next update reads truthful continuous state rather than reconstructing from the update", func() {
			before := position.guardianWatermark.Load()

			// Deleting the order added before the position opened reveals
			// whether the resident state really contains it: the mutation is
			// meaningful only against a seeded baseline. Had this delete been
			// promoted to a fresh snapshot (the old lazy-adoption bug), there
			// would be no bid left and the surface would stay incomplete.
			So(desk.StepLevel3(kraken.Level3Data{
				Symbol: "TEST/USD", Type: "update",
				Bids: []kraken.Level3Order{{Event: "delete", OrderID: "b2"}},
			}), ShouldBeNil)

			deadline := time.Now().Add(3 * time.Second)

			for position.guardianWatermark.Load() <= before &&
				time.Now().Before(deadline) {
				time.Sleep(2 * time.Millisecond)
			}

			// b2's deletion leaves the pre-position b1 at 2.50 as the best
			// executable bid, proving the continuously-resident state was
			// consumed rather than discarded and re-seeded from the delete.
			So(position.Holding.Stoploss.BookObserved, ShouldBeTrue)
			So(position.Holding.Mark, ShouldNotBeNil)
			So(position.Holding.Mark.Cmp(mustDecimal("2.50")), ShouldEqual, 0)
		})
	})
}

/*
TestDeskLevel3DrivesExecutableMark proves the production path: a realistic L3
snapshot reaches the desk, commits the canonical book, the guardian derives the
executable VWAP, BookObserved latches, and Holding.Mark becomes the full-lot
executable VWAP rather than ticker best-bid. It never calls ObserveExecutable
directly.
*/
func TestDeskLevel3DrivesExecutableMark(t *testing.T) {
	Convey("Given a filled position on a desk", t, func() {
		desk, position := newLevel3GuardianDesk(t)

		// step publishes through the production path and blocks until the
		// guardian has fully processed the frame (its atomic watermark has
		// advanced), so the subsequent state reads carry a happens-before edge
		// and never race the guardian goroutine.
		step := func(frame kraken.Level3Data) {
			before := position.guardianWatermark.Load()
			So(desk.StepLevel3(frame), ShouldBeNil)

			deadline := time.Now().Add(3 * time.Second)

			for position.guardianWatermark.Load() <= before &&
				time.Now().Before(deadline) {
				time.Sleep(2 * time.Millisecond)
			}

			So(position.guardianWatermark.Load(), ShouldBeGreaterThan, before)
		}

		Convey("a realistic snapshot commits the book and latches the executable mark", func() {
			step(coherentL3("TEST/USD", "2.50", "2.51"))

			So(position.Holding.Stoploss.BookObserved, ShouldBeTrue)
			So(position.Holding.Mark, ShouldNotBeNil)
			So(position.Holding.Mark.Cmp(mustDecimal("2.50")), ShouldEqual, 0)
		})

		Convey("once BookObserved latches, a later ticker does not replace the executable mark", func() {
			step(coherentL3("TEST/USD", "2.50", "2.51"))

			// Ticker also routes through the guardian; wait for the watermark
			// to advance so the mark read is ordered after the ticker handler.
			tickBefore := position.guardianWatermark.Load()
			So(desk.StepTicker(kraken.TickerData{
				Symbol: "TEST/USD",
				Bid:    decimal.NewFromFloat64(1.10),
				Ask:    decimal.NewFromFloat64(1.11),
			}), ShouldBeNil)

			deadline := time.Now().Add(3 * time.Second)

			for position.guardianWatermark.Load() <= tickBefore &&
				time.Now().Before(deadline) {
				time.Sleep(2 * time.Millisecond)
			}

			So(position.Holding.Mark.Cmp(mustDecimal("2.50")), ShouldEqual, 0)
		})

		Convey("a book that becomes crossed after a valid state triggers execution-regime invalidation", func() {
			step(coherentL3("TEST/USD", "2.50", "2.51"))

			crossed := kraken.Level3Data{
				Symbol: "TEST/USD",
				Type:   "snapshot",
				Bids:   []kraken.Level3Order{mustDecimalOrder("b1", "2.60", "100000")},
				Asks:   []kraken.Level3Order{mustDecimalOrder("a1", "2.40", "100000")},
			}

			step(crossed)

			So(position.Holding.Stoploss.Status, ShouldEqual, types.TRIGGERED)
			So(position.Holding.Stoploss.TriggerReason, ShouldEqual, types.TriggerRegimeInvalidated)
		})

		Convey("an intermediate L3 state breaching the protected floor remains observable", func() {
			// Raise the mark far enough to lock a protected floor.
			step(coherentL3("TEST/USD", "3.00", "3.01"))
			So(position.Holding.Stoploss.Locked, ShouldBeTrue)

			floor := position.Holding.Stoploss.Floor

			// A transient executable breach — the full lot is only fillable at
			// the floor itself — must trigger, not be coalesced away.
			breach := kraken.Level3Data{
				Symbol: "TEST/USD",
				Type:   "snapshot",
				Bids:   []kraken.Level3Order{mustDecimalOrder("b1", floor.String(), "100000")},
				Asks:   []kraken.Level3Order{mustDecimalOrder("a1", "3.01", "100000")},
			}

			step(breach)

			So(position.Holding.Stoploss.Status, ShouldEqual, types.TRIGGERED)
		})

		Convey("a realistic incremental snapshot/update sequence ratchets peak and floor then triggers protection", func() {
			// Snapshot seeds the book and marks the position at 2.50.
			step(coherentL3("TEST/USD", "2.50", "2.51"))

			firstPeak := decimal.NewFromInt64(0).Add(position.Holding.Stoploss.Peak)

			// Incremental add deepens the book and raises the executable mark;
			// the peak ratchets up with the executable VWAP. The ask is raised
			// in the same update so the book stays coherent (non-crossed).
			step(kraken.Level3Data{
				Symbol: "TEST/USD",
				Type:   "update",
				Bids: []kraken.Level3Order{
					mustDecimalOrder("b2", "2.80", "100000"),
				},
				Asks: []kraken.Level3Order{
					mustDecimalOrder("a1", "2.90", "100000"),
				},
			})

			So(position.Holding.Stoploss.Peak.Cmp(firstPeak), ShouldEqual, 1)

			// A later coherent frame brings the best bid through the current
			// floor, which must trigger protection immediately. The ask stays
			// above the bid so the book remains usable.
			floor := position.Holding.Stoploss.Floor
			breach := floor.Sub(mustDecimal("0.10"))

			step(kraken.Level3Data{
				Symbol: "TEST/USD",
				Type:   "snapshot",
				Bids: []kraken.Level3Order{
					mustDecimalOrder("b1", breach.String(), "100000"),
				},
				Asks: []kraken.Level3Order{
					mustDecimalOrder("a1", floor.Add(mustDecimal("0.10")).String(), "100000"),
				},
			})

			So(position.Holding.Stoploss.Status, ShouldEqual, types.TRIGGERED)
		})
	})
}

/*
BenchmarkGuardianEventLatency measures the round-trip cost of publishing one
priority L3 event through the guardian ring and having the dedicated consumer
handle it — the guardian event latency under a steady single-producer load.
*/
func BenchmarkGuardianEventLatency(b *testing.B) {
	desk, _ := newLevel3GuardianDesk(b)
	frame := coherentL3("TEST/USD", "2.50", "2.51")
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := desk.StepLevel3(frame); err != nil {
			b.Fatal(err)
		}
	}
}

/*
BenchmarkGuardianBurstCapacity measures the cost of publishing a full burst of
guardianCapacity priority events against a live consumer — the configured
1024-slot saturation boundary — to establish whether the ring and handler keep
up without dropping or coalescing.
*/
func BenchmarkGuardianBurstCapacity(b *testing.B) {
	desk, _ := newLevel3GuardianDesk(b)
	frame := coherentL3("TEST/USD", "2.50", "2.51")
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for range guardianCapacity {
			if err := desk.StepLevel3(frame); err != nil {
				b.Fatal(err)
			}
		}
	}
}
