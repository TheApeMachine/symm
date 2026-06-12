package cmd

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
)

type stubSystem struct {
	tickErr error
	closed  bool
}

func (system *stubSystem) Tick() error {
	return system.tickErr
}

func (system *stubSystem) Close() error {
	system.closed = true

	return nil
}

type blockingSystem struct {
	ctx        context.Context
	tickExited chan struct{}
	closeCount atomic.Int32
}

func (system *blockingSystem) Tick() error {
	defer close(system.tickExited)

	<-system.ctx.Done()

	return system.ctx.Err()
}

func (system *blockingSystem) Close() error {
	system.closeCount.Add(1)

	return nil
}

func TestNewEngine(t *testing.T) {
	viper.Set("ui.addr", "127.0.0.1:0")

	Convey("Given a qpool", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		engine, err := NewEngine(ctx, pool)

		So(err, ShouldBeNil)

		Convey("It should register and tick systems", func() {
			stub := &stubSystem{}
			engine.systems = append(engine.systems, stub)

			So(engine.Start(), ShouldBeNil)
			So(stub.closed, ShouldBeTrue)
		})

		Convey("It should surface tick errors", func() {
			stub := &stubSystem{tickErr: errors.New("tick failed")}
			engine.systems = append(engine.systems, stub)

			So(engine.Start(), ShouldNotBeNil)
		})

		Convey("It should treat engine context cancellation as clean shutdown", func() {
			cancel()
			stub := &stubSystem{tickErr: context.Canceled}
			engine.systems = append(engine.systems, stub)

			So(engine.Start(), ShouldBeNil)
			So(stub.closed, ShouldBeTrue)
		})

		Convey("It should close systems when bootstrap send fails", func() {
			engine.bus = internal.NewBus(ctx, pool, nil, nil)
			stub := &blockingSystem{
				ctx:        engine.Context(),
				tickExited: make(chan struct{}),
			}
			engine.systems = append(engine.systems, stub)

			So(engine.Start(), ShouldNotBeNil)
			So(stub.closeCount.Load(), ShouldEqual, 1)

			_, open := <-stub.tickExited
			So(open, ShouldBeFalse)
		})
	})
}

func BenchmarkEngineStart(b *testing.B) {
	viper.Set("ui.addr", "127.0.0.1:0")

	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 1, 4, nil)
	engine, _ := NewEngine(ctx, pool)
	engine.systems = append(engine.systems, &stubSystem{})

	for b.Loop() {
		_ = engine.Start()
	}
}
