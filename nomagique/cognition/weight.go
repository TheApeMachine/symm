package cognition

import (
	"encoding/binary"
	"errors"
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
WeightSize is the exact byte length of one packed cognitive weight record.
*/
const WeightSize = 24

/*
PackedWeight is the unpacked reading of one 24-byte cognitive record. It is
plain wire payload: no methods.
*/
type PackedWeight struct {
	Count       uint64
	Probability float64
	WriteStep   uint64
}

/*
WeightRecord is one packed weight exactly as the engine stores it in the trie.
*/
type WeightRecord []byte

/*
Weight unpacks the engine's packed weight records. It is the one public owner
of the record layout outside the engine itself.
*/
type Weight struct {
	err error
	out PackedWeight
}

/*
NewWeight instantiates the packed weight unpacking Primitive.
*/
func NewWeight() core.Primitive {
	return &Weight{}
}

/*
Next unpacks each arriving record and yields the weight it holds. A record
shorter than the wire layout is recorded as a shape failure and ends the run.
*/
func (op *Weight) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if op.err != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			record := (*WeightRecord)(arriving)

			if len(*record) < WeightSize {
				op.Error(fmt.Errorf(
					"%w: cognition: packed weight is %d bytes, want %d",
					core.ErrShape,
					len(*record),
					WeightSize,
				))
				return
			}

			op.out = decodeWeight(*record)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Weight) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
encodeWeight writes the weight into a 24-byte buffer without heap allocation.
*/
func encodeWeight(dst []byte, weight PackedWeight) {
	binary.LittleEndian.PutUint64(dst[0:8], weight.Count)
	binary.LittleEndian.PutUint64(dst[8:16], math.Float64bits(weight.Probability))
	binary.LittleEndian.PutUint64(dst[16:24], weight.WriteStep)
}

/*
decodeWeight reads a weight from a full-length record.
*/
func decodeWeight(src []byte) PackedWeight {
	return PackedWeight{
		Count:       binary.LittleEndian.Uint64(src[0:8]),
		Probability: math.Float64frombits(binary.LittleEndian.Uint64(src[8:16])),
		WriteStep:   binary.LittleEndian.Uint64(src[16:24]),
	}
}

/*
effective returns the decay-adjusted weight at the current step:
w_eff = w * decay^(currentStep - writeStep).
*/
func (weight PackedWeight) effective(currentStep uint64, decayFactor float64) PackedWeight {
	if weight.WriteStep >= currentStep || decayFactor <= 0 || decayFactor >= 1 {
		return weight
	}

	multiplier := math.Pow(decayFactor, float64(currentStep-weight.WriteStep))
	weight.Count = uint64(math.Ceil(float64(weight.Count) * multiplier))
	weight.Probability *= multiplier

	return weight
}

/*
reinforce updates the association in place. Signed feedback contributes its
absolute mass to the denominator, and positive mass to the numerator:
(p + max(feedback, 0)) / (1 + abs(feedback)). One is the current unit of
association mass. Thus losses inhibit, larger grades adjust more, and zero
leaves the weight unchanged. An ungraded observation strengthens the
association by one unit: p += (1 - p) / (count + 1). Probability denotes
association strength, not a calibrated probability of profit.
*/
func reinforce(weight *PackedWeight, feedback float64, graded bool) {
	if !graded {
		weight.Probability += (1 - weight.Probability) / (float64(weight.Count) + 1)
		return
	}

	weight.Probability /= 1 + math.Abs(feedback)

	if feedback > 0 {
		weight.Probability += feedback / (1 + feedback)
	}
}
