package leadlag

import "github.com/theapemachine/symm/market/perspectives"

// rawRecord is leadlag's bespoke reading: the classified cross-asset lead/lag
// output plus the standout the SNR is scored from. Written to
// runs/leadlag_raw.jsonl when signals.leadlag.raw_dump is enabled.
type rawRecord struct {
	TimestampUnixNano int64                     `json:"timestamp_unixnano"`
	Symbol            string                    `json:"symbol"`
	Category          perspectives.CategoryType `json:"category"`
	Strength          float64                   `json:"strength"`
	Confidence        float64                   `json:"confidence"`
	SNR               float64                   `json:"snr"`
	Standout          float64                   `json:"standout"`
	Last              float64                   `json:"last"`
	SpreadBPS         float64                   `json:"spread_bps"`
}
