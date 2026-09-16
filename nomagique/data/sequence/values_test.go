package sequence_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestValuesNext(t *testing.T) {
	Convey("Values supplies every configured value on each run", t, func() {
		values := sequence.NewValues(3.0, -2.0, 7.0)
		for range values.Next(nil) {
			break
		}
		So(tests.CollectSeq[float64](values.Next(nil)), ShouldResemble, []float64{3, -2, 7})
		So(values.Error(), ShouldBeNil)
		So(tests.CollectSeq[float64](sequence.NewValues[float64]().Next(nil)), ShouldBeEmpty)
	})
}
