package runtime

type Node[T any, U any] interface {
	Step(T) U
}
