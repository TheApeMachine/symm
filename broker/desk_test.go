package broker

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
)

func TestDeskUpdateNotesTape(t *testing.T) {
	Convey("a trade's quote dollars land on the price tape", t, func() {
		price := &Price{tape: NewTape()}
		desk := &Desk{price: price}
		at := time.Now()

		desk.Update([]*market.TradeData{{
			Symbol:    "AAA/USD",
			Price:     decimal.RequireFromString("10"),
			Qty:       decimal.RequireFromString("25"),
			Timestamp: at,
		}})

		recent, prior := price.tape.Uptake("AAA/USD", at)
		So(recent, ShouldEqual, 250)
		So(prior, ShouldEqual, 0)
	})
}
