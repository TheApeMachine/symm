//go:build darwin && cgo

package manifold

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/types"
)

func TestSolver_Advance(t *testing.T) {
	Convey("Given one shared domain with a later epoch from only one symbol", t, func() {
		solver, err := NewSolver(newTestBookSource("BTC/USD", "ETH/USD"), 8)
		So(err, ShouldBeNil)
		Reset(solver.Close)
		at := time.Unix(3, 0)
		first := types.NewThesis(nil)
		initial := solver.candidates(staticHawkesSource{
			symbols: []string{"BTC/USD", "ETH/USD"},
			outcomes: map[string]excitation.Outcome{
				"BTC/USD": solverOutcome(at, 4, 2),
				"ETH/USD": solverOutcome(at, 8, 4),
			},
		})
		result := solver.advance(first, initial)
		So(result.failures, ShouldBeEmpty)
		initialParticles := len(solver.particles)

		later := solverOutcome(at.Add(time.Second), 16, 4)
		later.EventCount++
		second := types.NewThesis(nil)
		result = solver.advance(second, solver.candidates(staticHawkesSource{
			symbols: []string{"BTC/USD", "ETH/USD"},
			outcomes: map[string]excitation.Outcome{
				"BTC/USD": initial[0].outcome,
				"ETH/USD": later,
			},
		}))
		bitcoinValue, bitcoinFound := second.Manifold.Load("BTC/USD")
		etherValue, etherFound := second.Manifold.Load("ETH/USD")
		So(bitcoinFound, ShouldBeTrue)
		So(etherFound, ShouldBeTrue)

		if !bitcoinFound || !etherFound {
			return
		}

		bitcoin := bitcoinValue.(State)
		ether := etherValue.(State)

		Convey("It should refresh the complete population and advance once", func() {
			So(result.failures, ShouldBeEmpty)
			So(result.advanced, ShouldEqual, 1)
			So(result.replayed, ShouldEqual, 1)
			So(len(solver.particles), ShouldEqual, initialParticles)
			So(bitcoin.Replay, ShouldBeTrue)
			So(ether.Replay, ShouldBeFalse)
			So(ether.Epoch, ShouldEqual, 2)
			So(ether.GasReady(), ShouldBeTrue)
		})

		Convey("It should remove a market that leaves the current universe", func() {
			bitcoinLater := solverOutcome(at.Add(2*time.Second), 6, 2)
			bitcoinLater.EventCount++
			third := types.NewThesis(nil)
			result = solver.advance(third, solver.candidates(staticHawkesSource{
				symbols: []string{"BTC/USD"},
				outcomes: map[string]excitation.Outcome{
					"BTC/USD": bitcoinLater,
				},
			}))
			So(result.failures, ShouldBeEmpty)
			So(solver.particles, ShouldHaveLength, 2)
			So(solver.active, ShouldHaveLength, 1)
		})
	})
}

func TestSymbolSlot_Preserve(t *testing.T) {
	Convey("Given a surviving order with evolved wave and thermal state", t, func() {
		slot := &symbolSlot{state: map[string]pfluid.Particle{
			"order": {Phase: 1.25, Heat: 0.4, Energy: 0.7},
		}}
		observation := pfluid.Particle{
			Position: pfluid.Vector{X: 0.75},
			Phase:    0.1,
			Energy:   1,
		}
		preserved := slot.preserve("order", observation)

		Convey("It should retain evolved state while accepting new geometry", func() {
			So(preserved.Position, ShouldResemble, observation.Position)
			So(preserved.Phase, ShouldEqual, float32(1.25))
			So(preserved.Heat, ShouldEqual, float32(0.4))
			So(preserved.Energy, ShouldEqual, float32(0.7))
		})
	})
}

func BenchmarkSolver_Advance(b *testing.B) {
	solver, err := NewSolver(newTestBookSource("BTC/USD"), 8)

	if err != nil {
		b.Fatal(err)
	}

	defer solver.Close()
	at := time.Unix(3, 0)
	b.ResetTimer()

	for b.Loop() {
		at = at.Add(time.Second)
		outcome := solverOutcome(at, 4, 2)
		outcome.EventCount = int(at.Unix())
		thesis := types.NewThesis(nil)
		result := solver.advance(thesis, solver.candidates(staticHawkesSource{
			symbols:  []string{"BTC/USD"},
			outcomes: map[string]excitation.Outcome{"BTC/USD": outcome},
		}))

		if len(result.failures) > 0 {
			b.Fatal(result.failures)
		}
	}
}

/*
BenchmarkSolver_PhaseProjection measures the focused production path that reads
the resident wave, scans retained phase history, and projects the shared field.
*/
func BenchmarkSolver_PhaseProjection(b *testing.B) {
	solver, err := NewSolver(newTestBookSource("BTC/USD"), 8)

	if err != nil {
		b.Fatal(err)
	}

	defer solver.Close()
	at := time.Unix(3, 0)
	outcome := solverOutcome(at, 4, 2)
	outcome.EventCount = 1
	seed := types.NewThesis(nil)
	result := solver.advance(seed, solver.candidates(staticHawkesSource{
		symbols:  []string{"BTC/USD"},
		outcomes: map[string]excitation.Outcome{"BTC/USD": outcome},
	}))

	if len(result.failures) > 0 {
		b.Fatal(result.failures)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		at = at.Add(time.Second)
		outcome = solverOutcome(at, 4, 2)
		outcome.EventCount = int(at.Unix())
		thesis := types.NewThesis(nil)
		result = solver.advance(thesis, solver.candidates(staticHawkesSource{
			symbols:  []string{"BTC/USD"},
			outcomes: map[string]excitation.Outcome{"BTC/USD": outcome},
		}))

		if len(result.failures) > 0 {
			b.Fatal(result.failures)
		}
	}
}
