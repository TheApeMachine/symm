package associative

import (
	"encoding/binary"
	"iter"

	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

/*
Context turns the regions that lit up into the sequence an agent recognises.
*/
type Context struct {
	core.Base[grid.Impulse, cognition.Association]
}

func NewContext() *Context {
	return &Context{}
}

func (op *Context) Next(
	in iter.Seq[core.Primitive[grid.Impulse, grid.Impulse]],
) iter.Seq[core.Primitive[cognition.Association, cognition.Association]] {
	return func(yield func(core.Primitive[cognition.Association, cognition.Association]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(op.Encode(arriving.Read()))) {
				return
			}
		}
	}
}

func (op *Context) Encode(impulse grid.Impulse) cognition.Association {
	if !impulse.Ready || len(impulse.Regions) == 0 {
		return cognition.Association{}
	}

	sequence := make([]byte, 0, len(impulse.Regions)*8)
	var token [8]byte

	for _, region := range impulse.Regions {
		binary.BigEndian.PutUint64(token[:], region.Condition)
		sequence = append(sequence, token[:]...)
	}

	assoc := cognition.Association{
		Context:   sequence,
		Step:      impulse.Version,
		Retention: cognition.DefaultConfig().DecayFactor(),
		Class:     []byte(impulse.Moment),
		Feedback:  impulse.Grade,
		Graded:    impulse.Graded,
	}

	if impulse.Moment == "" {
		assoc.Class = nil
	}

	return assoc
}
