package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
	markettest "github.com/theapemachine/symm/tests/market"
	"github.com/theapemachine/symm/types"
)

/* agentBooks owns the actual SDK book used by the test's multi-leg input tape. */
type agentBooks struct{ current *spotbook.Book }

func (books *agentBooks) Book(_ string, read func(*spotbook.Book)) { read(books.current) }

func (books *agentBooks) update(message kraken.Level3Data) {
	for side, orders := range [][]kraken.Level3Order{message.Bids, message.Asks} {
		direction := spotbook.BookDirection(spotbook.Bid)

		if side == 1 {
			direction = spotbook.Ask
		}

		for _, order := range orders {
			quantity := order.OrderQty

			if order.Event == "delete" {
				quantity = decimal.NewFromInt64(0)
			}

			books.current.Update(&spotbook.UpdateOptions{Direction: direction, ID: order.OrderID,
				Price: order.LimitPrice, Quantity: quantity, Timestamp: order.Timestamp, Silent: true})
		}
	}
}

func agentFixture(testingTB testing.TB, record func(hindsight.LearningEvent) error) (*Agent, *agentBooks) {
	testingTB.Helper()
	wallet, book := virtualFixture()
	books := &agentBooks{current: book}
	agent, err := NewAgent(testingTB.Context(), learning.NewGrid(), books,
		func(string) kraken.InstrumentPair { return wallet.pair },
		func(string) *kraken.TradeVolumeFee { return &kraken.TradeVolumeFee{Fee: decimal.NewFromInt64(1)} },
		decimal.NewFromInt64(200), record)

	if err != nil {
		testingTB.Fatal(err)
	}
	return agent, books
}

func TestAgentStep(t *testing.T) {
	Convey("Given the real multi-leg market tape and symbol-only notifications", t, func() {
		events := []hindsight.LearningEvent{}
		agent, books := agentFixture(t, func(event hindsight.LearningEvent) error {
			if _, err := json.Marshal(event); err != nil {
				return err
			}
			events = append(events, event)
			return nil
		})
		books.current = spotbook.New()
		books.current.NoBookCrossing = false
		tape := markettest.NewLevel3ChurnTape("TEST/USD", time.Unix(100, 0), 64)
		measurement := data.NewMeasurement[float64]("", "TEST/USD", "source", time.Time{}, time.Time{})

		for index, message := range tape.Messages {
			books.update(message)
			at := message.Timestamp
			agent.now = func() time.Time { return at }
			// Numeric arrival ordinal supplies a changing, directly observed input.
			measurement.PutMetric(data.Metric[float64]{Label: "ordinal", Raw: float64(index)})
			So(agent.Grid.Step([]*data.Measurement[float64]{measurement}), ShouldBeNil)
			envelope := types.NewEnvelope(types.EnvelopeLevel3)
			envelope.Level3Data = kraken.Level3Data{Symbol: message.Symbol, Timestamp: at}
			So(agent.Step(envelope), ShouldEqual, envelope)
			So(agent.Error(), ShouldBeNil)
		}

		Convey("Each lane should account independently and settle only later evidence", func() {
			market := agent.markets["TEST/USD"]
			So(market.lanes, ShouldHaveLength, 5)
			So(agent.decisions, ShouldBeGreaterThan, 5)
			So(agent.resolved, ShouldBeGreaterThan, 0)
			issued := map[uint64]hindsight.LearningEvent{}
			fills := 0

			for _, event := range events {
				if event.Mode == "candidate" || strings.HasPrefix(event.Mode, "capital_") {
					continue
				}

				if event.Kind == "issued" {
					issued[event.ID] = event
					continue
				}

				prior, found := issued[event.ID]
				So(found, ShouldBeTrue)
				So(event.At.After(prior.At), ShouldBeTrue)
				So(event.Action, ShouldEqual, prior.Action)

				if event.Kind == "filled" {
					fills++
				}
			}
			So(fills, ShouldBeGreaterThan, 0)

			for _, lane := range market.lanes {
				So(lane.wallet.cash.Sign(), ShouldBeGreaterThanOrEqualTo, 0)
				So(lane.wallet.quantity.Sign(), ShouldBeGreaterThanOrEqualTo, 0)
				So(lane.ledger.initial.Equity, ShouldEqual, 200)
			}
		})

		Convey("Another symbol's observations cannot reissue this symbol's impulse", func() {
			before := agent.decisions
			measurement.Label = "ANOTHER/USD"
			So(agent.Grid.Step([]*data.Measurement[float64]{measurement}), ShouldBeNil)
			envelope := types.NewEnvelope(types.EnvelopeLevel3)
			envelope.Level3Data = kraken.Level3Data{Symbol: "TEST/USD"}
			agent.now = func() time.Time { return time.Unix(200, 0) }
			agent.Step(envelope)
			So(agent.Error(), ShouldBeNil)
			So(agent.decisions, ShouldEqual, before)
		})
	})

	Convey("Given a failed durable recorder", t, func() {
		failure := errors.New("journal unavailable")
		agent, _ := agentFixture(t, func(hindsight.LearningEvent) error { return failure })
		measurement := data.NewMeasurement[float64]("", "TEST/USD", "source", time.Time{}, time.Time{})

		for _, value := range []float64{-1, 1, -1, 1} {
			measurement.PutMetric(data.Metric[float64]{Label: "value", Raw: value})
			So(agent.Grid.Step([]*data.Measurement[float64]{measurement}), ShouldBeNil)
		}
		envelope := types.NewEnvelope(types.EnvelopeLevel3)
		envelope.Level3Data.Symbol = "TEST/USD"
		agent.Step(envelope)
		So(errors.Is(agent.Error(), failure), ShouldBeTrue)
	})
}

