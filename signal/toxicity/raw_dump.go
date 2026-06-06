package toxicity

import (
	"github.com/theapemachine/symm/market/perspectives/types"
)

// rawRecord is toxicity's bespoke reading: the classified book-quality output and
// the SNR scored from its confidence standout (see measurement.go). Written to
// runs/toxicity_raw.jsonl when signals.toxicity.raw_dump is enabled.
type rawRecord struct {
	Symbol     string             `json:"symbol"`
	Category   types.CategoryType `json:"category"`
	Strength   float64            `json:"strength"`
	Confidence float64            `json:"confidence"`
	SNR        float64            `json:"snr"`
	Last       float64            `json:"last"`
	SpreadBPS  float64            `json:"spread_bps"`
}
