package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
These cover the two properties that, when they were absent, left the agent
unable to learn anything at all: a decision must be measured over enough
forward tape to be answerable, and the past it is conditioned on must be
bounded.
*/
/*
trainedMarket returns a market whose movement has been measured from a steady
alternating step of the given size, one observation per second. A step of 0.001
is a dispersion of ten basis points per observation.
*/
func trainedMarket(step, cost float64) *learningMarket {
	market := &learningMarket{symbol: "TEST/USD"}
	at := time.Unix(100, 0)
	mid := 100.0

	for count := 1; count <= 4*movementReadiness; count++ {
		at = at.Add(time.Second)

		if count%2 == 0 {
			mid *= 1 + step
		} else {
			mid /= 1 + step
		}
		market.observe(mid, cost, at)
	}

	return market
}

func TestMeasurementWindow(t *testing.T) {
	Convey("A decision is measured over the window its own instrument needs", t, func() {
		/*
			Movement is measured per observation and the window is how long that
			movement needs to cover the round trip the decision pays. A market
			that moves ten basis points per observation covers a forty basis
			point round trip after (40/10)^2 = 16 observations.
		*/
		Convey("The window is unavailable until movement has been measured", func() {
			market := &learningMarket{symbol: "TEST/USD"}
			So(market.horizon(), ShouldEqual, time.Duration(0))

			market.observe(100, 0.004, time.Unix(100, 0))
			So(market.hasSigma, ShouldBeFalse)
			So(market.horizon(), ShouldEqual, time.Duration(0))
		})

		Convey("The window is the time that movement needs to cover the cost", func() {
			market := trainedMarket(0.001, 0.004)

			So(market.hasSigma, ShouldBeTrue)
			So(market.sigma, ShouldAlmostEqual, 0.001, 0.0002)
			So(market.observationMean, ShouldAlmostEqual, 1.0, 0.01)
			So(market.horizon().Seconds(), ShouldAlmostEqual, 16, 6)
		})

		/*
			The same movement against a wider round trip needs a longer window,
			and a market that moves more covers the same cost sooner. An
			instrument is judged on its own economics, never on a window
			borrowed from a different asset.
		*/
		Convey("A costlier instrument is measured over a longer window", func() {
			So(trainedMarket(0.001, 0.008).horizon(), ShouldBeGreaterThan,
				trainedMarket(0.001, 0.004).horizon())
		})

		Convey("A market that moves more is answered sooner", func() {
			So(trainedMarket(0.004, 0.004).horizon(), ShouldBeLessThan,
				trainedMarket(0.001, 0.004).horizon())
		})

		Convey("A window beyond the attribution ceiling is capped, not stretched", func() {
			So(trainedMarket(0.001, 1.0).horizon(), ShouldEqual,
				system.Cfg.Learning.MaximumHorizon)
		})

		Convey("A decision does not settle before its window closes", func() {
			market := &learningMarket{symbol: "TEST/USD", at: time.Unix(100, 0)}
			lane := &learningLane{}
			horizon := 16 * time.Second
			lane.trace = append(lane.trace, learningExperience{id: 1, at: time.Unix(100, 0)})

			// Two seconds into a sixteen second window: nothing is due, and the
			// decision is still retained rather than dropped.
			market.at = time.Unix(102, 0)
			So(lane.settle(nil, market, 0, market.at, horizon), ShouldBeNil)
			So(lane.trace, ShouldHaveLength, 1)
			So(lane.trace[0].horizon, ShouldEqual, horizon)

			// An unmeasured window resolves nothing rather than settling
			// everything against a window it never measured.
			market.at = time.Unix(200, 0)
			So(lane.settle(nil, market, 0, market.at, 0), ShouldBeNil)
			So(lane.trace, ShouldHaveLength, 1)
		})
	})
}

func TestPrecursorHistoryIsBounded(t *testing.T) {
	Convey("Precursor history cannot grow without bound", t, func() {
		market := &learningMarket{symbol: "TEST/USD"}
		frames := system.Cfg.Learning.PrecursorFrames

		for step := range 200 {
			market.AdvanceImpulse([]learning.Region{
				{Condition: uint64(step + 1), Strength: 1, Authority: 1},
			})
		}

		So(len(market.history), ShouldBeLessThanOrEqualTo, frames)

		/*
			A context that carries the whole run is unique to the moment it was
			built: it trains a path nothing ever revisits while the cost of
			binding and recalling it grows with uptime.
		*/
		So(len(market.PrecursorContext()), ShouldBeLessThanOrEqualTo, 2*(frames+1))

		Convey("The retained frames are the most recent ones", func() {
			So(market.currentConditions, ShouldResemble, []uint64{200})
			So(market.history[len(market.history)-1], ShouldResemble, []uint64{199})
		})
	})
}

