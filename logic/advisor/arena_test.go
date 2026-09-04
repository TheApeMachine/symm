package advisor

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

type arenaNode struct {
	calls        int
	perspectives []*types.Perspective
}

func (node *arenaNode) Step(envelope *types.Envelope) *types.Envelope {
	node.calls++
	node.perspectives = envelope.Perspectives

	return envelope
}

func TestNewArena(t *testing.T) {
	Convey("Given a named Advisor, its Features, and a wrapped Node", t, func() {
		arena, err := NewArena("midpoint", predictiveMidpointFeatures(), &arenaNode{}, 2)

		Convey("Arena constructs with an empty bounded round set", func() {
			So(err, ShouldBeNil)
			So(arena.Active(), ShouldEqual, 0)
		})
	})

	Convey("Given an invalid capacity", t, func() {
		arena, err := NewArena("midpoint", predictiveMidpointFeatures(), &arenaNode{}, 0)

		Convey("Arena refuses an unbounded configuration", func() {
			So(err, ShouldNotBeNil)
			So(errnie.IsValidation(err), ShouldBeTrue)
			So(arena, ShouldBeNil)
		})
	})
}

func TestArenaStep(t *testing.T) {
	Convey("Given an issued recovery Perspective", t, func() {
		node := &arenaNode{}
		arena, err := NewArena("midpoint", predictiveMidpointFeatures(), node, 2)
		So(err, ShouldBeNil)
		issued := arenaEnvelope("BTC/USD", 1, 1, 1)
		issued.Perspectives = []*types.Perspective{arenaPerspective("BTC/USD", "recovery", 1, 3)}

		Convey("Arena publishes it immediately and tracks it for accountability", func() {
			// The advisor speaks now and is judged later (MCTS.md §3, §6).
			// Withholding the reading until it had survived a round left the
			// War Room permanently empty on a live symbol.
			So(arena.Step(issued), ShouldEqual, issued)
			So(arena.Active(), ShouldEqual, 1)
			So(node.perspectives, ShouldHaveLength, 1)
			So(node.perspectives[0].Lifecycle, ShouldEqual, types.PerspectiveIssued)
		})

		Convey("support releases the Perspective to the wrapped Node", func() {
			So(arena.Step(issued), ShouldNotBeNil)
			support := arenaEnvelope("BTC/USD", 1.2, 1, 2)

			So(arena.Step(support), ShouldEqual, support)
			So(arena.Active(), ShouldEqual, 0)
			So(node.perspectives, ShouldHaveLength, 1)
			So(node.perspectives[0].Lifecycle, ShouldEqual, types.PerspectiveSurvived)
			So(node.perspectives[0].ResolvedBy, ShouldEqual,
				types.PerspectiveEvent("pumpdump/positive_midpoint_return"))
			So(node.perspectives[0].ResolvedCoordinate, ShouldEqual, uint64(2))
			So(node.perspectives[0].Support, ShouldEqual, uint64(1))
			So(node.perspectives[0].Maturity(), ShouldEqual, 0.0)

			next := arenaEnvelope("BTC/USD", 1, 1, 3)
			next.Perspectives = []*types.Perspective{
				arenaPerspective("BTC/USD", "recovery", 3, 5),
			}
			So(arena.Step(next), ShouldNotBeNil)
			secondSupport := arenaEnvelope("BTC/USD", 1.2, 1, 4)

			So(arena.Step(secondSupport), ShouldNotBeNil)
			So(node.perspectives, ShouldHaveLength, 1)
			So(node.perspectives[0].Support, ShouldEqual, uint64(2))
			So(node.perspectives[0].Maturity(), ShouldEqual, 0.5)
		})

		Convey("contradiction evicts it without exposing it to the wrapped Node", func() {
			So(arena.Step(issued), ShouldNotBeNil)
			contradiction := arenaEnvelope("BTC/USD", 1, 1.6, 2)

			So(arena.Step(contradiction), ShouldEqual, contradiction)
			So(arena.Active(), ShouldEqual, 0)
			So(node.perspectives, ShouldBeEmpty)
		})

		Convey("expiry evicts it on its declared adaptive clock", func() {
			So(arena.Step(issued), ShouldNotBeNil)
			So(arena.Step(arenaEnvelope("BTC/USD", 1, 1, 2)), ShouldNotBeNil)
			expired := arenaEnvelope("BTC/USD", 1, 1, 3)

			So(arena.Step(expired), ShouldEqual, expired)
			So(arena.Active(), ShouldEqual, 0)
			So(node.perspectives, ShouldBeEmpty)
		})

		Convey("a non-winning support event remains counterfactual evidence", func() {
			So(arena.Step(issued), ShouldNotBeNil)
			counterfactual := arenaEnvelope("BTC/USD", 1, 1.2, 2)

			So(arena.Step(counterfactual), ShouldEqual, counterfactual)
			So(arena.Active(), ShouldEqual, 1)
			So(node.perspectives, ShouldBeEmpty)
		})
	})

	Convey("Given a full Arena", t, func() {
		arena, err := NewArena("midpoint", predictiveMidpointFeatures(), &arenaNode{}, 1)
		So(err, ShouldBeNil)
		first := arenaEnvelope("BTC/USD", 1, 1, 1)
		first.Perspectives = []*types.Perspective{arenaPerspective("BTC/USD", "recovery", 1, 3)}
		So(arena.Step(first), ShouldNotBeNil)
		second := arenaEnvelope("ETH/USD", 1, 1, 1)
		second.Perspectives = []*types.Perspective{arenaPerspective("ETH/USD", "recovery", 1, 3)}

		Convey("a new structural key is refused instead of evicting another round", func() {
			// The refusal is logged and counted, but it does not kill the
			// Arena: capacity pressure is a normal operating condition, and
			// latching it as terminal silenced the advisor and everything
			// downstream of it for the rest of the process.
			So(arena.Step(second), ShouldEqual, second)
			So(arena.Error(), ShouldBeNil)
			So(arena.Active(), ShouldEqual, 1)
		})

		Convey("the Arena keeps serving the stream afterwards", func() {
			So(arena.Step(second), ShouldNotBeNil)
			third := arenaEnvelope("BTC/USD", 1, 1, 2)

			So(arena.Step(third), ShouldEqual, third)
			So(arena.Error(), ShouldBeNil)
		})
	})
}

