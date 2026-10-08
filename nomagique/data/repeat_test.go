package data_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestRepeat(t *testing.T) {
	Convey("Given a Repeat primitive with count 2", t, func() {
		rep := data.NewRepeat(2)

		Convey("It yields the incoming sequence twice", func() {
			var got []float64

			for ptr := range rep.Next(data.NewValue(1.0, 2.0).Next(nil)) {
				got = append(got, *(*float64)(ptr))
			}

			So(got, ShouldResemble, []float64{1.0, 2.0, 1.0, 2.0})
		})
	})
}
