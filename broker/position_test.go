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
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})

		Convey("When Update is called with a mark", func() {
			So(pos.Status(), ShouldEqual, runtime.READY)
			setBook(price, "120", "121", "30")
			err := pos.Update(dec("120"))

			Convey("Mark is the bid hint; PnL is exact full-qty book walk", func() {
				// feeIn=0.40, feeOut=0.48 -> pnl=120-0.48-(100+0.40)=19.12
				// pct=19.12/100*100
				So(err, ShouldBeNil)
				So(pos.Mark.String(), ShouldEqual, "120")
				So(pos.PnL.String(), ShouldEqual, "19.12")
				So(pos.PnLPct.String(), ShouldEqual, "19.12")
				So(pos.Stoploss, ShouldNotBeNil)
				So(pos.Status() == runtime.BUSY, ShouldBeFalse)
				So(pos.Status(), ShouldEqual, runtime.READY)
			})
		})

		Convey("When the visible book cannot absorb Qty", func() {
			setBook(price, "120", "121", "0.1")
			err := pos.Update(dec("120"))

			Convey("Mark stamps; exact PnL is nil so Stoploss stays nil", func() {
				So(err, ShouldBeNil)
				So(pos.Mark.String(), ShouldEqual, "120")
				So(pos.PnL, ShouldBeNil)
				So(pos.Stoploss, ShouldBeNil)
				So(pos.Status(), ShouldEqual, runtime.READY)
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

		Convey("When AbortExit resets a pending sell", func() {
			pos.ExitPath = "R00/M1"
			So(pos.exit(false), ShouldBeNil)
			So(pos.Status(), ShouldEqual, runtime.BUSY)
			oldClOrd := pos.ExitOrder.ClOrdId

			pos.AbortExit()

			Convey("Position returns to READY with cleared ExitOrder and fresh retry ClOrdId", func() {
				So(pos.Status(), ShouldEqual, runtime.READY)
				So(pos.ExitOrder, ShouldBeNil)
				So(pos.ExitPath, ShouldEqual, "")
				So(pos.exit(false), ShouldBeNil)
				So(pos.ExitOrder.ClOrdId, ShouldNotEqual, oldClOrd)
			})
		})

		Convey("When Update is called with a nil mark", func() {
			err := pos.Update(nil)

			Convey("It returns a conflict for a nil price", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "[position.Update] price is nil")
			})
		})

		Convey("When Update runs before any fill", func() {
			open := NewPosition(t.Context(), testSymbol, "paper", "path", price, nil, nil)
			So(open.Enter("R00/M1", dec("1"), price.balance), ShouldBeNil)
			So(open.Status(), ShouldEqual, runtime.WAITING)
			err := open.Update(dec("120"))

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
			err := pos.Update(dec("120"))

			Convey("DONE is a silent no-op so mark ticks skip Save", func() {
				So(err, ShouldBeNil)
				So(pos.Mark, ShouldBeNil)
			})
		})

		Convey("When the position is Closing awaiting exit fill", func() {
			So(pos.exit(false), ShouldBeNil)
			So(pos.Status(), ShouldEqual, runtime.BUSY)
			So(pos.Status() == runtime.READY, ShouldBeFalse)
			err := pos.Update(dec("120"))

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
				So(pos.ExitOrder, ShouldNotBeNil)
				So(pos.ExitOrder.Type, ShouldEqual, "sell")
				So(pos.ExitOrder.Volume, ShouldEqual, "1")
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

		Convey("When exit is called on a READY position", func() {
			So(pos.Status(), ShouldEqual, runtime.READY)
			err := pos.exit(false)

			Convey("Closing is armed and stage moves READY → BUSY", func() {
				So(err, ShouldBeNil)
				So(pos.Status() == runtime.BUSY, ShouldBeTrue)
				So(pos.ExitTrigger, ShouldEqual, "policy exit match R00/M1")
				So(pos.ClosedAt, ShouldBeNil)
				So(pos.Status(), ShouldEqual, runtime.BUSY)
				So(pos.Status() == runtime.READY, ShouldBeFalse)
			})
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

		Convey("When a buy fill is applied", func() {
			pos.Apply(market.ExecutionData{
				ExecID: "b1", Side: "buy",
				LastQty: dec("1"), LastPrice: dec("100"),
				Timestamp: time.Unix(1, 0).UTC(),
			})

			Convey("Qty, Entry, and mark PnL are set", func() {
				So(len(pos.Fills), ShouldEqual, 1)
				So(pos.Qty.String(), ShouldEqual, "1")
				So(pos.Entry.String(), ShouldEqual, "100")
				So(pos.PnL, ShouldNotBeNil)
				So(pos.PnLPct, ShouldNotBeNil)
				So(pos.ClosedAt, ShouldBeNil)
				So(pos.Status(), ShouldEqual, runtime.READY)
				So(pos.Status() == runtime.READY, ShouldBeTrue)
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
			pos.Apply(market.ExecutionData{
				ExecID: "b1", Side: "buy",
				LastQty: dec("1"), LastPrice: dec("100"),
				Timestamp: time.Unix(1, 0).UTC(),
			})
			beforePnL := pos.PnL.String()
			beforePct := pos.PnLPct.String()

			pos.Apply(market.ExecutionData{
				ExecID: "s1", Side: "sell",
				LastQty: dec("1"), LastPrice: dec("110"),
				Timestamp: time.Unix(2, 0).UTC(),
			})

			Convey("ClosedAt stamps and PnL is left alone", func() {
				So(pos.Qty.String(), ShouldEqual, "0")
				So(pos.ClosedAt, ShouldNotBeNil)
				So(pos.PnL.String(), ShouldEqual, beforePnL)
				So(pos.PnLPct.String(), ShouldEqual, beforePct)
				So(pos.Status(), ShouldEqual, runtime.DONE)
				So(pos.Status() == runtime.READY, ShouldBeFalse)
			})
		})
	}))
}

func TestHasExitOrder(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When ExitOrder is nil", func() {
			pos := NewPosition(t.Context(), testSymbol, "paper", "path", price, nil, nil)
			So(pos.HasExitOrder(), ShouldBeFalse)
			So((*Position)(nil).HasExitOrder(), ShouldBeFalse)
		})

		Convey("When ExitOrder is armed", func() {
			pos := NewPosition(t.Context(), testSymbol, "paper", "path", price, nil, nil)
			pos.ExitOrder = &spot.OrderRequest{ClOrdId: "exit-1"}
			So(pos.HasExitOrder(), ShouldBeTrue)
		})
	}))
}

