package depthflow

import (
	"github.com/theapemachine/symm/market/perspectives/types"
)

// rawRecord is depthflow's bespoke reading. The multi-level imbalance and
// depth-weighted pressure are fused inside DepthSymbol.Measure(), so the dump
// captures the classified book-shape output plus the standout the SNR is scored
// from. Written to runs/depthflow_raw.jsonl when signals.depthflow.raw_dump is
// enabled.
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
