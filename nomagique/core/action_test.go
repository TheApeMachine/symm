package core_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestActionNext(t *testing.T) {
	Convey("Action emits its configured operations in order on every run", t, func() {
		action := core.NewAction(core.ActionWrite, core.ActionRead)
		expected := []core.ActionType{core.ActionWrite, core.ActionRead}
		So(tests.CollectSeq[core.ActionType](action.Next(nil)), ShouldResemble, expected)
		for range action.Next(nil) {
			break
		}
		So(tests.CollectSeq[core.ActionType](action.Next(nil)), ShouldResemble, expected)
		So(tests.CollectSeq[core.ActionType](core.NewAction().Next(nil)), ShouldBeEmpty)
		So(action.Error(), ShouldBeNil)
	})
}
