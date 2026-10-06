package crosssection

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
UpdateMember retains the focal member's price, derives its causal change
against the price it held before (calculus.RelativeChange), and retains the
change in the shared member store. The member identity arrives as the text
"member" on the adapter; the price is the configured metric label. The stage
owns only the wiring; the mathematics lives in the primitives it drives.

The member store is any keyed-map Primitive (store.NewKV[string, float64]),
shared with the reductions that read the cross-section back.
*/
type UpdateMember struct {
	*core.PrimitiveError
	label   string
	members core.Primitive
	change  core.Primitive
	scratch *data.Adapter
	prices  map[string]float64
	member  data.Map[string]
	price   data.Map[string]
	result  data.Map[string]
	pair    data.Map[float64]
}

func NewUpdateMember(label string, members core.Primitive) *UpdateMember {
	return &UpdateMember{
		PrimitiveError: core.NewPrimitiveError(),
		label:          label,
		members:        members,
		change:         calculus.NewRelativeChange(),
		scratch:        data.NewAdapter(nil, data.NewState(data.NewMap())),
		prices:         make(map[string]float64),
		member:         data.NewLiteral("member"),
		price:          data.NewMap(label, label),
		result:         data.NewMap("relative_change", "relative_change"),
		pair:           data.NewOutputMap(),
	}
}

func (op *UpdateMember) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var identity data.Map[string]

			for pointer := range adapter.Next(data.NewValue(op.member)) {
				identity = *(*data.Map[string])(pointer)
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.price)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			member, memberOK := identity.Values["member"]
			price, priceOK := values.Values[op.label]

			if !memberOK || !priceOK {
				op.Error(core.ErrNotHeld)
				return
			}

			prior, held := op.prices[member]
			op.prices[member] = price

			if !held {
				if !yield(arriving) {
					return
				}

				continue
			}

			op.pair.Values["current"] = price
			op.pair.Values["previous"] = prior

			for range op.scratch.Next(data.NewValue(op.pair)) {
			}

			for range op.change.Next(data.NewValue(op.scratch)) {
			}

			if err := op.change.Error(); err != nil {
				op.Error(err)
				return
			}

			var changed data.Map[float64]

			for pointer := range op.scratch.Next(data.NewValue(op.result)) {
				changed = *(*data.Map[float64])(pointer)
			}

			if err := op.scratch.Error(); err != nil {
				op.Error(err)
				return
			}

			for range op.members.Next(data.NewValue(map[string]float64{
				member: changed.Values["relative_change"],
			})) {
			}

			if err := op.members.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
