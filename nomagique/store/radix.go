package store

import (
	"bytes"
	"errors"
	"iter"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Radix associates values with ordered keys. It never mutates its configured
source: each arrival is applied to a tree and the next tree is handed over.
*/
type Radix struct {
	err  error
	held *iradix.Tree[[]byte]
	out  *iradix.Tree[[]byte]
}

func NewRadix(current ...*iradix.Tree[[]byte]) core.Primitive {
	held := iradix.New[[]byte]()

	if len(current) > 0 && current[0] != nil {
		held = current[0]
	}

	return &Radix{held: held}
}

func (op *Radix) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if op.held == nil {
				op.held = iradix.New[[]byte]()
			}

			fields := *(*map[string][]byte)(arriving)
			selector, selecting := fields["selector"]
			data, writing := fields["data"]

			if !writing {
				op.out = op.held

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			if !selecting {
				op.Error(core.ErrShape)
				op.out = op.held

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			written, _, _ := op.held.Insert(selector, bytes.Clone(data))
			op.held = written
			op.out = written

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Radix) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
