package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIsManifoldBinary(t *testing.T) {
	Convey("Given an SMF1 lattice frame", t, func() {
		payload := []byte{'S', 'M', 'F', '1', 2, 0, 0}

		Convey("It is treated as binary fanout", func() {
			So(isManifoldBinary(payload), ShouldBeTrue)
		})
	})

	Convey("Given an SMF1 display frame", t, func() {
		payload := []byte{'S', 'M', 'F', '1', 5, 0, 0}

		Convey("It is treated as binary fanout", func() {
			So(isManifoldBinary(payload), ShouldBeTrue)
		})
	})

	Convey("Given a JSON UI frame", t, func() {
		So(isManifoldBinary([]byte(`{"balances":[]}`)), ShouldBeFalse)
	})
}
