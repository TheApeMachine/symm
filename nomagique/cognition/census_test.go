package cognition

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/core"
)

func TestCensusNext(t *testing.T) {
	Convey("Given one new enter association", t, func() {
		memory := NewAssociate()
		_, err := drive(memory, map[string]string{
			"context": "a/b/c",
			"class":   "enter",
		}, map[string]float64{
			"feedback": 1,
			"graded":   core.Unit,
		})
		So(err, ShouldBeNil)

		_, err = drive(memory, map[string]string{
			"context": "a/b/c",
			"class":   "enter",
		}, map[string]float64{
			"feedback": 1,
			"graded":   core.Unit,
		})
		So(err, ShouldBeNil)

		reading, censusErr := drive(NewCensus(memory), nil, nil)
		So(censusErr, ShouldBeNil)
		records, recordsErr := number(reading, "records")
		span, spanErr := number(reading, "span")
		enter, enterErr := number(reading, "enter")
		So(recordsErr, ShouldBeNil)
		So(spanErr, ShouldBeNil)
		So(enterErr, ShouldBeNil)
		So(records, ShouldEqual, 2)
		So(span, ShouldEqual, 3)
		So(enter, ShouldEqual, 1)
	})
}
