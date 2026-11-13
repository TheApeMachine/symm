package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
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
			err := pos.Update(dec("120"))

			Convey("Mark and fee-net PnL match entry 100 → mark 120 at 0.40% taker", func() {
				// feeIn=0.40, feeOut=0.48 → pnl=120-0.48-(100+0.40)=19.12
				// pct=19.12/100.4*100
				So(err, ShouldBeNil)
				So(pos.Mark.String(), ShouldEqual, "120")
				So(pos.PnL.String(), ShouldEqual, "19.12")
				So(pos.PnLPct.String(), ShouldEqual, "19.04382470119522")
				So(pos.Stoploss, ShouldNotBeNil)
				So(pos.Closing, ShouldBeFalse)
			})
		})

		Convey("When Update is called with a nil mark", func() {
			err := pos.Update(nil)

			Convey("It returns a conflict for a nil price", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "[position.Update] price is nil")
			})
		})

		Convey("When the position is already closed", func() {
			closed := time.Unix(2, 0).UTC()
			pos.ClosedAt = &closed
			err := pos.Update(dec("120"))

			Convey("It returns a conflict for a closed position", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "[position.Update] closed position")
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

		Convey("When exit is called on an open position", func() {
			err := pos.exit()

			Convey("Closing is armed with the exit trigger", func() {
				So(err, ShouldBeNil)
				So(pos.Closing, ShouldBeTrue)
				So(pos.ExitTrigger, ShouldEqual, "policy exit match R00/M1")
				So(pos.ClosedAt, ShouldBeNil)
			})
		})

		Convey("When exit is called again", func() {
			So(pos.exit(), ShouldBeNil)
			trigger := pos.ExitTrigger
			So(pos.exit(), ShouldBeNil)

			Convey("State is unchanged", func() {
				So(pos.Closing, ShouldBeTrue)
				So(pos.ExitTrigger, ShouldEqual, trigger)
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
			})
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
		_ = pos.exit()
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