func TestNewAgent(t *testing.T) {
	Convey("Missing execution or recording dependencies cannot invent a working agent", t, func() {
		_, err := NewAgent(context.Background(), nil, nil, nil, nil, nil, nil)
		So(err, ShouldNotBeNil)
	})
}

func TestAgentGlobalSkillWindow(t *testing.T) {
	Convey("Given an agent measuring global skill across multiple markets", t, func() {
		agent, _ := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
		agent.Skill = NewSkillMeter(AccountReal, time.Unix(100, 0))

		marketA := &learningMarket{symbol: "BTC/USD", lanes: make([]learningLane, 1)}
		marketB := &learningMarket{symbol: "ETH/USD", lanes: make([]learningLane, 1)}
		marketA.lanes[0].paper = true
		marketB.lanes[0].paper = true
		marketA.lanes[0].equity = 202.0
		marketB.lanes[0].equity = 202.0

		t0 := time.Unix(100, 0)
		t1 := time.Unix(110, 0)
		t0_5 := time.Unix(105, 0)
		t1_5 := time.Unix(115, 0)
		t2 := time.Unix(120, 0)

		// Market A issues decision at t0, resolves at t1.
		idA1, errIssue := agent.Model.Issue([2]string{"BTC/USD", "virtual"}, []uint64{1}, LearningAction{Kind: types.ActionHold}, 1.0)
		So(errIssue, ShouldBeNil)
		marketA.at = t1
		expA1 := learningExperience{id: idA1, at: t0, value: 200.0, authority: 1.0}
		err := marketA.lanes[0].resolve(agent.LocalLearning, marketA, 0, t1, []learningExperience{expA1}, false)
		So(err, ShouldBeNil)
		So(agent.Skill.Reading().Samples, ShouldEqual, 1)
		So(agent.Skill.window, ShouldEqual, t1)

		Convey("Market B decision overlapping Market A's window is rejected globally", func() {
			// Market B issues decision at t0_5 (before t1), resolves at t1_5.
			idB1, errIssueB1 := agent.Model.Issue([2]string{"ETH/USD", "virtual"}, []uint64{1}, LearningAction{Kind: types.ActionHold}, 1.0)
			So(errIssueB1, ShouldBeNil)
			marketB.at = t1_5
			expB1 := learningExperience{id: idB1, at: t0_5, value: 200.0, authority: 1.0}
			err := marketB.lanes[0].resolve(agent.LocalLearning, marketB, 0, t1_5, []learningExperience{expB1}, false)
			So(err, ShouldBeNil)
			// Samples should still be 1 because t0_5 is before agent.Skill.window (t1).
			So(agent.Skill.Reading().Samples, ShouldEqual, 1)

			Convey("Market B decision starting at or after t1 is admitted", func() {
				// Market B issues decision at t1, resolves at t2.
				idB2, errIssueB2 := agent.Model.Issue([2]string{"ETH/USD", "virtual"}, []uint64{1}, LearningAction{Kind: types.ActionHold}, 1.0)
				So(errIssueB2, ShouldBeNil)
				marketB.at = t2
				expB2 := learningExperience{id: idB2, at: t1, value: 200.0, authority: 1.0}
				err := marketB.lanes[0].resolve(agent.LocalLearning, marketB, 0, t2, []learningExperience{expB2}, false)
				So(err, ShouldBeNil)
				So(agent.Skill.Reading().Samples, ShouldEqual, 2)
				So(agent.Skill.window, ShouldEqual, t2)
			})
		})
	})
}

