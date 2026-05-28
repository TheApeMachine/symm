package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSymbolBaseAsset(t *testing.T) {
	Convey("Given a Kraken wsname", t, func() {
		Convey("It should return the base asset prefix", func() {
			So(Symbol("BTC/EUR").BaseAsset(), ShouldEqual, "BTC")
		})

		Convey("It should pass through symbols without a separator", func() {
			So(Symbol("BTC").BaseAsset(), ShouldEqual, "BTC")
		})
	})
}

func TestSymbolPaperOrderID(t *testing.T) {
	Convey("Given a paper order kind", t, func() {
		Convey("It should build a deterministic order id", func() {
			So(Symbol("BTC/EUR").PaperOrderID("buy"), ShouldEqual, "paper-buy-BTC/EUR")
		})
	})
}
