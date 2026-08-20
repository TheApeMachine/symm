package websocket

import (
	"context"
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAPIRun(t *testing.T) {
	Convey("Given a running transport supervisor", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		api := &API{
			ctx:      ctx,
			cancel:   cancel,
			failures: make(chan error, 1),
		}
		expected := errors.New("level3 checksum mismatch")

		Convey("It should return the first fatal ingestion error", func() {
			api.reportFailure(expected)
			So(api.Run(), ShouldEqual, expected)
			So(api.Error(), ShouldEqual, expected)
		})
	})
}
