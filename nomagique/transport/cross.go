package transport

import "github.com/theapemachine/symm/nomagique/core"

// Cross forms the Cartesian relation between two configured outputs. It does
// not compare, multiply or interpret pairs. Branch runs are captured once; pairs
// are delivered lazily, without allocating an N*M intermediate collection.
type Cross struct {
	core.PrimitiveError
	left, right core.Primitive
	a, b        []core.Primitive
	row, column int
	open        bool
	current     core.Primitive
}

func NewCross(left, right core.Primitive) *Cross { return &Cross{left: left, right: right} }
func (cross *Cross) Next(in core.Primitive) core.Primitive {
	if !cross.open {
		input := []core.Primitive{}
		core.Yield(NewIO(core.From(0)), in, func(n int, v core.Primitive) int { input = append(input, v); return n }, cross)
		core.Yield(
			NewIO(core.From(0)),
			NewApply(cross.left, NewIO(input...)),
			func(n int, v core.Primitive) int { cross.a = append(cross.a, v); return n },
			cross,
		)
		core.Yield(
			NewIO(core.From(0)),
			NewApply(cross.right, NewIO(input...)),
			func(n int, v core.Primitive) int { cross.b = append(cross.b, v); return n },
			cross,
		)
		cross.open = true
	}
	if cross.row == len(cross.a) || len(cross.b) == 0 {
		cross.a, cross.b = nil, nil
		cross.row, cross.column = 0, 0
		cross.open = false
		return nil
	}
	cross.current = core.From([]core.Primitive{cross.a[cross.row], cross.b[cross.column]})
	cross.column++
	if cross.column == len(cross.b) {
		cross.column = 0
		cross.row++
	}
	return cross.current
}
func (cross *Cross) Read() any { return core.To[any](cross.current) }