func TestArenaTiedPerspective(t *testing.T) {
	Convey("Given a fully booted Arena and a tied distribution", t, func() {
		node := &arenaNode{}
		arena, err := NewArena("midpoint", predictiveMidpointFeatures(), node, 2)
		So(err, ShouldBeNil)

		tied := arenaEnvelope("BTC/USD", 1, 1, 1)
		perspective := arenaPerspective("BTC/USD", "recovery", 1, 3)
		perspective.Classes[0].Probability = 0.5
		perspective.Classes[1].Probability = 0.5
		tied.Perspectives = []*types.Perspective{perspective}

		Convey("the tie is refused without killing the Arena", func() {
			So(arena.Step(tied), ShouldEqual, tied)
			So(arena.Error(), ShouldBeNil)
			So(arena.Active(), ShouldEqual, 0)
		})

		Convey("a later decisive Perspective is still admitted and published", func() {
			// This is the regression that mattered: one uninformative frame
			// used to latch arena.err, so every following envelope returned
			// nil and no decision ever reached the frontend again.
			So(arena.Step(tied), ShouldNotBeNil)

			decisive := arenaEnvelope("BTC/USD", 1, 1, 2)
			decisive.Perspectives = []*types.Perspective{
				arenaPerspective("BTC/USD", "recovery", 2, 4),
			}

			So(arena.Step(decisive), ShouldEqual, decisive)
			So(arena.Error(), ShouldBeNil)
			So(arena.Active(), ShouldEqual, 1)
			So(node.perspectives, ShouldHaveLength, 1)
			So(node.perspectives[0].Lifecycle, ShouldEqual, types.PerspectiveIssued)
		})

		Convey("classes separated only by float noise count as tied", func() {
			noisy := arenaEnvelope("ETH/USD", 1, 1, 1)
			near := arenaPerspective("ETH/USD", "recovery", 1, 3)
			near.Classes[0].Probability = 0.5
			near.Classes[1].Probability = 0.5 + 1e-15
			noisy.Perspectives = []*types.Perspective{near}

			So(arena.Step(noisy), ShouldEqual, noisy)
			So(arena.Error(), ShouldBeNil)
			So(arena.Active(), ShouldEqual, 0)
		})

		Convey("a genuine separation is still admitted", func() {
			clear := arenaEnvelope("ETH/USD", 1, 1, 1)
			clear.Perspectives = []*types.Perspective{
				arenaPerspective("ETH/USD", "breakdown", 1, 3),
			}

			So(arena.Step(clear), ShouldEqual, clear)
			So(arena.Error(), ShouldBeNil)
			So(arena.Active(), ShouldEqual, 1)
		})
	})
}

