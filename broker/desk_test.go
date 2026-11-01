package broker

import (
	"context"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPendingKeyScopesBooks(t *testing.T) {
	Convey("Pending keys keep concurrent books from colliding", t, func() {
		So(LedgerPaper.PendingKey("BTC/USD"), ShouldEqual, "paper:BTC/USD")
		So(LedgerVirtual.PendingKey("BTC/USD"), ShouldEqual, "virtual:BTC/USD")
		So(LedgerReal.PendingKey("BTC/USD"), ShouldEqual, "real:BTC/USD")

		book, symbol := ParsePendingKey("virtual:ETH/USD")
		So(book, ShouldEqual, LedgerVirtual)
		So(symbol, ShouldEqual, "ETH/USD")
	})
}

func TestPerBookSlots(t *testing.T) {
	Convey("Desk allows the same symbol on virtual and paper at once", t, func() {
		ctx := context.Background()
		desk := &Desk{
			papers:   NewCache(ctx),
			reals:    NewCache(ctx),
			virtuals: NewCache(ctx),
			pending:  &sync.Map{},
		}

		virtual := &Position{Symbol: "BTC/USD", Book: "virtual", Virtual: true}
		paper := &Position{Symbol: "BTC/USD", Book: "paper"}

		desk.virtuals.Add("BTC/USD", virtual, true)
		desk.papers.Add("BTC/USD", paper, true)

		So(desk.PositionOn("BTC/USD", LedgerVirtual), ShouldEqual, virtual)
		So(desk.PositionOn("BTC/USD", LedgerPaper), ShouldEqual, paper)
		So(len(desk.Positions("BTC/USD")), ShouldEqual, 2)
		So(desk.cache(LedgerVirtual).Has("BTC/USD"), ShouldBeTrue)
		So(desk.cache(LedgerPaper).Has("BTC/USD"), ShouldBeTrue)
		So(desk.cache(LedgerReal).Has("BTC/USD"), ShouldBeFalse)
	})
}
