package tests

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

func TestConsume(t *testing.T) {
	Convey("Given a Tick that returns an error", t, func() {
		want := errors.New("tick failed")

		Convey("It should surface that error on the afterStep", func() {
			So(Consume(func() (*types.Thesis, error) {
				return nil, want
			})(), ShouldEqual, want)
		})
	})

	Convey("Given a Tick with no measurements yet", t, func() {
		Convey("It should treat PreconditionFailed as an idle step", func() {
			So(Consume(func() (*types.Thesis, error) {
				return nil, errnie.Err(
					errnie.PreconditionFailed,
					"crypto: no signal measurements",
					nil,
				)
			})(), ShouldBeNil)
		})
	})

	Convey("Given a Tick that succeeds", t, func() {
		Convey("It should return nil", func() {
			So(Consume(func() (*types.Thesis, error) {
				return &types.Thesis{}, nil
			})(), ShouldBeNil)
		})
	})
}
