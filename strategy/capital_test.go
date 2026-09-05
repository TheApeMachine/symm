package strategy

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/types"
)

/* capitalDesk records submissions without contributing any fabricated market outcomes. */
type capitalDesk struct {
	intents []ExecutionIntent
	state   AccountState
}

func (desk *capitalDesk) Submit(intent ExecutionIntent) error {
	desk.intents = append(desk.intents, intent)
	return nil
}
func (desk *capitalDesk) Account() AccountState { return desk.state }

func TestCapitalLearnerAllocate(t *testing.T) {
	Convey("Given multiple local candidates competing for one finite account", t, func() {
		events := []hindsight.LearningEvent{}
		agent, _ := agentFixture(t, func(event hindsight.LearningEvent) error { events = append(events, event); return nil })
		at := time.Unix(100, 0)
		agent.now = func() time.Time { return at }
		agent.Skill.mode = ModeTrading
		agent.allowed.Store(true)
		desk := &capitalDesk{state: AccountState{Cash: "150", Complete: true, Positions: map[string]string{}, Mark: EquityMark{At: at, Version: 1, Equity: 150, HasFunding: true}}}
		agent.Desk = desk
		capital := agent.Capital
		So(capital.Actual.Observe(desk.state), ShouldBeNil)
		first := candidateFixture("A/USD", at)
		second := candidateFixture("B/USD", at)
		So(capital.Candidates.Publish(first), ShouldBeNil)
		So(capital.Candidates.Publish(second), ShouldBeNil)
		candidates := []*EntryCandidate{first, second}
		train := func(action CapitalAction, target float64) {
			for range 20 {
				So(capital.Model.Observe("capital", nil, action, target, 1), ShouldBeNil)
			}
		}
		train(CapitalAction{Symbol: "A/USD", Kind: types.ActionEnter}, 0.01)
		train(CapitalAction{Symbol: "B/USD", Kind: types.ActionEnter}, 0.1)
		Convey("Learned advantage beats arrival order and does not train local selected-position evidence", func() {
			So(capital.allocate(agent.LocalLearning, capital.Actual, candidates, time.Second, false), ShouldBeNil)
			So(desk.intents, ShouldHaveLength, 1)
			So(desk.intents[0].Symbol, ShouldEqual, "B/USD")
			So(first.State, ShouldEqual, "lost learned competition")
			So(capital.LastChoice.Symbol, ShouldEqual, "B/USD")
			So(agent.Knowledge.Reading("B/USD", nil, LearningAction{Kind: types.ActionEnter}).Global.Defined, ShouldBeFalse)
			So(capital.Actual.pending.state.Cash, ShouldEqual, "150")
			So(agent.Realization.AllowsTrading(), ShouldBeTrue)
		})

		Convey("Local and actual inventory select separate broker mechanics for the same buy claim", func() {
			capital.Actual.State.Positions["B/USD"] = "1"
			So(capital.allocate(agent.LocalLearning, capital.Actual, candidates, time.Second, false), ShouldBeNil)
			So(desk.intents, ShouldHaveLength, 1)
			So(desk.intents[0].Kind, ShouldEqual, types.ActionScale)
			So(second.Record.Action, ShouldEqual, "enter")
		})
		Convey("A local scale claim remains fundable when the actual account missed the virtual entry", func() {
			second.action.Kind = types.ActionScale
			second.Record.Action = "scale"
			train(CapitalAction{Symbol: "B/USD", Kind: types.ActionScale}, 0.2)
			So(capital.allocate(agent.LocalLearning, capital.Actual, candidates, time.Second, false), ShouldBeNil)
			So(desk.intents, ShouldHaveLength, 1)
			So(desk.intents[0].Kind, ShouldEqual, types.ActionEnter)
			So(second.Record.Action, ShouldEqual, "scale")
		})
		Convey("Learned WAIT can win even with enough cash for an entry", func() {
			train(CapitalAction{Kind: types.ActionHold}, 1)
			So(capital.allocate(agent.LocalLearning, capital.Actual, candidates, time.Second, false), ShouldBeNil)
			So(desk.intents, ShouldBeEmpty)
			So(capital.LastChoice.Kind, ShouldEqual, types.ActionHold)
			So(first.State, ShouldEqual, "wait chosen")
		})
		Convey("Insufficient capital refuses upstream and leaves local evidence and realization untouched", func() {
			capital.Actual.State.Cash = "1"
			So(capital.allocate(agent.LocalLearning, capital.Actual, candidates, time.Second, false), ShouldBeNil)
			So(desk.intents, ShouldBeEmpty)
			So(first.State, ShouldEqual, "insufficient capital")
			So(agent.Realization.AllowsTrading(), ShouldBeTrue)
			So(agent.Knowledge.Reading("A/USD", nil, LearningAction{Kind: types.ActionEnter}).Selected.Defined, ShouldBeFalse)
		})
	})
}

func BenchmarkCapitalLearnerAllocate(b *testing.B) {
	agent, _ := agentFixture(b, func(hindsight.LearningEvent) error { return nil })
	at := time.Unix(100, 0)
	agent.now = func() time.Time { return at }
	agent.Skill.mode = ModeTrading
	agent.allowed.Store(true)
	desk := &capitalDesk{}
	agent.Desk = desk
	capital := agent.Capital
	state := AccountState{Cash: "150", Complete: true, Positions: map[string]string{}, Mark: EquityMark{At: at, Version: 1, Equity: 150, HasFunding: true}}

	if err := capital.Actual.Observe(state); err != nil {
		b.Fatal(err)
	}
	// A burst of 64 viable claims tests competition, without scanning inactive universe rows.
	candidates := make([]*EntryCandidate, 64)
	for index := range candidates {
		candidates[index] = candidateFixture(fmt.Sprintf("ASSET%03d/USD", index), at)
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := capital.allocate(agent.LocalLearning, capital.Actual, candidates, time.Second, false); err != nil {
			b.Fatal(err)
		}
		at = at.Add(time.Second)
		state.Mark.At, state.Mark.Version = at, state.Mark.Version+1

		if err := capital.Actual.Observe(state); err != nil {
			b.Fatal(err)
		}
		desk.intents = desk.intents[:0]

		if err := agent.flush(); err != nil {
			b.Fatal(err)
		}
	}
}
