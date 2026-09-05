package manifold

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestOrderContentID pins the identity the physics domain merges and evicts on.
Two different orders must never fold to one particle, and one order must keep
the same particle for as long as it rests.
*/
func TestOrderContentID(t *testing.T) {
	Convey("Given order identities", t, func() {
		order := orderIdentity{symbol: "BTC/USD", orderID: "abc"}

		Convey("The same identity always resolves to the same content id", func() {
			So(orderContentID(order), ShouldEqual, orderContentID(order))
		})

		Convey("A different order on the same symbol is a different particle", func() {
			other := orderIdentity{symbol: "BTC/USD", orderID: "abd"}
			So(orderContentID(order), ShouldNotEqual, orderContentID(other))
		})

		Convey("The same order id on a different symbol is a different particle", func() {
			other := orderIdentity{symbol: "ETH/USD", orderID: "abc"}
			So(orderContentID(order), ShouldNotEqual, orderContentID(other))
		})

		Convey("A content id is never negative", func() {
			// It is stored as an int64 on the wire and used as a map key on
			// both sides, so the sign bit must never be part of the hash.
			So(orderContentID(order), ShouldBeGreaterThanOrEqualTo, 0)
			So(orderContentID(orderIdentity{symbol: "", orderID: ""}),
				ShouldBeGreaterThanOrEqualTo, 0)
		})

		Convey("A symbol/order split cannot be forged by concatenation", func() {
			// Folding length with content keeps "AB"+"C" from colliding with
			// "A"+"BC", which a naive concatenating hash would let happen.
			left := orderIdentity{symbol: "AB", orderID: "C"}
			right := orderIdentity{symbol: "A", orderID: "BC"}
			So(orderContentID(left), ShouldNotEqual, orderContentID(right))
		})
	})
}
