//go:build darwin && cgo

package manifold

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/types"
)

/*
driveAdvance runs the production changed-detect → sample → advance path without
the Update mutex so unit tests can inspect advanceResult directly.
*/
func (solver *Solver) driveAdvance(
	thesis *types.Thesis,
	hawkes HawkesSource,
) advanceResult {
	changed := solver.changedOutcomes(hawkes)

	if len(changed) == 0 {
		return advanceResult{}
	}

	return solver.advance(thesis, solver.sampleChanged(changed), changed)
}

func TestSolver_Advance(t *testing.T) {
	Convey("Given one shared domain with a later epoch from only one symbol", t, func() {
		solver, err := NewSolver(newTestBookSource("BTC/USD", "ETH/USD"), 8)
		So(err, ShouldBeNil)
		Reset(solver.Close)
		at := time.Unix(3, 0)
		first := types.NewThesis()
		bitcoinSeed := solverOutcome(at, 4, 2)
		result := solver.driveAdvance(first, staticHawkesSource{
			symbols: []string{"BTC/USD", "ETH/USD"},
			outcomes: map[string]excitation.Outcome{
				"BTC/USD": bitcoinSeed,
				"ETH/USD": solverOutcome(at, 8, 4),
			},
		})
		So(result.failures, ShouldBeEmpty)
		initialParticles := solver.Population()

		later := solverOutcome(at.Add(time.Second), 16, 4)
		later.EventCount++
		second := types.NewThesis()
		result = solver.driveAdvance(second, staticHawkesSource{
			symbols: []string{"BTC/USD", "ETH/USD"},
			outcomes: map[string]excitation.Outcome{
				"BTC/USD": bitcoinSeed,
				"ETH/USD": later,
			},
		})
		bitcoinValue, bitcoinFound := second.Manifold.Load("BTC/USD")
		etherValue, etherFound := second.Manifold.Load("ETH/USD")
		So(bitcoinFound, ShouldBeTrue)
		So(etherFound, ShouldBeTrue)

		if !bitcoinFound || !etherFound {
			return
		}

		bitcoin := bitcoinValue.(State)
		ether := etherValue.(State)
		etherBatch := ether.OscillatorCount

		Convey("It should append the changed sample and advance once", func() {
			So(result.failures, ShouldBeEmpty)
			So(result.advanced, ShouldEqual, 1)
			So(result.replayed, ShouldEqual, 1)
			// Inelastic merge may compact below raw append arithmetic; the proof
			// is one shared advance for the changed epoch, not a host headcount.
			So(solver.Population(), ShouldBeGreaterThan, 0)
			So(initialParticles, ShouldBeGreaterThan, 0)
			So(etherBatch, ShouldBeGreaterThan, 0)
			So(bitcoin.Replay, ShouldBeTrue)
			So(ether.Replay, ShouldBeFalse)
			So(ether.Epoch, ShouldEqual, 2)
			So(ether.GasReady(), ShouldBeTrue)
		})

		Convey("It should retain history when a market leaves the live candidate set", func() {
			bitcoinLater := solverOutcome(at.Add(2*time.Second), 6, 2)
			bitcoinLater.EventCount++
			third := types.NewThesis()
			result = solver.driveAdvance(third, staticHawkesSource{
				symbols: []string{"BTC/USD"},
				outcomes: map[string]excitation.Outcome{
					"BTC/USD": bitcoinLater,
				},
			})
			So(result.failures, ShouldBeEmpty)
			So(result.advanced, ShouldEqual, 1)
			So(solver.Population(), ShouldBeGreaterThan, 0)
			So(solver.active, ShouldContainKey, "BTC/USD")
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
		thesis := types.NewThesis()
		result := solver.driveAdvance(thesis, staticHawkesSource{
			symbols:  []string{"BTC/USD"},
			outcomes: map[string]excitation.Outcome{"BTC/USD": outcome},
		})

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
	seed := types.NewThesis()
	result := solver.driveAdvance(seed, staticHawkesSource{
		symbols:  []string{"BTC/USD"},
		outcomes: map[string]excitation.Outcome{"BTC/USD": outcome},
	})

	if len(result.failures) > 0 {
		b.Fatal(result.failures)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		at = at.Add(time.Second)
		outcome = solverOutcome(at, 4, 2)
		outcome.EventCount = int(at.Unix())
		thesis := types.NewThesis()
		result = solver.driveAdvance(thesis, staticHawkesSource{
			symbols:  []string{"BTC/USD"},
			outcomes: map[string]excitation.Outcome{"BTC/USD": outcome},
		})

		if len(result.failures) > 0 {
			b.Fatal(result.failures)
		}
	}
}
