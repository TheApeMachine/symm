package replay

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewRest(t *testing.T) {
	Convey("Given a replay REST client", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		rest, err := NewRest(ctx)

		Convey("It should construct a no-op REST shim", func() {
			So(err, ShouldBeNil)
			So(rest, ShouldNotBeNil)
		})

		Convey("It should ignore GET and POST during replay", func() {
			So(rest.Get(ctx, nil, nil), ShouldBeNil)
			So(rest.Post(ctx, nil, nil), ShouldBeNil)
		})

		Convey("It should close without error", func() {
			So(rest.Close(), ShouldBeNil)
			So(rest.Error(), ShouldBeNil)
		})
	})
}
