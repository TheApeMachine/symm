package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
)

func TestFinishAllocateEntryErr_IOWrappedValidation(t *testing.T) {
	Convey("Given a validation error wrapped as IO", t, func() {
		inner := errnie.Err(
			errnie.Validation,
			"[price] allocated quantity below instrument minimum for SENT/USD",
			nil,
		)
		wrapped := errnie.Err(errnie.IO, "[price] get entry cost failed for SENT/USD", inner)

		Convey("When finishAllocateEntryErr maps the outcome", func() {
			_, err := finishAllocateEntryErr("SENT/USD", wrapped)

			So(err, ShouldNotBeNil)
			So(errnie.IsNotFound(err), ShouldBeTrue)
			So(errnie.IsIO(err), ShouldBeFalse)
			So(sizingAbstain(wrapped), ShouldBeTrue)
		})
	})
}
