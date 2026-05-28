package trader

import (
	"sync"

	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
confidenceAverages keeps one running mean of per-signal confidence
per source. The dashboard's gauges show this mean, not the raw latest
reading, so a single anomalous measurement doesn't whip the gauge
across the dial. The mean is also what tells us "Hawkes is averaging
0.05 right now" at a glance, which is the question the gauge is
actually answering — current signal strength as a stable rolling
quantity, not the most recent percentile rank.

EMAs are per-source so altcoin pairs and BTC/EUR don't blend together,
and per-source because each signal has its own scale and we want the
gauge to reflect that signal's typical strength, not a cross-source
average.

Concurrency: emas is protected by mu; the inner *adaptive.EMA carries
its own state but Next is not thread-safe, so we serialize per-source
access through the same lock. Contention is per-source and microsecond-
scale; the alternative (atomic snapshot of EMA state) would be over-
engineering for a gauge that ticks at signal rate.
*/
type confidenceAverages struct {
	mu   sync.Mutex
	emas map[string]*adaptive.EMA
}

func newConfidenceAverages() *confidenceAverages {
	return &confidenceAverages{
		emas: make(map[string]*adaptive.EMA),
	}
}

/*
Observe pushes a fresh confidence reading for source into its EMA and
returns the updated running mean. Sources unseen until now get a fresh
EMA seeded with the first observation so the gauge starts at the right
value instead of crawling up from zero.
*/
func (averages *confidenceAverages) Observe(source string, confidence float64) float64 {
	if averages == nil || source == "" {
		return confidence
	}

	averages.mu.Lock()
	defer averages.mu.Unlock()

	ema, ok := averages.emas[source]

	if !ok {
		ema = adaptive.NewEMA(confidence)
		averages.emas[source] = ema

		return confidence
	}

	value, err := ema.Next(0, confidence)

	if err != nil {
		return confidence
	}

	return value
}

/*
Snapshot returns a copy of the current source → mean confidence map
for diagnostic emit (run_stats).
*/
func (averages *confidenceAverages) Snapshot() map[string]float64 {
	if averages == nil {
		return nil
	}

	averages.mu.Lock()
	defer averages.mu.Unlock()

	out := make(map[string]float64, len(averages.emas))

	for source, ema := range averages.emas {
		out[source] = ema.Value()
	}

	return out
}
