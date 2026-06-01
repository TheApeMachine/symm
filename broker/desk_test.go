package broker

import (
	"strings"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestDesk_NextClOrdID(t *testing.T) {
	convey.Convey("Given a desk", t, func() {
		desk := &Desk{}

		convey.Convey("When NextClOrdID is called", func() {
			clOrdID := desk.NextClOrdID()

			convey.Convey("It should return a client order id with the desk prefix", func() {
				convey.So(clOrdID, convey.ShouldStartWith, "s")
				convey.So(len(clOrdID), convey.ShouldBeGreaterThan, 1)
			})
		})

		convey.Convey("When NextClOrdID is called twice", func() {
			first := desk.NextClOrdID()
			second := desk.NextClOrdID()

			convey.Convey("It should return distinct ids", func() {
				convey.So(first, convey.ShouldNotEqual, second)
				convey.So(strings.HasPrefix(first, "s"), convey.ShouldBeTrue)
				convey.So(strings.HasPrefix(second, "s"), convey.ShouldBeTrue)
			})
		})
	})
}

func BenchmarkDesk_NextClOrdID(b *testing.B) {
	desk := &Desk{}

	for b.Loop() {
		_ = desk.NextClOrdID()
	}
}
