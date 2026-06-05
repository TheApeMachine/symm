package pumpdump

// rawRecord is pumpdump's bespoke pre-classification reading: the trade that drove
// the update, the two self-scaled axes (RVOL, precursor), the window state they
// were derived from, and Observation — the scalar the classifier actually banded
// (post-projection, post-clamp). Observation is what the signal's pooled, online
// BandCalibrator fits the shared band edges to. Written to runs/pumpdump_raw.jsonl
// when signals.pumpdump.raw_dump is enabled.
type rawRecord struct {
	TimestampUnixNano int64   `json:"timestamp_unixnano"`
	Symbol            string  `json:"symbol"`
	Price             float64 `json:"price"`
	Qty               float64 `json:"qty"`
	Side              string  `json:"side"`
	Anchor            float64 `json:"anchor"`
	VolumeSum         float64 `json:"volume_sum"`
	RVOL              float64 `json:"rvol"`
	Precursor         float64 `json:"precursor"`
	Observation       float64 `json:"observation"`
}
