package manifold

import (
	"encoding/json"
	"testing"
	"time"

	mgrbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

type staticBookSource struct {
	book *mgrbook.Book
}

func (source *staticBookSource) Book(_ string, read func(*mgrbook.Book)) {
	read(source.book)
}

func TestUpdate(t *testing.T) {
	Convey("Given a pending book snapshot whose Hawkes stage produced no measurement", t, func() {
		originalSteps := system.Cfg.Manifold.RelaxationSteps
		originalMinimum := system.Cfg.Manifold.MinSteps
		system.Cfg.Manifold.RelaxationSteps = 1
		system.Cfg.Manifold.MinSteps = 1
		Reset(func() {
			system.Cfg.Manifold.RelaxationSteps = originalSteps
			system.Cfg.Manifold.MinSteps = originalMinimum
		})
		managed := mgrbook.New()
		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.Status = types.BUSY
		thesis.Symbols.Store("BTC/USD", symbol)
		solver := NewSolver(nil, nil, nil, nil)
		solver.api = &staticBookSource{book: managed}
		solver.tokenizer = NewTokenizer(solver.config, []string{"BTC/USD"})
		Reset(func() { So(solver.Close(), ShouldBeNil) })

		pendingErr := solver.Update(thesis)

		Convey("It should wait without stamping or reporting an error", func() {
			So(pendingErr, ShouldBeNil)
			So(solver.WaitingForBook(), ShouldBeTrue)
			So(solver.domain.ParticleCount(), ShouldEqual, 0)
		})

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
		err := solver.Update(thesis)

		Convey("It should inject the unit-energy book when the snapshot arrives", func() {
			So(err, ShouldBeNil)
			So(solver.WaitingForBook(), ShouldBeFalse)
			So(solver.domain.ParticleCount(), ShouldEqual, 2)
		})
	})

	Convey("Given a fitted Hawkes measurement and an authoritative populated book", t, func() {
		originalSteps := system.Cfg.Manifold.RelaxationSteps
		originalMinimum := system.Cfg.Manifold.MinSteps
		system.Cfg.Manifold.RelaxationSteps = 1
		system.Cfg.Manifold.MinSteps = 1
		Reset(func() {
			system.Cfg.Manifold.RelaxationSteps = originalSteps
			system.Cfg.Manifold.MinSteps = originalMinimum
		})
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
		buyExcitation := 0.25
		sellExcitation := 0.5
		measurement := &types.Measurement{
			Source: types.SourceHawkes,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricExcitationAmplitude, types.SideBuyToBuy): {
					Normalized: &buyExcitation,
				},
				types.MetricKey(types.MetricExcitationAmplitude, types.SideSellToSell): {
					Normalized: &sellExcitation,
				},
			},
		}
		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.Measurements = []*types.Measurement{measurement}
		symbol.Status = types.BUSY
		thesis.Symbols.Store("BTC/USD", symbol)
		solver := NewSolver(nil, nil, nil, nil)
		solver.api = &staticBookSource{book: managed}
		solver.tokenizer = NewTokenizer(solver.config, []string{"BTC/USD"})
		Reset(func() { So(solver.Close(), ShouldBeNil) })

		err := solver.Update(thesis)
		reading, readingErr := solver.domain.Reading()

		Convey("It should append, advance, read, and stamp one manifold cut", func() {
			So(err, ShouldBeNil)
			So(readingErr, ShouldBeNil)
			So(solver.domain.ParticleCount(), ShouldEqual, 2)
			So(thesis.Manifold, ShouldResemble, reading)
		})
	})
}

