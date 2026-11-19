package broker

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
)

func TestNewPosition(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When NewPosition is called", func() {
			pos := NewPosition(t.Context(), testSymbol, "paper", "path", price, nil, nil)

			So(pos, ShouldNotBeNil)
			So(pos.ID, ShouldNotBeEmpty)
			So(pos.Symbol, ShouldEqual, testSymbol)
			So(pos.TierKey, ShouldEqual, "paper")
			So(pos.EntryPath, ShouldEqual, "path")
			So(pos.Fills, ShouldNotBeNil)
			So(len(pos.Fills), ShouldEqual, 0)
			So(pos.Qty, ShouldBeNil)
			So(pos.Entry, ShouldBeNil)
			So(pos.ClosedAt, ShouldBeNil)
			So(pos.Status(), ShouldEqual, runtime.INIT)
			So(pos.Status() == runtime.READY, ShouldBeFalse)
		})
	}))
}

func TestQty(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", price, nil, nil)

		Convey("When a buy fill is applied", func() {
			pos.Apply(market.ExecutionData{
				ExecID: "b1", Side: "buy",
				LastQty: dec("1"), LastPrice: dec("100"),
				Timestamp: time.Unix(1, 0).UTC(),
			})

			Convey("Qty is the buy size", func() {
				So(pos.Qty.String(), ShouldEqual, "1")
			})
		})

		Convey("When a partial sell follows the buy", func() {
			pos.Apply(market.ExecutionData{
				ExecID: "b1", Side: "buy",
				LastQty: dec("1"), LastPrice: dec("100"),
				Timestamp: time.Unix(1, 0).UTC(),
			})
			pos.Apply(market.ExecutionData{
				ExecID: "s1", Side: "sell",
				LastQty: dec("0.4"), LastPrice: dec("110"),
				Timestamp: time.Unix(2, 0).UTC(),
			})

			Convey("Qty is buys minus sells", func() {
				So(pos.Qty.String(), ShouldEqual, "0.6")
				So(pos.ClosedAt, ShouldBeNil)
			})
		})

		Convey("When sells flatten the buys", func() {
			pos.Apply(market.ExecutionData{
				ExecID: "b1", Side: "buy",
				LastQty: dec("1"), LastPrice: dec("100"),
				Timestamp: time.Unix(1, 0).UTC(),
			})
			pos.Apply(market.ExecutionData{
				ExecID: "s1", Side: "sell",
				LastQty: dec("1"), LastPrice: dec("110"),
				Timestamp: time.Unix(2, 0).UTC(),
			})

			Convey("Qty is zero and ClosedAt is stamped", func() {
				So(pos.Qty.String(), ShouldEqual, "0")
				So(pos.ClosedAt, ShouldNotBeNil)
				So(pos.ClosedAt.Equal(time.Unix(2, 0).UTC()), ShouldBeTrue)
				So(pos.Status(), ShouldEqual, runtime.DONE)
				So(pos.Status() == runtime.READY, ShouldBeFalse)
			})
		})

	}))
}

func TestEntry(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", price, nil, nil)

		Convey("When a buy fill is applied", func() {
			pos.Apply(market.ExecutionData{
				ExecID: "b1", Side: "buy",
				LastQty: dec("1"), LastPrice: dec("100"),
				Timestamp: time.Unix(1, 0).UTC(),
			})

			Convey("Entry is the buy price", func() {
				So(pos.Entry.String(), ShouldEqual, "100")
			})
		})

		Convey("When a sell fill follows", func() {
			pos.Apply(market.ExecutionData{
				ExecID: "b1", Side: "buy",
				LastQty: dec("1"), LastPrice: dec("100"),
				Timestamp: time.Unix(1, 0).UTC(),
			})
			pos.Apply(market.ExecutionData{
				ExecID: "s1", Side: "sell",
				LastQty: dec("1"), LastPrice: dec("110"),
				Timestamp: time.Unix(2, 0).UTC(),
			})

			Convey("Entry stays the buy price", func() {
				So(pos.Entry.String(), ShouldEqual, "100")
			})
		})
	}))
}

