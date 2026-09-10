package cognition

import (
	"encoding/binary"
	"iter"
	"math"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Association is one observed precursor, the class that followed it, and the
grade when one has been earned.
*/
type Association struct {
	Context   []byte
	Class     []byte
	Step      uint64
	Retention float64
	Feedback  float64
	Graded    bool
}

/*
Write is the store mutation Observe names: the basin key and packed weight.
*/
type Write struct {
	Selector []byte
	Data     []byte
}

/*
ObserveInput is the store as it stands and the association being recorded.
*/
type ObserveInput struct {
	Tree        *iradix.Tree[[]byte]
	Association Association
}

/*
Observe turns one observed association into the write that records it.
Reading the link, moving it and addressing the write are one step because they
are one fact.
*/
type Observe struct {
	core.Base[ObserveInput, Write]
}

func NewObserve() *Observe {
	return &Observe{}
}

func (op *Observe) Next(
	in iter.Seq[core.Primitive[ObserveInput, ObserveInput]],
) iter.Seq[core.Primitive[Write, Write]] {
	return func(yield func(core.Primitive[Write, Write]) bool) {
		for arriving := range in {
			write, err := op.Record(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(write)) {
				return
			}
		}
	}
}

func (op *Observe) Record(input ObserveInput) (Write, error) {
	if input.Tree == nil {
		return Write{}, core.ErrNotHeld
	}

	assoc := input.Association

	if len(assoc.Context) == 0 || len(assoc.Class) == 0 {
		return Write{}, nil
	}

	key := makeBasinKey(assoc.Class, assoc.Context)
	weight := PackedWeight{Probability: 1, WriteStep: assoc.Step}

	if assoc.Graded {
		weight.Probability = 0.5
	}

	if existing, found := input.Tree.Get(key); found {
		weight = DecodeWeight(existing).Effective(assoc.Step, assoc.Retention)
	}

	weight.Count++
	weight.WriteStep = assoc.Step

	if assoc.Graded {
		weight.Reinforce(assoc.Feedback)
	}

	if !assoc.Graded {
		weight.Reinforce()
	}

	encoded := make([]byte, WeightSize)
	weight.Encode(encoded)
	return Write{Selector: key, Data: encoded}, nil
}

func (op *Observe) Map(write Write) map[string][]byte {
	if len(write.Data) == 0 {
		return map[string][]byte{}
	}

	return map[string][]byte{"selector": write.Selector, "data": write.Data}
}

/* Count reads an observation counter out of a record. */
func Count(encoded []byte) uint64 {
	if len(encoded) < 8 {
		return 0
	}

	return binary.LittleEndian.Uint64(encoded)
}

/* Retention reads a real-valued setting out of a record. */
func Retention(encoded []byte) float64 {
	if len(encoded) < 8 {
		return 0
	}

	return math.Float64frombits(binary.LittleEndian.Uint64(encoded))
}

/* Counted writes an observation counter into a record. */
func Counted(value uint64) []byte {
	encoded := make([]byte, 8)
	binary.LittleEndian.PutUint64(encoded, value)

	return encoded
}

/* Measured writes a real-valued setting into a record. */
func Measured(value float64) []byte {
	encoded := make([]byte, 8)
	binary.LittleEndian.PutUint64(encoded, math.Float64bits(value))

	return encoded
}
