package probability

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
MarshalBinary is the calibrator's retained history (its retention is
configuration, rebuilt by its owner).
*/
func (op *Calibrator) MarshalBinary() ([]byte, error) {
	if op.err != nil {
		return nil, fmt.Errorf("probability: calibrator failed, its state is not saved: %w", op.err)
	}

	var buf bytes.Buffer

	if err := gob.NewEncoder(&buf).Encode(op.history); err != nil {
		return nil, fmt.Errorf("probability: calibrator state: %w", err)
	}

	return buf.Bytes(), nil
}

/*
UnmarshalBinary restores a MarshalBinary history.
*/
func (op *Calibrator) UnmarshalBinary(blob []byte) error {
	var history []float64

	if err := gob.NewDecoder(bytes.NewReader(blob)).Decode(&history); err != nil {
		return fmt.Errorf("%w: probability: calibrator state does not decode: %v", core.ErrShape, err)
	}

	op.history = history
	return nil
}
