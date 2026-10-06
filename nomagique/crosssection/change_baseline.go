package crosssection

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
ChangeBaseline evaluates an adaptive causal baseline over the signed fraction
published by ChangeCounts. The baseline runs on a private adapter, so its
working names never leak into the caller's state; only the baseline, the
divergence from it, and (when the scale is defined) its z-score are
published.
*/
type ChangeBaseline struct {
	*core.PrimitiveError
	baseline core.Primitive
	scratch  *data.Adapter
	valid    data.Map[string]
	fraction data.Map[string]
	moments  data.Map[string]
	value    data.Map[float64]
	output   data.Map[float64]
}

func NewChangeBaseline() *ChangeBaseline {
	return &ChangeBaseline{
		PrimitiveError: core.NewPrimitiveError(),
		baseline:       adaptive.NewBaseline(adaptive.NewWindow()),
		scratch:        data.NewAdapter(nil, data.NewState(data.NewMap())),
		valid:          data.NewMap("valid_member_count", "valid_member_count"),
		fraction:       data.NewMap("signed_fraction", "signed_fraction"),
		moments:        data.NewMap("center", "center", "scale", "scale"),
		value:          data.NewOutputMap(),
		output:         data.NewOutputMap(),
	}
}

func (op *ChangeBaseline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			var support data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.valid)) {
				support = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if support.Values["valid_member_count"] <= 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.fraction)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			fraction := values.Values["signed_fraction"]
			op.value.Values["value"] = fraction

			for range op.scratch.Next(data.NewValue(op.value)) {
			}

			for range op.baseline.Next(data.NewValue(op.scratch)) {
			}

			if err := op.baseline.Error(); err != nil {
				op.Error(err)
				return
			}

			var moments data.Map[float64]

			for pointer := range op.scratch.Next(data.NewValue(op.moments)) {
				moments = *(*data.Map[float64])(pointer)
			}

			if err := op.scratch.Error(); err != nil {
				op.Error(err)
				return
			}

			center := moments.Values["center"]
			scale := moments.Values["scale"]
			residual := fraction - center

			clear(op.output.Values)
			op.output.Values["signed_fraction_baseline"] = center
			op.output.Values["signed_fraction_divergence"] = residual

			if scale > 0 {
				op.output.Values["signed_fraction_zscore"] = residual / scale
			}

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