func TestUpdate(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", price, nil, nil)
		pos.EntryOrder = &spot.OrderRequest{ClOrdId: "enter-1", Type: "buy", OrderType: "market"}
		pos.EntryVenueOrderID = "OID-1"
		pos.onSettled = func(*Position) {}
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})

		Convey("When Update is called with a mark", func() {
			So(pos.Status(), ShouldEqual, runtime.WAITING)
			setBook(price, "120", "121", "30")
			_, err := pos.Update(dec("120"))

			Convey("Mark is the bid hint; PnL is exact full-qty book walk", func() {
				// feeIn=0.40, feeOut=0.48 -> pnl=120-0.48-(100+0.40)=19.12
				// pct=19.12 → ladder banks Arm/Arm+1/Arm+2 in one tick → BUSY settle
				So(err, ShouldBeNil)
				So(pos.Mark.String(), ShouldEqual, "120")
				So(pos.PnL.String(), ShouldEqual, "19.12")
				So(pos.PnLPct.String(), ShouldEqual, "19.12")
				So(pos.Stoploss, ShouldNotBeNil)
				So(pos.Stoploss.WantsExit(), ShouldBeTrue)
				So(pos.Status(), ShouldEqual, runtime.BUSY)
			})
		})

		Convey("When the visible book cannot absorb Qty", func() {
			setBook(price, "120", "121", "0.1")
			_, err := pos.Update(dec("120"))

			Convey("Mark stamps; stay WAITING until the book can mark", func() {
				So(err, ShouldBeNil)
				So(pos.Mark.String(), ShouldEqual, "120")
				So(pos.PnL, ShouldBeNil)
				So(pos.Stoploss, ShouldBeNil)
				So(pos.Status(), ShouldEqual, runtime.WAITING)
			})
		})

		Convey("When buys fill across multiple prints", func() {
			split := NewPosition(t.Context(), testSymbol, "paper", "path", price, nil, nil)
			split.Apply(market.ExecutionData{
				ExecID: "b1", Side: "buy",
				LastQty: dec("99"), LastPrice: dec("1.00"),
				Timestamp: time.Unix(1, 0).UTC(),
			})
			split.Apply(market.ExecutionData{
				ExecID: "b2", Side: "buy",
				LastQty: dec("1"), LastPrice: dec("0.85"),
				Timestamp: time.Unix(2, 0).UTC(),
			})

			Convey("Entry is quantity-weighted VWAP", func() {
				So(split.Entry.String(), ShouldEqual, "0.9985")
			})
		})

		Convey("When Update is called with a nil mark", func() {
			_, err := pos.Update(nil)

			Convey("It returns a conflict for a nil price", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "[position.Update] price is nil")
			})
		})

		Convey("When Update runs before any fill", func() {
			open := NewPosition(t.Context(), testSymbol, "paper", "path", price, nil, nil)
			So(open.Enter("R00/M1", dec("1"), price.balance), ShouldBeNil)
			So(open.Status(), ShouldEqual, runtime.WAITING)
			_, err := open.Update(dec("120"))

			Convey("WAITING is a silent no-op — marks wait for Apply → READY", func() {
				So(err, ShouldBeNil)
				So(open.Status() == runtime.READY, ShouldBeFalse)
				So(open.Stoploss, ShouldBeNil)
				So(open.Mark, ShouldBeNil)
			})
		})

		Convey("When the position is already closed", func() {
			pos.Apply(market.ExecutionData{
				ExecID: "s1", Side: "sell",
				LastQty: dec("1"), LastPrice: dec("110"),
				Timestamp: time.Unix(2, 0).UTC(),
			})
			So(pos.Status(), ShouldEqual, runtime.DONE)
			So(pos.Status() == runtime.READY, ShouldBeFalse)
			_, err := pos.Update(dec("120"))

			Convey("DONE is a silent no-op so mark ticks skip Save", func() {
				So(err, ShouldBeNil)
				So(pos.Mark, ShouldBeNil)
			})
		})

		Convey("When the position is Closing awaiting exit fill", func() {
			So(pos.exit(false), ShouldBeNil)
			So(pos.Status(), ShouldEqual, runtime.BUSY)
			So(pos.Status() == runtime.READY, ShouldBeFalse)
			_, err := pos.Update(dec("120"))

			Convey("BUSY is a silent no-op without mutating mark", func() {
				So(err, ShouldBeNil)
				So(pos.Mark, ShouldBeNil)
			})
		})
	}))
}

