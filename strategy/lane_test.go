package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

func TestLearningLaneResolve(t *testing.T) {
	Convey("Forward returns do not reward a wallet for its previous losses", t, func() {
		agent, _ := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
		at := time.Unix(100, 0)
		market := &learningMarket{symbol: "TEST/USD", at: at.Add(time.Second)}
		for _, growth := range []float64{0, -2, 2} {
			for _, historicalRate := range []float64{-10, 0, 10} {
				action := LearningAction{Kind: types.ActionHold}
				identity, err := agent.Knowledge.Issue(market.symbol, nil, action, 1)
				So(err, ShouldBeNil)
				lane := learningLane{equity: 20 + growth}
				experience := learningExperience{id: identity, action: action, at: at, value: 20, rate: historicalRate, authority: 1}
				So(lane.resolve(agent.LocalLearning, market, 0, market.at, []learningExperience{experience}, false), ShouldBeNil)
				event := market.events[len(market.events)-1]
				So(event.Target, ShouldAlmostEqual, growth/200)
				So(*event.AbsoluteSkillTarget, ShouldAlmostEqual, growth/200)
				So(event.TargetUnit, ShouldEqual, "absolute_return_per_second")
			}
		}
	})
}

func TestLearningLaneRecycle(t *testing.T) {
	Convey("A flat wallet has cash for a lot but not the venue minimum order cost", t, func() {
		agent, books := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
		market := &learningMarket{symbol: "TEST/USD", at: time.Unix(100, 0)}
		So(agent.initialize(market), ShouldBeNil)
		lane := &market.lanes[0]
		lane.wallet.cash.SetInt64(2)
		lane.wallet.pricing.Lot.SetFrac64(1, 100)
		lane.wallet.pricing.Minimum.SetFrac64(1, 100)
		lane.wallet.pricing.CostMinimum.SetInt64(5)
		lane.equity = 2
		lane.version = 1
		So(lane.wallet.maximum(books.current, true).Sign(), ShouldEqual, 1)
		So(lane.recycle(agent.LocalLearning, market, 0, books.current, market.at), ShouldBeNil)
		So(lane.episodes, ShouldEqual, 1)
		So(lane.realized, ShouldEqual, -198)
		So(lane.equity, ShouldEqual, 200)
		So(lane.outcome.TotalReward, ShouldEqual, 0)
		So(lane.ledger.initial.Equity, ShouldEqual, 200)
		Convey("An affordable wallet is not recycled", func() {
			So(lane.recycle(agent.LocalLearning, market, 0, books.current, market.at), ShouldBeNil)
			So(lane.episodes, ShouldEqual, 1)
		})
	})
}

func BenchmarkLearningLaneResolve(b *testing.B) {
	agent, _ := agentFixture(b, func(hindsight.LearningEvent) error { return nil })
	market := &learningMarket{symbol: "TEST/USD", at: time.Unix(100, 0)}
	lane := learningLane{equity: 202}
	action := LearningAction{Kind: types.ActionEnter}
	context := []uint64{1, 0, 0}
	b.ReportAllocs()
	for b.Loop() {
		identity, err := agent.Knowledge.Issue(market.symbol, context, action, 1)
		if err != nil {
			b.Fatal(err)
		}
		experience := learningExperience{id: identity, action: action, at: market.at.Add(-time.Second), value: 200, authority: 1, reading: KnowledgeReading{Selected: learning.PriorReading{}}}
		market.events = market.events[:0]
		if err := lane.resolve(agent.LocalLearning, market, 0, market.at, []learningExperience{experience}, false); err != nil {
			b.Fatal(err)
		}
	}
}
