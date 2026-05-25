package instrument

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestNewSubscribe(t *testing.T) {
	convey.Convey("Given instrument subscribe params", t, func() {
		convey.Convey("It should target the instrument channel with snapshot", func() {
			request := NewSubscribe()
			params, ok := request.Params.(Params)

			convey.So(ok, convey.ShouldBeTrue)
			convey.So(params.Channel, convey.ShouldEqual, "instrument")
			convey.So(params.Snapshot, convey.ShouldBeTrue)
		})
	})
}
