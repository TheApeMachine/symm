package types

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStoplossBindRefusesZeroTrail(t *testing.T) {
	Convey("Given a fresh stoploss", t, func() {
		stop := NewStoploss(context.Background())

		Convey("When Bind is called with zero distance", func() {
			stop.Bind(1.0, 0)

			Convey("Then the regulator stays unarmed", func() {
				So(stop.Armed(), ShouldBeFalse)
				So(stop.Reason, ShouldContainSubstring, "refused")
			})
		})
	})
}
