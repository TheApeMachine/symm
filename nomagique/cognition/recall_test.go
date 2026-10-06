package cognition

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/core"
)

func TestRecallNext(t *testing.T) {
	Convey("Given an empty context", t, func() {
		_, err := drive(NewRecall(NewAssociate()), map[string]string{
			"context": "",
		}, nil)

		So(errors.Is(err, core.ErrDomain), ShouldBeTrue)
	})

	Convey("Given two classes of equal mass", t, func() {
		memory := NewAssociate()
		_, err := drive(memory, map[string]string{
			"context": "ctx",
			"class":   "enter",
		}, map[string]float64{
			"feedback": 1,
			"graded":   core.Unit,
		})
		So(err, ShouldBeNil)

		_, err = drive(memory, map[string]string{
			"context": "ctx",
			"class":   "exit",
		}, map[string]float64{
			"feedback": 1,
			"graded":   core.Unit,
		})
		So(err, ShouldBeNil)

		reading, recallErr := drive(NewRecall(memory), map[string]string{
			"context": "ctx",
		}, nil)
		So(recallErr, ShouldBeNil)
		winner, winnerErr := literal(reading, "winner")
		So(winnerErr, ShouldBeNil)
		So(winner, ShouldEqual, "")
	})
}
