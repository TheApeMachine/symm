package algo

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
squareRootRLSState is what a SquareRootRLS has learned: its posterior,
whether it is ready, and the prior variance it was built with (checked on
load: a state is restored only into the learner it was taken from).
*/
type squareRootRLSState struct {
	State    RLSState
	Ready    bool
	Variance float64
}

/*
MarshalBinary is the learner's posterior (encoding.BinaryMarshaler). A
learner that failed has nothing trustworthy to save.
*/
func (op *SquareRootRLS) MarshalBinary() ([]byte, error) {
	if op.err != nil {
		return nil, fmt.Errorf("algo: square-root RLS failed, its state is not saved: %w", op.err)
	}

	var buf bytes.Buffer

	if err := gob.NewEncoder(&buf).Encode(squareRootRLSState{State: op.state, Ready: op.ready, Variance: op.variance}); err != nil {
		return nil, fmt.Errorf("algo: square-root RLS state: %w", err)
	}

	return buf.Bytes(), nil
}

/*
UnmarshalBinary restores a MarshalBinary state into a learner built with
the same prior variance.
*/
func (op *SquareRootRLS) UnmarshalBinary(blob []byte) error {
	var state squareRootRLSState

	if err := gob.NewDecoder(bytes.NewReader(blob)).Decode(&state); err != nil {
		return fmt.Errorf("%w: algo: square-root RLS state does not decode: %v", core.ErrShape, err)
	}

	if state.Variance != op.variance {
		return fmt.Errorf(
			"%w: algo: square-root RLS state has prior variance %g, this learner %g",
			core.ErrShape, state.Variance, op.variance,
		)
	}

	op.state, op.ready = state.State, state.Ready
	return nil
}
