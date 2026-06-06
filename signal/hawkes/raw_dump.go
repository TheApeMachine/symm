package hawkes

import (
	"github.com/theapemachine/symm/market/perspectives/types"
)

// rawRecord is hawkes's bespoke reading: the classified output plus the standout
// the SNR is scored from. Its deeper internals are fused inside the signal's
// Measure helper; this captures enough to certify whether the reading is steady
// or jitter. Written to runs/hawkes_raw.jsonl when signals.hawkes.raw_dump is enabled.
type rawRecord struct {
	TimestampUnixNano int64              `json:"timestamp_unixnano"`
	Symbol            string             `json:"symbol"`
	Category          types.CategoryType `json:"category"`
	Strength          float64            `json:"strength"`
	Confidence        float64            `json:"confidence"`
	SNR               float64            `json:"snr"`
	Standout          float64            `json:"standout"`
	Last              float64            `json:"last"`
	SpreadBPS         float64            `json:"spread_bps"`
}
