package statistic

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
MarshalBinary is the estimator's running moments.
*/
func (op *Estimator) MarshalBinary() ([]byte, error) {
	if op.err != nil {
		return nil, fmt.Errorf("statistic: estimator failed, its state is not saved: %w", op.err)
	}

	var buf bytes.Buffer

	if err := gob.NewEncoder(&buf).Encode(op.moments); err != nil {
		return nil, fmt.Errorf("statistic: estimator state: %w", err)
	}

	return buf.Bytes(), nil
}

/*
UnmarshalBinary restores MarshalBinary moments.
*/
func (op *Estimator) UnmarshalBinary(blob []byte) error {
	var moments Moments

	if err := gob.NewDecoder(bytes.NewReader(blob)).Decode(&moments); err != nil {
		return fmt.Errorf("%w: statistic: estimator state does not decode: %v", core.ErrShape, err)
	}

	op.moments = moments
	return nil
}
