package ui

import (
	"context"
	"iter"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
)

func TestBroadcast(t *testing.T) {
	Convey("Given a ui.Broadcast offramp", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		hub := NewHub(ctx, nil, nil)
		broadcast := NewBroadcast(hub)

		Convey("When non-evaluation input arrives", func() {
			out := broadcast("test-data")
			So(out, ShouldEqual, "test-data")
		})

		Convey("When evaluation input arrives", func() {
			var called bool
			eval := cognition.Evaluation(func() (
				[]byte, []byte, float64, float64, uint64, float64, bool, float64, iter.Seq2[[]byte, float64],
			) {
				called = true
				return []byte("enter"), nil, 0.9, 1.2, 50, 0.1, false, 0.05, nil
			})

			out := broadcast(eval)
			So(out, ShouldNotBeNil)
			So(called, ShouldBeTrue)
		})
	})
}
