package trader

import (
	"sync"
	"time"

	"github.com/theapemachine/nomagique/geometry"
	"github.com/theapemachine/symm/logic"
)

const (
	corpusCapacity    = 10000
	corpusRetrievalK  = 8
	corpusBackfillMax = 64
)

/*
CortexRouter performs continuous similarity-based routing over market
state observations using PhaseDial geometry. It replaces the string-prefix
radix tree match with cosine distance over the full continuous feature
space, so ρ=0.90 and ρ=0.99 resolve to genuinely different neighbourhoods
rather than collapsing into the same "saturation" token.

Each symbol gets its own FloatEncoder so normalization statistics track
that symbol's own distribution (BTC Reynolds ≠ ETH Reynolds). The corpus
is shared across symbols since cross-symbol analogues are valuable.
*/
type CortexRouter struct {
	mu       sync.RWMutex
	encoders map[string]*geometry.FloatEncoder
	corpus   *geometry.Corpus
	pending  []pendingEntry
}

/*
pendingEntry is a corpus entry awaiting outcome backfill. When the next
observation arrives for the same symbol, we compute the realized return
and backfill the entry into the corpus with the actual outcome.
*/
type pendingEntry struct {
	symbol   string
	dial     geometry.PhaseDial
	at       time.Time
	price    float64
	category string
}

/*
ContinuousRouting holds the result of similarity-based corpus retrieval
for a single observation. It provides the weighted predicted outcome and
the raw match list so the caller can inspect confidence.
*/
type ContinuousRouting struct {
	PredictedReturnBps float64                `json:"predictedReturnBps"`
	MatchCount         int                    `json:"matchCount"`
	TopSimilarity      float64                `json:"topSimilarity"`
	Matches            []geometry.CorpusMatch `json:"-"`
}

/*
NewCortexRouter creates a router with an empty shared corpus.
*/
func NewCortexRouter() *CortexRouter {
	return &CortexRouter{
		encoders: make(map[string]*geometry.FloatEncoder),
		corpus:   geometry.NewCorpus(corpusCapacity),
		pending:  make([]pendingEntry, 0, corpusBackfillMax),
	}
}

/*
Route encodes the observation's continuous feature vector into phase
space, queries the corpus for historical analogues, and returns the
similarity-weighted predicted outcome.
*/
func (router *CortexRouter) Route(
	observation *cortexObservation,
) ContinuousRouting {
	if observation == nil {
		return ContinuousRouting{}
	}

	features := router.extractFeatures(observation)

	if len(features) == 0 {
		return ContinuousRouting{}
	}

	encoder := router.encoder(observation.symbol)
	dial := encoder.Encode(features)
	encoder.Update(features)

	router.backfill(observation)

	router.mu.Lock()
	router.pending = append(router.pending, pendingEntry{
		symbol:   observation.symbol,
		dial:     dial,
		at:       observation.at,
		price:    router.currentPrice(observation),
		category: observation.class(),
	})

	if len(router.pending) > corpusBackfillMax {
		router.pending = router.pending[len(router.pending)-corpusBackfillMax:]
	}

	router.mu.Unlock()

	matches := router.corpus.Scan(dial, corpusRetrievalK)
	predicted := geometry.WeightedOutcome(matches)

	topSimilarity := 0.0

	if len(matches) > 0 {
		topSimilarity = matches[0].Similarity
	}

	return ContinuousRouting{
		PredictedReturnBps: predicted,
		MatchCount:         len(matches),
		TopSimilarity:      topSimilarity,
		Matches:            matches,
	}
}

/*
CorpusSize returns the current corpus population.
*/
func (router *CortexRouter) CorpusSize() int {
	return router.corpus.Size()
}

func (router *CortexRouter) encoder(symbol string) *geometry.FloatEncoder {
	router.mu.RLock()
	encoder := router.encoders[symbol]
	router.mu.RUnlock()

	if encoder != nil {
		return encoder
	}

	router.mu.Lock()
	defer router.mu.Unlock()

	encoder = router.encoders[symbol]

	if encoder != nil {
		return encoder
	}

	encoder = geometry.NewFloatEncoder()
	router.encoders[symbol] = encoder

	return encoder
}

