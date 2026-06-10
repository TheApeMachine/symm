package cmd

import (
	"context"
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
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

func TestNewEngine(t *testing.T) {
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
	})
}

func BenchmarkEngineStart(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 1, 4, nil)
	engine, _ := NewEngine(ctx, pool)
	engine.systems = append(engine.systems, &stubSystem{})

	for b.Loop() {
		_ = engine.Start()
	}
}
