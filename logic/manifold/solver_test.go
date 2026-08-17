package manifold

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	mgrbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
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
		Convey("It should wait without stamping or reporting an error", func() {
			So(pendingErr, ShouldBeNil)
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

		Convey("It should inject the unit-energy book when the snapshot arrives", func() {
			So(err, ShouldBeNil)
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
		symbol.AppendMeasurement(measurement)
		symbol.Status = types.BUSY
		thesis.Symbols.Store("BTC/USD", symbol)
		solver := NewSolver(nil, nil, nil, nil)
		solver.api = &staticBookSource{book: managed}
		Reset(func() { So(solver.Close(), ShouldBeNil) })

		err := solver.Update(thesis)
		reading := solver.reading
		storedReading, found := thesis.ManifoldSnapshot()

		Convey("It should append, advance, read, and stamp one manifold cut", func() {
			So(err, ShouldBeNil)
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
		thesis := types.NewThesis(t.Context(), nil)
		thesis.At = time.Unix(3, 0).UTC()
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.Status = types.BUSY
		thesis.Symbols.Store("BTC/USD", symbol)
		solver := NewSolver(nil, nil, nil, nil)
		solver.api = &staticBookSource{book: managed}
		Reset(func() { So(solver.Close(), ShouldBeNil) })

		So(solver.Update(thesis), ShouldBeNil)

		Convey("It should map the book, step once, and publish before returning", func() {
			So(solver.ParticleCount(), ShouldEqual, 2)
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

		err = solver.Step(thesis, at, map[string][]pmanifold.Oscillator{"BTC/USD": oscillators}, HawkesSignal{})
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
				var particlePayload struct {
					Particles []OrderNode `json:"particles"`
				}
				So(json.Unmarshal(fluidFrame.Payload, &particlePayload), ShouldBeNil)
				So(particlePayload.Particles, ShouldNotBeEmpty)
				So(math.IsNaN(float64(particlePayload.Particles[0].Position.X)), ShouldBeFalse)
				So(math.IsNaN(float64(particlePayload.Particles[0].Position.Y)), ShouldBeFalse)
				So(math.IsNaN(float64(particlePayload.Particles[0].Position.Z)), ShouldBeFalse)
				So(math.IsNaN(float64(particlePayload.Particles[0].Particle.Position.X)), ShouldBeFalse)
				continue
			}

			if fluidFrame.Channel == types.FluidPhaseChannel {
				phaseFrames++
				continue
			}

			fieldFrames++
			var fieldPayload struct {
				Fields ManifoldFields `json:"fields"`
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

		Convey("It should publish one phase row and one domain frame per step", func() {
			So(err, ShouldBeNil)
			So(math.IsNaN(solver.reading.Divergence), ShouldBeFalse)
			So(found, ShouldBeTrue)
			So(storedReading, ShouldResemble, reading)
			So(payloads, ShouldHaveLength, 1)
			So(phaseFrames, ShouldEqual, 1)
			So(fieldFrames, ShouldEqual, 1)
			So(particleFrames, ShouldEqual, 1)
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
			types.NewThesis(t.Context(), nil), time.Unix(1, 0), nil, HawkesSignal{},
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
		bitcoinOsc, bitcoinErr := solver.bookOscillators("BTC/USD", 0, 2, time.Unix(3, 0).UTC(), HawkesSignal{})
		etherOsc, etherErr := solver.bookOscillators("ETH/USD", 1, 2, time.Unix(3, 0).UTC(), HawkesSignal{})

		Convey("It should keep every resting order as an oscillator", func() {
			So(bitcoinErr, ShouldBeNil)
			So(etherErr, ShouldBeNil)
			So(len(bitcoinOsc), ShouldEqual, 2)
			So(len(etherOsc), ShouldEqual, 2)
			So(bitcoinOsc[0].Amplitude, ShouldBeGreaterThan, 0)
			So(etherOsc[0].Amplitude, ShouldBeGreaterThan, 0)
			So(bitcoinOsc[0].PosX, ShouldNotEqual, bitcoinOsc[1].PosX)
			So(bitcoinOsc[0].PosZ, ShouldBeLessThan, solver.config.DomainZ)
			So(bitcoinOsc[0].PosZ, ShouldBeGreaterThan, 0)
			So(bitcoinOsc[0].Omega, ShouldBeGreaterThanOrEqualTo, solver.config.GateWidthMin())
			So(bitcoinOsc[0].Omega, ShouldBeLessThanOrEqualTo, solver.config.GateWidthMax())
			So(bitcoinOsc[0].Heat, ShouldBeGreaterThan, 0)
		})

		Convey("A subsequent tick with a large price shift should clamp velocities to CFL limits", func() {
			bitcoin.Update(&mgrbook.UpdateOptions{
				Direction: mgrbook.Bid,
				ID:        "bid",
				Price:     decimal.NewFromInt64(10),
				Quantity:  decimal.NewFromInt64(2),
				Timestamp: time.Unix(4, 0).UTC(),
				Silent:    true,
			})

			shiftedOsc, shiftErr := solver.bookOscillators("BTC/USD", 0, 2, time.Unix(4, 0).UTC(), HawkesSignal{})
			So(shiftErr, ShouldBeNil)
			So(shiftedOsc, ShouldHaveLength, 3)

			maxVelocity := solver.config.MinGasCellSpacing() / solver.config.DeltaT

			for _, osc := range shiftedOsc {
				So(math.Abs(osc.VelX), ShouldBeLessThanOrEqualTo, maxVelocity)
				So(math.Abs(osc.VelY), ShouldBeLessThanOrEqualTo, maxVelocity)
				So(math.Abs(osc.VelZ), ShouldBeLessThanOrEqualTo, maxVelocity)
			}
		})
	})
}

func TestBookOscillatorsDegenerate(t *testing.T) {
	Convey("Given one healthy order beside an underflowed dust order", t, func() {
		book := mgrbook.New()
		book.Update(&mgrbook.UpdateOptions{
			Direction: mgrbook.Bid,
			ID:        "healthy",
			Price:     decimal.NewFromInt64(99),
			Quantity:  decimal.NewFromInt64(2),
			Timestamp: time.Unix(1, 0).UTC(),
			Silent:    true,
		})
		book.Update(&mgrbook.UpdateOptions{
			Direction: mgrbook.Bid,
			ID:        "dust",
			Price:     decimal.NewFromInt64(99),
			Quantity:  decimal.NewFromFloat64(1e-340),
			Timestamp: time.Unix(1, 0).UTC(),
			Silent:    true,
		})
		solver := NewSolver(nil, nil, nil, nil)
		Reset(func() { So(solver.Close(), ShouldBeNil) })
		solver.api = &mapBookSource{books: map[string]*mgrbook.Book{
			"BTC/USD": book,
		}}
		oscillators, err := solver.bookOscillators("BTC/USD", 0, 1, time.Unix(3, 0).UTC(), HawkesSignal{})

		Convey("It should exclude the zero-heat row and keep the healthy one", func() {
			So(err, ShouldBeNil)
			So(oscillators, ShouldHaveLength, 1)

			for _, oscillator := range oscillators {
				So(oscillator.Heat, ShouldBeGreaterThan, 0)
				So(oscillator.Amplitude, ShouldBeGreaterThan, 0)
				So(math.IsInf(oscillator.Omega, 0), ShouldBeFalse)
			}
		})

		Convey("An infinite hawkes tempo should drop every field-validated row", func() {
			deg, degErr := solver.bookOscillators(
				"BTC/USD", 0, 1, time.Unix(3, 0).UTC(),
				HawkesSignal{LambdaBuy: math.Inf(1), LambdaSell: math.Inf(1)},
			)

			So(degErr, ShouldBeNil)
			So(deg, ShouldBeEmpty)
		})
	})
}

func TestExtractSymbolHawkes(t *testing.T) {
	Convey("Given a symbol retaining a fitted Hawkes measurement", t, func() {
		symbol := types.NewSymbol("BTC/USD", nil)

		Convey("When a metric sample is positive and finite it is admitted", func() {
			symbol.Latest.Store(string(types.SourceHawkes), &types.Measurement{
				Source: types.SourceHawkes,
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricConditionalIntensity, types.SideBuy): {
						Raw: 2.5,
					},
					types.MetricKey(types.MetricConditionalIntensity, types.SideSell): {
						Raw: 1.5,
					},
				},
			})

			signal := extractSymbolHawkes(symbol)
			So(signal.LambdaBuy, ShouldEqual, 2.5)
			So(signal.LambdaSell, ShouldEqual, 1.5)
		})

		Convey("When a metric sample is infinite the defaults survive", func() {
			symbol.Latest.Store(string(types.SourceHawkes), &types.Measurement{
				Source: types.SourceHawkes,
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricConditionalIntensity, types.SideBuy): {
						Raw: math.Inf(1),
					},
					types.MetricKey(types.MetricArrivalRate, types.SideSell): {
						Raw: math.Inf(1),
					},
				},
			})

			signal := extractSymbolHawkes(symbol)
			So(signal.LambdaBuy, ShouldEqual, 1.0)
			So(signal.LambdaSell, ShouldEqual, 1.0)
		})

		Convey("When no measurement is retained the defaults survive", func() {
			signal := extractSymbolHawkes(symbol)
			So(signal.LambdaBuy, ShouldEqual, 1.0)
			So(signal.LambdaSell, ShouldEqual, 1.0)
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

func TestUpdateMapping(t *testing.T) {
	Convey("Given a populated authoritative book", t, func() {
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
		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		thesis.Symbols.Store("BTC/USD", symbol)
		solver := NewSolver(nil, nil, nil, nil)
		solver.api = &staticBookSource{book: managed}
		Reset(func() { So(solver.Close(), ShouldBeNil) })

		err := solver.Update(thesis)

		Convey("It should map the orders and step the field in one pass", func() {
			So(err, ShouldBeNil)
			So(solver.ParticleCount(), ShouldEqual, 2)
			So(solver.oscillators[0].Amplitude, ShouldAlmostEqual, 1.0, 6)
			So(solver.oscillators[1].Amplitude, ShouldAlmostEqual, 1.0, 6)
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
		if err := solver.Step(thesis, time.Unix(1, 0), map[string][]pmanifold.Oscillator{"BTC/USD": oscillators}, HawkesSignal{}); err != nil {
			b.Fatal(err)
		}
	}
}
