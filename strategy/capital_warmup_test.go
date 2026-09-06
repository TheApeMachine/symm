package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

func TestCapitalHistoryWarmup(t *testing.T) {
	Convey("Given completed prospective capital decisions from earlier producer runs", t, func() {
		knowledge := NewKnowledge(learning.NewGrid())
		knowledge.grid.Column("unrelated", "first")
		model := NewCapitalKnowledge()
		history := CapitalHistory{knowledge: knowledge, capital: model}
		at := time.Unix(100, 0)
		issued := hindsight.LearningEvent{Run: "past", At: at, Mode: "capital_virtual", ID: 1, PortfolioID: "allocation", Kind: "portfolio_issued", CapitalSymbol: "A/USD", Action: "enter", Authority: 1, Context: []uint64{1, 0, 1}, Quantities: [][2]string{{"source", "impulse"}}, Account: &EquityMark{At: at, Version: 1, Equity: 200, HasFunding: true}}
		resolved := hindsight.LearningEvent{Run: "past", At: at.Add(time.Second), Mode: "capital_virtual", ID: 1, PortfolioID: "allocation", Kind: "portfolio_resolved", TargetUnit: "return_per_second", Target: 0.1, Allocation: &hindsight.AllocationResult{State: "filled", At: at}, Account: &EquityMark{At: at.Add(time.Second), Version: 2, Equity: 220, HasFunding: true}}
		count, err := history.Warmup([]hindsight.LearningEvent{issued, resolved})
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 1)
		reading := model.scope("capital_virtual", []uint64{2, 0, 1}, CapitalAction{Symbol: "A/USD", Kind: types.ActionEnter}).Selected
		So(reading.Mean, ShouldEqual, 0.1)
		So(reading.Depth, ShouldEqual, 3)
		So(reading.Pending, ShouldEqual, 0)
		So(knowledge.Reading("A/USD", nil, LearningAction{Kind: types.ActionEnter}).Selected.Defined, ShouldBeFalse)
		Convey("Legacy buy labels without execution proof remain unverified", func() {
			issued.Mode, resolved.Mode, resolved.Allocation = "capital_account", "capital_account", nil
			count, err := history.Warmup([]hindsight.LearningEvent{issued, resolved})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)
			So(history.Unverified, ShouldEqual, 1)
			So(model.Reading(nil, CapitalAction{Kind: types.ActionEnter}).Actual.Global.Defined, ShouldBeFalse)
		})
		Convey("WAIT requires no fabricated execution receipt", func() {
			issued.Action, issued.CapitalSymbol, resolved.Allocation = "hold", "", nil
			count, err := history.Warmup([]hindsight.LearningEvent{issued, resolved})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)
			So(history.Unverified, ShouldEqual, 0)
		})
		Convey("Matching actual and virtual outcomes retain separate support", func() {
			issued.Mode, resolved.Mode = "capital_account", "capital_account"
			count, err := history.Warmup([]hindsight.LearningEvent{issued, resolved})
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)
			reading := model.Reading([]uint64{2, 0, 1}, CapitalAction{Symbol: "NEW/USD", Kind: types.ActionEnter})
			So(reading.Actual.Global.Samples, ShouldEqual, 1)
			So(reading.Virtual.Global.Samples, ShouldEqual, 1)
			So(reading.Selected.Samples, ShouldEqual, 1)
		})
		Convey("An aborted or orphaned ticket cannot be restored by a later label", func() {
			aborted := issued
			aborted.Kind = "portfolio_aborted"
			_, err := history.Warmup([]hindsight.LearningEvent{issued, aborted, resolved})
			So(err, ShouldNotBeNil)
		})
		Convey("A mismatched source cannot contaminate another teacher", func() {
			resolved.Mode = "capital_account"
			_, err := history.Warmup([]hindsight.LearningEvent{issued, resolved})
			So(err, ShouldNotBeNil)
		})
		Convey("Malformed causal identity is not disguised as an unverified legacy label", func() {
			issued.PortfolioID, resolved.Allocation = "", nil
			_, err := history.Warmup([]hindsight.LearningEvent{issued, resolved})
			So(err, ShouldNotBeNil)
			So(history.Unverified, ShouldEqual, 0)
		})
		Convey("A fill after the labelled account mark cannot explain that outcome", func() {
			resolved.Allocation.At = resolved.At.Add(time.Second)
			_, err := history.Warmup([]hindsight.LearningEvent{issued, resolved})
			So(err, ShouldNotBeNil)
		})
	})
}