func TestArenaBootGate(t *testing.T) {
	Convey("Given an Arena gated on a system that has not finished booting", t, func() {
		node := &arenaNode{}
		arena, err := NewArena("midpoint", predictiveMidpointFeatures(), node, 2)
		So(err, ShouldBeNil)

		booted := false
		arena.Booted(func() bool { return booted })

		issued := arenaEnvelope("BTC/USD", 1, 1, 1)
		issued.Perspectives = []*types.Perspective{
			arenaPerspective("BTC/USD", "recovery", 1, 3),
		}

		Convey("no Perspective is issued and no round is opened", func() {
			So(arena.Step(issued), ShouldEqual, issued)
			So(arena.Active(), ShouldEqual, 0)
			So(node.perspectives, ShouldBeEmpty)
		})

		Convey("market data still reaches the wrapped Node", func() {
			So(arena.Step(issued), ShouldNotBeNil)
			So(node.calls, ShouldEqual, 1)
		})

		Convey("a tied distribution during boot does not fail the Arena", func() {
			// A cold classifier yields a uniform distribution, whose classes
			// carry bit-identical probabilities. That reached winningClass and
			// latched a terminal error, silencing the advisor for the whole
			// process. Behind the gate it is simply never admitted.
			tied := arenaEnvelope("BTC/USD", 1, 1, 1)
			perspective := arenaPerspective("BTC/USD", "recovery", 1, 3)
			perspective.Classes[0].Probability = 0.5
			perspective.Classes[1].Probability = 0.5
			tied.Perspectives = []*types.Perspective{perspective}

			So(arena.Step(tied), ShouldEqual, tied)
			So(arena.Error(), ShouldBeNil)
			So(arena.Active(), ShouldEqual, 0)
		})

		Convey("once booted the Arena admits and publishes normally", func() {
			So(arena.Step(issued), ShouldNotBeNil)
			So(arena.Active(), ShouldEqual, 0)

			booted = true
			later := arenaEnvelope("BTC/USD", 1, 1, 1)
			later.Perspectives = []*types.Perspective{
				arenaPerspective("BTC/USD", "recovery", 1, 3),
			}

			So(arena.Step(later), ShouldEqual, later)
			So(arena.Active(), ShouldEqual, 1)
			So(node.perspectives, ShouldHaveLength, 1)
			So(node.perspectives[0].Lifecycle, ShouldEqual, types.PerspectiveIssued)
		})
	})

	Convey("Given an Arena with no boot gate attached", t, func() {
		node := &arenaNode{}
		arena, err := NewArena("midpoint", predictiveMidpointFeatures(), node, 2)
		So(err, ShouldBeNil)
		issued := arenaEnvelope("BTC/USD", 1, 1, 1)
		issued.Perspectives = []*types.Perspective{
			arenaPerspective("BTC/USD", "recovery", 1, 3),
		}

		Convey("it runs unguarded", func() {
			So(arena.Step(issued), ShouldNotBeNil)
			So(arena.Active(), ShouldEqual, 1)
		})
	})
}

