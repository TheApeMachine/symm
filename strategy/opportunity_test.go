package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestHighVelocityOpportunity(t *testing.T) {
	Convey("Given a high-velocity category", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Categories.Store("BTC/USD", []types.Category{
			{Type: types.CategoryVerticalIgnition},
		})

		Convey("It should identify the opportunity without disqualifying it", func() {
			opportunity, disqualified := highVelocityOpportunity(thesis, "BTC/USD")

			So(opportunity, ShouldBeTrue)
			So(disqualified, ShouldBeFalse)
		})
	})

	Convey("Given an adverse category mixed with opportunity", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Categories.Store("BTC/USD", []types.Category{
			{Type: types.CategoryVerticalIgnition},
			{Type: types.CategorySpoofTrap},
		})

		Convey("It should let the adverse evidence veto the opportunity", func() {
			opportunity, disqualified := highVelocityOpportunity(thesis, "BTC/USD")

			So(opportunity, ShouldBeFalse)
			So(disqualified, ShouldBeTrue)
		})
	})

	Convey("Given absorption mixed with apparent ignition", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Categories.Store("BTC/USD", []types.Category{
			{Type: types.CategoryVerticalIgnition},
			{Type: types.CategoryHiddenAbsorption},
		})

		Convey("It should reject flow that price is absorbing", func() {
			opportunity, disqualified := highVelocityOpportunity(thesis, "BTC/USD")

			So(opportunity, ShouldBeFalse)
			So(disqualified, ShouldBeTrue)
		})
	})

	Convey("Given model-classified absorption before categories mature", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Measurements.Store(types.SourceCVD, []*types.Measurement{{
			Source: types.SourceCVD,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricAbsorption, types.SideNone): {Raw: 0.8},
				types.MetricKey(types.MetricDrive, types.SideNone):      {Raw: 0.2},
			},
		}})

		Convey("It should veto the apparent opportunity", func() {
			opportunity, disqualified := highVelocityOpportunity(thesis, "BTC/USD")

			So(opportunity, ShouldBeFalse)
			So(disqualified, ShouldBeTrue)
		})
	})

	Convey("Given a model-classified thin book before categories mature", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Measurements.Store(types.SourceDepthFlow, []*types.Measurement{{
			Source: types.SourceDepthFlow,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricLoadedScore, types.SideNone): {Raw: 0.1},
				types.MetricKey(types.MetricSpoofScore, types.SideNone):  {Raw: 0.2},
				types.MetricKey(types.MetricThinScore, types.SideNone):   {Raw: 0.9},
				types.MetricKey(types.MetricNeutralScore, types.SideNone): {
					Raw: 0.3,
				},
			},
		}})

		Convey("It should veto the apparent opportunity", func() {
			opportunity, disqualified := highVelocityOpportunity(thesis, "BTC/USD")

			So(opportunity, ShouldBeFalse)
			So(disqualified, ShouldBeTrue)
		})
	})

	Convey("Given no category evidence", t, func() {
		Convey("It should report neither opportunity nor disqualification", func() {
			opportunity, disqualified := highVelocityOpportunity(nil, "BTC/USD")

			So(opportunity, ShouldBeFalse)
			So(disqualified, ShouldBeFalse)
		})
	})
}
