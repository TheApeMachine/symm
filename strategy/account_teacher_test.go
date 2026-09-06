package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/types"
)

func TestAccountTeacherObserve(t *testing.T) {
	Convey("Given a prospectively issued finite-capital decision", t, func() {
		at := time.Unix(100, 0)
		model := NewCapitalKnowledge()
		events := []hindsight.LearningEvent{}
		teacher := NewAccountTeacher(model, "capital_account", func(event hindsight.LearningEvent) error { events = append(events, event); return nil })
		state := AccountState{Mark: EquityMark{At: at, Version: 1, Equity: 200, HasFunding: true}, Cash: "200", Positions: map[string]string{}, Complete: true}
		So(teacher.Observe(state), ShouldBeNil)
		action := CapitalAction{Symbol: "A/USD", Kind: types.ActionEnter}
		identity, err := teacher.Issue(action, []uint64{1}, "candidate", time.Second, 1, at, nil, [][2]string{{"source", "impulse"}}, "test measured interval")
		So(err, ShouldBeNil)
		Convey("An allocation awaiting execution cannot learn from unrelated wallet movement", func() {
			state.Mark.At = at.Add(time.Second)
			state.Mark.Version++
			state.Mark.Equity = 202
			So(teacher.Observe(state), ShouldBeNil)
			So(teacher.Resolved, ShouldEqual, 0)
			So(model.scope("capital_account", nil, action).Selected.Samples, ShouldEqual, 0)
		})
		Convey("Continuous marked equity resolves before any position closes and preserves frozen inputs", func() {
			teacher.pending.receipt.Report(hindsight.AllocationResult{State: "filled", At: at})
			state.Positions["A/USD"] = "2"
			state.Mark.At = at.Add(time.Second / 2)
			state.Mark.Version++
			state.Mark.Equity = 201
			So(teacher.Observe(state), ShouldBeNil)
			So(teacher.Resolved, ShouldEqual, 0)
			So(teacher.pending.state.Mark.Equity, ShouldEqual, 200)
			So(teacher.pending.state.Positions, ShouldBeEmpty)
			state.Mark.At = at.Add(time.Second)
			state.Mark.Version++
			state.Mark.Equity = 202
			So(teacher.Observe(state), ShouldBeNil)
			So(teacher.Resolved, ShouldEqual, 1)
			So(teacher.Target, ShouldAlmostEqual, 0.01)
			So(teacher.MFE, ShouldEqual, 2)
			So(teacher.TimeToPositive, ShouldEqual, time.Second/2)
			So(events[len(events)-1].PortfolioID, ShouldEqual, identity)
			So(events[len(events)-1].CandidateID, ShouldEqual, "candidate")
			So(teacher.Trajectory, ShouldHaveLength, 3)
		})
		Convey("Funding is removed from the return and WAIT is an ordinary learned action", func() {
			teacher.pending.receipt.Report(hindsight.AllocationResult{State: "filled", At: at})
			state.Mark.At = at.Add(time.Second)
			state.Mark.Version++
			state.Mark.Equity = 250
			state.Mark.NetFunding = 50
			So(teacher.Observe(state), ShouldBeNil)
			So(teacher.Target, ShouldEqual, 0)
			wait := CapitalAction{Kind: types.ActionHold}
			_, err := teacher.Issue(wait, nil, "", time.Second, 1, state.Mark.At, nil, nil, "test measured interval")
			So(err, ShouldBeNil)
			state.Mark.At = state.Mark.At.Add(time.Second)
			state.Mark.Version++
			So(teacher.Observe(state), ShouldBeNil)
			So(model.scope("capital_account", nil, wait).Selected.Defined, ShouldBeTrue)
			So(model.scope("capital_account", nil, wait).Selected.Mean, ShouldEqual, 0)
		})
		Convey("Future marks do not rewrite prospective journal facts", func() {
			teacher.pending.receipt.Report(hindsight.AllocationResult{State: "filled", At: at})
			state.Mark.At = at.Add(20 * time.Second)
			state.Mark.Version++
			state.Mark.Equity = 202
			So(teacher.Observe(state), ShouldBeNil)
			So(teacher.Target, ShouldAlmostEqual, 0.0005)
			So(events[1].Kind, ShouldEqual, "portfolio_issued")
			So(events[1].Account.Equity, ShouldEqual, 200)
			So(events[1].Account.Version, ShouldEqual, 1)
			So(events[1].AccountPositions, ShouldBeEmpty)
		})
	})
}

