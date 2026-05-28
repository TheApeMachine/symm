package toxicity

import "time"

// defaultTracker is the process-wide tracker the toxicity signal feeds and that
// the book-imbalance reader (§16.3) consults via the package-level IsToxic and
// Measure helpers. There is exactly one toxicity component, so a single shared
// instance avoids threading a *Tracker through the depthflow/fluid book readers
// (which are independent engine components with no handle to the signal).
var defaultTracker = NewTracker()

// Default returns the process-wide tracker fed by the toxicity signal.
func Default() *Tracker {
	return defaultTracker
}

// IsToxic reports whether the shared tracker has flagged a resting level at the
// given price as toxic: a large, young, near-touch block that was cancelled
// rather than filled. Safe to call before the signal has produced any data (an
// unflagged level returns false), so the weighted-book reader can call it
// unconditionally to exclude toxic levels before distance-decay weighting.
func IsToxic(symbol string, price float64, at time.Time) bool {
	return defaultTracker.IsToxic(symbol, price, at)
}
