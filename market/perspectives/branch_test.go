package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestBranchListClone(t *testing.T) {
	convey.Convey("Given nested branches", t, func() {
		list := BranchList{
			{
				Category: CategoryLaminar,
				Branches: BranchList{
					{Category: CategoryAggressiveDrive},
				},
			},
		}

		clone := list.Clone()

		convey.Convey("It should deep-copy nested branches", func() {
			convey.So(len(clone), convey.ShouldEqual, 1)
			convey.So(len(clone[0].Branches), convey.ShouldEqual, 1)
			convey.So(clone[0].Branches[0].Category, convey.ShouldEqual, CategoryAggressiveDrive)

			clone[0].Branches[0].Category = CategoryFrenzy
			convey.So(list[0].Branches[0].Category, convey.ShouldEqual, CategoryAggressiveDrive)
		})
	})
}