func TestExitAttempt(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", price, nil, nil)
		pos.ExitPath = "R00/M1"
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})

		Convey("When exit has not been armed", func() {
			So(pos.ExitAttempt(), ShouldEqual, 0)
			So((*Position)(nil).ExitAttempt(), ShouldEqual, 0)
		})

		Convey("When exit is armed twice", func() {
			So(pos.exit(false), ShouldBeNil)
			So(pos.ExitAttempt(), ShouldEqual, 1)
			So(pos.exit(false), ShouldBeNil)
			So(pos.ExitAttempt(), ShouldEqual, 2)
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

func TestBind(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", price, nil, nil)

		Convey("When Bind attaches a buy order", func() {
			pos.Bind("OID-1", "buy", &spot.OrderRequest{ClOrdId: "enter-1"})

			Convey("Entry order and venue id stamp", func() {
				So(pos.EntryOrder.ClOrdId, ShouldEqual, "enter-1")
				So(pos.EntryVenueOrderID, ShouldEqual, "OID-1")
			})
		})

		Convey("When Bind attaches a sell order", func() {
			pos.Bind("OID-2", "sell", &spot.OrderRequest{ClOrdId: "exit-1"})

			Convey("Exit order and venue id stamp", func() {
				So(pos.ExitOrder.ClOrdId, ShouldEqual, "exit-1")
				So(pos.ExitVenueOrderID, ShouldEqual, "OID-2")
			})
		})
	}))
}

func TestOwns(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", price, nil, nil)
		pos.EntryVenueOrderID = "OID-1"
		pos.ExitVenueOrderID = "OID-X"
		pos.EntryOrder = &spot.OrderRequest{ClOrdId: "enter-1"}
		pos.ExitOrder = &spot.OrderRequest{ClOrdId: "exit-1"}
		pos.Fills = []*market.ExecutionData{{
			ExecID: "b0", OrderID: "OID-FILL",
		}}

		Convey("When ClientOrderID matches entry", func() {
			So(pos.owns(&market.ExecutionData{ClientOrderID: "enter-1"}), ShouldBeTrue)
		})

		Convey("When OrderID matches entry venue id", func() {
			So(pos.owns(&market.ExecutionData{OrderID: "OID-1"}), ShouldBeTrue)
		})

		Convey("When OrderID matches a prior fill", func() {
			So(pos.owns(&market.ExecutionData{OrderID: "OID-FILL"}), ShouldBeTrue)
		})

		Convey("When ids do not match", func() {
			So(pos.owns(&market.ExecutionData{OrderID: "other"}), ShouldBeFalse)
			So(pos.owns(nil), ShouldBeFalse)
		})
	}))
}

func TestIsEntryOrder(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", price, nil, nil)
		pos.EntryOrder = &spot.OrderRequest{ClOrdId: "enter-1"}
		pos.EntryVenueOrderID = "OID-1"

		Convey("When ClientOrderID matches", func() {
			So(pos.isEntryOrder(&market.ExecutionData{ClientOrderID: "enter-1"}), ShouldBeTrue)
		})

		Convey("When OrderID matches venue id", func() {
			So(pos.isEntryOrder(&market.ExecutionData{OrderID: "OID-1"}), ShouldBeTrue)
		})

		Convey("When nothing matches", func() {
			So(pos.isEntryOrder(&market.ExecutionData{OrderID: "x"}), ShouldBeFalse)
			So(pos.isEntryOrder(nil), ShouldBeFalse)
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
		_ = pos.Update(mark)
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
