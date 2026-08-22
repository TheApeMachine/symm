package sensorium

import (
	"math"

	"github.com/theapemachine/errnie"
)

/*
Tokenizer is the original UniversalTokenizer: (byte, sequence index) → State.
*/
type Tokenizer struct {
	gridX, gridY, gridZ int
	segmentLen          int64
	spacing             float32
	Compressor          *Compressor
	Datasets            []Dataset
}

/*
Dataset yields (byte, sequence index) pairs the loader streams into MakeBatch.
*/
type Dataset interface {
	Name() string
	Generate() [][2]int64
}

func NewTokenizer(
	gridX, gridY, gridZ int,
	segmentLen int64,
	datasets ...Dataset,
) (*Tokenizer, error) {
	if gridX <= 0 || gridY <= 0 || gridZ <= 0 || segmentLen <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"tokenizer: grid dims and segment length must be positive",
			nil,
		))
	}

	maximumAxis := max(gridX, gridY, gridZ)

	return &Tokenizer{
		gridX:      gridX,
		gridY:      gridY,
		gridZ:      gridZ,
		segmentLen: segmentLen,
		spacing:    1 / float32(maximumAxis),
		Compressor: NewCompressor(int(segmentLen)),
		Datasets:   datasets,
	}, nil
}

func (tokenizer *Tokenizer) MakeBatch(bytes []int64, seqs []int64) (*State, error) {
	if len(bytes) == 0 || len(seqs) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"tokenizer: byte_vals and seq_idxs must not be empty",
			nil,
		))
	}

	if len(bytes) != len(seqs) {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"tokenizer: byte_vals and seq_idxs must have the same length",
			nil,
		))
	}

	filteredBytes, filteredSeqs, counts := tokenizer.Compressor.Filter(bytes, seqs)

	if len(filteredBytes) == 0 {
		return nil, nil
	}

	state := newState(len(filteredBytes))

	for index, byteValue := range filteredBytes {
		tokenizer.writeParticle(state, index, byteValue, filteredSeqs[index], counts[index], 1, 0)
	}

	return state, nil
}

/*
MakeDarkBatch is the original vacuum superposition: every byte at one sequence
index, tiny mass, heat=1. Darks bypass the compressor.
*/
func (tokenizer *Tokenizer) MakeDarkBatch(seqIdx int64, mass float32) *State {
	state := newState(256)

	for byteValue := int64(0); byteValue < 256; byteValue++ {
		tokenizer.writeParticle(state, int(byteValue), byteValue, seqIdx, mass, mass, 1)
		state.Dark[byteValue] = true
	}

	return state
}

func (tokenizer *Tokenizer) writeParticle(
	state *State,
	index int,
	byteValue, seq int64,
	mass, energy, heat float32,
) {
	rel := modSeg(seq, tokenizer.segmentLen)
	state.Bytes[index] = byteValue
	state.Seqs[index] = seq
	state.TokenIDs[index] = (rel << 8) | byteValue
	state.ContentIDs[index] = byteValue & 0xFF
	state.Phase[index] = tokenizer.positionPhase(seq)
	state.Omega[index] = byteOmega(byteValue)
	state.Energy[index] = energy
	state.Mass[index] = mass
	state.Heat[index] = heat
	state.Pos[index*3+0] = tokenizer.axis(rel, tokenizer.gridX)
	state.Pos[index*3+1] = tokenizer.axis(rel/int64(tokenizer.gridX), tokenizer.gridY)
	state.Pos[index*3+2] = tokenizer.axis(rel/int64(tokenizer.gridX*tokenizer.gridY), tokenizer.gridZ)
}

func (tokenizer *Tokenizer) axis(index int64, extent int) float32 {
	wrapped := index % int64(extent)

	if wrapped < 0 {
		wrapped += int64(extent)
	}

	return float32(wrapped) * tokenizer.spacing
}

func (tokenizer *Tokenizer) positionPhase(seq int64) float32 {
	rel := float64(modSeg(seq, tokenizer.segmentLen))
	seg := math.Floor(float64(seq) / float64(tokenizer.segmentLen))
	beat := math.Mod(seg, 32) / 32
	phase := (2*math.Pi)*(rel/float64(tokenizer.segmentLen)) + math.Pi*beat
	wrapped := math.Mod(phase, 2*math.Pi)

	if wrapped < 0 {
		wrapped += 2 * math.Pi
	}

	return float32(wrapped)
}

func byteOmega(byteValue int64) float32 {
	return float32(-4 + 8*(float64(byteValue)/255.0))
}
