package cognition

import (
	"encoding/binary"
	"math"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Observe turns one observed association into the write that records it.

It is configured with the observation being made — the precursor sequence, the
class that followed it, and the grade when one has been earned — and is handed
the store as it currently stands. What it answers with is the question that
writes the result back, so it drops straight into the store it just read.

Reading the link, moving it and addressing the write are one step because they
are one fact. Split across two owners they can disagree about what was there in
between.

The reinforcement is the recurrence a link follows when its context is seen
again. Without a grade the link strengthens on having been observed, at a rate
that falls as it is seen more often. With one, the grade's magnitude divides the
link down and only its positive part adds back, so a loss inhibits, a larger
grade moves it further, and a zero grade leaves it exactly where it was. An
ungraded observation and one graded zero are opposite readings and never share a
representation.
*/
type Observe struct {
	core.PrimitiveError
	current core.Primitive
}

func NewObserve(state core.Primitive) *Observe {
	return &Observe{current: state}
}

func (observe *Observe) Next(in core.Primitive) core.Primitive {
	return core.Yield(
		observe.current,
		in,
		func(held map[string][]byte, arriving *iradix.Tree[[]byte]) map[string][]byte {
			if arriving == nil {
				observe.Error(core.ErrNotHeld)

				return held
			}

			// Nothing was recognised, so there is nothing to record. An empty
			// question asks the store nothing rather than writing an empty
			// association, which would be a link to a situation that never was.
			if len(held["context"]) == 0 || len(held["class"]) == 0 {
				return map[string][]byte{}
			}
			step := Count(held["step"])
			key := makeBasinKey(held["class"], held["context"])
			grade, graded := held["feedback"]
			weight := PackedWeight{Probability: 1, WriteStep: step}

			if graded {
				// Neutral between reinforcement and inhibition, so a first
				// graded observation is not read as a link already believed.
				weight.Probability = 0.5
			}

			if existing, found := arriving.Get(key); found {
				weight = DecodeWeight(existing).Effective(step, Retention(held["retention"]))
			}
			weight.Count++
			weight.WriteStep = step

			if graded {
				weight.Reinforce(Retention(grade))
			} else {
				weight.Reinforce()
			}
			encoded := make([]byte, WeightSize)
			weight.Encode(encoded)

			// A sequence is also recorded as having been seen at all, which is
			// what lets a context that has never been followed by anything
			// still answer how surprising it was.
			return map[string][]byte{"selector": key, "data": encoded}
		},
		observe,
	)
}

func (observe *Observe) Read() any { return core.To[any](observe.current) }

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
