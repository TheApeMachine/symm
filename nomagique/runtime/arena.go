//go:build goexperiment.arenas

package arenahelper

import "arena"

type Arena *arena.Arena

func NewArena() Arena {
	return arena.NewArena()
}

func Free(a Arena) {
	if a == nil {
		return
	}
	(*arena.Arena)(a).Free()
}

func MakeSlice[T any](a Arena, l, c int) []T {
	if a == nil {
		return make([]T, l, c)
	}
	return arena.MakeSlice[T](a, l, c)
}

func AppendA[T any](data []T, v T, a Arena) []T {
	if a == nil {
		return append(data, v)
	}
	if len(data) >= cap(data) {
		c := 2 * len(data)
		if c == 0 {
			c = 1
		}
		newData := arena.MakeSlice[T](a, len(data)+1, c)
		copy(newData, data)
		data = newData
		data[len(data)-1] = v
	} else {
		data = append(data, v)
	}
	return data
}