func TestEnter(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		pos := NewPosition(t.Context(), testSymbol, "paper", "", price, nil, nil)

		Convey("When Enter is called with a path and qty", func() {
			err := pos.Enter("R00/M1", dec("1"), price.balance)

			Convey("Orders and path are armed; fills still own Qty", func() {
				So(err, ShouldBeNil)
				So(pos.EntryPath, ShouldEqual, "R00/M1")
				So(pos.EntryOrder, ShouldNotBeNil)
				So(pos.EntryOrder.Type, ShouldEqual, "buy")
				So(pos.EntryOrder.Volume, ShouldEqual, "1")
				// ExitOrder is created only in exit() — pre-allocating it
				// deadlocked Tier.Exit on every live stoploss park.
				So(pos.ExitOrder, ShouldBeNil)
				So(pos.OpenedAt.IsZero(), ShouldBeFalse)
				So(pos.Qty, ShouldBeNil)
				So(pos.Entry, ShouldBeNil)
				So(pos.Status(), ShouldEqual, runtime.WAITING)
				So(pos.Status() == runtime.READY, ShouldBeFalse)
			})
		})

		Convey("When Enter is called without a path", func() {
			err := pos.Enter("", dec("1"), price.balance)

			Convey("It returns not-found for the missing path", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "[position.Enter] missing path for "+testSymbol)
			})
		})
	}))
}

func TestExit(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", price, nil, nil)
		pos.ExitPath = "R00/M1"
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})

		Convey("When exit is called on a filled position", func() {
			So(pos.Status(), ShouldEqual, runtime.WAITING)
			err := pos.exit(false)

			Convey("Closing is armed and stage moves WAITING → BUSY", func() {
				So(err, ShouldBeNil)
				So(pos.Status() == runtime.BUSY, ShouldBeTrue)
				So(pos.ExitTrigger, ShouldEqual, "policy exit match R00/M1")
				So(pos.ClosedAt, ShouldBeNil)
				So(pos.Status(), ShouldEqual, runtime.BUSY)
				So(pos.Status() == runtime.READY, ShouldBeFalse)
				So(pos.ExitOrder.Volume, ShouldEqual, "1.00000000")
			})
		})

		Convey("When exit wires the full fill-derived qty", func() {
			pos.Apply(market.ExecutionData{
				ExecID: "b2", Side: "buy",
				LastQty: dec("0.25"), LastPrice: dec("100"),
				Timestamp: time.Unix(2, 0).UTC(),
			})
			So(pos.exit(true), ShouldBeNil)
			So(pos.ExitOrder.Volume, ShouldEqual, "1.25000000")
		})

		Convey("When a tranche wants a quarter of BasisQty", func() {
			pos.ExitPath = ""
			pos.Stoploss = NewStoploss(1.0)
			pos.Stoploss.BasisQty = 1
			pos.Stoploss.PendingExitPct = 0.25
			pos.Stoploss.TriggerReason = "tranche_1_arm"
			So(pos.exit(false), ShouldBeNil)
			So(pos.ExitOrder.Volume, ShouldEqual, "0.25000000")
			So(pos.ExitTrigger, ShouldEqual, "tranche_1_arm")
		})

		Convey("When a tranche slice is under QtyMin", func() {
			pos.ExitPath = ""
			pair := price.Instrument.Pair(testSymbol)
			pair.QtyMin = dec("0.30")
			price.Instrument.cache.Store(testSymbol, pair)
			pos.Stoploss = NewStoploss(1.0)
			pos.Stoploss.BasisQty = 1
			pos.Stoploss.PendingExitPct = 0.25
			pos.Stoploss.TriggerReason = "tranche_1_arm"
			So(pos.exit(false), ShouldBeNil)
			So(pos.ExitOrder.Volume, ShouldEqual, "1.00000000")
		})

		Convey("When a tranche would leave a dust remainder under QtyMin", func() {
			// Open 0.40; equal 0.25 would leave 0.15 dust. Shrink mid to 0.20 so
			// the final stage keeps a venue-legal 0.20 stub.
			pos.ExitPath = ""
			pos.Apply(market.ExecutionData{
				ExecID: "s-dust", Side: "sell",
				LastQty: dec("0.60"), LastPrice: dec("110"),
				Timestamp: time.Unix(2, 0).UTC(),
			})
			pair := price.Instrument.Pair(testSymbol)
			pair.QtyMin = dec("0.20")
			price.Instrument.cache.Store(testSymbol, pair)
			pos.Stoploss = NewStoploss(1.0)
			pos.Stoploss.BasisQty = 1
			pos.Stoploss.PendingExitPct = 0.25
			pos.Stoploss.TriggerReason = "tranche_3_target"
			So(pos.Qty.String(), ShouldEqual, "0.4")
			So(pos.exit(false), ShouldBeNil)
			So(pos.ExitOrder.Volume, ShouldEqual, "0.20000000")
		})

		Convey("When a tranche would leave a dust remainder under CostMin", func() {
			// Mark bid 100: equal 0.25 leaves 0.15 ($15) under CostMin $20.
			// Shrink mid to 0.20 so final keeps $20 notional.
			pos.ExitPath = ""
			pos.Apply(market.ExecutionData{
				ExecID: "s-cost", Side: "sell",
				LastQty: dec("0.60"), LastPrice: dec("110"),
				Timestamp: time.Unix(2, 0).UTC(),
			})
			pair := price.Instrument.Pair(testSymbol)
			pair.CostMin = dec("20")
			price.Instrument.cache.Store(testSymbol, pair)
			pos.Stoploss = NewStoploss(1.0)
			pos.Stoploss.BasisQty = 1
			pos.Stoploss.PendingExitPct = 0.25
			pos.Stoploss.TriggerReason = "tranche_3_target"
			So(pos.exit(false), ShouldBeNil)
			So(pos.ExitOrder.Volume, ShouldEqual, "0.20000000")
		})

		Convey("When exit is called again", func() {
			So(pos.exit(false), ShouldBeNil)
			trigger := pos.ExitTrigger
			So(pos.exit(false), ShouldBeNil)

			Convey("State is unchanged", func() {
				So(pos.Status() == runtime.BUSY, ShouldBeTrue)
				So(pos.ExitTrigger, ShouldEqual, trigger)
				So(pos.Status(), ShouldEqual, runtime.BUSY)
			})
		})
	}))
}

