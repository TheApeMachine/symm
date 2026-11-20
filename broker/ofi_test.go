package broker_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
)

func TestNoteOFI(t *testing.T) {
	Convey("Setup", t, func() {
		price := &broker.Price{}

		Convey("When NoteOFI stores a Cont event reading", func() {
			price.NoteOFI("AAA/USD", -0.4)

			Convey("OFIReading returns it for exit pacing", func() {
				So(price.OFIReading("AAA/USD"), ShouldEqual, -0.4)
			})
		})

		Convey("When the symbol was never noted", func() {
			So(price.OFIReading("MISSING/USD"), ShouldEqual, 0)
		})
	})
}
