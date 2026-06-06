package pumpdump

// rawRecord is pumpdump's pre-classification reading written to runs/pumpdump_raw.jsonl
// when signals.pumpdump.raw_dump is enabled.
type rawRecord struct {
	TimestampUnixNano int64   `json:"timestamp_unixnano"`
	Symbol            string  `json:"symbol"`
	Price             float64 `json:"price"`
	Qty               float64 `json:"qty"`
	Side              string  `json:"side"`
	Anchor            float64 `json:"anchor"`
	GrossVolume       float64 `json:"gross_volume"`
	SignedVolume      float64 `json:"signed_volume"`
	RVOL              float64 `json:"rvol"`
	Precursor         float64 `json:"precursor"`
	Skew              float64 `json:"skew"`
	Lift              float64 `json:"lift"`
	Observation       float64 `json:"observation"`
}
