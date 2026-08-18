package manifold

import (
	"math"
	"testing"
	"time"

	mgrbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/geometry"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/types"
)

func TestRealizedDirection(t *testing.T) {
	Convey("Given a book scale that distinguishes signal from noise", t, func() {
		Convey("It should label a move inside the scale as flat", func() {
			So(realizedDirection(0.005, 0.01), ShouldEqual, "flat")
		})

		Convey("It should label a lift that clears the scale as up", func() {
			So(realizedDirection(0.02, 0.01), ShouldEqual, "up")
		})

		Convey("It should label a drop that clears the scale as down", func() {
			So(realizedDirection(-0.02, 0.01), ShouldEqual, "down")
		})
	})
}

func TestOmegaBin(t *testing.T) {
	Convey("Given the carrier lattice spanning a signed omega interval", t, func() {
		Convey("It should place the lattice centre on the middle bin", func() {
			So(omegaBin(0, -1, 2, int(phaseLatticeWidth)), ShouldEqual, 128)
		})

		Convey("It should clamp the rails rather than wrapping", func() {
			So(omegaBin(-1, -1, 2, int(phaseLatticeWidth)), ShouldEqual, 0)
			So(omegaBin(1, -1, 2, int(phaseLatticeWidth)), ShouldEqual, 255)
		})
	})
}

func TestProjectSourceDial(t *testing.T) {
	Convey("Given resident oscillators on the omega lattice", t, func() {
		oscillators := []pmanifold.Oscillator{{
			Omega:     0,
			Amplitude: 1,
			Phase:     math.Pi / 2,
		}}

		dial, err := projectSourceDial(oscillators, -1, 1)
		bin := omegaBin(0, -1, 2, int(phaseLatticeWidth))

		Convey("It should project occupancy onto the resident wave", func() {
			So(err, ShouldBeNil)
			So(dial, ShouldHaveLength, int(phaseLatticeWidth))
			So(real(dial[bin]), ShouldAlmostEqual, -1, 1e-6)
			So(imag(dial[bin]), ShouldAlmostEqual, 0, 1e-6)
		})
	})

	Convey("Given no oscillators to occupy the lattice", t, func() {
		dial, err := projectSourceDial(nil, -1, 1)

		Convey("It should refuse to invent a fingerprint", func() {
			So(err, ShouldBeNil)
			So(dial, ShouldBeNil)
		})
	})
}

func TestStampPhase(t *testing.T) {
	Convey("Given a resident universe wave and no priced books", t, func() {
		solver := NewSolver(nil, nil, nil, nil)
		Reset(func() { So(solver.Close(), ShouldBeNil) })
		solver.oscillators = []pmanifold.Oscillator{{
			Omega:     0,
			Amplitude: 1,
			Phase:     0,
		}}
		thesis := types.NewThesis(t.Context(), nil)
		at := time.Unix(1, 0).UTC()
		reading := solver.stampPhase(thesis, at, map[string][]pmanifold.Oscillator{"BTC/USD": []pmanifold.Oscillator{{
			Amplitude: 1, Phase: 0, Omega: 0,
		}}})
		stored, found := thesis.PhaseSnapshot()

		Convey("It should sweep without retaining an unpriced universe cut", func() {
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, reading)
			So(reading.Ready, ShouldBeFalse)
			So(reading.Reason, ShouldEqual, "retaining history")
			So(solver.pending, ShouldHaveLength, 0)
		})
	})
}

