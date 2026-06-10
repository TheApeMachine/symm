package internal

import (
	"context"
	"errors"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestIsShutdown(t *testing.T) {
	convey.Convey("Given context cancellation", t, func() {
		convey.Convey("It should report shutdown without treating other errors as shutdown", func() {
			convey.So(IsShutdown(context.Canceled), convey.ShouldBeTrue)
			convey.So(IsShutdown(context.DeadlineExceeded), convey.ShouldBeFalse)
			convey.So(IsShutdown(errors.New("boom")), convey.ShouldBeFalse)
			convey.So(IsShutdown(nil), convey.ShouldBeFalse)
		})
	})
}

func TestReportError(t *testing.T) {
	convey.Convey("Given shutdown and real errors", t, func() {
		convey.Convey("It should pass cancellation through without logging", func() {
			convey.So(ReportError(context.Canceled), convey.ShouldEqual, context.Canceled)
			convey.So(ReportError(nil), convey.ShouldBeNil)
		})
	})
}
