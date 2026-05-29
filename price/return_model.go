package price

import (
	"math"
	"sync"

	"github.com/theapemachine/symm/config"
)

const (
	// MinForwardSamples is the fallback number of settled, anchored predictions
	// a (source, regime) bucket must accumulate before it is permitted to emit
	// a nonzero forward-return forecast. config.ForwardReturnMinSamples can
	// override this at runtime.
	MinForwardSamples = 30

	// forwardSlopeAlpha is the fallback EWMA smoothing factor for the per-sample
	// realized/confidence slope. config.ForwardReturnSlopeAlpha can override it.
	forwardSlopeAlpha = 0.05

	// significanceZ is the fallback one-sided normal quantile. The bucket's
	// mean realized forward return must clear zero by this many standard errors
	// before any trade is allowed.
	significanceZ = 1.96

	// PumpMinForwardSamples is the fallback reduced warmup bar for pump-regime
	// buckets. config.PumpForwardReturnMinSamples can override it.
	PumpMinForwardSamples = 8
)

type returnModelKey struct {
	source string
	regime string
}

type returnBucket struct {
	slope      float64 // EWMA of realizedReturn / confidence
	slopeSeen  bool
	count      int
	meanReturn float64 // Welford mean of realized forward return
	m2Return   float64 // Welford sum of squared deviations
}

// ReturnModel maps signal confidence to expected forward return from settled
// predictions, keyed by (source, regime). It is safe for concurrent callers:
// Observe and Forecast serialize access through mu. Callers record every
// settled anchored prediction with Observe and ask Forecast whether a fresh
// confidence value has enough positive evidence to trade.
type ReturnModel struct {
	mu      sync.Mutex
	buckets map[returnModelKey]*returnBucket
}

// NewReturnModel returns an initialized ReturnModel with an empty bucket map.
func NewReturnModel() *ReturnModel {
	return &ReturnModel{buckets: make(map[returnModelKey]*returnBucket)}
}

func (model *ReturnModel) bucketLocked(source, regime string) *returnBucket {
	key := returnModelKey{source: source, regime: regime}
	bucket := model.buckets[key]

	if bucket == nil {
		bucket = &returnBucket{}
		model.buckets[key] = bucket
	}

	return bucket
}

// Observe records one settled (confidence, realizedForwardReturn) pair.
// confidence is the joint confidence at prediction time; realizedReturn is the
// direction-signed forward return over the runway. Called once per settlement.
func (model *ReturnModel) Observe(source, regime string, confidence, realizedReturn float64) {
	if confidence <= 0 {
		return
	}

	model.mu.Lock()
	defer model.mu.Unlock()

	bucket := model.bucketLocked(source, regime)

	bucket.count++
	delta := realizedReturn - bucket.meanReturn
	bucket.meanReturn += delta / float64(bucket.count)
	delta2 := realizedReturn - bucket.meanReturn
	bucket.m2Return += delta * delta2

	sample := realizedReturn / confidence

	if !bucket.slopeSeen {
		bucket.slope = sample
		bucket.slopeSeen = true

		return
	}

	bucket.slope += configuredForwardSlopeAlpha() * (sample - bucket.slope)
}

// Forecast returns the expected forward return for a fresh measurement and
// whether the bucket is tradable. tradable is false (and expected is 0) until
// the bucket has enough settlements AND its mean realized forward return is
// statistically positive. The returned expected return is capped by the lower
// confidence bound of the bucket's realized forward-return mean; this prevents
// confidence spikes from projecting returns an order of magnitude larger than
// the edge the bucket has actually demonstrated.
func (model *ReturnModel) Forecast(source, regime string, confidence float64) (float64, bool) {
	return model.ForecastWithMin(source, regime, confidence, configuredForwardMinSamples())
}

// ForecastWithMin is Forecast with a caller-supplied minimum sample bar. Pump
// regimes pass the reduced pump sample bar; all other callers use Forecast. The
// significance test is identical regardless of the bar, so a low bar still
// cannot trade on noise.
func (model *ReturnModel) ForecastWithMin(
	source, regime string,
	confidence float64,
	minSamples int,
) (float64, bool) {
	if confidence <= 0 {
		return 0, false
	}

	if minSamples <= 0 {
		minSamples = configuredForwardMinSamples()
	}

	model.mu.Lock()
	defer model.mu.Unlock()

	bucket := model.buckets[returnModelKey{source: source, regime: regime}]

	if bucket == nil || bucket.count < minSamples || !bucket.slopeSeen {
		return 0, false
	}

	variance := 0.0

	if bucket.count > 1 {
		variance = bucket.m2Return / float64(bucket.count-1)
	}

	stderr := math.Sqrt(variance / float64(bucket.count))
	lowerBound := bucket.meanReturn - configuredForwardSignificanceZ()*stderr

	if lowerBound <= 0 {
		return 0, false
	}

	expected := bucket.slope * confidence

	if expected <= 0 {
		return 0, false
	}

	if expected > lowerBound {
		expected = lowerBound
	}

	return expected, true
}

