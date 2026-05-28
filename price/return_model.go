package price

import (
	"math"
	"sync"
)

const (
	// MinForwardSamples is the number of settled, anchored predictions a
	// (source, regime) bucket must accumulate before it is permitted to emit
	// a nonzero forward-return forecast. Below this the bucket's forecast is
	// 0, so the entry edge gate rejects the trade while feedback keeps
	// accumulating.
	MinForwardSamples = 30

	// forwardSlopeAlpha smooths the per-sample realized/confidence slope.
	forwardSlopeAlpha = 0.05

	// significanceZ is the 97.5 % one-sided normal quantile. The bucket's
	// mean realized forward return must clear zero by this many standard
	// errors before any trade is allowed.
	significanceZ = 1.96
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

// ReturnModel maps signal confidence to expected forward return, learned from
// settled predictions, keyed by (source, regime).
type ReturnModel struct {
	mu      sync.Mutex
	buckets map[returnModelKey]*returnBucket
}

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

	bucket.slope += forwardSlopeAlpha * (sample - bucket.slope)
}

// Forecast returns the expected forward return for a fresh measurement and
// whether the bucket is tradable. tradable is false (and expected is 0) until
// the bucket has >= MinForwardSamples settlements AND its mean realized forward
// return is statistically positive at significanceZ.
func (model *ReturnModel) Forecast(source, regime string, confidence float64) (float64, bool) {
	if confidence <= 0 {
		return 0, false
	}

	model.mu.Lock()
	defer model.mu.Unlock()

	bucket := model.buckets[returnModelKey{source: source, regime: regime}]

	if bucket == nil || bucket.count < MinForwardSamples || !bucket.slopeSeen {
		return 0, false
	}

	variance := 0.0

	if bucket.count > 1 {
		variance = bucket.m2Return / float64(bucket.count-1)
	}

	stderr := math.Sqrt(variance / float64(bucket.count))

	if bucket.meanReturn-significanceZ*stderr <= 0 {
		return 0, false
	}

	expected := bucket.slope * confidence

	if expected <= 0 {
		return 0, false
	}

	return expected, true
}
