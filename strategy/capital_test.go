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
		first.Record.Context = []uint64{uint64(agent.Grid.Column("capital-fixture", "weak-structure") + 1)}
		second.Record.Context = []uint64{uint64(agent.Grid.Column("capital-fixture", "strong-structure") + 1)}
		So(capital.Candidates.Publish(first), ShouldBeNil)
		So(capital.Candidates.Publish(second), ShouldBeNil)
		candidates := []*EntryCandidate{first, second}
		train := func(action CapitalAction, target float64) {
			context := first.Record.Context
			if action.Symbol == second.Record.Symbol {
				context = second.Record.Context
			}
			if action.Kind == types.ActionHold {
				context = nil
			}
			for range 20 {
				So(capital.Knowledge.Observe("capital_account", context, action, target, 1), ShouldBeNil)
			}
		}
		train(CapitalAction{Symbol: "A/USD", Kind: types.ActionEnter}, 0.01)
		train(CapitalAction{Symbol: "B/USD", Kind: types.ActionEnter}, 0.1)
		Convey("Learned advantage beats arrival order and does not train local selected-position evidence", func() {
			So(capital.allocate(agent.LocalLearning, capital.Actual, candidates, false), ShouldBeNil)
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
			So(capital.allocate(agent.LocalLearning, capital.Actual, candidates, false), ShouldBeNil)
			So(desk.intents, ShouldHaveLength, 1)
			So(desk.intents[0].Kind, ShouldEqual, types.ActionScale)
			So(second.Record.Action, ShouldEqual, "enter")
		})
		Convey("A local scale claim remains fundable when the actual account missed the virtual entry", func() {
			second.action.Kind = types.ActionScale
			second.Record.Action = "scale"
			train(CapitalAction{Symbol: "B/USD", Kind: types.ActionScale}, 0.2)
			So(capital.allocate(agent.LocalLearning, capital.Actual, candidates, false), ShouldBeNil)
			So(desk.intents, ShouldHaveLength, 1)
			So(desk.intents[0].Kind, ShouldEqual, types.ActionEnter)
			So(second.Record.Action, ShouldEqual, "scale")
		})
		Convey("Learned WAIT can win even with enough cash for an entry", func() {
			train(CapitalAction{Kind: types.ActionHold}, 1)
			So(capital.allocate(agent.LocalLearning, capital.Actual, candidates, false), ShouldBeNil)
			So(desk.intents, ShouldBeEmpty)
			So(capital.LastChoice.Kind, ShouldEqual, types.ActionHold)
			So(first.State, ShouldEqual, "wait chosen")
		})
		Convey("Insufficient capital refuses upstream and leaves local evidence and realization untouched", func() {
			capital.Actual.State.Cash = "1"
			So(capital.allocate(agent.LocalLearning, capital.Actual, candidates, false), ShouldBeNil)
			So(desk.intents, ShouldBeEmpty)
			So(first.State, ShouldEqual, "insufficient capital")
			So(agent.Realization.AllowsTrading(), ShouldBeTrue)
			So(agent.Knowledge.Reading("A/USD", nil, LearningAction{Kind: types.ActionEnter}).Selected.Defined, ShouldBeFalse)
		})
	})
}

func TestCapitalLearnerHorizon(t *testing.T) {
	Convey("WAIT evaluates the capital set rather than the event-triggering symbol", t, func() {
		agent, _ := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
		at := time.Unix(100, 0)
		first, second := candidateFixture("A/USD", at), candidateFixture("B/USD", at)
		first.Record.Horizon, second.Record.Horizon = 10*time.Second, 30*time.Second
		So(agent.Capital.Candidates.Publish(first), ShouldBeNil)
		So(agent.Capital.Candidates.Publish(second), ShouldBeNil)
		horizon, source := agent.Capital.horizon(agent.LocalLearning, agent.Capital.Actual, []*EntryCandidate{second, first}, at.Add(2*time.Second))
		So(horizon, ShouldEqual, 8*time.Second)
		So(source, ShouldEqual, "earliest viable candidate expiry")
		reordered, _ := agent.Capital.horizon(agent.LocalLearning, agent.Capital.Actual, []*EntryCandidate{first, second}, at.Add(2*time.Second))
		So(reordered, ShouldEqual, horizon)

		Convey("An unrelated fast market does not shorten WAIT", func() {
			agent.markets["UNRELATED/USD"] = &learningMarket{epochs: 1, epochMean: 0.001}
			unchanged, _ := agent.Capital.horizon(agent.LocalLearning, agent.Capital.Actual, []*EntryCandidate{first, second}, at.Add(2*time.Second))
			So(unchanged, ShouldEqual, horizon)
		})
		Convey("An account with no candidates uses its measured mark cadence", func() {
			state := AccountState{Mark: EquityMark{At: at, Version: 1, Equity: 200, HasFunding: true}, Cash: "200", Complete: true}
			So(agent.Capital.Actual.Observe(state), ShouldBeNil)
			state.Mark.At, state.Mark.Version = at.Add(3*time.Second), 2
			So(agent.Capital.Actual.Observe(state), ShouldBeNil)
			horizon, source := agent.Capital.horizon(agent.LocalLearning, agent.Capital.Actual, nil, state.Mark.At)
			So(horizon, ShouldEqual, 3*time.Second)
			So(source, ShouldEqual, "account observation interval")
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
		candidates[index].valid.Store(true)
	}
	b.ReportAllocs()
	for b.Loop() {
		for _, candidate := range candidates {
			candidate.Record.At = at
		}
		if err := capital.allocate(agent.LocalLearning, capital.Actual, candidates, false); err != nil {
			b.Fatal(err)
		}
		at = at.Add(capital.Actual.pending.horizon)
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
