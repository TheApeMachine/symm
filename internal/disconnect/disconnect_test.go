package disconnect

import (
	"errors"
	"net"
	"testing"

	"github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
)

func TestExpected(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When err is an abnormal websocket close", func() {
			err := &websocket.CloseError{Code: websocket.CloseAbnormalClosure}

			Convey("Expected is true", func() {
				So(Expected(err), ShouldBeTrue)
			})
		})

		Convey("When err is net.ErrClosed", func() {
			Convey("Expected is true", func() {
				So(Expected(net.ErrClosed), ShouldBeTrue)
			})
		})

		Convey("When err is a net.OpError", func() {
			err := &net.OpError{Op: "read", Err: errors.New("reset")}

			Convey("Expected is true", func() {
				So(Expected(err), ShouldBeTrue)
			})
		})

		Convey("When err is an unrelated application error", func() {
			Convey("Expected is false", func() {
				So(Expected(errors.New("json: invalid")), ShouldBeFalse)
			})
		})

		Convey("When err is nil", func() {
			Convey("Expected is false", func() {
				So(Expected(nil), ShouldBeFalse)
			})
		})
	})
}
