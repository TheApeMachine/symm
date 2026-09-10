package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
EvidenceShare selects one member after normalization. An absent index reports
a shape error; zero total mass remains undefined.
*/
type EvidenceShare[U core.Floating] struct {
	core.Base[U, U]
	normalize *Normalize[U]
	at        *collection.At[U]
}

func NewEvidenceShare[U core.Floating](index int) *EvidenceShare[U] {
	return &EvidenceShare[U]{
		normalize: NewNormalize[U](),
		at:        collection.NewAt[U](index),
	}
}

func (op *EvidenceShare[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		var shares []U

		for share := range op.normalize.Next(in) {
			shares = append(shares, share.Read())
		}

		op.Error(op.normalize.Error())

		for selected := range op.at.Next(func(yield func(core.Primitive[[]U, []U]) bool) {
			carrier := &core.Carrier[[]U]{}
			yield(carrier.Carrier(shares))
		}) {
			if !yield(op.Carrier(selected.Read())) {
				return
			}
		}

		op.Error(op.at.Error())
	}
}
