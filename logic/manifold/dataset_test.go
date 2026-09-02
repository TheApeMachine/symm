package manifold

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/tests/market"
)

func datasetOrder(event string, price, quantity float64, at time.Time, id string) kraken.Level3Order {
	return kraken.Level3Order{
		Event:      event,
		OrderID:    id,
		LimitPrice: decimal.NewFromFloat64(price),
		OrderQty:   decimal.NewFromFloat64(quantity),
		Timestamp:  at,
	}
}

/*
collect drains a projection into a slice, returning the particles to the pool
the way the solver does so the pool contract is exercised too.
*/
func collect(dataset *Dataset, message kraken.Level3Data) []sensorium.State {
	states := []sensorium.State{}

	for state := range dataset.Step(message, forcingState{}) {
		copied := *state
		copied.Bytes = append([]int64(nil), state.Bytes...)
		copied.Seqs = append([]int64(nil), state.Seqs...)
		copied.TokenIDs = append([]int64(nil), state.TokenIDs...)
		copied.ContentIDs = append([]int64(nil), state.ContentIDs...)
		copied.Phase = append([]float32(nil), state.Phase...)
		copied.Omega = append([]float32(nil), state.Omega...)
		copied.Energy = append([]float32(nil), state.Energy...)
		copied.Mass = append([]float32(nil), state.Mass...)
		copied.Heat = append([]float32(nil), state.Heat...)
		copied.Amp = append([]float32(nil), state.Amp...)
		copied.Pos = append([]float32(nil), state.Pos...)
		copied.Vel = append([]float32(nil), state.Vel...)
		copied.Clamped = append([]bool(nil), state.Clamped...)
		copied.Dark = append([]bool(nil), state.Dark...)
		states = append(states, copied)
		sensorium.StatePool.Put(state)
	}

	return states
}

/*
TestDatasetStep pins the projection contract.

The rule that matters: Kraken sends Level-3 ONE SIDE AT A TIME. A projection
that needs both sides of a touch before it can place a particle places none at
all, because the two sides are never in the same message. Every assertion here
uses one-sided messages for that reason, and the tape replay below asserts the
whole stream produces particles rather than silence.
*/
/*
TestValidParticle pins the data-arrival gate: an order only becomes a particle
when every field it carries is determined. Zero, NaN and Inf seeds are dropped
before they reach the resident domain.
*/
func TestValidParticle(t *testing.T) {
	Convey("Given a particle whose energy would be non-positive or non-finite", t, func() {
		Convey("A zero-energy particle is rejected", func() {
			state := sensorium.State{
				N:      1,
				Energy: []float32{0},
				Mass:   []float32{0},
				Amp:    []float32{0},
				Phase:  []float32{1},
				Omega:  []float32{1},
				Pos:    []float32{0.5, 0.5, 0.5},
				Vel:    []float32{0, 0, 0},
				Heat:   []float32{0},
			}

			So(validParticle(&state), ShouldBeFalse)
		})

		Convey("A NaN energy is rejected", func() {
			state := sensorium.State{
				N:      1,
				Energy: []float32{float32(math.NaN())},
				Mass:   []float32{1},
				Amp:    []float32{1},
				Phase:  []float32{1},
				Omega:  []float32{1},
				Pos:    []float32{0.5, 0.5, 0.5},
				Vel:    []float32{0, 0, 0},
				Heat:   []float32{0},
			}

			So(validParticle(&state), ShouldBeFalse)
		})

		Convey("An infinite energy is rejected", func() {
			state := sensorium.State{
				N:      1,
				Energy: []float32{float32(math.Inf(1))},
				Mass:   []float32{1},
				Amp:    []float32{1},
				Phase:  []float32{1},
				Omega:  []float32{1},
				Pos:    []float32{0.5, 0.5, 0.5},
				Vel:    []float32{0, 0, 0},
				Heat:   []float32{0},
			}

			So(validParticle(&state), ShouldBeFalse)
		})

		Convey("A NaN coordinate is rejected", func() {
			state := sensorium.State{
				N:      1,
				Energy: []float32{1},
				Mass:   []float32{1},
				Amp:    []float32{1},
				Phase:  []float32{1},
				Omega:  []float32{1},
				Pos:    []float32{float32(math.NaN()), 0.5, 0.5},
				Vel:    []float32{0, 0, 0},
				Heat:   []float32{0},
			}

			So(validParticle(&state), ShouldBeFalse)
		})
	})

	Convey("Given a fully determined particle", t, func() {
		state := sensorium.State{
			N:      1,
			Energy: []float32{1},
			Mass:   []float32{1},
			Amp:    []float32{1},
			Phase:  []float32{1},
			Omega:  []float32{1},
			Pos:    []float32{0.5, 0.5, 0.5},
			Vel:    []float32{0, 0, 0},
			Heat:   []float32{0},
		}

		Convey("It passes the gate", func() {
			So(validParticle(&state), ShouldBeTrue)
		})
	})
}

