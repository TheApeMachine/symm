package transport

import "github.com/theapemachine/symm/nomagique/core"

// Zip pairs corresponding outputs from two configured branches. Neither branch
// rereads the original source. Unequal lengths are an explicit shape error.
type Zip struct {
	core.PrimitiveError
	left, right, output, current core.Primitive
}

func NewZip(left, right core.Primitive) *Zip { return &Zip{left: left, right: right} }
func (z *Zip) Next(in core.Primitive) core.Primitive {
	if z.output == nil {
		input := []core.Primitive{}
		left := []core.Primitive{}
		right := []core.Primitive{}
		core.Yield(NewIO(core.From(0)), in, func(n int, v core.Primitive) int { input = append(input, v); return n }, z)
		core.Yield(
			NewIO(core.From(0)),
			NewApply(z.left, NewIO(input...)),
			func(n int, v core.Primitive) int { left = append(left, v); return n },
			z,
		)
		core.Yield(
			NewIO(core.From(0)),
			NewApply(z.right, NewIO(input...)),
			func(n int, v core.Primitive) int { right = append(right, v); return n },
			z,
		)
		if len(left) != len(right) {
			z.Error(core.ErrShape)
			return nil
		}
		pairs := make([]core.Primitive, len(left))
		for i := range left {
			pairs[i] = core.From([]core.Primitive{left[i], right[i]})
		}
		z.output = NewIO(pairs...)
	}
	value := z.output.Next(nil)
	if value == nil {
		z.output = nil
	} else {
		z.current = value
	}
	return value
}
func (z *Zip) Read() any { return core.To[any](z.current) }
