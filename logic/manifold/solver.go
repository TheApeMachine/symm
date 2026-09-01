package manifold

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

// WaveMode and State are defined in types so types.Envelope can carry a
// manifold advance without an import cycle back to logic/manifold.
type WaveMode = types.WaveMode
type State = types.ManifoldState

/*
Solver owns one resident Sensorium domain for the complete market universe.
Symbols contribute orders to the same gas and wave fields; they are not split
into independent simulations that cannot interfere.

It satisfies nomagique/runtime.Node[*types.Envelope]. A Level3 envelope projects
its message's orders into oscillators exactly once, loads them into the resident
domain, and advances the field once — every message, forward only. There is no
retained book and no separate trade trigger: the Level3 stream is the whole
input, and each message is one load-and-step.
*/
type Solver struct {
	ctx           context.Context
	cancel        context.CancelFunc
	advanceMu     sync.Mutex
	err           error
	status        *runtime.Status
	dataset       *Dataset
	physics       *sensorium.Manifold
	ObserveModule func(string, time.Duration)

	// forcing retains the latest causally-available Hawkes excitation fraction
	// per symbol. A Trade event records it; the next Level3 event lifts the
	// matching side's resting-order energy above the unit baseline. It is
	// guarded by forcingMu alone — a Trade's two-float update never waits on a
	// full physics advance, and advance reads only a coherent per-symbol
	// snapshot under the read lock.
	forcingMu sync.RWMutex
	forcing   map[string]forcingState
	batch     sensorium.State
	reading   State

	// fieldMomRho/fieldEnergy/fieldWaveReal/fieldWaveImag are Step's private
	// working buffers for PackFields, reused across advances so gathering the
	// grid fields does not itself allocate; State.MomRho etc. is always a
	// fresh copy handed to the caller, never these buffers.
	fieldMomRho   []float32
	fieldEnergy   []float32
	fieldWaveReal []float32
	fieldWaveImag []float32
}

/*
forcingState is one symbol's retained Hawkes excitation fractions above the
unit oscillator baseline: buy excitation lifts ask-side resting orders, sell
excitation lifts bid-side resting orders.
*/
type forcingState struct {
	buyExcitation  float32
	sellExcitation float32
}

func NewSolver(ctx context.Context) *Solver {
	ctx, cancel := context.WithCancel(ctx)

	dataset := NewDataset()

	solver := &Solver{
		ctx:     ctx,
		cancel:  cancel,
		status:  runtime.NewStatus(),
		dataset: dataset,
		forcing: make(map[string]forcingState),
		physics: sensorium.NewManifold(
			system.Cfg.Manifold.Grid.X,
			system.Cfg.Manifold.Grid.Y,
			system.Cfg.Manifold.Grid.Z,
		),
	}

	return solver
}

func (solver *Solver) Name() string { return "manifold" }

func (solver *Solver) Error() error { return solver.err }

/*
Step dispatches on the envelope kind:

  - EnvelopeTrade: if envelope.Hawkes carries valid excitation fractions, record
    them as the symbol's forcing state. No physics field advance and no
    envelope.Manifold emission happen here — the trade event only updates the
    resident forcing, never steps the domain.
  - EnvelopeLevel3: project the message's orders into a batch, loading the
    latest causally-available forcing into the resting-order energy, and
    advance the field once.
  - Any other kind is a no-op.
*/
func (solver *Solver) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil {
		return envelope
	}

	switch envelope.TypeID {
	case types.EnvelopeTrade:
		symbol := ""

		if envelope.Hawkes != nil {
			symbol = envelope.Hawkes.Label
		}

		if symbol == "" {
			symbol = envelope.TradeData.Symbol
		}

		solver.recordForcing(symbol, envelope.Hawkes)

		return envelope

	case types.EnvelopeLevel3:
		envelope.Manifold = solver.advance(envelope.Level3Data)

		return envelope
	}

	return envelope
}

