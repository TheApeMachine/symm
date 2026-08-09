package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStatusFromMarket(t *testing.T) {
	Convey("Given a known exec_type", t, func() {
		status, err := StatusFromMarket("filled")
		So(err, ShouldBeNil)
		So(status, ShouldEqual, FILLED)

		status, err = StatusFromMarket("partially_filled")
		So(err, ShouldBeNil)
		So(status, ShouldEqual, PARTIAL_FILLED)
	})

	Convey("Given an unknown exec_type", t, func() {
		_, err := StatusFromMarket("mystery")
		So(err, ShouldNotBeNil)
	})
}
