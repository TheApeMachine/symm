package adaptive

import "sync/atomic"

/*
BernoulliRatio counts binary outcomes for unsupervised calibration (did the
label we trained on still win after the update?).
*/
type BernoulliRatio struct {
	hits   int64
	trials int64
}

/*
Observe records one trial, crediting a hit when success is true.
*/
func (ratio *BernoulliRatio) Observe(success bool) {
	atomic.AddInt64(&ratio.trials, 1)

	if success {
		atomic.AddInt64(&ratio.hits, 1)
	}
}

/*
Total is the number of recorded trials as float64 to match min-sample gates
that are configured in float form elsewhere.
*/
func (ratio *BernoulliRatio) Total() float64 {
	return float64(atomic.LoadInt64(&ratio.trials))
}

/*
Ratio returns successes divided by trials, or zero when empty.
*/
func (ratio *BernoulliRatio) Ratio() float64 {
	trials := atomic.LoadInt64(&ratio.trials)

	if trials == 0 {
		return 0
	}

	return float64(atomic.LoadInt64(&ratio.hits)) / float64(trials)
}

/*
Reset clears hit and trial counters.
*/
func (ratio *BernoulliRatio) Reset() {
	atomic.StoreInt64(&ratio.trials, 0)
	atomic.StoreInt64(&ratio.hits, 0)
}
