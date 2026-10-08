package transport

import (
	"context"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"golang.org/x/sync/errgroup"
)

/*
Parallel distributes sequential arrivals from an inbound sequence across its
configured branch primitives. Arrival i maps to branch i. Each branch's yields
are streamed downstream.
*/
type Parallel struct {
	*core.PrimitiveError
	Branches []core.Primitive
}

func NewParallel(branches ...core.Primitive) core.Primitive {
	return &Parallel{
		PrimitiveError: core.NewPrimitiveError(),
		Branches:       branches,
	}
}

func (op *Parallel) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		index := 0
		out := make([]iter.Seq[unsafe.Pointer], len(op.Branches))

		group, ctx := errgroup.WithContext(context.Background())

		for arriving := range in {
			i := index
			arr := arriving
			index++

			group.Go(func() error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				out[i] = op.Branches[i].Next(
					data.NewValue(arr).Next(nil),
				)

				return nil
			})
		}

		if err := group.Wait(); err != nil {
			op.Error(err)
		}

		for _, seq := range out {
			if seq == nil {
				continue
			}

			for o := range seq {
				if !yield(o) {
					return
				}
			}
		}
	}
}
