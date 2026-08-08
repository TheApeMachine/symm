package ui

import (
	"errors"
	"fmt"
	"testing"

	"github.com/fasthttp/websocket"
	. "github.com/smartystreets/goconvey/convey"
)

func TestExpectedDashboardWriteClosure(t *testing.T) {
	Convey("Given the close sentinel returned by the underlying connection", t, func() {
		err := fmt.Errorf("dashboard write: %w", websocket.ErrCloseSent)

		Convey("It should classify the completed close handshake as expected", func() {
			So(expectedDashboardWriteClosure(err), ShouldBeTrue)
		})
	})

	Convey("Given an unrelated write failure", t, func() {
		err := errors.New("unexpected write failure")

		Convey("It should preserve the failure for logging", func() {
			So(expectedDashboardWriteClosure(err), ShouldBeFalse)
		})
	})
}
