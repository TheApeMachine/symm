package logic

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestProjectCategoriesFromCognition(t *testing.T) {
	Convey("Given ready cognition winners on a thesis", t, func() {
		analyzer := &Analyzer{}
		thesis := types.NewThesis(nil, nil)
		analyzer.projectCategories(thesis, []types.Cognition{
			{
				Symbol:         "PENGU/USD",
				At:             time.Unix(1, 0).UTC(),
				Winner:         "buy",
				Ready:          true,
				Confidence:     0.72,
				EntropyBits:    1.25,
				LookaheadScore: 0.4,
				Cohort:         3,
			},
			{
				Symbol: "SKIP/USD",
				Winner: "buy",
				Ready:  false,
			},
		})

		Convey("Then only ready winners become category rows", func() {
			So(len(thesis.Categories), ShouldEqual, 1)
			So(thesis.Categories[0].Symbol, ShouldEqual, "PENGU/USD")
			So(string(thesis.Categories[0].Type), ShouldEqual, "buy")
			So(thesis.Categories[0].Confidence, ShouldEqual, 0.72)
			So(thesis.Categories[0].Strength, ShouldEqual, 0.4)
		})
	})
}