func TestTwoKeyExecutionGating(t *testing.T) {
	Convey("Given an agent promoted to ModeTrading on measured edge", t, func() {
		agent, _ := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
		at := time.Unix(100, 0)
		agent.Skill = NewSkillMeter(AccountReal, at)

		for index := range 64 {
			agent.Skill.Observe(0.010, 1.0, at.Add(time.Duration(index)*time.Second))
		}

		So(agent.Skill.Mode(), ShouldEqual, ModeTrading)
		So(agent.Mode(), ShouldEqual, ModeTrading)

		Convey("execution circuit breaker trips and vetoes live trading", func() {
			errSample := errors.New("exchange rejected order")
			agent.Realization.ObserveSubmission(errSample)
			agent.Realization.ObserveSubmission(errSample)
			So(agent.Mode(), ShouldEqual, ModeTrading)

			agent.Realization.ObserveSubmission(errSample)
			So(agent.Realization.AllowsTrading(), ShouldBeFalse)
			So(agent.Mode(), ShouldEqual, ModeLearning)

			Convey("re-enabling realization restores trading", func() {
				agent.Realization.Reset()
				So(agent.Realization.AllowsTrading(), ShouldBeTrue)
				So(agent.Mode(), ShouldEqual, ModeTrading)
			})
		})
	})
}

func TestPolicyLaneUpdatesVirtualModel(t *testing.T) {
	Convey("Given an agent with a policy lane issuing decisions", t, func() {
		agent, books := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
		wallet, _ := virtualFixture()
		market := &learningMarket{
			symbol:  "TEST/USD",
			lanes:   make([]learningLane, 1),
			regions: []learning.Region{{ID: 1, Strength: 1.0, Authority: 1.0}},
		}
		market.lanes[0].paper = true
		market.lanes[0].equity = 200.0
		market.lanes[0].wallet = wallet
		market.context = []uint64{1, 2, 0, 0}
		market.sequence = []uint64{1, 2}
		agent.Grid.Column("fixture", "first")
		agent.Grid.Column("fixture", "second")

		at := time.Unix(100, 0)
		market.at = at

		err := market.lanes[0].issue(agent.LocalLearning, market, 0, books.current, at)
		So(err, ShouldBeNil)
		So(market.lanes[0].pending, ShouldNotEqual, 0)

		Convey("resolving policy decision directly trains the virtual model prior", func() {
			pendingID := market.lanes[0].pending
			So(market.lanes[0].trace, ShouldHaveLength, 1)
			experience := market.lanes[0].trace[0]
			So(experience.id, ShouldEqual, pendingID)

			market.at = at.Add(10 * time.Second)
			market.lanes[0].equity = 205.0

			err = market.lanes[0].resolve(agent.LocalLearning, market, 0, market.at, []learningExperience{experience}, false)
			So(err, ShouldBeNil)

			// Recall under [market.symbol, "virtual"] should now have recorded evidence!
			recalled := agent.Model.Recall([2]string{"TEST/USD", "virtual"}, market.context, experience.action)
			So(recalled.Defined, ShouldBeTrue)
			So(recalled.Samples, ShouldEqual, 1)
		})
	})
}

type testDesk struct{}

func (desk *testDesk) Submit(ExecutionIntent) error {
	return nil
}

