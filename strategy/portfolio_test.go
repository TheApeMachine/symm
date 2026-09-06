package strategy

import (
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/types"
)

func TestVirtualPortfolioAllocate(t *testing.T) {
	Convey("Given a single shared wallet and later executable spot depth", t, func() {
		agent, books := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
		portfolio := NewVirtualPortfolio(decimal.NewFromInt64(150))
		at := time.Unix(100, 0)
		first := candidateFixture("TEST/USD", at)
		second := candidateFixture("OTHER/USD", at)
		So(portfolio.Allocate(first, nil), ShouldBeTrue)
		So(portfolio.Allocate(second, nil), ShouldBeFalse)
		market := &learningMarket{symbol: "TEST/USD", at: at.Add(time.Second)}
		So(portfolio.Step(agent.LocalLearning, market, books.current), ShouldBeNil)
		state := portfolio.Snapshot(market.at)
		So(state.Cash, ShouldEqual, "4799/100")
		So(state.Positions["TEST/USD"], ShouldEqual, "1")
		So(portfolio.Allocate(second, nil), ShouldBeFalse)
		So(agent.Knowledge.Reading("TEST/USD", nil, first.action).Global.Defined, ShouldBeFalse)
		So(agent.Skill.Reading().Samples, ShouldEqual, 0)
	})
}

func TestVirtualPortfolioSnapshot(t *testing.T) {
	Convey("A commitment reduces available cash without changing marked equity", t, func() {
		portfolio := NewVirtualPortfolio(decimal.NewFromInt64(150))
		at := time.Unix(100, 0)
		candidate := candidateFixture("TEST/USD", at)
		So(portfolio.Allocate(candidate, nil), ShouldBeTrue)
		state := portfolio.Snapshot(at)
		So(state.ActualCash, ShouldEqual, "150")
		So(state.Mark.Equity, ShouldEqual, 150)
		So(state.Committed, ShouldEqual, candidate.cost.RatString())
		So(state.Cash, ShouldEqual, "4799/100")
	})
}

func TestVirtualPortfolioStep(t *testing.T) {
	Convey("A shared wallet reports only fills against later surviving depth", t, func() {
		agent, books := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
		portfolio, teacher := agent.Capital.Virtual, agent.Capital.Exploration
		at := time.Unix(100, 0)
		candidate := candidateFixture("TEST/USD", at)
		So(teacher.Observe(portfolio.Snapshot(at)), ShouldBeNil)
		action := CapitalAction{Symbol: "TEST/USD", Kind: types.ActionEnter}
		_, err := teacher.Issue(action, nil, candidate.Record.ID, time.Second, 1, at, nil, nil, "test measured interval")
		So(err, ShouldBeNil)
		receipt := teacher.pending.receipt
		So(portfolio.Allocate(candidate, receipt), ShouldBeTrue)
		market := &learningMarket{symbol: "TEST/USD", at: at.Add(time.Second)}
		Convey("Withdrawing the quoted level aborts without a capital sample", func() {
			books.current.Update(&spotbook.UpdateOptions{Direction: spotbook.Ask, ID: "ask", Price: decimal.NewFromInt64(101), Quantity: decimal.NewFromInt64(0), Silent: true})
			So(portfolio.Step(agent.LocalLearning, market, books.current), ShouldBeNil)
			So(teacher.Observe(portfolio.Snapshot(market.at)), ShouldBeNil)
			So(receipt.Result.Load().State, ShouldEqual, "aborted")
			So(teacher.Aborted, ShouldEqual, 1)
			So(teacher.Resolved, ShouldEqual, 0)
			So(agent.Capital.Knowledge.Reading(nil, action).Virtual.Global.Samples, ShouldEqual, 0)
			So(portfolio.cash.RatString(), ShouldEqual, "200")
		})
		Convey("A surviving fill resolves only the virtual evidence source", func() {
			So(portfolio.Step(agent.LocalLearning, market, books.current), ShouldBeNil)
			So(teacher.Observe(portfolio.Snapshot(market.at)), ShouldBeNil)
			So(receipt.Result.Load().State, ShouldEqual, "filled")
			So(teacher.Resolved, ShouldEqual, 1)
			reading := agent.Capital.Knowledge.Reading(nil, action)
			So(reading.Virtual.Global.Samples, ShouldEqual, 1)
			So(reading.Actual.Global.Samples, ShouldEqual, 0)
		})
	})
}

func BenchmarkVirtualPortfolioSnapshot(b *testing.B) {
	portfolio := NewVirtualPortfolio(decimal.NewFromInt64(200))
	b.ReportAllocs()
	for b.Loop() {
		portfolio.Snapshot(time.Unix(100, 0))
	}
}
