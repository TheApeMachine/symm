//go:build goexperiment.arenas

package data

import "arena"

type Allocator *arena.Arena

func NewAllocator() Allocator {
	return arena.NewArena()
}

func Free(allocator Allocator) {
	(*arena.Arena)(allocator).Free()
}

func MakeSlice[T any](allocator Allocator, l, c int) []T {
	if allocator == nil {
		return make([]T, l, c)
	}

	return arena.MakeSlice[T](allocator, l, c)
}

func AppendA[T any](data []T, v T, allocator Allocator) []T {
	if allocator == nil {
		return append(data, v)
	}

	if len(data) < cap(data) {
		return append(data, v)
	}

	c := 2 * len(data)

	if c == 0 {
		c = 1
	}

	newData := arena.MakeSlice[T](allocator, len(data)+1, c)
	copy(newData, data)
	newData[len(data)] = v

	return newData
}
