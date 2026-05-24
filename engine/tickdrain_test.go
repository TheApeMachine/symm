package engine

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWithTickDrain(t *testing.T) {
	Convey("Given a context with a tick drain hook", t, func() {
		calls := 0
		ctx := WithTickDrain(context.Background(), func() {
			calls++
		})

		Convey("When DrainTicks runs", func() {
			DrainTicks(ctx)

			Convey("It should invoke the hook once", func() {
				So(calls, ShouldEqual, 1)
			})
		})
	})
}

func TestDrainTicksWithoutHook(t *testing.T) {
	Convey("Given a plain context", t, func() {
		Convey("When DrainTicks runs", func() {
			DrainTicks(context.Background())

			Convey("It should not panic", func() {
				So(true, ShouldBeTrue)
			})
		})
	})
}
