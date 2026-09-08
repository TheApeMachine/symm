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

/*
A policy lane that never took a position leaves no exposure spans behind. That
emptiness is not ignorance — from the moment the desk started watching, it
demonstrably held nothing, so every move inside that window was sat out.

Reporting it as unknowable was visible on the surface as a contradiction: every
episode marked "not reviewable" while the same episodes were being trained on.
*/
func TestExposureOfALaneThatNeverHeld(t *testing.T) {
	Convey("A desk that held nothing has sat out every move it watched", t, func() {
		market := &learningMarket{symbol: "TEST/USD"}
		at := time.Unix(1000, 0)

		Convey("Before it watches anything, nothing can be judged", func() {
			held, known := market.heldDuring(10, 20, at, at.Add(time.Minute))
			So(held, ShouldBeFalse)
			So(known, ShouldBeFalse)
		})

		market.markExposure(false, 100, at)

		Convey("A move inside the watched window was sat out, not unknowable", func() {
			held, known := market.heldDuring(110, 120, at.Add(time.Minute), at.Add(2*time.Minute))
			So(held, ShouldBeFalse)
			So(known, ShouldBeTrue)
		})

		Convey("A move that predates the watch stays unknowable", func() {
			held, known := market.heldDuring(10, 20, at.Add(-time.Hour), at.Add(-time.Minute))
			So(held, ShouldBeFalse)
			So(known, ShouldBeFalse)
		})

		Convey("A move it actually held through is reported as held", func() {
			market.markExposure(true, 130, at.Add(2*time.Minute))
			market.markExposure(false, 160, at.Add(3*time.Minute))

			held, known := market.heldDuring(140, 150, at.Add(2*time.Minute), at.Add(3*time.Minute))
			So(held, ShouldBeTrue)
			So(known, ShouldBeTrue)
		})
	})
}
