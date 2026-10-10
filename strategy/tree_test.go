package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestKeyTree(t *testing.T) {
	Convey("Given stored token paths", t, func() {
		export := keyTree([]string{
			"R01/R02/R03/enter.json",
			"R01/R02/R04/enter.json",
			"R01/R05/exit.json",
		})

		Convey("the root counts every path and branches at the first divergence", func() {
			So(export.Root, ShouldNotBeNil)
			So(export.Root.Count, ShouldEqual, 3)
			So(export.Root.Children, ShouldHaveLength, 1)

			r01 := export.Root.Children[0]
			So(r01.Tokens, ShouldResemble, []string{"R01"})
			So(r01.State, ShouldEqual, "ESTIMATED")
			So(r01.Children, ShouldHaveLength, 2)

			r02 := r01.Children[0]
			So(r02.TokenPrefix, ShouldEqual, "R01/R02")
			So(r02.Probability, ShouldAlmostEqual, 2.0/3.0)
			So(r02.State, ShouldEqual, "POLICY CHOICE")

			// R05 has one path: its chain folds into one evaluated leaf.
			r05 := r01.Children[1]
			So(r05.Tokens, ShouldResemble, []string{"R05", "exit"})
			So(r05.State, ShouldEqual, "EVALUATED")
		})

		Convey("feasible ranks each action by its share", func() {
			So(export.Feasible, ShouldHaveLength, 2)
			So(export.Feasible[0].Action, ShouldEqual, "enter")
			So(export.Feasible[0].Probability, ShouldAlmostEqual, 2.0/3.0)
		})
	})

	Convey("Given no stored paths, the export is empty lists", t, func() {
		export := keyTree(nil)
		So(export.Root, ShouldBeNil)
		So(export.Feasible, ShouldNotBeNil)
		So(export.Branches, ShouldNotBeNil)
	})
}
