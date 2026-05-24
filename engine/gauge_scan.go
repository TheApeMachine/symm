package engine

import "sync"

/*
GaugeScan accumulates per-symbol normalized scores for one measure pass.
*/
type GaugeScan struct {
	mu    sync.Mutex
	peak  float64
	sum   float64
	count int
}

/*
ResetGaugeScan clears scan accumulators before the next measure pass.
*/
func (gaugeScan *GaugeScan) ResetGaugeScan() {
	gaugeScan.mu.Lock()
	defer gaugeScan.mu.Unlock()

	gaugeScan.peak = 0
	gaugeScan.sum = 0
	gaugeScan.count = 0
}

/*
ObserveGaugeScore records one symbol-level normalized confidence for the scan set.
Zero and negative readings are ignored so unscored symbols do not dilute the gauge.
*/
func (gaugeScan *GaugeScan) ObserveGaugeScore(score float64) {
	if score <= 0 {
		return
	}

	gaugeScan.mu.Lock()
	defer gaugeScan.mu.Unlock()

	gaugeScan.sum += score
	gaugeScan.count++

	if score > gaugeScan.peak {
		gaugeScan.peak = score
	}
}

/*
PeakGaugeConfidence returns the strongest normalized score observed this scan.
*/
func (gaugeScan *GaugeScan) PeakGaugeConfidence() float64 {
	gaugeScan.mu.Lock()
	defer gaugeScan.mu.Unlock()

	return gaugeScan.peak
}

/*
MeanGaugeConfidence returns the mean normalized score across the latest scan set.
*/
func (gaugeScan *GaugeScan) MeanGaugeConfidence() float64 {
	gaugeScan.mu.Lock()
	defer gaugeScan.mu.Unlock()

	if gaugeScan.count == 0 {
		return 0
	}

	return gaugeScan.sum / float64(gaugeScan.count)
}
