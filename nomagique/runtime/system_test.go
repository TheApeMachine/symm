package runtime

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSystemFail(t *testing.T) {
	Convey("The composed runtime owner retains and exposes stage failure", t, func() {
		system := NewSystem(t.Context(), "agent")
		system.Transition(READY)
		err := errors.New("execution acceptance unknown")
		system.Fail(err)
		So(system.Error(), ShouldEqual, err)
		So(system.Status(), ShouldEqual, ERROR)
		system.Fail(nil)
		So(system.Error(), ShouldEqual, err)
		So(system.Status(), ShouldEqual, ERROR)
		So(system.Close(), ShouldNotBeNil)
	})
}
