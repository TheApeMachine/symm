package paper

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIdentifierMintIDs(t *testing.T) {
	Convey("Given an identifier", t, func() {
		identifier := NewIdentifier()

		Convey("It should mint distinct Kraken-shaped ids", func() {
			orderID := identifier.OrderID()
			execID := identifier.ExecID()
			clOrdID := identifier.ClOrdID()

			So(orderID, ShouldStartWith, "PAPER-")
			So(clOrdID, ShouldStartWith, "p")
			So(len(execID), ShouldBeGreaterThan, 8)
			So(orderID, ShouldNotEqual, identifier.OrderID())
		})
	})
}

func BenchmarkIdentifierOrderID(b *testing.B) {
	identifier := NewIdentifier()

	for b.Loop() {
		_ = identifier.OrderID()
	}
}
