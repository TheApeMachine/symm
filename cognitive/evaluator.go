package cognitive

import (
	"sort"
	"strings"
	"time"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type token struct {
	origin     string
	category   string
	confidence float64
	stamp      int64
}

type observation struct {
	symbol string
	tokens []token
	stamp  int64
}

/*
Evaluator owns the DMT cognitive engine and caches bounded live readings.
*/
type Evaluator struct {
	tree  *dmt.Tree
	cache map[string]market.CognitiveReading
}

const (
	beamWidth = 4
	maxHops   = 3
)

/*
NewEvaluator creates a DMT-backed cognitive evaluator.
*/
func NewEvaluator(tree *dmt.Tree) *Evaluator {
	return &Evaluator{
		tree:  tree,
		cache: make(map[string]market.CognitiveReading),
	}
}

/*
Readings encodes measurements, trains the DMT cognitive engine, and returns
plain market readings for the UI/trader boundary.
*/
func Readings(
	tree *dmt.Tree,
	measurements []*logic.Measurement,
) map[string]market.CognitiveReading {
	if tree == nil {
		return nil
	}

	return readObservations(tree, observations(measurements), nil)
}

/*
Readings trains and reads as much current state as the budget allows.
*/
func (evaluator *Evaluator) Readings(
	measurements []*logic.Measurement,
	budget time.Duration,
) map[string]market.CognitiveReading {
	if evaluator == nil || evaluator.tree == nil {
		return nil
	}

	observations := observations(measurements)
	if len(observations) == 0 {
		return nil
	}

	readings := make(map[string]market.CognitiveReading, len(observations))
	if budget <= 0 {
		for _, observation := range observations {
			cached, ok := evaluator.cache[observation.symbol]
			if ok {
				readings[observation.symbol] = cached
			}
		}

		return readings
	}

	start := time.Now()
	deadline := start.Add(budget)
	classifyScratch := &dmt.ClassificationScratch{}
	beamScratch := &dmt.BeamSearchScratch{}

	for _, observation := range observations {
		if time.Now().After(deadline) {
			break
		}

		reading := readObservation(
			evaluator.tree,
			observation,
			classifyScratch,
			beamScratch,
		)
		evaluator.cache[observation.symbol] = reading
		readings[observation.symbol] = reading
	}

	for _, observation := range observations {
		if _, ok := readings[observation.symbol]; ok {
			continue
		}

		cached, ok := evaluator.cache[observation.symbol]
		if ok {
			readings[observation.symbol] = cached
		}
	}

	return readings
}

func observations(measurements []*logic.Measurement) []observation {
	bySymbol := make(map[string]map[string]token)
	stampBySymbol := make(map[string]int64)

	for _, measurement := range measurements {
		if measurement == nil || measurement.Symbol == "" {
			continue
		}

		if measurement.Source == logic.SourceNone {
			continue
		}

		confidence := measurement.Confidence

		if confidence <= 0 {
			continue
		}

		category := measurement.DominantCategory()
		if category == logic.CategoryTypeNone {
			continue
		}

		symbol := measurement.Symbol
		origin := string(measurement.Source)
		if bySymbol[symbol] == nil {
			bySymbol[symbol] = make(map[string]token)
		}

		stamp := measurement.At.UnixNano()
		token := token{
			origin:     origin,
			category:   string(category),
			confidence: confidence,
			stamp:      stamp,
		}

		prior, exists := bySymbol[symbol][origin]

		if !exists || token.confidence > prior.confidence ||
			(token.confidence == prior.confidence && token.stamp > prior.stamp) {
			bySymbol[symbol][origin] = token
		}

		if stamp > stampBySymbol[symbol] {
			stampBySymbol[symbol] = stamp
		}
	}

	observations := make([]observation, 0, len(bySymbol))

	for symbol, byOrigin := range bySymbol {
		tokens := make([]token, 0, len(byOrigin))

		for _, token := range byOrigin {
			tokens = append(tokens, token)
		}

		observations = append(observations, observation{
			symbol: symbol,
			tokens: tokens,
			stamp:  stampBySymbol[symbol],
		})
	}

	sort.SliceStable(observations, func(left, right int) bool {
		if observations[left].stamp != observations[right].stamp {
			return observations[left].stamp > observations[right].stamp
		}

		return observations[left].symbol < observations[right].symbol
	})

	return observations
}

func sequence(tokens []token) string {
	parts := make([]string, 0, len(tokens))

	for _, token := range tokens {
		category := strings.ReplaceAll(token.category, "_", "-")

		if category == "" {
			continue
		}

		parts = append(parts, category)
	}

	return strings.Join(parts, "_")
}

func appendToken(buffer []byte, prefix []byte, token []byte) []byte {
	if len(prefix) > 0 {
		buffer = append(buffer, prefix...)
		buffer = append(buffer, '_')
	}

	return append(buffer, token...)
}