func TestSweep(t *testing.T) {
	Convey("Given a corpus still shorter than the readiness floor", t, func() {
		solver := NewSolver(nil, nil, nil, nil)
		Reset(func() { So(solver.Close(), ShouldBeNil) })
		reading := solver.sweep(time.Unix(1, 0).UTC(), geometry.PhaseDial{1})

		Convey("It should report that history is still being retained", func() {
			So(reading.Ready, ShouldBeFalse)
			So(reading.Reason, ShouldEqual, "retaining history")
		})
	})

	Convey("Given a corpus that has cleared the readiness floor", t, func() {
		solver := NewSolver(nil, nil, nil, nil)
		Reset(func() { So(solver.Close(), ShouldBeNil) })
		dial := make(geometry.PhaseDial, int(phaseLatticeWidth))
		dial[0] = 1
		at := time.Unix(1, 0).UTC()

		for index := range phaseCorpusMinimum {
			So(solver.corpus.Insert(geometry.CorpusEntry[types.PhaseOutcome]{
				Dial: dial,
				At:   at.Add(time.Duration(index) * time.Second),
				Outcome: types.PhaseOutcome{
					Direction: "up",
					Return:    0.01,
					Horizon:   phaseOutcomeHorizon,
				},
			}), ShouldBeNil)
		}

		query := make(geometry.PhaseDial, int(phaseLatticeWidth))
		query[0] = 1
		reading := solver.sweep(at.Add(time.Hour), query)
		leaders := 0

		for _, response := range reading.Responses {
			if response.Angle == 0 {
				leaders++
			}
		}

		Convey("It should retain the ranked corpus at every rotation", func() {
			So(reading.Ready, ShouldBeTrue)
			So(reading.Responses, ShouldHaveLength, phaseScanAngles*phaseScanTopK)
			So(leaders, ShouldEqual, phaseScanTopK)
		})
	})
}

func TestRecordPhase(t *testing.T) {
	Convey("Given a thesis and a completed universe sweep", t, func() {
		solver := &Solver{}
		thesis := types.NewThesis(t.Context(), nil)
		reading := types.PhaseReading{Ready: true, Reason: ""}

		solver.recordPhase(thesis, reading)
		stored, found := thesis.PhaseSnapshot()

		Convey("It should publish the sweep onto the thesis", func() {
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, reading)
		})
	})
}

func TestMature(t *testing.T) {
	Convey("Given a held universe dial whose horizon has elapsed", t, func() {
		managed := mgrbook.New()
		managed.Update(&mgrbook.UpdateOptions{
			Direction: mgrbook.Bid,
			ID:        "bid",
			Price:     decimal.NewFromInt64(110),
			Quantity:  decimal.NewFromInt64(2),
			Timestamp: time.Unix(2, 0).UTC(),
			Silent:    true,
		})
		managed.Update(&mgrbook.UpdateOptions{
			Direction: mgrbook.Ask,
			ID:        "ask",
			Price:     decimal.NewFromInt64(112),
			Quantity:  decimal.NewFromInt64(2),
			Timestamp: time.Unix(2, 0).UTC(),
			Silent:    true,
		})
		solver := NewSolver(nil, nil, nil, nil)
		Reset(func() { So(solver.Close(), ShouldBeNil) })
		solver.api = &staticBookSource{book: managed}
		solver.converged["BTC/USD"] = 0.01
		dial := make(geometry.PhaseDial, int(phaseLatticeWidth))
		dial[0] = 1
		solver.pending = []pendingDial{{
			dial: dial,
			at:   time.Unix(1, 0).UTC(),
			cuts: phaseOutcomeHorizon - 1,
			weights: []phaseWeight{{
				symbol: "BTC/USD",
				mid:    100,
				mass:   1,
			}},
		}}

		solver.mature()

		Convey("It should retain the cut tagged with the universe direction", func() {
			So(solver.pending, ShouldHaveLength, 0)
			So(solver.corpus.Size(), ShouldEqual, 1)
		})
	})
}

func TestMidpoint(t *testing.T) {
	Convey("Given an authoritative book with a bid and an ask", t, func() {
		managed := mgrbook.New()
		managed.Update(&mgrbook.UpdateOptions{
			Direction: mgrbook.Bid,
			ID:        "bid",
			Price:     decimal.NewFromInt64(99),
			Quantity:  decimal.NewFromInt64(2),
			Timestamp: time.Unix(1, 0).UTC(),
			Silent:    true,
		})
		managed.Update(&mgrbook.UpdateOptions{
			Direction: mgrbook.Ask,
			ID:        "ask",
			Price:     decimal.NewFromInt64(101),
			Quantity:  decimal.NewFromInt64(3),
			Timestamp: time.Unix(2, 0).UTC(),
			Silent:    true,
		})
		solver := &Solver{api: &staticBookSource{book: managed}}

		Convey("It should read the book's midpoint", func() {
			So(solver.midpoint("BTC/USD"), ShouldEqual, 100)
		})
	})
}

