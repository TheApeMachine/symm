package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// meas is a small helper for building a category measurement at a given
// confidence on the current window. Strength/volume/spread are irrelevant to
// the category/confidence gates the entry playbooks use.
func meas(source SourceType, category CategoryType, confidence float64) Measurement {
	return NewMeasurement(
		source,
		"BTC/USD",
		100,
		1,
		1,
		1,
		1,
		category,
		RegimeTypeTrending,
		PositionTypeNone,
		confidence,
		1,
	)
}

// TestEmbeddedTreeDriveEntryIsReachable guards against the regression that made
// the old playbooks unenterable: entries were deep temporal AND-chains, so the
// confirmations could never all land on one window. The redesigned "drive"
// entry is a flat AND-group and MUST fire when its confirmations co-occur on a
// single measurement window with a healthy, non-toxic book.
func TestEmbeddedTreeDriveEntryIsReachable(t *testing.T) {
	Convey("Given the embedded playbook tree", t, func() {
		tree, err := LoadTree()
		So(err, ShouldBeNil)
		So(tree, ShouldNotBeNil)

		holdings := NewHoldings()
		permissive := ThresholdContext{EntryConfidenceBaseline: 0.40}

		Convey("A flat-CVD-drive window with backing should enter", func() {
			window := []Measurement{
				meas(SourceCVD, CategoryAggressiveDrive, 0.7),
				meas(SourceDepthFlow, CategoryLoadedImbalance, 0.7),
				meas(SourceFluid, CategoryLaminar, 0.7),
				meas(SourceExhaustion, CategoryOrganic, 0.7),
				meas(SourceToxicity, CategoryHardSupport, 0.7),
				meas(SourceLiquidity, CategoryRobustLiquidity, 0.7),
			}

			evaluation, _, evalErr := tree.EvaluateContinuing(window, holdings, nil, nil, &permissive)
			So(evalErr, ShouldBeNil)
			So(evaluation, ShouldNotBeNil)
			So(evaluation.Action, ShouldNotBeNil)
			So(evaluation.Action.Type, ShouldEqual, ActionMarket)
		})

		Convey("A hot macro market raises the bar and vetoes a marginal entry", func() {
			window := []Measurement{
				meas(SourceCVD, CategoryAggressiveDrive, 0.70),
				meas(SourceDepthFlow, CategoryLoadedImbalance, 0.70),
				meas(SourceFluid, CategoryLaminar, 0.70),
				meas(SourceExhaustion, CategoryOrganic, 0.70),
				meas(SourceToxicity, CategoryHardSupport, 0.70),
				meas(SourceLiquidity, CategoryRobustLiquidity, 0.70),
			}

			// cold/hot bracket the adaptive entry bar: cold (0.40) must allow the
			// 0.70-confidence drive path; hot (0.90) must veto the same window.
			cold := ThresholdContext{EntryConfidenceBaseline: 0.40}
			hot := ThresholdContext{EntryConfidenceBaseline: 0.90}

			allowed, _, coldErr := tree.EvaluateContinuing(window, holdings, nil, nil, &cold)
			So(coldErr, ShouldBeNil)
			So(allowed, ShouldNotBeNil)
			So(allowed.Action.Type, ShouldEqual, ActionMarket)

			vetoed, _, hotErr := tree.EvaluateContinuing(window, holdings, nil, nil, &hot)
			So(hotErr, ShouldBeNil)
			So(vetoed, ShouldBeNil)
		})

		Convey("A sequential ignition setup fires only after compression THEN ignition", func() {
			// Timeline is oldest->newest. Compression must precede the ignition
			// confirmation for the nested (after-parent) child to match.
			sequenced := []Measurement{
				meas(SourcePumpDump, CategoryCoiledCompression, 0.7),
				meas(SourcePumpDump, CategoryVerticalIgnition, 0.7),
				meas(SourceHawkes, CategoryFrenzy, 0.7),
				meas(SourceToxicity, CategoryHardSupport, 0.7),
				meas(SourceDepthFlow, CategoryLoadedImbalance, 0.7),
				meas(SourceLiquidity, CategoryRobustLiquidity, 0.7),
			}

			evaluation, _, evalErr := tree.EvaluateContinuing(sequenced, holdings, nil, nil, &permissive)
			So(evalErr, ShouldBeNil)
			So(evaluation, ShouldNotBeNil)
			So(evaluation.Action.Type, ShouldEqual, ActionMarket)

			// Compression alone (no later ignition) must NOT enter — it parks
			// waiting for the confirmation tick.
			triggerOnly := []Measurement{
				meas(SourcePumpDump, CategoryCoiledCompression, 0.7),
				meas(SourceLiquidity, CategoryRobustLiquidity, 0.7),
			}

			pending, _, pendingErr := tree.EvaluateContinuing(triggerOnly, holdings, nil, nil, nil)
			So(pendingErr, ShouldBeNil)
			So(pending, ShouldBeNil)
		})

		Convey("The same drive window on an extreme-scarcity book must NOT enter", func() {
			window := []Measurement{
				meas(SourceCVD, CategoryAggressiveDrive, 0.7),
				meas(SourceDepthFlow, CategoryLoadedImbalance, 0.7),
				meas(SourceFluid, CategoryLaminar, 0.7),
				meas(SourceExhaustion, CategoryOrganic, 0.7),
				meas(SourceToxicity, CategoryHardSupport, 0.7),
				meas(SourceLiquidity, CategoryExtremeScarcity, 0.7),
			}

			evaluation, _, evalErr := tree.EvaluateContinuing(window, holdings, nil, nil, nil)
			So(evalErr, ShouldBeNil)
			So(evaluation, ShouldBeNil)
		})
	})
}