/*
recordForcing stores the symbol's latest Hawkes excitation fractions under the
forcing lock alone. A non-finite or invalid fraction is rejected rather than
silently poisoning resident forcing state. Trade events never advance the field,
so this path never contends with the physics advance lock.
*/
func (solver *Solver) recordForcing(symbol string, hawkes *data.Measurement[float64]) {
	if hawkes == nil || hawkes.Err != nil || symbol == "" {
		return
	}

	buyMetric, buyFound := hawkes.Metrics["excitation_fraction:buy"]
	sellMetric, sellFound := hawkes.Metrics["excitation_fraction:sell"]

	if !buyFound && !sellFound {
		return
	}

	buy := float32(0)
	sell := float32(0)

	if buyFound {
		if !isFiniteFloat(buyMetric.Raw) || buyMetric.Raw < 0 {
			return
		}
		buy = float32(buyMetric.Raw)
	}

	if sellFound {
		if !isFiniteFloat(sellMetric.Raw) || sellMetric.Raw < 0 {
			return
		}
		sell = float32(sellMetric.Raw)
	}

	solver.forcingMu.Lock()
	solver.forcing[symbol] = forcingState{buyExcitation: buy, sellExcitation: sell}
	solver.forcingMu.Unlock()
}

/*
latestForcing returns a symbol's retained forcing state (or the zero-value
unit baseline when none has been observed yet). It must be called while the
forcing read lock is held by advance.
*/
func (solver *Solver) latestForcing(symbol string) forcingState {
	if solver.forcing == nil {
		return forcingState{}
	}

	return solver.forcing[symbol]
}

func isFiniteFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

/*
advance projects one Level3 message's orders into a single batch, loads it into
the resident domain, and advances the field once.
*/
func (solver *Solver) advance(message kraken.Level3Data) *State {
	if solver.physics == nil {
		return nil
	}

	// Read the latest causally-available forcing under the read lock only, so
	// a Trade's forcing update never blocks behind this physics advance. The
	// snapshot is coherent: recordForcing writes the whole forcingState under
	// the write lock, and this copies it under the read lock.
	solver.forcingMu.RLock()
	forcing := solver.latestForcing(message.Symbol)
	solver.forcingMu.RUnlock()

	solver.advanceMu.Lock()
	defer solver.advanceMu.Unlock()

	started := time.Now()
	defer func() {
		if solver.ObserveModule != nil {
			solver.ObserveModule("manifold", time.Since(started))
		}
	}()

	batch := solver.project(message, forcing)

	if batch == nil || batch.N == 0 {
		return nil
	}

	state, err := solver.physics.Step(batch)

	if err != nil {
		solver.err = err
		return nil
	}

	if state == nil || state.N == 0 {
		return nil
	}

	solver.reading.State.N = state.N
	solver.reading.Reading = solver.physics.Reading()

	return &solver.reading
}

/*
Snapshot materializes the resident particles and fields only for a connected
manifold viewer. The streaming path retains and publishes only Reading.
*/
func (solver *Solver) Snapshot() *State {
	if solver == nil || solver.physics == nil {
		return nil
	}

	solver.advanceMu.Lock()
	defer solver.advanceMu.Unlock()

	state := solver.physics.State()

	if state == nil || state.N == 0 {
		return nil
	}

	gridX, gridY, gridZ, gridSpacing := solver.physics.Grid()
	cells := gridX * gridY * gridZ

	if len(solver.fieldMomRho) != cells*4 {
		solver.fieldMomRho = make([]float32, cells*4)
		solver.fieldEnergy = make([]float32, cells)
		solver.fieldWaveReal = make([]float32, cells)
		solver.fieldWaveImag = make([]float32, cells)
	}

	densityScale, momentumScale, energyScale, waveScale := solver.physics.PackFields(
		solver.fieldMomRho,
		solver.fieldEnergy,
		solver.fieldWaveReal,
		solver.fieldWaveImag,
	)
	modeOmega, modeReal, modeImag, modeLinewidth := solver.physics.SpectralModes()
	modes := make([]WaveMode, len(modeOmega))

	for index := range modeOmega {
		modes[index] = WaveMode{
			Omega:     modeOmega[index],
			Real:      modeReal[index],
			Imag:      modeImag[index],
			Linewidth: modeLinewidth[index],
		}
	}

	return &State{
		State:         cloneState(state),
		Reading:       solver.physics.Reading(),
		GridX:         gridX,
		GridY:         gridY,
		GridZ:         gridZ,
		GridSpacing:   gridSpacing,
		MomRho:        append([]float32(nil), solver.fieldMomRho...),
		FieldEnergy:   append([]float32(nil), solver.fieldEnergy...),
		WaveReal:      append([]float32(nil), solver.fieldWaveReal...),
		WaveImag:      append([]float32(nil), solver.fieldWaveImag...),
		DensityScale:  densityScale,
		MomentumScale: momentumScale,
		EnergyScale:   energyScale,
		WaveScale:     waveScale,
		Modes:         modes,
	}
}

