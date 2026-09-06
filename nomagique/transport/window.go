package transport

import (
	"github.com/theapemachine/symm/nomagique/core"
)

// Window exposes fixed-width overlapping groups within a delivery run. Width
// and stride are structural configuration, not feature modes. Width 2 / stride
// 1 supplies consecutive pairs; width N / stride N supplies disjoint groups.
type Window struct {
	core.PrimitiveError
	width, stride   int
	output, current core.Primitive
}

func NewWindow(width, stride int) *Window {
	window := &Window{width: width, stride: stride}
	if width < 1 || stride < 1 {
		window.Error(core.ErrShape)
	}
	return window
}
func (window *Window) Next(in core.Primitive) core.Primitive {
	if window.Error() != nil {
		return nil
	}
	if window.output == nil {
		values := []core.Primitive{}
		core.Yield(NewIO(core.From(0)), in, func(n int, v core.Primitive) int { values = append(values, v); return n }, window)
		groups := []core.Primitive{}
		for start := 0; start+window.width <= len(values); start += window.stride {
			groups = append(groups, core.From(append([]core.Primitive(nil), values[start:start+window.width]...)))
		}
		window.output = NewIO(groups...)
	}
	value := window.output.Next(nil)
	if value == nil {
		window.output = nil
	} else {
		window.current = value
	}
	return value
}
func (window *Window) Read() any { return core.To[any](window.current) }
