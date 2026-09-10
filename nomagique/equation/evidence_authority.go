package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
EvidenceAuthorityInput is a measurement's maturity and SNR facts. Unknown-SNR
and zero-SNR factors are configuration, not new statistical laws.
*/
type EvidenceAuthorityInput struct {
	Estimated  bool
	SNRDefined bool
	SNR        float64
	Maturity   float64
	Unknown    float64
	Zero       float64
}

/*
EvidenceAuthority owns the supplied measurement's maturity/SNR weighting,
bounded to [0, 1].
*/
type EvidenceAuthority struct {
	core.Base[EvidenceAuthorityInput, float64]
}

func NewEvidenceAuthority() *EvidenceAuthority {
	return &EvidenceAuthority{}
}

func (op *EvidenceAuthority) Next(
	in iter.Seq[core.Primitive[EvidenceAuthorityInput, EvidenceAuthorityInput]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			input := arriving.Read()
			factor := 1.0

			if input.Estimated {
				factor = input.Unknown
			}

			if input.Estimated && input.SNRDefined {
				factor = input.Zero
			}

			if input.Estimated && input.SNRDefined && input.SNR > 0 {
				factor = input.SNR / (1 + input.SNR)
			}

			maturity := input.Maturity

			if maturity < 0 {
				maturity = 0
			}

			if maturity > 1 {
				maturity = 1
			}

			value := maturity * factor

			if value < 0 {
				value = 0
			}

			if value > 1 {
				value = 1
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}
	}
}
