//go:build darwin && cgo

package manifold

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/symm/types"
)

/*
staticHawkesSource exposes deterministic market epochs to shared-domain tests.
*/
type staticHawkesSource struct {
	symbols  []string
	outcomes map[string]excitation.Outcome
}

/*
Symbols returns every configured test market.
*/
func (source staticHawkesSource) Symbols() []string {
	return source.symbols
}

/*
Outcome returns one configured empirical arrival process.
*/
func (source staticHawkesSource) Outcome(symbol string) (excitation.Outcome, bool) {
	outcome, found := source.outcomes[symbol]
	return outcome, found
}

func TestSolver_Update(t *testing.T) {
	Convey("Given two booked markets entering one shared domain", t, func() {
		solver, err := NewSolver(newTestBookSource("BTC/USD", "ETH/USD"), 8)
		So(err, ShouldBeNil)
		Reset(solver.Close)
		at := time.Unix(3, 0)
		source := staticHawkesSource{
			symbols: []string{"ETH/USD", "BTC/USD"},
			outcomes: map[string]excitation.Outcome{
				"BTC/USD": solverOutcome(at, 4, 2),
				"ETH/USD": solverOutcome(at, 8, 4),
			},
		}
		thesis := types.NewThesis()
		err = solver.Update(thesis, source)
		So(err, ShouldBeNil)

		if err != nil {
			return
		}

		bitcoinValue, bitcoinFound := thesis.Manifold.Load("BTC/USD")
		etherValue, etherFound := thesis.Manifold.Load("ETH/USD")
		So(bitcoinFound, ShouldBeTrue)
		So(etherFound, ShouldBeTrue)

		if !bitcoinFound || !etherFound {
			return
		}

		bitcoin := bitcoinValue.(State)
		ether := etherValue.(State)

		Convey("It should advance both symbols through the same physical reading", func() {
			So(bitcoin.GasReady(), ShouldBeTrue)
			So(ether.GasReady(), ShouldBeTrue)
			So(bitcoin.Reading, ShouldResemble, ether.Reading)
			So(solver.Population(), ShouldBeGreaterThan, 0)
			So(solver.domain, ShouldNotBeNil)
		})

		Convey("It should project the shared field onto every GasReady symbol", func() {
			population := solver.Population()
			So(bitcoin.Rho, ShouldNotBeEmpty)
			So(bitcoin.PsiMag2, ShouldNotBeEmpty)
			So(bitcoin.Particles, ShouldHaveLength, population)
			So(bitcoin.OscillatorCount, ShouldEqual, population)
			So(bitcoin.SharedOscillatorCount, ShouldEqual, population)
			So(bitcoin.Wave, ShouldHaveLength, solver.config.Grid.X)
			So(bitcoin.PhaseReady, ShouldBeFalse)
			So(bitcoin.PhaseReason, ShouldEqual,
				"awaiting a prior outcome-labeled phase observation")
			So(ether.Rho, ShouldNotBeEmpty)
			So(ether.Particles, ShouldHaveLength, population)
			So(ether.SharedOscillatorCount, ShouldEqual, population)
			So(ether.Wave, ShouldNotBeEmpty)
		})

		Convey("It should scan the current phase dial against prior resident waves", func() {
			So(solver.CommitPhase(types.Cognition{
				Symbol:     "BTC/USD",
				At:         at,
				Winner:     "buy",
				Ready:      true,
				Confidence: 0.75,
				Cohort:     4,
			}), ShouldBeNil)
			laterAt := at.Add(time.Second)
			source.outcomes["BTC/USD"] = solverOutcome(laterAt, 16, 2)
			source.outcomes["ETH/USD"] = solverOutcome(laterAt, 4, 8)
			next := types.NewThesis()
			So(solver.Update(next, source), ShouldBeNil)
			So(solver.Population(), ShouldBeGreaterThan, 0)
			value, found := next.Manifold.Load("BTC/USD")
			So(found, ShouldBeTrue)

			if !found {
				return
			}

			state := value.(State)
			So(state.Wave, ShouldHaveLength, solver.config.Grid.X)
			So(state.PhaseReady, ShouldBeTrue)
			So(state.PhaseReason, ShouldBeEmpty)
			So(state.PhaseScan, ShouldHaveLength, len(state.Wave))

			for _, response := range state.PhaseScan {
				So(math.IsNaN(response.Similarity), ShouldBeFalse)
				So(math.IsInf(response.Similarity, 0), ShouldBeFalse)
				So(response.ObservedAt, ShouldEqual, at)
				So(response.Outcome, ShouldResemble, PhaseOutcome{
					Symbol:     "BTC/USD",
					Class:      "buy",
					Confidence: 0.75,
					Cohort:     4,
				})
			}
		})

		Convey("It should replay unchanged source epochs without appending particles", func() {
			before := solver.Population()
			next := types.NewThesis()
			So(solver.Update(next, source), ShouldBeNil)
			value, found := next.Manifold.Load("ETH/USD")
			replay := value.(State)
			So(found, ShouldBeTrue)
			So(replay.Replay, ShouldBeTrue)
			So(replay.Epoch, ShouldEqual, ether.Epoch)
			So(replay.Rho, ShouldNotBeEmpty)
			So(replay.Particles, ShouldHaveLength, before)
			So(replay.Wave, ShouldNotBeEmpty)
			So(solver.Population(), ShouldEqual, before)
		})
	})
}

func TestSolver_Candidates(t *testing.T) {
	Convey("Given intensity for a market without an authoritative L3 book", t, func() {
		solver, err := NewSolver(newTestBookSource("BTC/USD"), 8)
		So(err, ShouldBeNil)
		Reset(solver.Close)
		source := staticHawkesSource{
			symbols: []string{"ETH/USD"},
			outcomes: map[string]excitation.Outcome{
				"ETH/USD": solverOutcome(time.Unix(3, 0), 4, 2),
			},
		}

		Convey("It should not invent a physical population", func() {
			changed := solver.changedOutcomes(source)
			So(changed, ShouldContainKey, "ETH/USD")
			So(solver.sampleChanged(changed), ShouldBeEmpty)
		})
	})
}

func solverOutcome(at time.Time, buyRate, sellRate float64) excitation.Outcome {
	return excitation.Outcome{
		At:              at,
		Horizon:         time.Second,
		EventCount:      8,
		BuyArrivalRate:  buyRate,
		SellArrivalRate: sellRate,
		Maturity:        0.75,
		Readiness: excitation.Readiness{
			Observation: true,
			Intensity:   true,
			HawkesFit:   true,
		},
		Fit: hawkes.BivariateFit{
			MuX:            buyRate,
			MuY:            sellRate,
			AlphaXX:        buyRate * 0.1,
			AlphaYY:        sellRate * 0.1,
			AlphaXY:        buyRate * 0.01,
			AlphaYX:        sellRate * 0.01,
			Beta:           2,
			IntensityX:     buyRate,
			IntensityY:     sellRate,
			SpectralRadius: 0.35,
		},
	}
}
