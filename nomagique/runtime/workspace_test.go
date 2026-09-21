package runtime_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
recorder is a Step that counts what it was handed and reports the order in
which stages observed each sequence.
*/
type recorder struct {
	seen  atomic.Int64
	order *atomic.Int64
	stamp atomic.Int64
}

func (step *recorder) Step(ctx context.Context, payload []byte) ([]byte, error) {
	step.seen.Add(1)

	if step.order != nil {
		step.stamp.Store(step.order.Add(1))
	}

	return payload, nil
}

func settle(t *testing.T, condition func() bool) {
	deadline := time.After(5 * time.Second)

	for !condition() {
		select {
		case <-deadline:
			t.Fatal("stages did not settle")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestWorkspace(t *testing.T) {
	Convey("Given a workspace with concurrent stages", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		first := &recorder{}
		second := &recorder{}

		workspace, err := runtime.NewWorkspace(ctx, "ring", 1024, 1, [][]runtime.Step{
			{first, second},
		})
		So(err, ShouldBeNil)

		Convey("It refuses publication until it is admitted", func() {
			So(workspace.Push([]byte("early")), ShouldNotBeNil)
			So(first.seen.Load(), ShouldEqual, 0)
		})

		Convey("Every step in a group observes every observation", func() {
			workspace.Admit()

			const published = 512

			for index := 0; index < published; index++ {
				So(workspace.Push([]byte("observation")), ShouldBeNil)
			}

			settle(t, func() bool {
				return first.seen.Load() >= published && second.seen.Load() >= published
			})

			So(first.seen.Load(), ShouldEqual, published)
			So(second.seen.Load(), ShouldEqual, published)
		})

		So(workspace.Close(), ShouldBeNil)
	})

	Convey("Given a workspace with ordered stages", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		order := &atomic.Int64{}
		upstream := &recorder{order: order}
		downstream := &recorder{order: order}

		workspace, err := runtime.NewWorkspace(ctx, "ordered", 1024, 1, [][]runtime.Step{
			{upstream},
			{downstream},
		})
		So(err, ShouldBeNil)

		workspace.Admit()
		So(workspace.Push([]byte("observation")), ShouldBeNil)

		settle(t, func() bool {
			return upstream.seen.Load() > 0 && downstream.seen.Load() > 0
		})

		Convey("A later stage cannot pass a sequence before an earlier one", func() {
			So(upstream.stamp.Load(), ShouldBeLessThan, downstream.stamp.Load())
		})

		So(workspace.Close(), ShouldBeNil)
	})

	Convey("Given a ring mounted inside another ring", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		leaf := &recorder{}

		inner, err := runtime.NewWorkspace(ctx, "inner", 1024, 2, [][]runtime.Step{
			{leaf},
		})
		So(err, ShouldBeNil)
		inner.Admit()

		outer, err := runtime.NewWorkspace(ctx, "outer", 1024, 1, [][]runtime.Step{
			{inner},
		})
		So(err, ShouldBeNil)
		outer.Admit()

		const published = 128

		for index := 0; index < published; index++ {
			So(outer.Push([]byte("observation")), ShouldBeNil)
		}

		Convey("An observation published to the outer ring reaches the inner stage", func() {
			settle(t, func() bool { return leaf.seen.Load() >= published })
			So(leaf.seen.Load(), ShouldEqual, published)
		})

		So(outer.Close(), ShouldBeNil)
		So(inner.Close(), ShouldBeNil)
	})

	Convey("Given an invalid ring", t, func() {
		ctx := context.Background()

		Convey("It refuses a capacity that is not a power of two", func() {
			_, err := runtime.NewWorkspace(ctx, "bad", 1000, 1, [][]runtime.Step{
				{&recorder{}},
			})
			So(err, ShouldNotBeNil)
		})

		Convey("It refuses a ring with no stages", func() {
			_, err := runtime.NewWorkspace(ctx, "bad", 1024, 1, nil)
			So(err, ShouldNotBeNil)
		})

		Convey("It refuses a stage with no steps", func() {
			_, err := runtime.NewWorkspace(ctx, "bad", 1024, 1, [][]runtime.Step{{}})
			So(err, ShouldNotBeNil)
		})
	})
}