func TestLearningViewRealizationObservability(t *testing.T) {
	Convey("Given an agent with attached execution and realization", t, func() {
		agent, _ := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
		desk := &testDesk{}
		agent.SetExecution(desk, AccountPaper)

		Convey("Under normal operation, view reports realization allowed and matching mode", func() {
			v := agent.view("TEST/USD")
			So(v.RealizationAllowed, ShouldBeTrue)
			So(v.RealizationReason, ShouldBeEmpty)
			So(v.AuthorizedMode, ShouldEqual, ModeLearning.String())
		})

		Convey("When realization circuit breaker trips, view reflects veto while preserving skill reading", func() {
			base := time.Now()
			for range 20 {
				agent.Skill.Observe(0.02, 1.0, base)
			}

			So(agent.Skill.Mode(), ShouldEqual, ModeTrading)

			// Trip realization circuit breaker via catastrophic slippage
			agent.Realization.ObserveFill(100.0, 105.0, false) // 500 bps slippage
			So(agent.Realization.AllowsTrading(), ShouldBeFalse)

			// Effective mode is downgraded to learning
			So(agent.Mode(), ShouldEqual, ModeLearning)

			// View exposes the separation transparently
			v := agent.view("TEST/USD")
			So(v.Skill.Mode, ShouldEqual, "trading")
			So(v.AuthorizedMode, ShouldEqual, ModeLearning.String())
			So(v.RealizationAllowed, ShouldBeFalse)
			So(v.RealizationReason, ShouldContainSubstring, "catastrophic single-fill slippage exceeded bound")
		})
	})
}

func TestAgentWarmup(t *testing.T) {
	Convey("Given an agent and historical learning events from previous runs", t, func() {
		agent, _ := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
		context := []uint64{5, 12, 18}
		action := LearningAction{Kind: types.ActionEnter, Power: 50, Reduce: false}
		key := [2]string{"TEST/USD", "virtual"}

		So(agent.Model.Recall(key, context, action).Defined, ShouldBeFalse)
		So(agent.Mode(), ShouldEqual, ModeLearning)

		history := []hindsight.LearningEvent{
			{
				ID:        1,
				Symbol:    "TEST/USD",
				Kind:      "issued",
				Action:    string(action.Kind),
				Power:     action.Power,
				Context:   context,
				Authority: 0.75,
			},
			{
				ID:     1,
				Symbol: "TEST/USD",
				Kind:   "resolved",
				Action: string(action.Kind),
				Power:  action.Power,
				Reduce: action.Reduce,
				Target: 0.05, TargetUnit: "absolute_return_per_second",
			},
			{
				ID:        2,
				Symbol:    "TEST/USD",
				Kind:      "resolved",
				Context:   context,
				Authority: 0.85,
				Action:    string(action.Kind),
				Power:     action.Power,
				Reduce:    action.Reduce,
				Target:    0.08,
			},
		}

		warmed, err := agent.Warmup(history)
		So(err, ShouldBeNil)
		So(warmed.Resolved, ShouldEqual, 1)
		So(warmed.Unpaired, ShouldEqual, 1)

		reading := agent.Model.Recall(key, context, action)
		So(reading.Defined, ShouldBeTrue)
		So(reading.Samples, ShouldEqual, 1)
		So(reading.Mean, ShouldEqual, 0.05)

		// Live skill meter must remain in learning mode: stored model provides prior boost
		// but does not grant execution authority without live forward verification.
		So(agent.Mode(), ShouldEqual, ModeLearning)
		So(agent.Skill.Reading().Samples, ShouldEqual, 0)
	})
}

func BenchmarkAgentStep(b *testing.B) {
	agent, _ := agentFixture(b, func(hindsight.LearningEvent) error { return nil })
	measurement := data.NewMeasurement[float64]("", "TEST/USD", "source", time.Time{}, time.Time{})
	envelope := types.NewEnvelope(types.EnvelopeLevel3)
	envelope.Level3Data.Symbol = "TEST/USD"
	index := 0
	b.ReportAllocs()

	for b.Loop() {
		index++
		measurement.PutMetric(data.Metric[float64]{Label: "value", Raw: float64(index % 2)})

		if err := agent.Grid.Step([]*data.Measurement[float64]{measurement}); err != nil {
			b.Fatal(err)
		}
		agent.Step(envelope)

		if err := agent.Error(); err != nil {
			b.Fatal(err)
		}
	}
}
