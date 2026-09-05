package strategy

import (
	"context"
	"errors"
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
		agent, books := agentFixture(t, func(event hindsight.LearningEvent) error { events = append(events, event); return nil })
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
