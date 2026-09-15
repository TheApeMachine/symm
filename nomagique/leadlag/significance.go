package leadlag

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Significance reads the stamped correlation, overlap support, and search
count from the measurement, drives the Fisher-z approximation, and stamps
the p-value and search-adjusted p-value. Measurements without defined
significance pass through unchanged.
*/
type Significance struct {
	*core.PrimitiveError
	fisher  core.Primitive
	sample  nmcorrelation.FisherSample
	reading nmcorrelation.FisherReading
}

func NewSignificance() core.Primitive {
	return &Significance{
		PrimitiveError: core.NewPrimitiveError(),
		fisher:         nmcorrelation.NewFisher(),
	}
}

func (op *Significance) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			op.sample = nmcorrelation.FisherSample{
				Correlation: m.Metrics["best_lag_correlation"].Raw,
				Support:     m.Metrics["overlap_pair_count"].Raw,
				SearchCount: m.Metrics["search_count"].Raw,
			}

			for out := range op.fisher.Next(transport.NewOne(unsafe.Pointer(&op.sample)).Next(nil)) {
				op.reading = *(*nmcorrelation.FisherReading)(out)
			}

			if err := op.fisher.Error(); err != nil {
				op.Error(err)

				if !yield(arriving) {
					return
				}

				continue
			}

			if op.reading.Defined {
				m.Metrics["correlation_p_value"] = m.Metrics["correlation_p_value"].Write(op.reading.PValue)
				m.Metrics["search_adjusted_p_value"] = m.Metrics["search_adjusted_p_value"].Write(op.reading.SearchAdjustedPValue)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
