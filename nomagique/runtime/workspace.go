package runtime

import "context"

// Ingress accepts values from streaming producers under an explicit lifecycle.
type Ingress[T any] interface {
	Push(T)
	Status() Stage
}

// Workspace composes Workloads with the same stages and barriers as a Workload.
// Every stage, including the first, processes the same submitted observation.
type Workspace[T any] struct {
	*Workload[T]
}

func NewWorkspace[T any](ctx context.Context, name string, stages [][]Node[T]) *Workspace[T] {
	return &Workspace[T]{Workload: NewWorkload(ctx, name, stages)}
}
