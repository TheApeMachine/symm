package transport

import (
	"context"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"golang.org/x/sync/errgroup"
)

/*
Parallel distributes sequential arrivals from an inbound sequence across its
configured branch primitives. Arrival i maps to branch i. Each branch's yields
are streamed downstream.
*/
type Parallel struct {
	*core.PrimitiveError
	branches []core.Primitive
}

func NewParallel(branches ...core.Primitive) core.Primitive {
	return &Parallel{
		PrimitiveError: core.NewPrimitiveError(),
		branches:       branches,
	}
}

func (op *Parallel) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		values := make([]core.Primitive, 0, len(op.branches))
		out := make([]iter.Seq[unsafe.Pointer], len(op.branches))
		group, ctx := errgroup.WithContext(context.Background())

		for arriving := range in {
			values = append(values, *(*core.Primitive)(arriving))
		}

		for index, branch := range op.branches {
			group.Go(func() error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				out[index] = branch.Next(values[index].Next(nil))
				return nil
			})
		}

		if err := group.Wait(); err != nil {
			op.Error(err)
		}

		for _, seq := range out {
			for val := range seq {
				if !yield(val) {
					return
				}
			}
		}
	}
}
