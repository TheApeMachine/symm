package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestTreeWalk(t *testing.T) {
	convey.Convey("Given a tree whose deeper leaf is gated behind an extra confirmation", t, func() {
		tree := &Tree{Branches: []Branch{
			{
				Category:  CategoryAggressiveDrive,
				Unit:      UnitSNR,
				Condition: ConditionIsGreaterThan,
				Value:     1.0,
				Action:    ActionEnter,
				Branches: []Branch{
					{
						Category:  CategoryVerticalIgnition,
						Unit:      UnitSNR,
						Condition: ConditionIsGreaterThan,
						Value:     1.0,
						Action:    ActionShort,
					},
				},
			},
		}}

		convey.Convey("When only the shallow confirmation clears the floor", func() {
			action := tree.Walk(
				[]Measurement{measurement(CategoryAggressiveDrive, 1.4)},
				nil,
			)

			convey.Convey("It returns the shallow action", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionEnter)
			})
		})

		convey.Convey("When the deeper confirmation also clears the floor", func() {
			action := tree.Walk(
				[]Measurement{
					measurement(CategoryAggressiveDrive, 1.4),
					measurement(CategoryVerticalIgnition, 1.6),
				},
				nil,
			)

			convey.Convey("It walks past the shallow action to the deeper leaf", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionShort)
			})
		})
	})

	convey.Convey("Given two branches whose actions sit at different depths", t, func() {
		tree := &Tree{Branches: []Branch{
			{
				Category:  CategoryHiddenAbsorption,
				Unit:      UnitSNR,
				Condition: ConditionIsGreaterThan,
				Value:     1.0,
				Action:    ActionEnter,
			},
			{
				Category:  CategoryAggressiveDrive,
				Unit:      UnitSNR,
				Condition: ConditionIsGreaterThan,
				Value:     1.0,
				Branches: []Branch{
					{
						Category:  CategoryVerticalIgnition,
						Unit:      UnitSNR,
						Condition: ConditionIsGreaterThan,
						Value:     1.0,
						Action:    ActionShort,
					},
				},
			},
		}}

		convey.Convey("When both branches are reachable from the measurements", func() {
			action := tree.Walk(
				[]Measurement{
					measurement(CategoryHiddenAbsorption, 2.0),
					measurement(CategoryAggressiveDrive, 1.4),
					measurement(CategoryVerticalIgnition, 1.6),
				},
				nil,
			)

			convey.Convey("The deeper branch's action wins over the shallow one", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionShort)
			})
		})

		convey.Convey("When only the shallow branch is reachable", func() {
			action := tree.Walk(
				[]Measurement{measurement(CategoryHiddenAbsorption, 2.0)},
				nil,
			)

			convey.Convey("Its action is returned", func() {
				convey.So(action, convey.ShouldNotBeNil)
				convey.So(*action, convey.ShouldEqual, ActionEnter)
			})
		})
	})
}
