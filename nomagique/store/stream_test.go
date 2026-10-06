package store_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestStream_Deform(t *testing.T) {
	Convey("Given two symbol streams observing one metric label", t, func() {
		btc := store.NewStream()
		eth := store.NewStream()

		Convey("A first observation answers no movement", func() {
			So(btc.Deform(map[string]float64{"price": 60000}), ShouldBeEmpty)
		})

		Convey("Each stream deforms against its own previous value, never the other symbol's", func() {
			btc.Deform(map[string]float64{"price": 60000})
			eth.Deform(map[string]float64{"price": 3000})

			btcMove := btc.Deform(map[string]float64{"price": 60000})
			ethMove := eth.Deform(map[string]float64{"price": 3300})

			So(btcMove["price"], ShouldEqual, 0)
			So(ethMove["price"], ShouldAlmostEqual, 300.0/6300.0, 1e-12)
		})
	})
}
