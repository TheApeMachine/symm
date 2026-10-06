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
			"stance":  "",
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
			"stance":  "",
		}, nil)
		So(recallErr, ShouldBeNil)
		winner, winnerErr := literal(reading, "winner")
		So(winnerErr, ShouldBeNil)
		So(winner, ShouldEqual, "")

		Convey("A stance lets only its own action compete", func() {
			for _, stance := range []string{"enter", "exit"} {
				reading, recallErr := drive(NewRecall(memory), map[string]string{
					"context": "ctx",
					"stance":  stance,
				}, nil)
				So(recallErr, ShouldBeNil)
				winner, winnerErr := literal(reading, "winner")
				So(winnerErr, ShouldBeNil)
				So(winner, ShouldEqual, stance)
			}
		})
	})

	Convey("Given enter and exit taught on one shared context, exit with the larger feedback", t, func() {
		memory := NewAssociate()
		teach := func(class string, feedback float64) {
			_, err := drive(memory, map[string]string{
				"context": "R0/R1",
				"class":   class,
			}, map[string]float64{
				"feedback": feedback,
				"graded":   core.Unit,
			})
			So(err, ShouldBeNil)
		}

		teach("enter", 0.011)
		teach("exit", 0.0227)

		recall := func(stance string) string {
			reading, err := drive(NewRecall(memory), map[string]string{
				"context": "R0/R1",
				"stance":  stance,
			}, nil)
			So(err, ShouldBeNil)
			winner, err := literal(reading, "winner")
			So(err, ShouldBeNil)
			return winner
		}

		Convey("Without a stance exit outweighs enter", func() {
			So(recall(""), ShouldEqual, "exit")
		})

		Convey("A flat asker recalls enter, a holding asker exit", func() {
			So(recall("enter"), ShouldEqual, "enter")
			So(recall("exit"), ShouldEqual, "exit")
		})

		Convey("Once losing feedback pushes enter below the graded start, a flat asker abstains", func() {
			teach("enter", -0.5)
			So(recall("enter"), ShouldEqual, "")
			So(recall("exit"), ShouldEqual, "exit")
		})
	})
}
