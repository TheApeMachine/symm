package types

/*
Value is the universal atom: an interaction with
stateful, addressable memory.
*/
type Value[T, U any] func(T) U
