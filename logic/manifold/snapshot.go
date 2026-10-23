package manifold

import (
	"math"
	"slices"
	"time"

	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/types"
)

/*
SnapshotHost builds a ManifoldState from a resident sensorium host after a
step. Parallel and the L3 solver share this encoder so the fluid viewer sees
the same wire shape from either source.
*/
func SnapshotHost(host *sensorium.Manifold, version uint64, symbol string) *State {
	if host == nil {
		return nil
	}

	particles := host.State()
	if particles == nil || particles.N == 0 {
		return nil
	}

	modeOmega, modeReal, modeImag, modeLinewidth := host.SpectralModes()
	modes := make([]WaveMode, len(modeOmega))

	for index := range modeOmega {
		modes[index] = WaveMode{
			Omega:     modeOmega[index],
			Real:      modeReal[index],
			Imag:      modeImag[index],
			Linewidth: modeLinewidth[index],
		}
	}

	gridX, gridY, gridZ, gridSpacing := host.Grid()
	cells := gridX * gridY * gridZ
	momRho := make([]float32, cells*4)
	fieldEnergy := make([]float32, cells)
	waveReal := make([]float32, cells)
	waveImag := make([]float32, cells)
	densityScale, momentumScale, energyScale, waveScale := host.PackFields(
		momRho, fieldEnergy, waveReal, waveImag,
	)

	return &State{
		Symbol:  symbol,
		At:      time.Now(),
		Version: version,
		State: &sensorium.State{
			N:                 particles.N,
			CoherencePosition: slices.Clone(particles.CoherencePosition),
			Bytes:             slices.Clone(particles.Bytes),
			Seqs:              slices.Clone(particles.Seqs),
			TokenIDs:          slices.Clone(particles.TokenIDs),
			ContentIDs:        slices.Clone(particles.ContentIDs),
			Phase:             slices.Clone(particles.Phase),
			Omega:             slices.Clone(particles.Omega),
			Energy:            slices.Clone(particles.Energy),
			Mass:              slices.Clone(particles.Mass),
			Heat:              slices.Clone(particles.Heat),
			MaterialEnergy:    slices.Clone(particles.MaterialEnergy),
			Amp:               slices.Clone(particles.Amp),
			Pos:               slices.Clone(particles.Pos),
			Vel:               slices.Clone(particles.Vel),
			PilotVel:          slices.Clone(particles.PilotVel),
			PhasePotential:    slices.Clone(particles.PhasePotential),
			Clamped:           slices.Clone(particles.Clamped),
			Dark:              slices.Clone(particles.Dark),
		},
		GridX: gridX, GridY: gridY, GridZ: gridZ, GridSpacing: gridSpacing,
		MomRho: momRho, FieldEnergy: fieldEnergy, WaveReal: waveReal, WaveImag: waveImag,
		DensityScale: densityScale, MomentumScale: momentumScale,
		EnergyScale: energyScale, WaveScale: waveScale,
		Reading:    host.Reading(),
		Modes:      modes,
		Resultants: computePhaseResultants(particles),
	}
}

func computePhaseResultants(state *sensorium.State) []types.PhaseChannelResultant {
	if state == nil || state.N == 0 {
		return nil
	}

	var bidReal, bidImag, bidTotalAmp float64
	var bidCount int
	var askReal, askImag, askTotalAmp float64
	var askCount int

	for index := 0; index < state.N; index++ {
		phase := float64(state.Phase[index])
		amp := float64(state.Amp[index])
		cosP := math.Cos(phase)
		sinP := math.Sin(phase)

		if (state.TokenIDs[index] & 1) == 0 {
			bidReal += amp * cosP
			bidImag += amp * sinP
			bidTotalAmp += amp
			bidCount++
			continue
		}

		askReal += amp * cosP
		askImag += amp * sinP
		askTotalAmp += amp
		askCount++
	}

	bidCoherence := 0.0
	bidPhase := 0.0

	if bidTotalAmp > 0 {
		bidCoherence = math.Hypot(bidReal, bidImag) / bidTotalAmp
	}

	if bidCount > 0 {
		bidPhase = math.Atan2(bidImag, bidReal)
	}

	askCoherence := 0.0
	askPhase := 0.0

	if askTotalAmp > 0 {
		askCoherence = math.Hypot(askReal, askImag) / askTotalAmp
	}

	if askCount > 0 {
		askPhase = math.Atan2(askImag, askReal)
	}

	return []types.PhaseChannelResultant{
		{
			Side:           "bid",
			Count:          bidCount,
			TotalAmplitude: bidTotalAmp,
			Coherence:      bidCoherence,
			Phase:          bidPhase,
		},
		{
			Side:           "ask",
			Count:          askCount,
			TotalAmplitude: askTotalAmp,
			Coherence:      askCoherence,
			Phase:          askPhase,
		},
	}
}