func arenaPerspective(
	symbol string,
	winner types.PerspectiveState,
	from uint64,
	until uint64,
) *types.Perspective {
	classes := []types.PerspectiveClass{
		{State: "recovery", Probability: 0.8},
		{State: "breakdown", Probability: 0.2},
	}

	if winner == "breakdown" {
		classes[0].Probability = 0.2
		classes[1].Probability = 0.8
	}

	return &types.Perspective{
		Symbol:   symbol,
		Advisor:  "midpoint",
		Question: "midpoint",
		Classes:  classes,
		Predictions: []types.PerspectivePrediction{
			{Class: "recovery", Event: "pumpdump/positive_midpoint_return", Effect: types.PredictionSupports},
			{Class: "recovery", Event: "pumpdump/negative_midpoint_return", Effect: types.PredictionFalsifies},
			{Class: "breakdown", Event: "pumpdump/negative_midpoint_return", Effect: types.PredictionSupports},
			{Class: "breakdown", Event: "pumpdump/positive_midpoint_return", Effect: types.PredictionFalsifies},
		},
		Lease: types.PerspectiveLease{
			Clock: "pumpdump/completed_volume_bar_ordinal",
			From:  from,
			Until: until,
		},
		Lifecycle: types.PerspectiveIssued,
	}
}

func arenaEnvelope(
	symbol string,
	positive float64,
	negative float64,
	ordinal uint64,
) *types.Envelope {
	at := time.Unix(1_700_000_000+int64(ordinal), 0)
	measurement := data.NewMeasurement[float64](
		"pumpdump:"+symbol,
		symbol,
		"pumpdump",
		at,
		at,
	)
	measurement.PutMetric(data.NewMetric(
		"completed_volume_bar_ordinal",
		float64(ordinal),
		nil,
		nil,
		data.UnitCount,
		data.TimescaleInstantaneous,
	))
	measurement.PutMetric(data.NewMetric(
		"positive_midpoint_return",
		positive,
		nil,
		nil,
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
	))
	measurement.PutMetric(data.NewMetric(
		"negative_midpoint_return",
		negative,
		nil,
		nil,
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
	))

	envelope := types.NewEnvelope(types.EnvelopeTrade)
	envelope.TradeData.Symbol = symbol
	envelope.TradeData.Timestamp = at
	envelope.PumpDump = measurement

	return envelope
}

func TestArenaDirectionalCredibilityScoring(t *testing.T) {
	Convey("Given an Arena wired to a WarRoom court", t, func() {
		node := &arenaNode{}
		room := NewWarRoom()
		arena, err := NewArena("momentum", predictiveMidpointFeatures(), node, 4)
		So(err, ShouldBeNil)
		arena.Court(room)

		issued := arenaEnvelope("BTC/USD", 1, 1, 1)
		issued.TradeData.Price = *decimal.NewFromFloat64(100.0)
		perspective := arenaPerspective("BTC/USD", "recovery", 1, 3)
		issued.Perspectives = []*types.Perspective{perspective}
		So(arena.Step(issued), ShouldNotBeNil)

		Convey("when the claim resolves with positive price return", func() {
			support := arenaEnvelope("BTC/USD", 1.5, 1, 2)
			support.TradeData.Price = *decimal.NewFromFloat64(103.0)
			So(arena.Step(support), ShouldNotBeNil)

			Convey("the advisor credibility reflects the directional market outcome", func() {
				So(room.Credibility("momentum"), ShouldBeGreaterThanOrEqualTo, 1.0)
			})
		})
	})
}

func BenchmarkArenaStep(b *testing.B) {
	node := &arenaNode{}
	arena, err := NewArena("midpoint", predictiveMidpointFeatures(), node, 1)

	if err != nil {
		b.Fatal(err)
	}

	issued := arenaEnvelope("BTC/USD", 1, 1, 1)
	issued.Perspectives = []*types.Perspective{arenaPerspective("BTC/USD", "recovery", 1, ^uint64(0))}

	if arena.Step(issued) == nil {
		b.Fatal(arena.Error())
	}

	envelope := arenaEnvelope("BTC/USD", 1, 1, 1)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if arena.Step(envelope) == nil {
			b.Fatal(arena.Error())
		}
	}
}
