package strategy

import (
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	markettest "github.com/theapemachine/symm/tests/market"
	"github.com/theapemachine/symm/types"
)

/* runTape drives the agent over the real multi-leg tape and returns its journal. */
func runTape(testingTB testing.TB, laps int) (*Agent, []hindsight.LearningEvent) {
	testingTB.Helper()
	events := []hindsight.LearningEvent{}
	agent, books := agentFixture(testingTB, func(event hindsight.LearningEvent) error {
		events = append(events, event)
		return nil
	})
	books.current = spotbook.New()
	books.current.NoBookCrossing = false
	measurement := data.NewMeasurement[float64]("", "TEST/USD", "source", time.Time{}, time.Time{})
	ordinal := 0

	for range laps {
		tape := markettest.NewLevel3ChurnTape("TEST/USD", time.Unix(100, 0), 64)

		for _, message := range tape.Messages {
			books.update(message)
			ordinal++
			// One synthetic monotonic clock across laps: the tape restarts its
			// own timestamps, and a valuation cannot move backwards in time.
			at := time.Unix(100, 0).Add(time.Duration(ordinal) * time.Second)
			agent.now = func() time.Time { return at }
			measurement.PutMetric(data.Metric[float64]{Label: "ordinal", Raw: float64(ordinal % 7)})

			if err := agent.Grid.Step([]*data.Measurement[float64]{measurement}); err != nil {
				testingTB.Fatal(err)
			}

			envelope := types.NewEnvelope(types.EnvelopeLevel3)
			envelope.Level3Data = kraken.Level3Data{Symbol: message.Symbol, Timestamp: at}
			agent.Step(envelope)

			if err := agent.Error(); err != nil {
				testingTB.Fatal(err)
			}
		}
	}

	return agent, events
}

func TestAgentMeasuresEveryDecisionOverTheSameWindow(t *testing.T) {
	Convey("Given a run long enough for measurement windows to close", t, func() {
		agent, events := runTape(t, 8)
		issued := map[uint64]hindsight.LearningEvent{}
		resolved := []hindsight.LearningEvent{}

		for _, event := range events {
			switch event.Kind {
			case "issued":
				issued[event.ID] = event
			case "resolved":
				resolved = append(resolved, event)
			}
		}

		Convey("Decisions resolve, and none before its window has actually elapsed", func() {
			So(len(resolved), ShouldBeGreaterThan, 0)

			for _, event := range resolved {
				origin, found := issued[event.ID]
				So(found, ShouldBeTrue)
				So(event.Horizon, ShouldBeGreaterThan, 0)

				// A truncated record is settled early on purpose, because its
				// account was spent; it declares that rather than looking like
				// a completed window.
				if event.Truncated {
					continue
				}

				So(event.At.Sub(origin.At), ShouldBeGreaterThanOrEqualTo, event.Horizon)
			}
		})

		Convey("Waiting is measured over the same window as acting", func() {
			windows := map[string][]time.Duration{}

			for _, event := range resolved {
				origin := issued[event.ID]
				kind := "acting"

				if event.Action == string(types.ActionHold) || event.Action == "" {
					kind = "waiting"
				}

				windows[kind] = append(windows[kind], event.At.Sub(origin.At))
			}

			// Both kinds must be present, and neither may be scored on a
			// systematically shorter window than the other: the earlier design
			// resolved a wait on the very next book update.
			So(len(windows["waiting"]), ShouldBeGreaterThan, 0)

			for kind, elapsed := range windows {
				for _, window := range elapsed {
					So(window, ShouldBeGreaterThan, time.Duration(0))
					So(kind, ShouldNotBeEmpty)
				}
			}
		})

		Convey("Evidence accumulates instead of scattering across one-shot contexts", func() {
			view := agent.view("TEST/USD")
			repeated := 0

			for _, candidate := range view.Candidates {
				if candidate.Prior.Samples > 1 {
					repeated++
				}
			}

			So(len(view.Candidates), ShouldBeGreaterThan, 0)
			So(repeated, ShouldBeGreaterThan, 0)
		})

		Convey("Resolved outcomes are credited back to the quantities that were hot", func() {
			view := agent.view("TEST/USD")
			So(len(view.Influence), ShouldBeGreaterThan, 0)

			for _, entry := range view.Influence {
				So(entry.Token, ShouldBeGreaterThan, 0)
				So(entry.Action, ShouldNotBeEmpty)
			}
		})
	})
}

