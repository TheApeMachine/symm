package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
)

func TestForwardReviewJudgesAgainstActualExposure(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()

	episode := func(id string, from, to time.Time) hindsight.Episode {
		return hindsight.Episode{
			ID: id, Symbol: "TEST/USD", Kind: hindsight.EpisodeUpwardExcursion,
			FromAt: from, ToAt: to, Confirmed: true,
			ObservedExcursion: 0.04, HasObservedExcursion: true,
		}
	}

	episodeSeq := func(id string, fromSeq, toSeq hindsight.CaptureSequence, from, to time.Time) hindsight.Episode {
		ep := episode(id, from, to)
		ep.FromSequence = fromSeq
		ep.ToSequence = toSeq
		return ep
	}

	Convey("Given a policy lane that held inventory over one stretch of tape", t, func() {
		agent, _ := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
		agent.now = func() time.Time { return base.Add(time.Hour) }
		market := &learningMarket{symbol: "TEST/USD"}
		agent.markets["TEST/USD"] = market

		market.markExposure(true, 100, base.Add(10*time.Minute))
		market.markExposure(true, 120, base.Add(12*time.Minute))
		market.markExposure(false, 150, base.Add(15*time.Minute))

		Convey("An excursion it was holding through is exposed, not unexposed", func() {
			agent.review([]hindsight.Episode{
				episodeSeq("a", 110, 140, base.Add(11*time.Minute), base.Add(14*time.Minute)),
			})

			So(agent.forward.Reviewed, ShouldEqual, 1)
			So(agent.forward.Exposed, ShouldEqual, 1)
			So(agent.forward.Captured, ShouldEqual, 1)
			So(agent.forward.Unexposed, ShouldEqual, 0)
			So(agent.forward.Missed, ShouldEqual, 0)
			So(agent.forward.Recent[0].Exposed, ShouldBeTrue)
		})

		Convey("An excursion it sat out of is a miss", func() {
			agent.review([]hindsight.Episode{
				episode("b", base.Add(20*time.Minute), base.Add(25*time.Minute)),
			})

			So(agent.forward.Unexposed, ShouldEqual, 1)
			So(agent.forward.Missed, ShouldEqual, 1)
			So(agent.forward.Recent[0].Exposed, ShouldBeFalse)
			So(agent.forward.Recent[0].Unreviewable, ShouldBeFalse)
		})

		Convey("An excursion older than the retained history is not called a miss", func() {
			agent.review([]hindsight.Episode{
				episode("c", base.Add(-time.Hour), base.Add(-50*time.Minute)),
			})

			So(agent.forward.Unreviewable, ShouldEqual, 1)
			So(agent.forward.Missed, ShouldEqual, 0)
			So(agent.forward.Recent[0].Unreviewable, ShouldBeTrue)
		})

		Convey("The same episode is never judged twice", func() {
			same := []hindsight.Episode{episode("a", base.Add(11*time.Minute), base.Add(14*time.Minute))}
			agent.review(same)
			agent.review(same)

			So(agent.forward.Reviewed, ShouldEqual, 1)
		})

		Convey("An unconfirmed episode is not judged at all", func() {
			running := episode("d", base.Add(11*time.Minute), base.Add(14*time.Minute))
			running.Confirmed = false
			agent.review([]hindsight.Episode{running})

			So(agent.forward.Reviewed, ShouldEqual, 0)
		})

		Convey("A symbol the agent never observed cannot be judged", func() {
			foreign := episode("e", base.Add(11*time.Minute), base.Add(14*time.Minute))
			foreign.Symbol = "PF_TESTUSD"
			agent.review([]hindsight.Episode{foreign})

			So(agent.forward.Reviewed, ShouldEqual, 0)
		})
	})
}
