package types

/*
Value is the universal atom: an interaction with
stateful, addressable memory.
*/
type Value[T, U any] func(T) U

/*
Standard Flume port primitives: closures that evaluate the incoming context/frame
to yield a typed value, or return a constant if unwired.
*/
type String = Value[any, string]
type Integer = Value[any, int]
type Float = Value[any, float64]
type Boolean = Value[any, bool]
type Map = Value[any, map[string]any]
type Bytes = Value[any, []byte]
type Any = Value[any, any]

/*
Const returns a Value closure that always returns the constant value.
*/
func Const[T any](val T) Value[any, T] {
	return func(any) T {
		return val
	}
}
