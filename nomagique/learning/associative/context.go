package associative

import (
	"encoding/binary"

	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

/*
Context turns the regions that lit up into the sequence an agent recognises.

The grid hands over its regions strongest first, so writing them out in that
order makes a shorter sequence the opening of a longer one. That is what lets
the store answer for a situation it has never seen in full: the strongest few
regions it has seen before are the opening it reaches through.

Each region is written whole, widest byte first, so two situations name the same
sequence only when the same regions lit up in the same order. A region carries
the directions of its level and its change inside its own identity, so "this
region, rising" and "this region, falling" are different regions here, which is
the difference between a precursor and its opposite.
*/
type Context struct {
	core.PrimitiveError
	current core.Primitive
}

func NewContext(state core.Primitive) *Context {
	return &Context{current: state}
}

func (context *Context) Next(in core.Primitive) core.Primitive {
	return core.Yield(
		context.current,
		in,
		func(held map[string][]byte, arriving grid.Impulse) map[string][]byte {
			// Formation has not completed, so there are no regions to
			// recognise. Nothing is learned from a grid that cannot yet say
			// what lit up, which is not the same as a grid that says nothing did.
			if !arriving.Ready || len(arriving.Regions) == 0 {
				return nil
			}
			sequence := make([]byte, 0, len(arriving.Regions)*8)
			var token [8]byte

			for _, region := range arriving.Regions {
				binary.BigEndian.PutUint64(token[:], region.Condition)
				sequence = append(sequence, token[:]...)
			}
			observed := map[string][]byte{
				"context":   sequence,
				"step":      cognition.Counted(arriving.Version),
				"retention": cognition.Measured(cognition.DefaultConfig().DecayFactor()),
				"label":     []byte(arriving.Label),
			}

			// What followed is what the agent is being taught, and the record
			// is what says so. A frame the record put no moment on teaches
			// nothing: it is a situation nobody has confirmed anything about.
			if arriving.Moment != "" {
				observed["class"] = []byte(arriving.Moment)
			}

			// The record's grade is what makes two moments sharing one
			// situation separable at all: without it the moment simply seen
			// most often wins every context they have in common.
			if arriving.Graded {
				observed["feedback"] = cognition.Measured(arriving.Grade)
			}

			return observed
		},
		context,
	)
}

func (context *Context) Read() any { return core.To[any](context.current) }
