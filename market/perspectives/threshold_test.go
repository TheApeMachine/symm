package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestSetNoiseFloorSNR(t *testing.T) {
	convey.Convey("Given a raised noise floor", t, func() {
		original := NoiseFloorSNR()
		t.Cleanup(func() { SetNoiseFloorSNR(original) })

		SetNoiseFloorSNR(1.5)

		tree := &Tree{Branches: []Branch{
			{
				Category:  CategoryAggressiveDrive,
				Unit:      UnitSNR,
				Condition: ConditionIsGreaterThan,
				Value:     1.0,
				Action:    ActionEnter,
			},
		}}

		convey.Convey("When SNR is below the live floor", func() {
			action := tree.Walk([]Measurement{measurement(CategoryAggressiveDrive, 1.2)}, nil)

			convey.Convey("It should not authorize entry", func() {
				convey.So(action, convey.ShouldBeNil)
			})
		})

		convey.Convey("When SNR clears the live floor", func() {
			action := tree.Walk([]Measurement{measurement(CategoryAggressiveDrive, 1.6)}, nil)

			convey.Convey("It should authorize entry", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionEnter)
			})
		})
	})
}
