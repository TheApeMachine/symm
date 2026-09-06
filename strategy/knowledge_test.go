package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

func TestKnowledgeReading(t *testing.T) {
	Convey("Given continuously trained global and symbol-specific paths", t, func() {
		knowledge := NewKnowledge(learning.NewGrid())
		knowledge.Model = learning.NewModel[[2]string, LearningAction](8)
		action := LearningAction{Kind: types.ActionEnter}
		context := []uint64{438, 516, 91}
		train := func(symbol string, outcome float64, count int) {
			for range count {
				ticket, err := knowledge.Issue(symbol, context, action, 1)
				So(err, ShouldBeNil)
				_, err = knowledge.Model.Resolve(ticket, outcome)
				So(err, ShouldBeNil)
			}
		}
		train("A/USD", 1, 30)
		Convey("Another instrument consumes the global path, including reordered and subset contexts", func() {
			for _, query := range [][]uint64{context, {516, 438, 91}, {438, 516}, {999}} {
				reading := knowledge.Reading("B/USD", query, action)
				So(reading.Scope, ShouldEqual, "global")
				So(reading.Selected.Mean, ShouldAlmostEqual, 1)
				So(reading.Selected.Samples, ShouldEqual, 30)
				expected := min(3, len(query))

				if query[0] == 999 {
					expected = 0
				}
				So(reading.Selected.Depth, ShouldEqual, expected)
			}
		})
		Convey("Sparse local contradiction yields to mature shared evidence", func() {
			train("B/USD", -1, 2)
			reading := knowledge.Reading("B/USD", context, action)
			So(reading.Scope, ShouldEqual, "global")
			So(reading.Symbol.Samples, ShouldEqual, 2)
		})
		Convey("Current supported local evidence specializes, then yields when dormant", func() {
			train("B/USD", -1, 20)
			reading := knowledge.Reading("B/USD", context, action)
			So(reading.Scope, ShouldEqual, "symbol")
			So(reading.Selected.Mean, ShouldAlmostEqual, -1)
			So(reading.Selected.Samples, ShouldEqual, 20)
			// Activity in B itself ages B's entry evidence; unrelated A cannot.
			for range 80 {
				So(knowledge.Model.Observe([2]string{"B/USD", "virtual"}, nil, LearningAction{Kind: types.ActionHold}, 0, 1), ShouldBeNil)
			}
			train("A/USD", 1, 80)
			reading = knowledge.Reading("B/USD", context, action)
			So(reading.Scope, ShouldEqual, "global")
			So(reading.Symbol.EvidenceAuthority, ShouldBeLessThan, reading.Global.EvidenceAuthority)
		})
		Convey("One experience trains both scopes but never doubles its selected sample count", func() {
			reading := knowledge.Reading("A/USD", context, action)
			So(reading.Global.Samples, ShouldEqual, 30)
			So(reading.Symbol.Samples, ShouldEqual, 30)
			So(reading.Selected.Samples, ShouldEqual, 30)
			ticket, err := knowledge.Issue("A/USD", context, action, 1)
			So(err, ShouldBeNil)
			So(knowledge.Reading("A/USD", context, action).Selected.Pending, ShouldEqual, 1)
			_, err = knowledge.Model.Resolve(ticket, 1)
			So(err, ShouldBeNil)
			So(knowledge.Reading("A/USD", context, action).Selected.Pending, ShouldEqual, 0)
		})
	})
}

func TestKnowledgeWarmup(t *testing.T) {
	Convey("Given complete experiences from separate producer runs and reordered quantity registration", t, func() {
		grid := learning.NewGrid()
		grid.Column("other", "quantity")
		knowledge := NewKnowledge(grid)
		at := time.Unix(10, 0)
		events := []hindsight.LearningEvent{
			{Run: "one", ID: 1, Symbol: "A/USD", Kind: "issued", At: at, Context: []uint64{1, 0, 2}, Quantities: [][2]string{{"source", "impulse"}}, Action: "enter", Authority: 1},
			{Run: "two", ID: 1, Symbol: "B/USD", Kind: "issued", At: at, Action: "hold", Authority: 1},
			{Run: "one", ID: 1, Symbol: "A/USD", Kind: "resolved", At: at.Add(time.Second), Target: 0.1, TargetUnit: "absolute_return_per_second"},
			{Run: "two", ID: 1, Symbol: "B/USD", Kind: "resolved", At: at.Add(time.Second), Target: 0.2, TargetUnit: "absolute_return_per_second"},
			{Run: "one", ID: 2, Symbol: "A/USD", Kind: "issued", At: at, Action: "enter", Authority: 1},
		}
		report, err := knowledge.Warmup(events)
		So(err, ShouldBeNil)
		So(report.Resolved, ShouldEqual, 2)
		reading := knowledge.Reading("C/USD", []uint64{2, 0, 2}, LearningAction{Kind: types.ActionEnter})
		So(reading.Selected.Mean, ShouldAlmostEqual, 0.1)
		So(reading.Selected.Depth, ShouldEqual, 3)
		So(reading.Selected.Pending, ShouldEqual, 0)
		So(knowledge.Reading("A/USD", nil, LearningAction{Kind: types.ActionEnter}).Symbol.Samples, ShouldEqual, 1)
	})
}

func TestKnowledgeWarmupAbsoluteTargets(t *testing.T) {
	Convey("Historical differential returns require a separately recorded absolute mark", t, func() {
		at := time.Unix(100, 0)
		zero := 0.0
		events := []hindsight.LearningEvent{
			{Run: "past", ID: 1, Symbol: "TEST/USD", Kind: "issued", At: at, Action: "hold", Authority: 1},
			{Run: "past", ID: 1, Symbol: "TEST/USD", Kind: "resolved", At: at.Add(time.Second), TargetUnit: "return_per_second", Target: 0.03, AbsoluteSkillTarget: &zero, BaselineRate: -6},
			{Run: "past", ID: 2, Symbol: "TEST/USD", Kind: "issued", At: at, Action: "enter", Authority: 1},
			{Run: "past", ID: 2, Symbol: "TEST/USD", Kind: "resolved", At: at.Add(time.Second), Target: 0.2},
		}
		knowledge := NewKnowledge(learning.NewGrid())
		report, err := knowledge.Warmup(events)
		So(err, ShouldBeNil)
		So(report.Resolved, ShouldEqual, 1)
		So(report.TargetUnavailable, ShouldEqual, 1)
		So(knowledge.Reading("TEST/USD", nil, LearningAction{Kind: types.ActionHold}).Selected.Mean, ShouldEqual, 0)
		So(knowledge.Reading("TEST/USD", nil, LearningAction{Kind: types.ActionEnter}).Selected.Defined, ShouldBeFalse)
	})
}

func BenchmarkKnowledgeSelect(b *testing.B) {
	knowledge := NewKnowledge(learning.NewGrid())
	actions := []LearningAction{{Kind: types.ActionHold}, {Kind: types.ActionEnter}}
	for _, action := range actions {
		for range 16 {
			if err := knowledge.Model.Observe([2]string{"A/USD", "virtual"}, []uint64{1, 2, 3}, action, 0.01, 1, [2]string{"", "virtual"}); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := knowledge.Select("B/USD", []uint64{2, 1, 3}, actions, false); err != nil {
			b.Fatal(err)
		}
	}
}
