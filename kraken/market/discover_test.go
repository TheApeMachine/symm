package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOnlinePairs(t *testing.T) {
	Convey("Given an instrument catalog", t, func() {
		pairs := []InstrumentPair{
			{Symbol: "BTC/EUR", Quote: "EUR", Status: "online"},
			{Symbol: "ETH/EUR", Quote: "EUR", Status: "online"},
			{Symbol: "SOL/USD", Quote: "USD", Status: "online"},
			{Symbol: "OLD/EUR", Quote: "EUR", Status: "delisted"},
			{Symbol: "", Quote: "EUR", Status: "online"},
		}

		Convey("It should return only online pairs in the quote currency, sorted", func() {
			So(onlinePairs(pairs, "EUR"), ShouldResemble, []string{"BTC/EUR", "ETH/EUR"})
		})

		Convey("It should return every online pair when quote is empty", func() {
			So(onlinePairs(pairs, ""), ShouldResemble, []string{"BTC/EUR", "ETH/EUR", "SOL/USD"})
		})
	})
}