/*
ExpectedReturn is the data-derived forward-return estimate for a fresh
(source, regime), with NO hard significance bar or minimum-sample cliff. It
shrinks the bucket's realized-return mean toward zero by the mean's own
signal-to-noise ratio:

	reliability = mean^2 / (mean^2 + stderr^2)
	expected    = mean * reliability

The shrinkage factor is the James-Stein / Wiener form: it is computed entirely
from the bucket's own statistics, not a tuned constant. A cold or noisy bucket
has a large standard error, so reliability -> 0 and the estimate collapses to
zero on its own -- there is no Z threshold and no MinForwardSamples gate to
guess. A bucket whose realized mean is consistently nonzero over many samples
has a small standard error, so reliability -> 1 and the full mean survives.

Sign comes from the data: a (source, regime) whose realized direction-signed
forward return is negative returns a negative expectation, which the caller's
edge>0 economics rejects without any explicit gate. reliability in [0,1] is
returned too, as a confidence weight the caller may surface or size on.
*/
func (model *ReturnModel) ExpectedReturn(source, regime string) (expected float64, reliability float64) {
	model.mu.Lock()
	defer model.mu.Unlock()

	bucket := model.buckets[returnModelKey{source: source, regime: regime}]

	// Need at least two samples to form a variance; with fewer, the standard
	// error is undefined and there is no basis to trust the mean as an edge.
	if bucket == nil || bucket.count < 2 {
		return 0, 0
	}

	mean := bucket.meanReturn
	variance := bucket.m2Return / float64(bucket.count-1)

	if variance <= 0 {
		// A reproducible mean across >=2 samples is as reliable as the data can
		// show; let it through at full weight.
		return mean, 1
	}

	stderr := math.Sqrt(variance / float64(bucket.count))
	denom := mean*mean + stderr*stderr

	if denom <= 0 {
		return 0, 0
	}

	reliability = mean * mean / denom

	return mean * reliability, reliability
}

// SampleCount returns the number of settled forward-return samples a
// (source, regime) bucket has accumulated. The trader uses it to decide whether
// a bucket is still cold (explore) or has enough evidence to be edge-gated
// (exploit). Locks ReturnModel.mu, matching ExpectedReturn.
func (model *ReturnModel) SampleCount(source, regime string) int {
	if model == nil {
		return 0
	}

	model.mu.Lock()
	defer model.mu.Unlock()

	bucket := model.buckets[returnModelKey{source: source, regime: regime}]

	if bucket == nil {
		return 0
	}

	return bucket.count
}

// Snapshot returns a serializable view of the forward-return buckets for run
// stats and offline analysis.
func (model *ReturnModel) Snapshot() []map[string]any {
	if model == nil {
		return nil
	}

	model.mu.Lock()
	defer model.mu.Unlock()

	rows := make([]map[string]any, 0, len(model.buckets))

	for key, bucket := range model.buckets {
		variance := 0.0

		if bucket.count > 1 {
			variance = bucket.m2Return / float64(bucket.count-1)
		}

		stderr := 0.0

		if bucket.count > 0 {
			stderr = math.Sqrt(variance / float64(bucket.count))
		}

		lowerBound := bucket.meanReturn - configuredForwardSignificanceZ()*stderr

		rows = append(rows, map[string]any{
			"source":        key.source,
			"regime":        key.regime,
			"sample_count":  bucket.count,
			"mean_return":   bucket.meanReturn,
			"stderr":        stderr,
			"lower_bound":   lowerBound,
			"slope":         bucket.slope,
			"slope_seen":    bucket.slopeSeen,
			"tradable_mean": lowerBound > 0,
		})
	}

	return rows
}

func configuredForwardMinSamples() int {
	if config.System != nil && config.System.ForwardReturnMinSamples > 0 {
		return config.System.ForwardReturnMinSamples
	}

	return MinForwardSamples
}

func configuredPumpForwardMinSamples() int {
	if config.System != nil && config.System.PumpForwardReturnMinSamples > 0 {
		return config.System.PumpForwardReturnMinSamples
	}

	return PumpMinForwardSamples
}

func configuredForwardSignificanceZ() float64 {
	if config.System != nil && config.System.ForwardReturnSignificanceZ > 0 {
		return config.System.ForwardReturnSignificanceZ
	}

	return significanceZ
}

func configuredForwardSlopeAlpha() float64 {
	if config.System != nil && config.System.ForwardReturnSlopeAlpha > 0 {
		if config.System.ForwardReturnSlopeAlpha > 1 {
			return 1
		}

		return config.System.ForwardReturnSlopeAlpha
	}

	return forwardSlopeAlpha
}
