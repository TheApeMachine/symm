package crosssection

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

type MemberUpdate struct {
	Member string
	Price  float64
}

/*
UpdateMember retains the focal member's price, derives its causal change
against the price it held before, and retains the change in the shared member
store.
*/
type UpdateMember struct {
	*core.PrimitiveError
	label   string
	members *MemberStore
	prices  map[string]float64
}

func NewUpdateMember(label string, members *MemberStore) *UpdateMember {
	return &UpdateMember{
		PrimitiveError: core.NewPrimitiveError(),
		label:          label,
		members:        members,
		prices:         make(map[string]float64),
	}
}

func (op *UpdateMember) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			update := (*MemberUpdate)(arriving)

			if update == nil || update.Member == "" {
				op.Error(core.ErrShape)
				return
			}

			prior, held := op.prices[update.Member]
			op.prices[update.Member] = update.Price

			if !held {
				continue
			}

			if prior <= 0 {
				op.Error(core.ErrDomain)
				return
			}

			relativeChange := (update.Price - prior) / prior
			op.members.Set(update.Member, relativeChange)

			for value := range data.NewValue(relativeChange).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