func TestAccountTeacherReconcile(t *testing.T) {
	Convey("Pre-execution refusal aborts every evidence path, including after the horizon passed", t, func() {
		knowledge := NewCapitalKnowledge()
		events := []hindsight.LearningEvent{}
		teacher := NewAccountTeacher(knowledge, "capital_account", func(event hindsight.LearningEvent) error { events = append(events, event); return nil })
		at := time.Unix(100, 0)
		state := AccountState{Mark: EquityMark{At: at, Version: 1, Equity: 200, HasFunding: true}, Cash: "200", Complete: true}
		So(teacher.Observe(state), ShouldBeNil)
		action := CapitalAction{Symbol: "A/USD", Kind: types.ActionEnter}
		identity, err := teacher.Issue(action, nil, "candidate", time.Second, 1, at, nil, nil, "measured candidate horizon")
		So(err, ShouldBeNil)
		receipt := teacher.pending.receipt
		receipt.Report(hindsight.AllocationResult{State: "submitted", At: at})
		state.Mark.At, state.Mark.Version, state.Mark.Equity = at.Add(2*time.Second), 2, 202
		So(teacher.Observe(state), ShouldBeNil)
		So(teacher.Resolved, ShouldEqual, 0)
		receipt.Report(hindsight.AllocationResult{State: "aborted", At: state.Mark.At, Detail: "repricing failed"})
		// An unchanged account mark must still deliver a newly arrived refusal.
		So(teacher.Observe(state), ShouldBeNil)
		So(teacher.pending, ShouldBeNil)
		So(teacher.Aborted, ShouldEqual, 1)
		So(teacher.Resolved, ShouldEqual, 0)
		reading := knowledge.Reading(nil, action)
		So(reading.Actual.Global.Samples, ShouldEqual, 0)
		So(reading.Actual.Symbol.Pending, ShouldEqual, 0)
		So(reading.Virtual.Global.Samples, ShouldEqual, 0)
		So(events[len(events)-1].Kind, ShouldEqual, "portfolio_aborted")
		So(events[len(events)-1].PortfolioID, ShouldEqual, identity)
		So(teacher.Observe(state), ShouldBeNil)
		So(teacher.Aborted, ShouldEqual, 1)
	})
}

func TestAccountTeacherIssue(t *testing.T) {
	Convey("Given equal wallet gains over materially different observed durations", t, func() {
		model := NewCapitalKnowledge()
		for _, choice := range []struct {
			symbol   string
			duration time.Duration
		}{{"fast", time.Second / 2}, {"slow", 20 * time.Second}} {
			teacher := NewAccountTeacher(model, "capital_virtual", func(hindsight.LearningEvent) error { return nil })
			at := time.Unix(100, 0)
			state := AccountState{Mark: EquityMark{At: at, Version: 1, Equity: 200, HasFunding: true}, Cash: "200", Complete: true}
			action := CapitalAction{Symbol: choice.symbol, Kind: types.ActionEnter}
			for range 3 {
				So(teacher.Observe(state), ShouldBeNil)
				_, err := teacher.Issue(action, nil, "", choice.duration, 1, state.Mark.At, nil, nil, "test measured interval")
				So(err, ShouldBeNil)
				teacher.pending.receipt.Report(hindsight.AllocationResult{State: "filled", At: state.Mark.At})
				state.Mark.At = state.Mark.At.Add(choice.duration)
				state.Mark.Version++
				state.Mark.Equity += 2
				So(teacher.Observe(state), ShouldBeNil)
			}
		}
		// Compare each observed duration's own evidence, before sparse symbols
		// back off to the same transferable allocation prior.
		fast := model.scope("capital_virtual", nil, CapitalAction{Symbol: "fast", Kind: types.ActionEnter}).Symbol
		slow := model.scope("capital_virtual", nil, CapitalAction{Symbol: "slow", Kind: types.ActionEnter}).Symbol
		So(fast.Mean, ShouldBeGreaterThan, slow.Mean)
	})
}

func BenchmarkAccountTeacherObserve(b *testing.B) {
	teacher := NewAccountTeacher(NewCapitalKnowledge(), "capital_virtual", func(hindsight.LearningEvent) error { return nil })
	state := AccountState{Mark: EquityMark{At: time.Unix(100, 0), Version: 1, Equity: 200, HasFunding: true}, Cash: "200", Complete: true}
	b.ReportAllocs()
	for b.Loop() {
		state.Mark.Version++
		state.Mark.At = state.Mark.At.Add(time.Millisecond)

		if err := teacher.Observe(state); err != nil {
			b.Fatal(err)
		}
	}
}
