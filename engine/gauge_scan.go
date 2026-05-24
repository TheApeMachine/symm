package engine

/*
GaugeScan accumulates per-symbol normalized scores for one measure pass.
*/
type GaugeScan struct {
	sum   float64
	count int
}

/*
ResetGaugeScan clears scan accumulators before the next measure pass.
*/
func (gaugeScan *GaugeScan) ResetGaugeScan() {
	gaugeScan.sum = 0
	gaugeScan.count = 0
}

/*
ObserveGaugeScore records one symbol-level normalized confidence for the scan set.
Zero scores are skipped so unscored symbols do not dilute the gauge mean.
*/
func (gaugeScan *GaugeScan) ObserveGaugeScore(score float64) {
	if score <= 0 {
		return
	}

	gaugeScan.sum += score
	gaugeScan.count++
}

/*
MeanGaugeConfidence returns the arithmetic mean across observed scan symbols.
*/
func (gaugeScan *GaugeScan) MeanGaugeConfidence() float64 {
	if gaugeScan.count == 0 {
		return 0
	}

	return gaugeScan.sum / float64(gaugeScan.count)
}
