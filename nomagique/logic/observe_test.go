package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestObserve(t *testing.T) {
	alpha := types.MustIntern("observe/test/alpha")
	beta := types.MustIntern("observe/test/beta")
	primitive := Observe(alpha, beta)

	Convey("Given every configured observation coordinate", t, func() {
		input := types.Frame{}
		input.Put(alpha, 3)
		input.Put(beta, 5)

		Convey("It preserves the exact input", func() {
			output := input
			primitive(&output)
			So(output.Err, ShouldBeNil)
			So(output.Equal(input), ShouldBeTrue)
		})
	})

	Convey("Given a missing configured coordinate", t, func() {
		input := types.Frame{}
		input.Put(alpha, 3)

		Convey("It rejects the incomplete observation by name", func() {
			output := input
			primitive(&output)
			So(output.Err, ShouldNotBeNil)
			So(output.Err.Error(), ShouldContainSubstring, "observe/test/beta")
		})
	})
}