func TestApply(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", price, nil, nil)
		pos.EntryOrder = &spot.OrderRequest{ClOrdId: "enter-1", Type: "buy", OrderType: "market"}
		pos.EntryVenueOrderID = "OID-1"
		pos.onSettled = func(*Position) {}

		Convey("When a buy fill is applied", func() {
			pos.Apply(market.ExecutionData{
				ExecID: "b1", Side: "buy",
				LastQty: dec("1"), LastPrice: dec("100"),
				Timestamp: time.Unix(1, 0).UTC(),
			})

			Convey("Qty and Entry stamp; row stays WAITING for the book", func() {
				So(len(pos.Fills), ShouldEqual, 1)
				So(pos.Qty.String(), ShouldEqual, "1")
				So(pos.Entry.String(), ShouldEqual, "100")
				So(pos.PnL, ShouldBeNil)
				So(pos.ClosedAt, ShouldBeNil)
				So(pos.Status(), ShouldEqual, runtime.WAITING)
			})
		})

		Convey("When non-trade lifecycle events arrive", func() {
			pos.Apply(market.ExecutionData{
				ExecID: "c1", ExecType: "canceled", Side: "sell",
				LastQty: dec("0"), LastPrice: dec("100"),
				Timestamp: time.Unix(1, 0).UTC(),
			})

			Convey("Fills stay empty", func() {
				So(len(pos.Fills), ShouldEqual, 0)
			})
		})

		Convey("When the same ExecID is applied twice", func() {
			pos.Apply(market.ExecutionData{
				ExecID: "b1", Side: "buy",
				LastQty: dec("1"), LastPrice: dec("100"),
				Timestamp: time.Unix(1, 0).UTC(),
			})
			pos.Apply(market.ExecutionData{
				ExecID: "b1", Side: "buy",
				LastQty: dec("1"), LastPrice: dec("100"),
				Timestamp: time.Unix(1, 0).UTC(),
			})

			Convey("The duplicate is ignored", func() {
				So(len(pos.Fills), ShouldEqual, 1)
				So(pos.Qty.String(), ShouldEqual, "1")
			})
		})

		Convey("When a flattening sell is applied", func() {
			setBook(price, "100", "101", "30")
			pos.Apply(market.ExecutionData{
				ExecID: "b1", Side: "buy",
				LastQty: dec("1"), LastPrice: dec("100"),
				Timestamp: time.Unix(1, 0).UTC(),
			})
			_, _ = pos.Update(dec("100"))
			beforePnL := pos.PnL.String()
			beforePct := pos.PnLPct.String()

			pos.Apply(market.ExecutionData{
				ExecID: "s1", Side: "sell",
				LastQty: dec("1"), LastPrice: dec("110"),
				Timestamp: time.Unix(2, 0).UTC(),
			})

			Convey("ClosedAt stamps and fill-realized PnL replaces the last mark", func() {
				So(pos.Qty.String(), ShouldEqual, "0")
				So(pos.ClosedAt, ShouldNotBeNil)
				So(beforePnL, ShouldNotBeEmpty)
				So(beforePct, ShouldNotBeEmpty)
				// Fee-net: buy/sell taker 0.40% on 100/110 → 9.16, not gross 10.
				So(pos.PnL.String(), ShouldEqual, "9.16")
				So(pos.PnLPct.String(), ShouldEqual, "9.16")
				So(pos.Status(), ShouldEqual, runtime.DONE)
				So(pos.Status() == runtime.READY, ShouldBeFalse)
			})
		})
	}))
}

