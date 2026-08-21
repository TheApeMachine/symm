package cmd

import (
	"context"
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
runnableFixture provides one controllable lifecycle component for System.Run.
*/
type runnableFixture struct {
	ctx context.Context
	err error
}

func (fixture *runnableFixture) Name() string { return "fixture" }
func (fixture *runnableFixture) Error() error { return fixture.err }

func (fixture *runnableFixture) Run() error {
	if fixture.err != nil {
		return fixture.err
	}

	<-fixture.ctx.Done()

	return fixture.ctx.Err()
}

func TestSystemRun(t *testing.T) {
	Convey("Given one failed system and one context-bound peer", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		failure := errors.New("fixture failed")
		system := &System{
			ctx:    ctx,
			cancel: cancel,
			Systems: []Runnable{
				&runnableFixture{ctx: ctx, err: failure},
				&runnableFixture{ctx: ctx},
			},
		}

		err := system.Run()

		Convey("It should cancel its peer and return the originating error", func() {
			So(errors.Is(err, failure), ShouldBeTrue)
			So(ctx.Err(), ShouldEqual, context.Canceled)
		})
	})

	Convey("Given expected component cancellation after system shutdown", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		system := &System{ctx: ctx, cancel: cancel}
		cancel()

		system.fail(context.Canceled)

		Convey("It should not retain shutdown as a system failure", func() {
			So(system.Error(), ShouldBeNil)
		})
	})
}

func TestSystemClose(t *testing.T) {
	Convey("Given resources recorded in acquisition order", t, func() {
		closed := make([]int, 0, 3)
		system := &System{closers: []func() error{
			func() error {
				closed = append(closed, 1)
				return nil
			},
			func() error {
				closed = append(closed, 2)
				return nil
			},
			func() error {
				closed = append(closed, 3)
				return nil
			},
		}}

		err := system.Close()

		Convey("It should release every resource in reverse acquisition order", func() {
			So(err, ShouldBeNil)
			So(closed, ShouldResemble, []int{3, 2, 1})
		})
	})
}
