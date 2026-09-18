package leadlag

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/statistic/correlation"
)

type pick struct {
	*core.PrimitiveError
	selectField func(*nmcorrelation.LeadLagReading) (float64, bool)
	out         float64
}

func (pick *pick) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			reading := (*nmcorrelation.LeadLagReading)(arriving)
			value, ok := pick.selectField(reading)

			if !ok {
				continue
			}

			pick.out = value

			if !yield(unsafe.Pointer(&pick.out)) {
				return
			}
		}
	}
}

func newPick(selectField func(*nmcorrelation.LeadLagReading) (float64, bool)) *pick {
	return &pick{PrimitiveError: core.NewPrimitiveError(), selectField: selectField}
}

func NewContemporaneous() core.Primitive {
	return newPick(func(reading *nmcorrelation.LeadLagReading) (float64, bool) {
		return reading.Contemporaneous, reading.Defined
	})
}

func NewBestLagCorrelation() core.Primitive {
	return newPick(func(reading *nmcorrelation.LeadLagReading) (float64, bool) {
		return reading.Correlation, reading.Defined
	})
}

func NewBestLagIndex() core.Primitive {
	return newPick(func(reading *nmcorrelation.LeadLagReading) (float64, bool) {
		return reading.LagIndex, reading.Defined
	})
}

func NewAbsoluteGain() core.Primitive {
	return newPick(func(reading *nmcorrelation.LeadLagReading) (float64, bool) {
		return reading.AbsoluteGain, reading.Defined
	})
}

func NewLagFraction() core.Primitive {
	return newPick(func(reading *nmcorrelation.LeadLagReading) (float64, bool) {
		return reading.LagFraction, reading.Defined
	})
}

func NewSearchCount() core.Primitive {
	return newPick(func(reading *nmcorrelation.LeadLagReading) (float64, bool) {
		return reading.SearchCount, reading.Defined
	})
}

func NewProminence() core.Primitive {
	return newPick(func(reading *nmcorrelation.LeadLagReading) (float64, bool) {
		return reading.Prominence, reading.ShapeDefined
	})
}

func NewCurvature() core.Primitive {
	return newPick(func(reading *nmcorrelation.LeadLagReading) (float64, bool) {
		return reading.Curvature, reading.ShapeDefined
	})
}