func TestValidate(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", price, nil, nil)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		pos.EntryOrder = &spot.OrderRequest{ClOrdId: "enter-1"}
		pos.EntryVenueOrderID = "OID-1"
		pos.onSettled = func(*Position) {}

		Convey("When required fields are present", func() {
			So(pos.Validate(), ShouldBeNil)
		})

		Convey("When EntryOrder is missing", func() {
			pos.EntryOrder = nil
			So(pos.Validate(), ShouldNotBeNil)
		})
	}))
}

func benchPosition(b *testing.B) (*Position, *Price) {
	b.Helper()
	var pos *Position
	withPrice(func(price *Price) {
		pos = NewPosition(b.Context(), testSymbol, "paper", "path", price, nil, nil)
	})()
	return pos, nil
}

func BenchmarkNewPosition(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		withPrice(func(price *Price) {
			_ = NewPosition(b.Context(), testSymbol, "paper", "path", price, nil, nil)
		})()
	}
}

func BenchmarkQty(b *testing.B) {
	pos, _ := benchPosition(b)
	pos.Apply(market.ExecutionData{
		ExecID: "b1", Side: "buy",
		LastQty: dec("1"), LastPrice: dec("100"),
		Timestamp: time.Unix(1, 0).UTC(),
	})

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = pos.Qty.String()
	}
}

func BenchmarkEntry(b *testing.B) {
	pos, _ := benchPosition(b)
	pos.Apply(market.ExecutionData{
		ExecID: "b1", Side: "buy",
		LastQty: dec("1"), LastPrice: dec("100"),
		Timestamp: time.Unix(1, 0).UTC(),
	})

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = pos.Entry.String()
	}
}

func BenchmarkUpdate(b *testing.B) {
	pos, _ := benchPosition(b)
	pos.Apply(market.ExecutionData{
		ExecID: "b1", Side: "buy",
		LastQty: dec("1"), LastPrice: dec("100"),
		Timestamp: time.Unix(1, 0).UTC(),
	})
	mark := dec("120")

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = pos.Update(mark)
	}
}

func BenchmarkEnter(b *testing.B) {
	var balance *market.Balance
	withPrice(func(price *Price) { balance = price.balance })()

	b.ReportAllocs()

	for b.Loop() {
		withPrice(func(price *Price) {
			pos := NewPosition(b.Context(), testSymbol, "paper", "", price, nil, nil)
			_ = pos.Enter("R00/M1", dec("1"), balance)
		})()
	}
}

func BenchmarkExit(b *testing.B) {
	pos, _ := benchPosition(b)
	pos.ExitPath = "R00/M1"

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = pos.exit(false)
	}
}

func BenchmarkApply(b *testing.B) {
	fill := market.ExecutionData{
		ExecID: "b1", Side: "buy",
		LastQty: dec("1"), LastPrice: dec("100"),
		Timestamp: time.Unix(1, 0).UTC(),
	}

	b.ReportAllocs()

	for b.Loop() {
		withPrice(func(price *Price) {
			pos := NewPosition(b.Context(), testSymbol, "paper", "path", price, nil, nil)
			pos.Apply(fill)
		})()
	}
}