func TestPolicyAdmission(t *testing.T) {
	Convey("The policy lane admits an action on comparative evidence", t, func() {
		grid := learning.NewGrid()
		knowledge := NewKnowledge(grid)
		local := &LocalLearning{Knowledge: knowledge}
		market := &learningMarket{symbol: "TEST/USD", context: []uint64{101}}
		lane := &learningLane{paper: true}

		enter := LearningAction{Kind: types.ActionEnter}
		hold := LearningAction{Kind: types.ActionHold}

		observe := func(action LearningAction, growth float64) {
			So(knowledge.Model.Observe(
				[2]string{"TEST/USD", "flat"}, market.context, action,
				growth, 1.0, 1.0, [2]string{"", "flat"},
			), ShouldBeNil)
		}

		Convey("An action with no completed evidence is held rather than guessed", func() {
			reading := knowledge.Reading("TEST/USD", "flat", market.context, enter)
			So(reading.Economic.Defined, ShouldBeFalse)
			So(lane.admit(local, market, "flat", enter, reading).Kind, ShouldEqual, types.ActionHold)
		})

		/*
			Waiting earns exactly zero by construction, so refusing every action
			whose rate is not positive is a comparison against waiting on a tape
			where acting always pays a spread first. It vetoes everything
			permanently. The gate compares like with like instead.
		*/
		Convey("An action the evidence prefers to holding is admitted", func() {
			observe(enter, 0.004)
			observe(hold, 0.001)

			reading := knowledge.Reading("TEST/USD", "flat", market.context, enter)
			So(reading.Economic.Defined, ShouldBeTrue)
			So(lane.admit(local, market, "flat", enter, reading), ShouldResemble, enter)
		})

		Convey("An action holding beats is refused", func() {
			observe(enter, -0.004)
			observe(hold, 0.001)

			reading := knowledge.Reading("TEST/USD", "flat", market.context, enter)
			So(lane.admit(local, market, "flat", enter, reading).Kind, ShouldEqual, types.ActionHold)
		})

		Convey("A genuine reduction is never refused on entry evidence", func() {
			observe(LearningAction{Kind: types.ActionExit, Reduce: true}, -0.004)
			exit := LearningAction{Kind: types.ActionExit, Reduce: true}
			reading := knowledge.Reading("TEST/USD", "holding", market.context, exit)
			So(lane.admit(local, market, "holding", exit, reading), ShouldResemble, exit)
		})
	})
}

/*
The retained journal is written by one build and read back by the next, so the
unit the resolver stamps and the units warmup knows how to read have to agree.
When they drifted apart, every boot after the first failed outright on a
journal the running system had just written itself.
*/
func TestWarmupReadsTheJournalItWrites(t *testing.T) {
	Convey("Warmup recovers the outcomes this build actually records", t, func() {
		_, events := runTape(t, 2)
		resolved := 0

		for _, event := range events {
			if event.Kind != "resolved" || event.Mode == "candidate" {
				continue
			}
			resolved++
			growth, readable := historicalGrowth(event, 1)
			So(event.TargetUnit, ShouldNotBeBlank)
			So(readable, ShouldBeTrue)
			So(growth, ShouldEqual, *event.AbsoluteSkillTarget)
		}

		So(resolved, ShouldBeGreaterThan, 0)

		knowledge := NewKnowledge(learning.NewGrid())
		report, err := knowledge.Warmup(events)

		So(err, ShouldBeNil)
		So(report.Resolved, ShouldBeGreaterThan, 0)
		So(report.TargetUnavailable, ShouldEqual, 0)
	})
}

/*
An unrecognised unit is unusable evidence, not a fatal condition: booting must
survive a journal written by a build that measured outcomes differently.
*/
func TestWarmupSurvivesAnUnknownUnit(t *testing.T) {
	Convey("A unit this build cannot read is reported, not fatal", t, func() {
		at := time.Unix(100, 0)
		growth := 0.02
		events := []hindsight.LearningEvent{
			{Run: "past", ID: 1, Symbol: "TEST/USD", Kind: "issued", At: at, Action: "enter", Authority: 1},
			{Run: "past", ID: 1, Symbol: "TEST/USD", Kind: "resolved", At: at.Add(time.Second),
				TargetUnit: "a_unit_from_a_later_build", Target: 0.2, AbsoluteSkillTarget: &growth},
			{Run: "past", ID: 2, Symbol: "TEST/USD", Kind: "issued", At: at, Action: "enter", Authority: 1},
			{Run: "past", ID: 2, Symbol: "TEST/USD", Kind: "resolved", At: at.Add(time.Second),
				TargetUnit: "compounded_growth", Target: 0.02, AbsoluteSkillTarget: &growth},
		}

		knowledge := NewKnowledge(learning.NewGrid())
		report, err := knowledge.Warmup(events)

		So(err, ShouldBeNil)
		So(report.Resolved, ShouldEqual, 1)
		So(report.TargetUnavailable, ShouldEqual, 1)
	})
}
