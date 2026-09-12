package probability

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Reading is one simplex and its named readouts.
*/
type Reading struct {
	Probabilities []float64
	Winner        int
	Confidence    float64
	Ambiguity     float64
	Sharpness     float64
}

/*
Distribution owns softmax then the winner, confidence, ambiguity, and sharpness.
*/
type Distribution struct {
	err error
	out Reading
}

func NewDistribution() core.Primitive {
	return &Distribution{}
}

func (op *Distribution) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		softmax := NewSoftmax()
		var probabilities []float64

		for pPtr := range softmax.Next(in) {
			probabilities = append(probabilities, *(*float64)(pPtr))
		}

		if err := softmax.Error(); err != nil {
			op.err = errors.Join(op.err, err)
			return
		}

		if len(probabilities) == 0 {
			op.err = errors.Join(op.err, core.ErrShape)
			return
		}

		winner := 0

		for index, p := range probabilities {
			if p > probabilities[winner] {
				winner = index
			}
		}

		ambiguityValEval := transport.NewEvaluate(NewAmbiguity())
		var ambiguityVal float64

		for out := range ambiguityValEval.Next(transport.NewValues(probabilities...).Next(nil)) {
			ambiguityVal = *(*float64)(out)
		}

		err := ambiguityValEval.Error()

		if err != nil {
			op.err = errors.Join(op.err, err)
			return
		}

		op.out = Reading{
			Probabilities: probabilities,
			Winner:        winner,
			Confidence:    probabilities[winner],
			Ambiguity:     ambiguityVal,
			Sharpness:     1 - ambiguityVal,
		}

		yield(unsafe.Pointer(&op.out))
	}
}

func (op *Distribution) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