/*
backfill resolves pending entries whose outcome can now be determined.
When a new observation arrives for a symbol that has a pending entry, the
price delta becomes the realized return in basis points.
*/
func (router *CortexRouter) backfill(observation *cortexObservation) {
	router.mu.Lock()
	defer router.mu.Unlock()

	remaining := router.pending[:0]

	for _, entry := range router.pending {
		if entry.symbol != observation.symbol {
			remaining = append(remaining, entry)
			continue
		}

		currentPrice := router.currentPrice(observation)

		if currentPrice <= 0 || entry.price <= 0 {
			remaining = append(remaining, entry)
			continue
		}

		returnBps := ((currentPrice - entry.price) / entry.price) * 10000

		router.corpus.Insert(geometry.CorpusEntry{
			Dial: entry.dial,
			Outcome: geometry.CorpusOutcome{
				ReturnBps: returnBps,
				Category:  entry.category,
				Horizon:   observation.at.Sub(entry.at),
			},
			At: entry.at,
		})
	}

	router.pending = remaining
}

func (router *CortexRouter) currentPrice(
	observation *cortexObservation,
) float64 {
	for _, source := range cortexSourceOrder {
		reading := observation.measurements[source]

		if reading.metrics == nil {
			continue
		}

		price := reading.metrics["price"]

		if price > 0 {
			return price
		}
	}

	return 0
}

/*
extractFeatures merges all continuous metrics from an observation into
a single named float map. Source-prefixed keys ensure dimension isolation
(fluid_reynolds vs hawkes_spectralRadius).
*/
func (router *CortexRouter) extractFeatures(
	observation *cortexObservation,
) map[string]float64 {
	features := make(map[string]float64)

	for source, reading := range observation.measurements {
		prefix := string(source) + "_"

		features[prefix+"confidence"] = reading.category.Confidence
		features[prefix+"strength"] = reading.category.Strength
		features[prefix+"surprisal"] = reading.category.Surprisal

		for key, value := range reading.metrics {
			features[prefix+key] = value
		}
	}

	if observation.manifold != nil {
		router.addManifoldFeatures(features, observation.manifold)
	}

	if observation.resonance != nil {
		router.addResonanceFeatures(features, observation.resonance)
	}

	if observation.causal != nil {
		router.addCausalFeatures(features, observation.causal)
	}

	return features
}

func (router *CortexRouter) addManifoldFeatures(
	features map[string]float64,
	frame *logic.ManifoldFrame,
) {
	features["manifold_strength"] = frame.Strength
	features["manifold_momentum"] = frame.Momentum
	features["manifold_pressure"] = frame.Pressure
	features["manifold_shock"] = frame.Shock
	features["manifold_resistance"] = frame.Resistance
	features["manifold_peak"] = frame.Peak
	features["manifold_centerX"] = frame.Summary.CenterX
	features["manifold_centerZ"] = frame.Summary.CenterZ
	features["manifold_gradient"] = frame.Summary.Gradient
	features["manifold_coherence"] = frame.Reading.CoherenceMag2
}

func (router *CortexRouter) addResonanceFeatures(
	features map[string]float64,
	frame *logic.ResonanceFrame,
) {
	features["resonance_confidence"] = frame.Confidence
	features["resonance_flow"] = frame.Flow
	features["resonance_stress"] = frame.Stress
	features["resonance_coupling"] = frame.Coupling
	features["resonance_baseline"] = frame.Baseline
	features["resonance_energy"] = frame.Energy
	features["resonance_surprise"] = frame.Surprise
}

func (router *CortexRouter) addCausalFeatures(
	features map[string]float64,
	frame *logic.CausalFrame,
) {
	features["causal_confidence"] = frame.Confidence
	features["causal_strength"] = frame.Strength
	features["causal_baseline"] = frame.Baseline
	features["causal_uplift"] = frame.Uplift
	features["causal_intervention"] = frame.Intervention
	features["causal_beta"] = frame.Beta
	features["causal_panic"] = frame.Panic
	features["causal_residual"] = frame.Residual
}
