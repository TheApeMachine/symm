package temporal

import (
	"iter"
	"math"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
RenewalRate accumulates quantity until a configured target is reached.
*/
type RenewalRate struct {
	*core.PrimitiveError
	target      float64
	origin      float64
	hasOrigin   bool
	accumulated float64
	spans       float64
	rate        float64
	lastSample  float64
	hasSample   bool
	input       data.Map[string]
	output      data.Map[float64]
}

func NewRenewalRate(target float64) *RenewalRate {
	output := data.NewOutputMap()
	output.Values["rate"] = 0
	output.Values["change"] = 0
	output.Values["maturity"] = 0
	output.Values["closed"] = 0
	output.Values["spans"] = 0
	output.Values["elapsed"] = 0
	output.Values["target"] = target

	return &RenewalRate{
		PrimitiveError: core.NewPrimitiveError(),
		target:         target,
		input: data.NewMap(
			"increment", "increment",
			"sample", "sample",
			"at", "at",
		),
		output: output,
	}
}

func (op *RenewalRate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			increment, incOK := values.Values["increment"]
			sample, sampleOK := values.Values["sample"]
			at, atOK := values.Values["at"]

			if !incOK || !sampleOK || !atOK {
				op.Error(core.ErrNotHeld)
				return
			}

			if increment < 0 || sample <= 0 || op.target <= 0 {
				op.Error(core.ErrDomain)
				return
			}

			if !op.hasOrigin {
				op.origin = at
				op.hasOrigin = true
			}

			op.accumulated += increment
			elapsed := (at - op.origin) / float64(time.Second)

			if elapsed < 0 {
				op.Error(core.ErrDomain)
				return
			}

			rate := op.rate
			change := 0.0
			closed := 0.0
			spans := op.spans
			maturity := spans / (spans + 1)

			if op.accumulated >= op.target && elapsed > 0 {
				rate = op.accumulated / elapsed
				closed = 1.0
				spans = op.spans + 1
				maturity = spans / (spans + 1)

				if op.hasSample {
					change = math.Log(sample / op.lastSample)
				}

				op.rate = rate
				op.spans = spans
				op.lastSample = sample
				op.hasSample = true
				op.accumulated = 0
				op.origin = at
			}

			op.output.Values["rate"] = rate
			op.output.Values["change"] = change
			op.output.Values["maturity"] = maturity
			op.output.Values["closed"] = closed
			op.output.Values["spans"] = spans
			op.output.Values["elapsed"] = elapsed
			op.output.Values["target"] = op.target

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