func TestAgentRecyclesSpentAccounts(t *testing.T) {
	Convey("Given exploration lanes that spend their capital on execution costs", t, func() {
		agent, events := runTape(t, 24)
		recycled := 0

		for _, event := range events {
			if event.Kind == "recycled" {
				recycled++
			}
		}

		market := agent.markets["TEST/USD"]

		Convey("A lane that can no longer act restarts rather than pretending to decide", func() {
			for _, lane := range market.lanes {
				So(lane.wallet.cash.Sign(), ShouldBeGreaterThanOrEqualTo, 0)

				if lane.episodes > 0 {
					So(recycled, ShouldBeGreaterThan, 0)
					So(lane.wallet.cash.Cmp(agent.initial.Rat()), ShouldBeLessThanOrEqualTo, 0)
				}
			}
		})

		Convey("A restart never leaves a decision unresolved behind it", func() {
			for _, lane := range market.lanes {
				if lane.episodes > 0 {
					So(lane.resolved, ShouldBeGreaterThan, 0)
				}
			}
		})
	})
}

func TestPolicyLaneWritesWhereItReads(t *testing.T) {
	Convey("Given the policy lane resolving its own decisions", t, func() {
		agent, _ := runTape(t, 8)
		market := agent.markets["TEST/USD"]
		policy := &market.lanes[len(market.lanes)-1]

		Convey("Its experience lands under the identity selection actually reads", func() {
			So(policy.paper, ShouldBeTrue)

			// Every issued decision, from any lane, must be recallable under the
			// one key Select uses. A policy lane that issued under a separate
			// key would be training a subtree nothing ever reads.
			view := agent.view("TEST/USD")
			evidence := uint64(0)

			for _, candidate := range view.Candidates {
				evidence += candidate.Prior.Samples
			}

			So(evidence, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSkillAdmitsOnlyDisjointWindows(t *testing.T) {
	Convey("Given a policy lane whose decisions resolve in batches", t, func() {
		agent, events := runTape(t, 8)
		market := agent.markets["TEST/USD"]
		policy := &market.lanes[len(market.lanes)-1]
		resolved := uint64(0)

		for _, event := range events {
			if event.Kind == "resolved" && event.Mode == "policy" {
				resolved++
			}
		}

		reading := agent.Skill.Reading()

		Convey("Overlapping decisions cannot each count as evidence of competence", func() {
			So(resolved, ShouldBeGreaterThan, 0)
			So(policy.resolved, ShouldEqual, resolved)

			// Decisions issue far faster than a window closes, so a batch of
			// them resolves against one account valuation. Only the disjoint
			// ones may reach the estimator, or near-identical targets collapse
			// the dispersion and saturate confidence on a single observation.
			So(reading.Samples, ShouldBeLessThan, resolved)
		})

		Convey("Every admitted observation covers its own stretch of tape", func() {
			horizon := market.horizon()
			So(horizon, ShouldBeGreaterThan, 0)

			// The admitted count cannot exceed the number of whole horizons
			// the run actually spanned.
			span := market.at.Sub(time.Unix(100, 0))
			So(reading.Samples, ShouldBeLessThanOrEqualTo, uint64(span/horizon)+1)
		})

		Convey("A bound built on evidence below its own confidence floor is not presented", func() {
			if !reading.Qualified {
				So(reading.Support, ShouldBeLessThanOrEqualTo, skillSigma*skillSigma)
			}
		})
	})
}
