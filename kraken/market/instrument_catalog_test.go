package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInstrumentCatalogApply(t *testing.T) {
	Convey("Given an instrument channel snapshot", t, func() {
		catalog := &InstrumentCatalog{pairs: make(map[string]InstrumentPair)}

		catalog.Apply(InstrumentUpdate{
			Pairs: []InstrumentPair{{
				Symbol:         "DOGE/USD",
				PricePrecision: 5,
				QtyPrecision:   8,
			}},
		})

		Convey("It should expose pair precision rules by symbol", func() {
			pair, ok := catalog.Pair("DOGE/USD")

			So(ok, ShouldBeTrue)
			So(pair.PricePrecision, ShouldEqual, 5)
			So(pair.QtyPrecision, ShouldEqual, 8)
		})
	})
}

func TestBookSidePruneBeyond(t *testing.T) {
	Convey("Given a bid side deeper than the subscription depth", t, func() {
		side := newBidSide()

		for index := range 12 {
			side.apply(BookLevel{Price: float64(100 - index), Qty: 1})
		}

		side.pruneBeyond(10)

		Convey("It should retain only the subscribed depth", func() {
			So(side.tree.Len(), ShouldEqual, 10)
		})
	})
}
