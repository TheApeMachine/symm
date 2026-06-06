package replay

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestReplayLedgerObserveSymbolPrice(t *testing.T) {
	Convey("Given a replay ledger price path", t, func() {
		ledger := newReplayLedger(triggerTestCosts())

		ledger.observeSymbolPrice("BTC/EUR", 100)
		ledger.observeSymbolPrice("BTC/EUR", 101)
		ledger.observeSymbolPrice("BTC/EUR", 101)
		ledger.observeSymbolPrice("BTC/EUR", 102)

		Convey("It should store distinct prices and derive volatility", func() {
			So(ledger.pricePaths["BTC/EUR"], ShouldHaveLength, 3)
			So(ledger.priceVolatility("BTC/EUR"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestReplayLedgerObserveSymbolPriceGuards(t *testing.T) {
	Convey("Given invalid price observations", t, func() {
		ledger := newReplayLedger(triggerTestCosts())

		Convey("It should ignore empty symbols and non-positive prices", func() {
			ledger.observeSymbolPrice("", 100)
			ledger.observeSymbolPrice("BTC/EUR", 0)

			So(ledger.pricePaths["BTC/EUR"], ShouldBeNil)
		})
	})
}

func TestReplayLedgerObserveSymbolPriceWindow(t *testing.T) {
	Convey("Given more distinct prices than the regime window", t, func() {
		ledger := newReplayLedger(triggerTestCosts())
		window := perspectives.RegimeWindow()

		for price := 1; price <= window+10; price++ {
			ledger.observeSymbolPrice("BTC/EUR", float64(price))
		}

		Convey("It should retain only the most recent window", func() {
			prices := ledger.pricePaths["BTC/EUR"]

			So(prices, ShouldHaveLength, window)
			So(prices[0], ShouldEqual, float64(11))
			So(prices[len(prices)-1], ShouldEqual, float64(window+10))
		})
	})
}

func TestReplayLedgerObservePrice(t *testing.T) {
	Convey("Given a measurement row", t, func() {
		ledger := newReplayLedger(triggerTestCosts())

		ledger.observePrice(types.Measurement{
			Symbol: "BTC/EUR",
			Last:   100,
		})

		Convey("It should route the last price into the symbol path", func() {
			So(ledger.pricePaths["BTC/EUR"], ShouldResemble, []float64{100})
		})
	})
}

func BenchmarkReplayLedgerObserveSymbolPrice(b *testing.B) {
	ledger := newReplayLedger(triggerTestCosts())

	for b.Loop() {
		ledger.observeSymbolPrice("BTC/EUR", 100+float64(b.N%100))
	}
}
