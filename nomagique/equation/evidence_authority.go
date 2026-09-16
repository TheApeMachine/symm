package equation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
EvidenceAuthorityInput is a measurement's maturity and SNR facts.
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
	*core.PrimitiveError

	out float64
}

func NewEvidenceAuthority() *EvidenceAuthority {
	return &EvidenceAuthority{PrimitiveError: core.NewPrimitiveError()}
}

func (evidenceAuthority *EvidenceAuthority) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*EvidenceAuthorityInput)(arriving)
			factor := 1.0

			if input.Estimated {
				factor = input.Unknown
			}

			if input.Estimated && input.SNRDefined {
				factor = input.Zero
			}

			if input.Estimated && input.SNRDefined && input.SNR > 0 {
				factor = input.SNR / (1.0 + input.SNR)
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

			evidenceAuthority.out = value

			if !yield(unsafe.Pointer(&evidenceAuthority.out)) {
				return
			}
		}
	}
}
