package strategy

import (
	"errors"
	"math/big"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/types"
)

func TestExecutionSubmit(t *testing.T) {
	Convey("Given genuine inventory reductions and exposure increases", t, func() {
		agent, _ := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
		desk := &capitalDesk{}
		agent.Desk = desk
		at := time.Unix(100, 0)
		for index := range 64 {
			agent.Skill.Observe(0.01, 1, at.Add(time.Duration(index)*time.Second))
		}
		agent.Skill.mode = ModeTrading
		for _, blocked := range []string{"skill", "realization"} {
			Convey(blocked+" blocks increases while preserving liquidation", func() {
				if blocked == "skill" {
					agent.Skill.mode = ModeLearning
				}

				if blocked == "realization" {
					for range 3 {
						agent.Realization.ObserveSubmission(errors.New("venue rejection"))
					}
				}
				for _, kind := range []types.Action{types.ActionEnter, types.ActionScale} {
					So(agent.Submit(ExecutionIntent{Kind: kind, Quantity: big.NewRat(1, 1)}), ShouldBeNil)
				}
				So(desk.intents, ShouldBeEmpty)
				for _, kind := range []types.Action{types.ActionExit, types.ActionScale} {
					So(agent.Submit(ExecutionIntent{Kind: kind, Quantity: big.NewRat(1, 1), Reduce: true}), ShouldBeNil)
				}
				So(desk.intents, ShouldHaveLength, 2)
			})
		}
	})
}

func TestExecutionReduce(t *testing.T) {
	Convey("Given authoritative inventory while the isolated policy wallet is flat and demoted", t, func() {
		agent, books := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
		desk := &capitalDesk{state: AccountState{Mark: EquityMark{Equity: 200}, Cash: "0", Positions: map[string]string{"TEST/USD": "2"}}}
		agent.Desk = desk
		action := LearningAction{Kind: types.ActionExit, Reduce: true}
		for range 30 {
			So(agent.Model.Observe([2]string{"TEST/USD", "virtual"}, nil, action, 0.1, 1, [2]string{"", "virtual"}), ShouldBeNil)
		}
		market := &learningMarket{symbol: "TEST/USD", at: time.Unix(100, 0)}
		So(agent.Execution.Reduce(agent.LocalLearning, market, books.current), ShouldBeNil)
		So(desk.intents, ShouldHaveLength, 1)
		So(desk.intents[0].Reduce, ShouldBeTrue)
		So(desk.intents[0].Quantity.RatString(), ShouldEqual, "2")
		So(agent.Skill.Mode(), ShouldEqual, ModeLearning)
	})
}

func TestExecutionRefresh(t *testing.T) {
	Convey("Given a still-current candidate when live increase authority changes", t, func() {
		agent, _ := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
		at := time.Unix(100, 0)
		candidate := candidateFixture("TEST/USD", at)
		So(agent.Capital.Candidates.Publish(candidate), ShouldBeNil)
		agent.Skill.mode = ModeTrading
		So(agent.Refresh(at), ShouldBeNil)
		So(candidate.Current(at), ShouldBeFalse)
		So(agent.allowed.Load(), ShouldBeTrue)
		candidate = candidateFixture("TEST/USD", at)
		So(agent.Capital.Candidates.Publish(candidate), ShouldBeNil)
		agent.Skill.mode = ModeLearning
		So(agent.Refresh(at), ShouldBeNil)
		So(candidate.Current(at), ShouldBeFalse)
		So(agent.allowed.Load(), ShouldBeFalse)
	})
}
