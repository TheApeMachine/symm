package toxicity

import "time"

/*
NearTouchToxic reports whether the shared tracker currently has an active
near-touch toxic flag on this symbol. Book-quality readers use it to withhold
churn amplification while spoofed liquidity is resting at the touch.
*/
func NearTouchToxic(symbol string, at time.Time) bool {
	defaultTracker.mu.Lock()
	defer defaultTracker.mu.Unlock()

	state := defaultTracker.symbols[symbol]

	if state == nil {
		return false
	}

	return bookQualitySnapshotLocked(state, at).toxicNear
}