func TestOscillatorWave(t *testing.T) {
	Convey("Given one resident oscillator", t, func() {
		wave := oscillatorWave([]pmanifold.Oscillator{{
			Omega:     1,
			Amplitude: 2,
			Phase:     math.Pi / 2,
		}})

		Convey("It should preserve the complex mode", func() {
			So(wave, ShouldHaveLength, 1)
			So(wave[0].Omega, ShouldEqual, float32(1))
			So(float64(wave[0].Real), ShouldAlmostEqual, 0, 1e-6)
			So(float64(wave[0].Imaginary), ShouldAlmostEqual, 2, 1e-6)
		})
	})
}

func TestObserveWeights(t *testing.T) {
	Convey("Given every contributing book can be priced", t, func() {
		managed := mgrbook.New()
		managed.Update(&mgrbook.UpdateOptions{
			Direction: mgrbook.Bid,
			ID:        "bid",
			Price:     decimal.NewFromInt64(99),
			Quantity:  decimal.NewFromInt64(2),
			Timestamp: time.Unix(1, 0).UTC(),
			Silent:    true,
		})
		managed.Update(&mgrbook.UpdateOptions{
			Direction: mgrbook.Ask,
			ID:        "ask",
			Price:     decimal.NewFromInt64(101),
			Quantity:  decimal.NewFromInt64(2),
			Timestamp: time.Unix(1, 0).UTC(),
			Silent:    true,
		})
		solver := &Solver{api: &staticBookSource{book: managed}}
		weights, complete := solver.observeWeights(map[string][]pmanifold.Oscillator{"BTC/USD": []pmanifold.Oscillator{
			{Amplitude: math.Sqrt(2)},
			{Amplitude: 1},
		}})

		Convey("It should snapshot mid and injected mass for the universe cut", func() {
			So(complete, ShouldBeTrue)
			So(weights, ShouldHaveLength, 1)
			So(weights[0].symbol, ShouldEqual, "BTC/USD")
			So(weights[0].mid, ShouldEqual, 100)
			So(weights[0].mass, ShouldAlmostEqual, 3, 1e-9)
		})
	})
}

func TestUniverseOutcome(t *testing.T) {
	Convey("Given two books that moved with unequal mass", t, func() {
		solver := NewSolver(nil, nil, nil, nil)
		Reset(func() { So(solver.Close(), ShouldBeNil) })
		solver.api = &mapBookSource{books: map[string]*mgrbook.Book{
			"BTC/USD": pricedBook(110, 112),
			"ETH/USD": pricedBook(49, 51),
		}}
		solver.converged["BTC/USD"] = 0.01
		solver.converged["ETH/USD"] = 0.01

		outcome, complete := solver.universeOutcome([]phaseWeight{
			{symbol: "BTC/USD", mid: 100, mass: 2},
			{symbol: "ETH/USD", mid: 50, mass: 2},
		})

		Convey("It should classify the mass-weighted log return", func() {
			So(complete, ShouldBeTrue)
			So(outcome.Direction, ShouldEqual, "up")
			So(outcome.Horizon, ShouldEqual, phaseOutcomeHorizon)
			So(outcome.Return, ShouldAlmostEqual, 0.5*math.Log(111.0/100.0), 1e-9)
		})
	})
}

type mapBookSource struct {
	books map[string]*mgrbook.Book
}

func (source *mapBookSource) Book(symbol string, read func(*mgrbook.Book)) {
	read(source.books[symbol])
}

func pricedBook(bid int64, ask int64) *mgrbook.Book {
	managed := mgrbook.New()
	managed.Update(&mgrbook.UpdateOptions{
		Direction: mgrbook.Bid,
		ID:        "bid",
		Price:     decimal.NewFromInt64(bid),
		Quantity:  decimal.NewFromInt64(1),
		Timestamp: time.Unix(1, 0).UTC(),
		Silent:    true,
	})
	managed.Update(&mgrbook.UpdateOptions{
		Direction: mgrbook.Ask,
		ID:        "ask",
		Price:     decimal.NewFromInt64(ask),
		Quantity:  decimal.NewFromInt64(1),
		Timestamp: time.Unix(1, 0).UTC(),
		Silent:    true,
	})

	return managed
}