func TestDatasetStep(t *testing.T) {
	Convey("Given one one-sided Level-3 message", t, func() {
		dataset := NewDataset()
		at := time.Unix(1_700_000_000, 0)

		Convey("Its resting orders each become a particle", func() {
			states := collect(dataset, kraken.Level3Data{
				Symbol:    "BTC/USD",
				Timestamp: at,
				Bids: []kraken.Level3Order{
					datasetOrder("add", 99.0, 10, at, "b1"),
					datasetOrder("add", 98.5, 5, at, "b2"),
				},
			})

			So(len(states), ShouldEqual, 2)

			// Mass is what the particle deposits as density, what scales the
			// heat it gathers, and what divides its pilot-wave guidance, so it
			// tracks the order's energy rather than a shared constant.
			Convey("carrying mass equal to its own energy", func() {
				for _, state := range states {
					So(state.Mass[0], ShouldEqual, state.Energy[0])
					So(state.Mass[0], ShouldBeGreaterThan, 0)
					So(state.N, ShouldEqual, 1)
				}
			})

			// Heat is earned from the gas in planckExchange; a projected order
			// arrives with none rather than an invented allowance.
			Convey("and no heat of its own", func() {
				for _, state := range states {
					So(state.Heat[0], ShouldEqual, 0)
				}
			})

			Convey("with amplitude the square root of energy", func() {
				for _, state := range states {
					So(float64(state.Amp[0]), ShouldAlmostEqual,
						math.Sqrt(float64(state.Energy[0])), 1e-6)
				}
			})

			Convey("and every coordinate finite", func() {
				for _, state := range states {
					for axis := 0; axis < 3; axis++ {
						So(math.IsNaN(float64(state.Pos[axis])), ShouldBeFalse)
						So(math.IsInf(float64(state.Pos[axis]), 0), ShouldBeFalse)
					}

					So(math.IsNaN(float64(state.Omega[0])), ShouldBeFalse)
				}
			})
		})

		Convey("A withdrawn order rests nowhere and projects no particle", func() {
			states := collect(dataset, kraken.Level3Data{
				Symbol:    "BTC/USD",
				Timestamp: at,
				Bids: []kraken.Level3Order{
					datasetOrder("add", 99.0, 10, at, "b1"),
					datasetOrder("delete", 98.5, 5, at, "b2"),
				},
			})

			So(len(states), ShouldEqual, 1)
		})

		Convey("A message of only withdrawals projects nothing", func() {
			states := collect(dataset, kraken.Level3Data{
				Symbol:    "BTC/USD",
				Timestamp: at,
				Bids:      []kraken.Level3Order{datasetOrder("delete", 99.0, 10, at, "b1")},
			})

			So(states, ShouldBeEmpty)
		})

		Convey("A lone resting order still projects, inside the domain", func() {
			states := collect(dataset, kraken.Level3Data{
				Symbol:    "BTC/USD",
				Timestamp: at,
				Bids:      []kraken.Level3Order{datasetOrder("add", 99.0, 10, at, "b1")},
			})

			So(len(states), ShouldEqual, 1)

			// The domain is periodic, so a coordinate outside it wraps onto a
			// face. Every projected order has to land strictly inside.
			So(states[0].Pos[0], ShouldBeGreaterThanOrEqualTo, float32(0))
			So(states[0].Pos[0], ShouldBeLessThanOrEqualTo, float32(1))
			So(states[0].Pos[1], ShouldBeGreaterThanOrEqualTo, float32(0))
			So(states[0].Pos[1], ShouldBeLessThanOrEqualTo, float32(1))
		})
	})

	Convey("Given orders spread across prices", t, func() {
		dataset := NewDataset()
		at := time.Unix(1_700_000_000, 0)

		states := collect(dataset, kraken.Level3Data{
			Symbol:    "BTC/USD",
			Timestamp: at,
			Bids: []kraken.Level3Order{
				datasetOrder("add", 100.0, 1, at, "b1"),
				datasetOrder("add", 99.0, 1, at, "b2"),
				datasetOrder("add", 98.0, 1, at, "b3"),
			},
		})

		So(len(states), ShouldEqual, 3)

		Convey("Every order lands inside the periodic domain", func() {
			for _, state := range states {
				So(state.Pos[0], ShouldBeGreaterThanOrEqualTo, float32(0))
				So(state.Pos[0], ShouldBeLessThanOrEqualTo, float32(1))
				So(state.Pos[1], ShouldBeGreaterThanOrEqualTo, float32(0))
				So(state.Pos[1], ShouldBeLessThanOrEqualTo, float32(1))
			}
		})

		Convey("The price axis orders them as the prices order", func() {
			// The resident frame scores against prior moments, so it has a
			// spread to measure against only once it has seen more than one
			// order. The ordering is asserted where the frame is warm.
			So(states[1].Pos[0], ShouldBeGreaterThan, states[2].Pos[0])
		})

		Convey("Frequency follows the same signed deviation", func() {
			So(states[1].Omega[0], ShouldBeGreaterThan, states[2].Omega[0])

			// tanh bounds it, so no order can run away with the spectrum.
			for _, state := range states {
				So(math.Abs(float64(state.Omega[0])), ShouldBeLessThanOrEqualTo, omegaHalfSpan)
			}
		})
	})

	Convey("Given bids and asks in one message", t, func() {
		dataset := NewDataset()
		at := time.Unix(1_700_000_000, 0)

		states := collect(dataset, kraken.Level3Data{
			Symbol:    "BTC/USD",
			Timestamp: at,
			Bids:      []kraken.Level3Order{datasetOrder("add", 99.0, 1, at, "b1")},
			Asks:      []kraken.Level3Order{datasetOrder("add", 101.0, 1, at, "a1")},
		})

		So(len(states), ShouldEqual, 2)

		Convey("The spread is a phase boundary: bids below pi, asks above", func() {
			bidPhase, askPhase := float32(0), float32(0)

			for _, state := range states {
				if state.TokenIDs[0]&1 == 1 {
					askPhase = state.Phase[0]

					continue
				}

				bidPhase = state.Phase[0]
			}

			So(float64(bidPhase), ShouldBeLessThan, math.Pi)
			So(float64(askPhase), ShouldBeGreaterThanOrEqualTo, math.Pi)
		})
	})

	Convey("Given the shared Level-3 tape", t, func() {
		tape := market.NewLevel3Tape("BTC/USD", time.Unix(1_700_000_000, 0))
		dataset := NewDataset()
		total := 0

		for _, message := range tape.Messages {
			total += len(collect(dataset, message))
		}

		Convey("A one-sided feed produces particles rather than silence", func() {
			// The touch-based projection this replaced yielded ZERO particles
			// across this entire tape, because no single message ever carried
			// both sides of the book it was waiting for.
			So(total, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a long churn tape", t, func() {
		tape := market.NewLevel3ChurnTape("BTC/USD", time.Unix(1_700_000_000, 0), 300)
		dataset := NewDataset()
		total := 0

		for _, message := range tape.Messages {
			for _, state := range collect(dataset, message) {
				total++

				for axis := 0; axis < 3; axis++ {
					So(math.IsNaN(float64(state.Pos[axis])), ShouldBeFalse)
				}
			}
		}

		Convey("Every message projects finite particles", func() {
			So(total, ShouldBeGreaterThan, 0)
		})
	})
}

/*
TestSymbolToken pins that a symbol's particle-token space is derived from the
symbol itself, so two symbols cannot silently share one token space.
*/
func TestSymbolToken(t *testing.T) {
	Convey("Given distinct symbols", t, func() {
		So(symbolToken("BTC/USD"), ShouldNotEqual, symbolToken("ETH/USD"))

		Convey("The same symbol always resolves to the same token", func() {
			So(symbolToken("BTC/USD"), ShouldEqual, symbolToken("BTC/USD"))
		})

		Convey("Every token fits the packed index space", func() {
			So(symbolToken("BTC/USD"), ShouldBeLessThanOrEqualTo, symbolIndexMask)
		})
	})
}

func BenchmarkDatasetStep(b *testing.B) {
	tape := market.NewLevel3ChurnTape(
		"BTC/USD",
		time.Unix(1_700_000_000, 0),
		300,
	)
	dataset := NewDataset()

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		message := tape.Messages[iteration%len(tape.Messages)]

		for state := range dataset.Step(message, forcingState{}) {
			sensorium.StatePool.Put(state)
		}
	}
}
