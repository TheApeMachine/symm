package public

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResponseStruct(t *testing.T) {
	Convey("Given a Kraken REST envelope", t, func() {
		response := Response{
			Error:  []string{"EGeneral:Invalid arguments"},
			Result: map[string]any{"count": 1},
		}

		Convey("It should retain error and result fields", func() {
			So(len(response.Error), ShouldEqual, 1)
			So(response.Result, ShouldNotBeNil)
		})
	})
}
