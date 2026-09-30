//go:build !goexperiment.arenas

package data

type Allocator any

func NewAllocator() Allocator {
	return nil
}

func Free(allocator Allocator) {}

func New[T any](allocator Allocator) *T {
	return new(T)
}

func MakeSlice[T any](allocator Allocator, l, c int) []T {
	return make([]T, l, c)
}

func AppendA[T any](data []T, v T, allocator Allocator) []T {
	return append(data, v)
}