func cloneState(state *sensorium.State) sensorium.State {
	return sensorium.State{
		N:          state.N,
		Bytes:      append([]int64(nil), state.Bytes...),
		Seqs:       append([]int64(nil), state.Seqs...),
		TokenIDs:   append([]int64(nil), state.TokenIDs...),
		ContentIDs: append([]int64(nil), state.ContentIDs...),
		Phase:      append([]float32(nil), state.Phase...),
		Omega:      append([]float32(nil), state.Omega...),
		Energy:     append([]float32(nil), state.Energy...),
		Mass:       append([]float32(nil), state.Mass...),
		Heat:       append([]float32(nil), state.Heat...),
		Amp:        append([]float32(nil), state.Amp...),
		Pos:        append([]float32(nil), state.Pos...),
		Vel:        append([]float32(nil), state.Vel...),
		Clamped:    append([]bool(nil), state.Clamped...),
		Dark:       append([]bool(nil), state.Dark...),
	}
}

/*
project folds one Level3 message's orders into a single State batch. Entries
are packed in arrival order with no sort; the batch is sized exactly to the
message so one message costs one forward pass and nothing is retained.
*/
func (solver *Solver) project(message kraken.Level3Data, forcing forcingState) *sensorium.State {
	count := len(message.Bids) + len(message.Asks)

	if count == 0 {
		return nil
	}

	batch := &sensorium.State{
		N:          count,
		Bytes:      make([]int64, count),
		Seqs:       make([]int64, count),
		TokenIDs:   make([]int64, count),
		ContentIDs: make([]int64, count),
		Phase:      make([]float32, count),
		Omega:      make([]float32, count),
		Energy:     make([]float32, count),
		Mass:       make([]float32, count),
		Heat:       make([]float32, count),
		Amp:        make([]float32, count),
		Pos:        make([]float32, count*3),
		Vel:        make([]float32, count*3),
		Clamped:    make([]bool, count),
		Dark:       make([]bool, count),
	}

	index := 0

	for state := range solver.dataset.Step(message, forcing) {
		if state == nil || state.N != 1 {
			sensorium.StatePool.Put(state)
			continue
		}

		batch.Bytes[index] = state.Bytes[0]
		batch.Seqs[index] = state.Seqs[0]
		batch.TokenIDs[index] = state.TokenIDs[0]
		batch.ContentIDs[index] = state.ContentIDs[0]
		batch.Phase[index] = state.Phase[0]
		batch.Omega[index] = state.Omega[0]
		batch.Energy[index] = state.Energy[0]
		batch.Mass[index] = state.Mass[0]
		batch.Heat[index] = state.Heat[0]
		batch.Amp[index] = state.Amp[0]
		batch.Pos[index*3+0] = state.Pos[0]
		batch.Pos[index*3+1] = state.Pos[1]
		batch.Pos[index*3+2] = state.Pos[2]
		batch.Vel[index*3+0] = state.Vel[0]
		batch.Vel[index*3+1] = state.Vel[1]
		batch.Vel[index*3+2] = state.Vel[2]
		batch.Clamped[index] = state.Clamped[0]
		batch.Dark[index] = state.Dark[0]
		index++

		sensorium.StatePool.Put(state)
	}

	batch.N = index

	if batch.N == 0 {
		return nil
	}

	return batch
}

func (solver *Solver) Close() error {
	if solver == nil {
		return nil
	}

	solver.advanceMu.Lock()
	defer solver.advanceMu.Unlock()

	if solver.cancel != nil {
		solver.cancel()
	}

	if solver.physics != nil {
		solver.physics.Close()
		solver.physics = nil
	}

	return nil
}
