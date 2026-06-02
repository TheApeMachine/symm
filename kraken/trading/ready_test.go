package trading

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMarkDeskReady(t *testing.T) {
	t.Cleanup(ResetDeskReady)

	Convey("Given a cold desk", t, func() {
		So(DeskReady(), ShouldBeFalse)

		Convey("When the private path publishes balances", func() {
			MarkDeskReady()

			Convey("It should allow story entry actions", func() {
				So(DeskReady(), ShouldBeTrue)
			})
		})
	})
}
