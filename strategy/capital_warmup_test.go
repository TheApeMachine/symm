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
		model := learning.NewModel[string, CapitalAction](knowledgeMemory)
		history := CapitalHistory{knowledge: knowledge, model: model}
		at := time.Unix(100, 0)
		issued := hindsight.LearningEvent{Run: "past", ID: 1, PortfolioID: "allocation", Kind: "portfolio_issued", CapitalSymbol: "A/USD", Action: "enter", Authority: 1, Context: []uint64{1, 0, 1}, Quantities: [][2]string{{"source", "impulse"}}, Account: &EquityMark{At: at, Version: 1, Equity: 200, HasFunding: true}}
		resolved := hindsight.LearningEvent{Run: "past", ID: 1, PortfolioID: "allocation", Kind: "portfolio_resolved", TargetUnit: "return_per_second", Target: 0.1, Account: &EquityMark{At: at.Add(time.Second), Version: 2, Equity: 220, HasFunding: true}}
		count, err := history.Warmup([]hindsight.LearningEvent{issued, resolved})
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 1)
		reading := model.Recall("capital", []uint64{2, 0, 1}, CapitalAction{Symbol: "A/USD", Kind: types.ActionEnter})
		So(reading.Mean, ShouldEqual, 0.1)
		So(reading.Depth, ShouldEqual, 3)
		So(reading.Pending, ShouldEqual, 0)
		So(knowledge.Reading("A/USD", nil, LearningAction{Kind: types.ActionEnter}).Selected.Defined, ShouldBeFalse)
	})
}
