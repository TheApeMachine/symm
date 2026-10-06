package crosssection_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/crosssection"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestCrossSection(t *testing.T) {
	Convey("Given a cross-section pipeline over a shared member store", t, func() {
		members := store.NewKV[string, float64](nil)
		update := crosssection.NewUpdateMember("price", members)
		counts := crosssection.NewChangeCounts(members)
		median := crosssection.NewChangeMedian(members)
		baseline := crosssection.NewChangeBaseline()
		stages := []core.Primitive{update, counts, median, baseline}

		var last *data.Adapter

		tick := func(member string, price float64) {
			state := data.NewState(data.NewMap())
			adapter := data.NewAdapter(nil, state)

			text := data.NewTextMap()
			text.Values["member"] = member
			issued := data.NewOutputMap()
			issued.Values["price"] = price

			for range adapter.Next(data.NewValue(text)) {
			}

			for range adapter.Next(data.NewValue(issued)) {
			}

			for _, stage := range stages {
				for range stage.Next(data.NewValue(adapter)) {
				}

				So(stage.Error(), ShouldBeNil)
			}

			last = adapter
		}

		Convey("It derives member changes and reduces the cross-section", func() {
			tick("A", 100)
			tick("B", 50)
			tick("C", 10)
			tick("A", 110)
			tick("B", 45)
			tick("C", 10)

			var got data.Map[float64]

			for pointer := range last.Next(data.NewValue(data.NewMap(
				"valid_member_count", "", "positive_count", "", "negative_count", "",
				"zero_count", "", "signed_fraction", "", "signed_median", "",
				"signed_fraction_baseline", "", "signed_fraction_divergence", "",
			))) {
				got = *(*data.Map[float64])(pointer)
			}

			So(last.Error(), ShouldBeNil)
			So(got.Values["valid_member_count"], ShouldEqual, 3)
			So(got.Values["positive_count"], ShouldEqual, 1)
			So(got.Values["negative_count"], ShouldEqual, 1)
			So(got.Values["zero_count"], ShouldEqual, 1)
			So(got.Values["signed_fraction"], ShouldEqual, 0)
			So(got.Values["signed_median"], ShouldEqual, 0)

			var text data.Map[string]

			for pointer := range last.Next(data.NewValue(data.NewLiteral("extreme_key"))) {
				text = *(*data.Map[string])(pointer)
			}

			So(last.Error(), ShouldBeNil)
			So(text.Values["extreme_key"], ShouldEqual, "A")
		})
	})
}
