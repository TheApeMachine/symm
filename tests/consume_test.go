package tests

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

/*
TestConsume proves the market callback adapter preserves both Tick success and
failure without treating a missing cut as an idle success.
*/
func TestConsume(t *testing.T) {
	Convey("Given a Tick that returns an error", t, func() {
		want := errors.New("tick failed")

		Convey("It should surface that error on the afterStep", func() {
			So(Consume(func() (*types.Thesis, error) {
				return nil, want
			})(), ShouldEqual, want)
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
