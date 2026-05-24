package fluid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFieldGaugeScore(t *testing.T) {
	Convey("Given cross-section field activity", t, func() {
		Convey("It should map robust activity into a unit gauge score", func() {
			score := fieldGaugeScore(FieldAggregate{
				Div:  -1.03,
				Vort: 0.2,
				Turb: 0.1,
				Re:   0,
			})

			So(score, ShouldBeBetween, 0.34, 0.35)
		})
	})
}
