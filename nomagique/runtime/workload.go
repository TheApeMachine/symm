package runtime

import "iter"

/*
Workload is a piece of work that can be scheduled.
*/
type Workload[T any] struct {
	work func(iter.Seq[T])
}

func NewWorkload[T any](work func(iter.Seq[T])) *Workload[T] {
	return &Workload[T]{
		work: work,
	}
}

func (workload *Workload[T]) Run() {
	
}