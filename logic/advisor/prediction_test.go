package advisor

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFalsifiableMatches(t *testing.T) {
	Convey("Given a raw direction-of-change Prediction", t, func() {
		increase := &Falsifiable{Label: "metric/value", Move: INCREASE, Unit: RAW}
		expand := &Falsifiable{Label: "metric/value", Move: EXPAND, Unit: RAW}
		dissolve := &Falsifiable{Label: "metric/value", Move: DISSOLVE, Unit: RAW}

		Convey("unchanged values do not satisfy a zero threshold", func() {
			matched, err := increase.matches(2, 2)
			So(err, ShouldBeNil)
			So(matched, ShouldBeFalse)
		})

		Convey("signed movement and magnitude movement remain distinct", func() {
			matched, err := increase.matches(-2, -1)
			So(err, ShouldBeNil)
			So(matched, ShouldBeTrue)

			matched, err = expand.matches(-2, -3)
			So(err, ShouldBeNil)
			So(matched, ShouldBeTrue)

			matched, err = dissolve.matches(-2, -1)
			So(err, ShouldBeNil)
			So(matched, ShouldBeTrue)
		})
	})

	Convey("Given a percentage Prediction with a zero baseline", t, func() {
		event := &Falsifiable{Label: "metric/value", Move: INCREASE, Unit: PERCENT}

		Convey("the undefined comparison fails explicitly", func() {
			matched, err := event.matches(0, 1)
			So(err, ShouldNotBeNil)
			So(matched, ShouldBeFalse)
		})
	})
}

func BenchmarkFalsifiableMatches(b *testing.B) {
	event := &Falsifiable{Label: "metric/value", Move: EXPAND, Unit: RAW}

	for b.Loop() {
		matched, err := event.matches(-2, -3)

		if err != nil || !matched {
			b.Fatal(err)
		}
	}
}