func TestStep(t *testing.T) {
	Convey("Given one resident market carrier and a regulated relaxation budget", t, func() {
		originalSteps := system.Cfg.Manifold.RelaxationSteps
		originalMinimum := system.Cfg.Manifold.MinSteps
		system.Cfg.Manifold.RelaxationSteps = 1
		system.Cfg.Manifold.MinSteps = 1
		Reset(func() {
			system.Cfg.Manifold.RelaxationSteps = originalSteps
			system.Cfg.Manifold.MinSteps = originalMinimum
		})
		ui := make(chan []byte, 1)
		solver := NewSolver(nil, ui, nil, nil)
		Reset(func() { So(solver.Close(), ShouldBeNil) })
		particle := pfluid.Particle{
			Position: pfluid.Vector{X: 0.25, Y: 0.25, Z: 0.25},
			Mass:     1,
			Heat:     0.5,
			Energy:   1,
			Phase:    0.1,
			Omega:    1,
		}
		particles := []pfluid.Particle{particle}
		_, err := solver.domain.Append(particles, []uint32{1})
		So(err, ShouldBeNil)
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Symbols.Store("BTC/USD", types.NewSymbol("BTC/USD", nil))
		at := time.Unix(1, 0).UTC()

		err = solver.Step(thesis, "BTC/USD", at, particles)
		reading, readingErr := solver.domain.Reading()
		var payload []byte

		select {
		case payload = <-ui:
		default:
		}

		var frame struct {
			Manifold []map[string]any `json:"manifold"`
		}
		marshalErr := json.Unmarshal(payload, &frame)
		stored, _ := thesis.Symbols.Load("BTC/USD")
		_, phaseFound := stored.(*types.Symbol).Phase.Load("BTC/USD")

		Convey("It should advance, read, and publish the same completed cut", func() {
			So(err, ShouldBeNil)
			So(readingErr, ShouldBeNil)
			So(solver.stepped, ShouldBeTrue)
			So(thesis.Manifold, ShouldResemble, reading)
			So(marshalErr, ShouldBeNil)
			So(frame.Manifold, ShouldHaveLength, 1)
			So(frame.Manifold[0]["symbol"], ShouldEqual, "BTC/USD")
			So(phaseFound, ShouldBeTrue)
		})
	})

	Convey("Given an invalid regulated relaxation budget", t, func() {
		originalSteps := system.Cfg.Manifold.RelaxationSteps
		system.Cfg.Manifold.RelaxationSteps = 0
		Reset(func() { system.Cfg.Manifold.RelaxationSteps = originalSteps })
		solver := &Solver{}

		err := solver.Step(
			types.NewThesis(t.Context(), nil), "BTC/USD", time.Unix(1, 0), nil,
		)

		Convey("It should reject the cut instead of silently stamping it", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkUpdate(b *testing.B) {
	originalSteps := system.Cfg.Manifold.RelaxationSteps
	originalMinimum := system.Cfg.Manifold.MinSteps
	system.Cfg.Manifold.RelaxationSteps = 1
	system.Cfg.Manifold.MinSteps = 1
	defer func() {
		system.Cfg.Manifold.RelaxationSteps = originalSteps
		system.Cfg.Manifold.MinSteps = originalMinimum
	}()
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
	solver := NewSolver(nil, nil, nil, nil)
	solver.api = &staticBookSource{book: managed}
	solver.tokenizer = NewTokenizer(solver.config, []string{"BTC/USD"})
	b.Cleanup(func() {
		if err := solver.Close(); err != nil {
			b.Fatal(err)
		}
	})
	thesis := types.NewThesis(b.Context(), nil)
	symbol := types.NewSymbol("BTC/USD", nil)
	symbol.Status = types.BUSY
	thesis.Symbols.Store("BTC/USD", symbol)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := solver.Update(thesis); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStep(b *testing.B) {
	originalSteps := system.Cfg.Manifold.RelaxationSteps
	originalMinimum := system.Cfg.Manifold.MinSteps
	system.Cfg.Manifold.RelaxationSteps = 1
	system.Cfg.Manifold.MinSteps = 1
	defer func() {
		system.Cfg.Manifold.RelaxationSteps = originalSteps
		system.Cfg.Manifold.MinSteps = originalMinimum
	}()
	solver := NewSolver(nil, nil, nil, nil)
	b.Cleanup(func() {
		if err := solver.Close(); err != nil {
			b.Fatal(err)
		}
	})
	particles := []pfluid.Particle{{
		Position: pfluid.Vector{X: 0.25, Y: 0.25, Z: 0.25},
		Mass:     1,
		Heat:     0.5,
		Energy:   1,
		Phase:    0.1,
		Omega:    1,
	}}

	if _, err := solver.domain.Append(particles, []uint32{1}); err != nil {
		b.Fatal(err)
	}

	thesis := types.NewThesis(b.Context(), nil)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := solver.Step(
			thesis, "BTC/USD", time.Unix(1, 0), particles,
		); err != nil {
			b.Fatal(err)
		}
	}
}
