package cvd

import "github.com/theapemachine/symm/market/perspectives"

// rawRecord is cvd's bespoke pre-classification reading: the executed-flow axes
// (conviction, activity, drift) and their fusion, plus the standout the SNR is
// scored from — the full state behind one classified absorption reading. Written
// to runs/cvd_raw.jsonl when signals.cvd.raw_dump is enabled.
type rawRecord struct {
	TimestampUnixNano int64                     `json:"timestamp_unixnano"`
	Symbol            string                    `json:"symbol"`
	Price             float64                   `json:"price"`
	Category          perspectives.CategoryType `json:"category"`
	Signed            float64                   `json:"signed"`
	Gross             float64                   `json:"gross"`
	Count             float64                   `json:"count"`
	Conviction        float64                   `json:"conviction"`
	Tempo             float64                   `json:"tempo"`
	Volume            float64                   `json:"volume"`
	Activity          float64                   `json:"activity"`
	Drift             float64                   `json:"drift"`
	Fused             float64                   `json:"fused"`
	Standout          float64                   `json:"standout"`
	Confidence        float64                   `json:"confidence"`
	SNR               float64                   `json:"snr"`
}
