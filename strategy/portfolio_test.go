package strategy

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
)

func TestVirtualPortfolioAllocate(t *testing.T) {
	Convey("Given a single shared wallet and later executable spot depth", t, func() {
		agent, books := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
		portfolio := NewVirtualPortfolio(decimal.NewFromInt64(150))
		at := time.Unix(100, 0)
		first := candidateFixture("TEST/USD", at)
		second := candidateFixture("OTHER/USD", at)
		So(portfolio.Allocate(first), ShouldBeTrue)
		So(portfolio.Allocate(second), ShouldBeFalse)
		market := &learningMarket{symbol: "TEST/USD", at: at.Add(time.Second)}
		So(portfolio.Step(agent.LocalLearning, market, books.current), ShouldBeNil)
		state := portfolio.Snapshot(market.at)
		So(state.Cash, ShouldEqual, "4799/100")
		So(state.Positions["TEST/USD"], ShouldEqual, "1")
		So(portfolio.Allocate(second), ShouldBeFalse)
		So(agent.Knowledge.Reading("TEST/USD", nil, first.action).Global.Defined, ShouldBeFalse)
		So(agent.Skill.Reading().Samples, ShouldEqual, 0)
	})
}

func TestVirtualPortfolioSnapshot(t *testing.T) {
	Convey("A commitment reduces available cash without changing marked equity", t, func() {
		portfolio := NewVirtualPortfolio(decimal.NewFromInt64(150))
		at := time.Unix(100, 0)
		candidate := candidateFixture("TEST/USD", at)
		So(portfolio.Allocate(candidate), ShouldBeTrue)
		state := portfolio.Snapshot(at)
		So(state.ActualCash, ShouldEqual, "150")
		So(state.Mark.Equity, ShouldEqual, 150)
		So(state.Committed, ShouldEqual, candidate.cost.RatString())
		So(state.Cash, ShouldEqual, "4799/100")
	})
}

func BenchmarkVirtualPortfolioSnapshot(b *testing.B) {
	portfolio := NewVirtualPortfolio(decimal.NewFromInt64(200))
	b.ReportAllocs()
	for b.Loop() {
		portfolio.Snapshot(time.Unix(100, 0))
	}
}
