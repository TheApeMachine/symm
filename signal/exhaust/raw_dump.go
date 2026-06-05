package exhaust

import "github.com/theapemachine/symm/market/perspectives"

// rawRecord is exhaust's bespoke reading. Its microstructure features (depth
// trend, spread widening, pressure fade, imbalance flip) are fused inside
// exhaustMeasurement, so the dump captures the classified exhaustion output plus
// the standout the SNR is scored from — enough to characterise whether the
// exhaustion read is steady or jitter. Written to runs/exhaust_raw.jsonl when
// signals.exhaust.raw_dump is enabled.
type rawRecord struct {
	Symbol     string                    `json:"symbol"`
	Category   perspectives.CategoryType `json:"category"`
	Strength   float64                   `json:"strength"`
	Confidence float64                   `json:"confidence"`
	SNR        float64                   `json:"snr"`
	Standout   float64                   `json:"standout"`
	Last       float64                   `json:"last"`
	SpreadBPS  float64                   `json:"spread_bps"`
}
