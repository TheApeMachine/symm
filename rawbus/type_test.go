package rawbus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTypeRoutingDiscriminators(t *testing.T) {
	Convey("Given raw bus discriminators", t, func() {
		Convey("It should keep story and desk routes distinct", func() {
			So(TypeActions, ShouldNotEqual, TypeOrder)
			So(TypeActions.String(), ShouldEqual, "actions")
			So(TypeOrder.String(), ShouldEqual, "order")
		})
	})
}
