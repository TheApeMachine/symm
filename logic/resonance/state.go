package resonance

import (
	"bytes"
	"encoding"
	"encoding/gob"
	"fmt"
	"slices"
	"sync/atomic"

	"github.com/theapemachine/errnie"
)

/*
symbolState is one symbol's saved solver state: its step count, whether it
held a reference (a touch midpoint) yet, its feature standardizers'
moments and latest standardized values, and its coder's state
(learning.PredictiveCoder.MarshalBinary). Features names the inputs it was
learned over, in order: a state over other features is refused.
*/
type symbolState struct {
	Features     []Feature
	Steps        int64
	Referenced   bool
	Moments      [][]byte
	Standardized []float64
	Coder        []byte
}

/*
Symbols lists the symbols the solver holds a coder for, sorted.
*/
func (solver *Solver) Symbols() []string {
	symbols := make([]string, 0)

	solver.detectors.Range(func(key, _ any) bool {
		symbols = append(symbols, key.(string))
		return true
	})

	slices.Sort(symbols)
	return symbols
}

/*
SaveSymbol is symbol's state, for a later solver to continue from
(LoadSymbol). The coder's pending predictions are dropped first, in this
solver too (learning.PredictiveCoder.DropPending: a later run must not
resolve them against prices after the gap); dropped is how many. Save at
the end of a run of steps, not between them.
*/
func (solver *Solver) SaveSymbol(symbol string) (blob []byte, dropped int, err error) {
	loaded, found := solver.detectors.Load(symbol)

	if !found {
		return nil, 0, errnie.Err(errnie.NotFound, "resonance: no coder for "+symbol+" to save", nil)
	}

	coder := loaded.(interface {
		encoding.BinaryMarshaler
		DropPending() (int, error)
	})

	if dropped, err = coder.DropPending(); err != nil {
		return nil, 0, errnie.Err(errnie.Internal, "resonance: dropping "+symbol+"'s pending predictions", err)
	}

	scorer := solver.scorer(symbol)
	state := symbolState{
		Features:     Features,
		Standardized: slices.Clone(scorer.standardized),
		Moments:      make([][]byte, len(scorer.moments)),
	}

	if steps, held := solver.steps.Load(symbol); held {
		state.Steps = steps.(*atomic.Int64).Load()
	}

	_, state.Referenced = solver.references.Load(symbol)

	for index, moments := range scorer.moments {
		if state.Moments[index], err = moments.(encoding.BinaryMarshaler).MarshalBinary(); err != nil {
			return nil, 0, errnie.Err(errnie.Internal, fmt.Sprintf("resonance: %s's %s moments", symbol, Features[index].Source), err)
		}
	}

	if state.Coder, err = coder.MarshalBinary(); err != nil {
		return nil, 0, errnie.Err(errnie.Internal, "resonance: "+symbol+"'s coder", err)
	}

	var buf bytes.Buffer

	if err := gob.NewEncoder(&buf).Encode(state); err != nil {
		return nil, 0, errnie.Err(errnie.Internal, "resonance: encoding "+symbol+"'s state", err)
	}

	return buf.Bytes(), dropped, nil
}

/*
LoadSymbol restores a SaveSymbol state as symbol's: the solver continues
from it as if it had stepped the symbol itself. Only a symbol the solver
has not stepped yet takes a state; a state that does not decode or does
not fit this solver's coder is an error.
*/
func (solver *Solver) LoadSymbol(symbol string, blob []byte) error {
	if _, stepped := solver.detectors.Load(symbol); stepped {
		return errnie.Err(errnie.Conflict, "resonance: "+symbol+" already has a coder: a state loads only before its first step", nil)
	}

	var state symbolState

	if err := gob.NewDecoder(bytes.NewReader(blob)).Decode(&state); err != nil {
		return errnie.Err(errnie.UnprocessableContent, "resonance: "+symbol+"'s state does not decode", err)
	}

	if !slices.Equal(state.Features, Features) || len(state.Moments) != len(Features) || len(state.Standardized) != len(Features) {
		return errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
			"resonance: %s's state was learned over features %v, this solver reads %v", symbol, state.Features, Features,
		), nil)
	}

	scorer := newFeatureScorer()

	for index, moments := range scorer.moments {
		if err := moments.(encoding.BinaryUnmarshaler).UnmarshalBinary(state.Moments[index]); err != nil {
			return errnie.Err(errnie.UnprocessableContent, fmt.Sprintf("resonance: %s's %s moments", symbol, Features[index].Source), err)
		}
	}

	copy(scorer.standardized, state.Standardized)

	coder, err := solver.coder(symbol, len(Features))

	if err != nil {
		return err
	}

	if err := coder.UnmarshalBinary(state.Coder); err != nil {
		solver.detectors.Delete(symbol)
		return errnie.Err(errnie.UnprocessableContent, "resonance: "+symbol+"'s coder state", err)
	}

	solver.standardizers.Store(symbol, scorer)
	steps := &atomic.Int64{}
	steps.Store(state.Steps)
	solver.steps.Store(symbol, steps)

	// Only whether a reference was held matters (Step passes the current
	// midpoint); the stored value marks it.
	if state.Referenced {
		solver.references.Store(symbol, 1.0)
	}

	return nil
}
