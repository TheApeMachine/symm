package crosssection_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/crosssection"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestCrossSection(t *testing.T) {
	Convey("Given a cross-section pipeline over a shared member store", t, func() {
		members := crosssection.NewMemberStore()
		update := crosssection.NewUpdateMember("price", members)
		counts := crosssection.NewChangeCounts(members)
		median := crosssection.NewChangeMedian(members)
		baseline := crosssection.NewChangeBaseline()

		tick := func(member string, price float64) {
			item := &crosssection.MemberUpdate{Member: member, Price: price}

			for range update.Next(data.NewValue(unsafe.Pointer(item)).Next(nil)) {
			}

			So(update.Error(), ShouldBeNil)
		}

		Convey("It derives member changes and reduces the cross-section", func() {
			tick("A", 100)
			tick("B", 50)
			tick("C", 10)
			tick("A", 110)
			tick("B", 45)
			tick("C", 10)

			var countVals []float64

			for ptr := range counts.Next(nil) {
				countVals = append(countVals, *(*float64)(ptr))
			}

			So(counts.Error(), ShouldBeNil)
			So(len(countVals), ShouldEqual, 5)
			So(countVals[0], ShouldEqual, 3)
			So(countVals[1], ShouldEqual, 1)
			So(countVals[2], ShouldEqual, 1)
			So(countVals[3], ShouldEqual, 1)
			So(countVals[4], ShouldEqual, 0)
			So(counts.ExtremeKey(), ShouldEqual, "A")

			var medianVal float64

			for ptr := range median.Next(nil) {
				medianVal = *(*float64)(ptr)
			}

			So(median.Error(), ShouldBeNil)
			So(medianVal, ShouldEqual, 0)

			var baselineVals []float64
			signedFraction := countVals[4]

			for ptr := range baseline.Next(data.NewValue(signedFraction).Next(nil)) {
				baselineVals = append(baselineVals, *(*float64)(ptr))
			}

			So(baseline.Error(), ShouldBeNil)
			So(len(baselineVals), ShouldEqual, 3)
		})
	})
}
