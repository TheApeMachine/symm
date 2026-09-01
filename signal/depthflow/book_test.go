package depthflow

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/kraken"
)

func bookAt() time.Time { return time.Unix(1_700_000_000, 0) }

func snapshot(symbol string, bids, asks []kraken.Level3Order) kraken.Level3Data {
	return kraken.Level3Data{
		Symbol:    symbol,
		Type:      "snapshot",
		Timestamp: bookAt(),
		Bids:      bids,
		Asks:      asks,
	}
}

func increment(symbol string, bids, asks []kraken.Level3Order) kraken.Level3Data {
	return kraken.Level3Data{
		Symbol:    symbol,
		Timestamp: bookAt(),
		Bids:      bids,
		Asks:      asks,
	}
}

/*
TestResidentBook_ChangeRestatesQuantity pins the defect that made book depth
drift. Kraken's "change" event carries an order's NEW quantity; the previous
signed-delta model credited it as a positive delta, so a single order that
merely shrank still INCREASED measured depth.
*/
func TestResidentBook_ChangeRestatesQuantity(t *testing.T) {
	Convey("Given a book holding one bid of 10 @ 100", t, func() {
		book := newResidentBook()
		book.apply(snapshot("BTC/USD",
			[]kraken.Level3Order{level3OrderID("a", "add", 100, 10, bookAt())},
			nil,
		))

		Convey("A change to quantity 4 lowers depth to the new quantity", func() {
			state, ok := book.apply(increment("BTC/USD",
				[]kraken.Level3Order{level3OrderID("a", "change", 100, 4, bookAt())},
				nil,
			))

			So(ok, ShouldBeTrue)

			// 100*4 = 400. The delta model produced 1000 + 400 = 1400.
			So(state.bidNotional, ShouldAlmostEqual, 400.0, 1e-9)
		})
	})
}

/*
TestResidentBook_DeleteRemovesResidentNotional pins the second defect: a delete
must withdraw the order's OWN resting notional, not whatever quantity the
delete message quotes.
*/
func TestResidentBook_DeleteRemovesResidentNotional(t *testing.T) {
	Convey("Given a book holding one bid of 10 @ 100", t, func() {
		book := newResidentBook()
		book.apply(snapshot("BTC/USD",
			[]kraken.Level3Order{level3OrderID("a", "add", 100, 10, bookAt())},
			nil,
		))

		Convey("A delete quoting a different quantity still removes the whole order", func() {
			state, _ := book.apply(increment("BTC/USD",
				[]kraken.Level3Order{level3OrderID("a", "delete", 100, 3, bookAt())},
				nil,
			))

			// The delta model subtracted the quoted 100*3 = 300, leaving 700
			// of depth that no longer existed.
			So(state.bidNotional, ShouldAlmostEqual, 0.0, 1e-9)
		})

		Convey("A delete naming an unknown order removes nothing", func() {
			state, _ := book.apply(increment("BTC/USD",
				[]kraken.Level3Order{level3OrderID("ghost", "delete", 100, 10, bookAt())},
				nil,
			))

			// The delta model drove depth to zero -- and on a later delete,
			// negative -- out of orders the book never held.
			So(state.bidNotional, ShouldAlmostEqual, 1000.0, 1e-9)
		})
	})
}

/*
TestResidentBook_RequiresSnapshot pins the third defect: increments arriving
before a snapshot describe a book this process never saw, so depth derived from
them is fabricated.
*/
func TestResidentBook_RequiresSnapshot(t *testing.T) {
	Convey("Given a book that has seen no snapshot", t, func() {
		book := newResidentBook()

		Convey("An increment reports no usable state", func() {
			_, ok := book.apply(increment("BTC/USD",
				[]kraken.Level3Order{level3OrderID("a", "add", 100, 10, bookAt())},
				nil,
			))

			So(ok, ShouldBeFalse)
		})

		Convey("A snapshot establishes the book and restates it wholesale", func() {
			book.apply(snapshot("BTC/USD",
				[]kraken.Level3Order{level3OrderID("a", "add", 100, 10, bookAt())},
				nil,
			))

			state, ok := book.apply(snapshot("BTC/USD",
				[]kraken.Level3Order{level3OrderID("b", "add", 50, 2, bookAt())},
				nil,
			))

			So(ok, ShouldBeTrue)

			// The second snapshot REPLACES the book: 50*2, not 1000 + 100.
			So(state.bidNotional, ShouldAlmostEqual, 100.0, 1e-9)
		})
	})
}

/*
TestResidentBook_RevertUndoesFrame pins the rollback. The book folds a frame in
before the metric pipeline validates it, so a rejected frame must leave no
trace -- otherwise a single crossed message poisons the book permanently.
*/
func TestResidentBook_RevertUndoesFrame(t *testing.T) {
	Convey("Given a seeded book", t, func() {
		book := newResidentBook()
		book.apply(snapshot("BTC/USD",
			[]kraken.Level3Order{level3OrderID("a", "add", 100, 10, bookAt())},
			nil,
		))

		Convey("Reverting an applied frame restores the prior order set", func() {
			book.apply(increment("BTC/USD",
				[]kraken.Level3Order{
					level3OrderID("b", "add", 101, 5, bookAt()),
					level3OrderID("a", "delete", 100, 10, bookAt()),
				},
				nil,
			))

			book.revert("BTC/USD")

			state, ok := book.notionals("BTC/USD")

			So(ok, ShouldBeTrue)
			So(state.bidNotional, ShouldAlmostEqual, 1000.0, 1e-9)
		})

		Convey("Reverting a snapshot restores the pre-snapshot book", func() {
			book.apply(snapshot("BTC/USD",
				[]kraken.Level3Order{level3OrderID("z", "add", 7, 1, bookAt())},
				nil,
			))

			book.revert("BTC/USD")

			state, _ := book.notionals("BTC/USD")
			So(state.bidNotional, ShouldAlmostEqual, 1000.0, 1e-9)
		})
	})
}
