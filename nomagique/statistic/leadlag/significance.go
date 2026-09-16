package leadlag

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	correlation "github.com/theapemachine/symm/nomagique/statistic/correlation"
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
	sample  correlation.FisherSample
	reading correlation.FisherReading
}

func NewSignificance() *Significance {
	return &Significance{
		PrimitiveError: core.NewPrimitiveError(),
		fisher:         correlation.NewFisher(),
	}
}

func (significance *Significance) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			significance.sample = correlation.FisherSample{
				Correlation: m.Metrics["best_lag_correlation"].Raw,
				Support:     m.Metrics["overlap_pair_count"].Raw,
				SearchCount: m.Metrics["search_count"].Raw,
			}

			for out := range significance.fisher.Next(sequence.NewOne(unsafe.Pointer(&significance.sample)).Next(nil)) {
				significance.reading = *(*correlation.FisherReading)(out)
			}

			if err := significance.fisher.Error(); err != nil {
				significance.Error(err)

				if !yield(arriving) {
					return
				}

				continue
			}

			if significance.reading.Defined {
				m.Metrics["correlation_p_value"] = m.Metrics["correlation_p_value"].Write(significance.reading.PValue)
				m.Metrics["search_adjusted_p_value"] = m.Metrics["search_adjusted_p_value"].Write(significance.reading.SearchAdjustedPValue)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
