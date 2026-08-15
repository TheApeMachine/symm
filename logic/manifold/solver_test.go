package manifold

import (
	"encoding/json"
	"runtime"
	"testing"
	"time"

	mgrbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

type staticBookSource struct {
	book    *mgrbook.Book
	entered chan<- struct{}
	release <-chan struct{}
}

func (source *staticBookSource) Book(_ string, read func(*mgrbook.Book)) {
	if source.entered != nil {
		select {
		case source.entered <- struct{}{}:
		default:
		}
	}

	if source.release != nil {
		<-source.release
	}

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
		Reset(func() { So(solver.Close(), ShouldBeNil) })

		pendingErr := solver.Update(thesis)
		deadline := time.Now().Add(10 * time.Second)

		for solver.settling.Load() && time.Now().Before(deadline) {
			runtime.Gosched()
		}

		Convey("It should wait without stamping or reporting an error", func() {
			So(pendingErr, ShouldBeNil)
			So(solver.settling.Load(), ShouldBeFalse)
			So(solver.WaitingForBook(), ShouldBeTrue)
			So(solver.ParticleCount(), ShouldEqual, 0)
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
		deadline = time.Now().Add(10 * time.Second)

		for solver.settling.Load() && time.Now().Before(deadline) {
			runtime.Gosched()
		}

		Convey("It should inject the unit-energy book when the snapshot arrives", func() {
			So(err, ShouldBeNil)
			So(solver.settling.Load(), ShouldBeFalse)
			So(solver.WaitingForBook(), ShouldBeFalse)
			So(solver.ParticleCount(), ShouldEqual, 2)
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
		symbol.AppendMeasurement(types.SourceHawkes, measurement)
		symbol.Status = types.BUSY
		thesis.Symbols.Store("BTC/USD", symbol)
		solver := NewSolver(nil, nil, nil, nil)
		solver.api = &staticBookSource{book: managed}
		Reset(func() { So(solver.Close(), ShouldBeNil) })

		err := solver.Update(thesis)
		deadline := time.Now().Add(10 * time.Second)

		for solver.settling.Load() && time.Now().Before(deadline) {
			runtime.Gosched()
		}

		reading := solver.reading
		storedReading, found := thesis.ManifoldSnapshot()

		Convey("It should append, advance, read, and stamp one manifold cut", func() {
			So(err, ShouldBeNil)
			So(solver.settling.Load(), ShouldBeFalse)
			So(solver.ParticleCount(), ShouldEqual, 2)
			So(found, ShouldBeTrue)
			So(storedReading, ShouldResemble, reading)
		})
	})

	Convey("Given market inputs arriving while the manifold owner is active", t, func() {
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
		entered := make(chan struct{}, 1)
		release := make(chan struct{})
		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.Status = types.BUSY
		thesis.Symbols.Store("BTC/USD", symbol)
		solver := NewSolver(nil, nil, nil, nil)
		solver.api = &staticBookSource{
			book: managed,
		}
		Reset(func() { So(solver.Close(), ShouldBeNil) })
		solver.settling.Store(true)
		go func() {
			entered <- struct{}{}
			<-release
			solver.settling.Store(false)
		}()

		select {
		case <-entered:
		case <-time.After(10 * time.Second):
			t.Fatal("manifold owner did not enter the authoritative book read")
		}

		for request := 0; request < system.Cfg.Manifold.RelaxationSteps+1; request++ {
			So(solver.Update(thesis), ShouldBeNil)
		}

		queuedState := solver.settling.Load()
		close(release)
		deadline := time.Now().Add(10 * time.Second)

		for solver.settling.Load() && time.Now().Before(deadline) {
			runtime.Gosched()
		}

		Convey("It should ignore additional semaphores until settlement completes", func() {
			So(queuedState, ShouldBeTrue)
			So(solver.settling.Load(), ShouldBeFalse)
			So(solver.stepped, ShouldBeFalse)
			So(solver.pending, ShouldHaveLength, 0)
			So(solver.ParticleCount(), ShouldEqual, 0)
		})

	})
}

func TestStep(t *testing.T) {
	Convey("Given one resident market carrier and a regulated relaxation budget", t, func() {
		originalSteps := system.Cfg.Manifold.RelaxationSteps
		originalMinimum := system.Cfg.Manifold.MinSteps
		system.Cfg.Manifold.RelaxationSteps = 2
		system.Cfg.Manifold.MinSteps = 1
		Reset(func() {
			system.Cfg.Manifold.RelaxationSteps = originalSteps
			system.Cfg.Manifold.MinSteps = originalMinimum
		})
		ui := make(chan []byte, 8)
		binui := make(chan types.FluidFrame, 8)
		solver := NewSolver(nil, ui, binui, nil)
		Reset(func() { So(solver.Close(), ShouldBeNil) })
		oscillators := []pmanifold.Oscillator{{
			Phase:     0.1,
			Omega:     1,
			Amplitude: 1,
			PosX:      0.25,
			PosY:      0.25,
			PosZ:      0.25,
			Heat:      0.5,
		}}
		err := solver.physics.SetOscillators(oscillators)
		So(err, ShouldBeNil)
		solver.oscillators = oscillators
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Symbols.Store("BTC/USD", types.NewSymbol("BTC/USD", nil))
		at := time.Unix(1, 0).UTC()

		err = solver.Step(thesis, at, []manifoldCut{{
			symbol: "BTC/USD", oscillators: oscillators,
		}})
		reading := solver.reading
		storedReading, found := thesis.ManifoldSnapshot()
		payloads := make([][]byte, 0, len(ui))

		for len(ui) > 0 {
			payloads = append(payloads, <-ui)
		}

		var frame struct {
			Manifold []map[string]any `json:"manifold"`
		}
		marshalErr := json.Unmarshal(payloads[len(payloads)-1], &frame)
		_, phaseFound := thesis.PhaseSnapshot()
		fieldFrames := 0
		particleFrames := 0
		phaseFrames := 0
		waveObserved := false

		for len(binui) > 0 {
			fluidFrame := <-binui

			if fluidFrame.Channel == types.FluidParticlesChannel {
				particleFrames++
				continue
			}

			if fluidFrame.Channel == types.FluidPhaseChannel {
				phaseFrames++
				continue
			}

			fieldFrames++
			var fieldPayload struct {
				Fields pfluid.Fields `json:"fields"`
			}
			So(json.Unmarshal(fluidFrame.Payload, &fieldPayload), ShouldBeNil)

			for cell := range fieldPayload.Fields.WaveReal {
				if fieldPayload.Fields.WaveReal[cell] != 0 ||
					fieldPayload.Fields.WaveImaginary[cell] != 0 {
					waveObserved = true
					break
				}
			}
		}

		Convey("It should publish the manifold and wave field after every step", func() {
			So(err, ShouldBeNil)
			So(solver.stepped, ShouldBeTrue)
			So(found, ShouldBeTrue)
			So(storedReading, ShouldResemble, reading)
			So(payloads, ShouldHaveLength, 2)
			So(fieldFrames, ShouldEqual, 2)
			So(particleFrames, ShouldEqual, 2)
			So(phaseFrames, ShouldEqual, 2)
			So(waveObserved, ShouldBeTrue)
			So(marshalErr, ShouldBeNil)
			So(frame.Manifold, ShouldHaveLength, 1)
			So(frame.Manifold[0]["source"], ShouldEqual, "manifold")
			_, hasSymbol := frame.Manifold[0]["symbol"]
			So(hasSymbol, ShouldBeFalse)
			So(phaseFound, ShouldBeTrue)
		})
	})

	Convey("Given an invalid regulated relaxation budget", t, func() {
		originalSteps := system.Cfg.Manifold.RelaxationSteps
		system.Cfg.Manifold.RelaxationSteps = 0
		Reset(func() { system.Cfg.Manifold.RelaxationSteps = originalSteps })
		solver := &Solver{}

		err := solver.Step(
			types.NewThesis(t.Context(), nil), time.Unix(1, 0), nil,
		)

		Convey("It should reject the cut instead of silently stamping it", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestNewSolver(t *testing.T) {
	Convey("Given the resident universe domain", t, func() {
		solver := NewSolver(nil, nil, nil, nil)
		Reset(func() { So(solver.Close(), ShouldBeNil) })

		Convey("It should build one universe domain on the 256-mode lattice", func() {
			So(solver.config.MaxModes, ShouldEqual, phaseLatticeWidth)
			So(solver.config.GridX, ShouldEqual, 64)
			So(solver.physics, ShouldNotBeNil)
		})
	})
}

func TestBookOscillators(t *testing.T) {
	Convey("Given two populated L3 books", t, func() {
		bitcoin := mgrbook.New()
		bitcoin.Update(&mgrbook.UpdateOptions{
			Direction: mgrbook.Bid,
			ID:        "bid",
			Price:     decimal.NewFromInt64(99),
			Quantity:  decimal.NewFromInt64(2),
			Timestamp: time.Unix(1, 0).UTC(),
			Silent:    true,
		})
		bitcoin.Update(&mgrbook.UpdateOptions{
			Direction: mgrbook.Ask,
			ID:        "ask",
			Price:     decimal.NewFromInt64(101),
			Quantity:  decimal.NewFromInt64(3),
			Timestamp: time.Unix(2, 0).UTC(),
			Silent:    true,
		})
		ether := mgrbook.New()
		ether.Update(&mgrbook.UpdateOptions{
			Direction: mgrbook.Bid,
			ID:        "bid",
			Price:     decimal.NewFromInt64(49),
			Quantity:  decimal.NewFromInt64(1),
			Timestamp: time.Unix(1, 0).UTC(),
			Silent:    true,
		})
		ether.Update(&mgrbook.UpdateOptions{
			Direction: mgrbook.Ask,
			ID:        "ask",
			Price:     decimal.NewFromInt64(51),
			Quantity:  decimal.NewFromInt64(1),
			Timestamp: time.Unix(2, 0).UTC(),
			Silent:    true,
		})
		solver := NewSolver(nil, nil, nil, nil)
		Reset(func() { So(solver.Close(), ShouldBeNil) })
		solver.api = &mapBookSource{books: map[string]*mgrbook.Book{
			"BTC/USD": bitcoin,
			"ETH/USD": ether,
		}}
		solver.universe = []string{"BTC/USD", "ETH/USD"}
		bitcoinOsc, bitcoinErr := solver.bookOscillators("BTC/USD", 0, 0, time.Unix(3, 0).UTC())
		etherOsc, etherErr := solver.bookOscillators("ETH/USD", 0, 0, time.Unix(3, 0).UTC())

		Convey("It should keep every resting order as an oscillator", func() {
			So(bitcoinErr, ShouldBeNil)
			So(etherErr, ShouldBeNil)
			So(len(bitcoinOsc), ShouldEqual, 2)
			So(len(etherOsc), ShouldEqual, 2)
			So(bitcoinOsc[0].Amplitude, ShouldBeGreaterThan, 0)
			So(etherOsc[0].Amplitude, ShouldBeGreaterThan, 0)
			So(bitcoinOsc[0].PosX, ShouldNotEqual, bitcoinOsc[1].PosX)
		})
	})
}

func TestSettling(t *testing.T) {
	Convey("Given a solver that is not relaxing", t, func() {
		Convey("It should report idle", func() {
			So((&Solver{}).Settling(), ShouldBeFalse)
			So((*Solver)(nil).Settling(), ShouldBeFalse)
		})
	})
}

func TestPhaseRow(t *testing.T) {
	Convey("Given a ready universe sweep", t, func() {
		solver := &Solver{}
		at := time.Unix(1, 0).UTC()
		reading := types.PhaseReading{
			At:    at,
			Ready: true,
			Responses: []types.PhaseResponse{{
				Angle: 0, Similarity: 0.5,
			}},
		}
		row := solver.phaseRow(at, reading)
		_, hasSymbol := row["symbol"]

		Convey("It should publish the sweep without a symbol key", func() {
			So(row["source"], ShouldEqual, "manifold")
			So(row["phaseReady"], ShouldEqual, true)
			So(row["phaseScan"], ShouldHaveLength, 1)
			So(hasSymbol, ShouldBeFalse)
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
		for request := 0; request < system.Cfg.Manifold.RelaxationSteps; request++ {
			if err := solver.Update(thesis); err != nil {
				b.Fatal(err)
			}
		}

		for solver.settling.Load() {
			runtime.Gosched()
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
	oscillators := []pmanifold.Oscillator{{
		Phase:     0.1,
		Omega:     1,
		Amplitude: 1,
		PosX:      0.25,
		PosY:      0.25,
		PosZ:      0.25,
		Heat:      0.5,
	}}

	if err := solver.physics.SetOscillators(oscillators); err != nil {
		b.Fatal(err)
	}

	solver.oscillators = oscillators

	thesis := types.NewThesis(b.Context(), nil)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := solver.Step(thesis, time.Unix(1, 0), []manifoldCut{{
			symbol: "BTC/USD", oscillators: oscillators,
		}}); err != nil {
			b.Fatal(err)
		}
	}
}
