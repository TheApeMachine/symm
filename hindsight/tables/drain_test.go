package tables

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestCatalog_Drain(t *testing.T) {
	Convey("An idle or empty storage queue can be drained without dereferencing nil", t, func() {
		tee := hindsight.NewStoreTee("drain-test", 4)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		catalog := &Catalog{storeTee: tee}

		Convey("Cancellation before activation finishes safely", func() {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			So(catalog.Drain(ctx, 1), ShouldBeNil)
		})

		Convey("An activated empty queue survives periodic polling and cancellation", func() {
			tee.Transition(runtime.READY)
			// Allow multiple 50ms flush ticks before exercising the final drain.
			ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
			defer cancel()
			So(catalog.Drain(ctx, 1), ShouldBeNil)
			So(ctx.Err(), ShouldEqual, context.DeadlineExceeded)
			So(tee.Error(), ShouldBeNil)
		})
	})
}
