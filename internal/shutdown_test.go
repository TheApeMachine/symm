package internal

import (
	"context"
	"errors"
	"syscall"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIsShutdown(t *testing.T) {
	Convey("Given context cancellation", t, func() {
		Convey("It should report shutdown without treating other errors as shutdown", func() {
			So(IsShutdown(context.Canceled), ShouldBeTrue)
			So(IsShutdown(context.DeadlineExceeded), ShouldBeFalse)
			So(IsShutdown(errors.New("boom")), ShouldBeFalse)
			So(IsShutdown(nil), ShouldBeFalse)
		})
	})
}

func TestIsClientDisconnect(t *testing.T) {
	Convey("Given websocket write failures", t, func() {
		Convey("It should treat client-side closes as expected", func() {
			So(IsClientDisconnect(syscall.EPIPE), ShouldBeTrue)
			So(IsClientDisconnect(syscall.ECONNRESET), ShouldBeTrue)
			So(
				IsClientDisconnect(errors.New("write tcp4 127.0.0.1:8765->127.0.0.1:51046: write: broken pipe")),
				ShouldBeTrue,
			)
			So(IsClientDisconnect(errors.New("boom")), ShouldBeFalse)
			So(IsClientDisconnect(nil), ShouldBeFalse)
		})
	})
}

func TestReportError(t *testing.T) {
	Convey("Given shutdown and real errors", t, func() {
		Convey("It should pass cancellation through without logging", func() {
			So(ReportError(context.Canceled), ShouldEqual, context.Canceled)
			So(ReportError(nil), ShouldBeNil)
		})
	})
}
