package sentiment

import (
	"math"

	"github.com/theapemachine/symm/engine"
)

func (sentiment *Sentiment) marketBreadth() (float64, float64, bool) {
	positive := 0
	total := 0
	topChange := 0.0

	sentiment.symbols.Range(func(key, value any) bool {
		state := value.(*symbolState)

		if state.changePct == 0 {
			return true
		}

		total++

		if state.changePct > topChange {
			topChange = state.changePct
		}

		if state.changePct <= 0 {
			return true
		}

		positive++

		return true
	})

	if total == 0 {
		return 0, 0, false
	}

	return float64(positive) / float64(total), topChange, true
}

func (sentiment *Sentiment) breadthAndLeaders() (float64, map[string]float64, float64, bool) {
	breadth, topChange, ok := sentiment.marketBreadth()

	if !ok {
		return 0, nil, 0, false
	}

	leaders := make(map[string]float64)

	sentiment.symbols.Range(func(key, value any) bool {
		state := value.(*symbolState)

		if state.changePct <= 0 {
			return true
		}

		leaders[key.(string)] = state.changePct

		return true
	})

	if len(leaders) == 0 {
		return breadth, nil, topChange, true
	}

	if breadth < minBreadth || topChange <= 0 {
		return breadth, leaders, topChange, true
	}

	return breadth, leaders, topChange, true
}

func (sentiment *Sentiment) sentimentConfidence(
	breadth float64,
	change float64,
	topChange float64,
	peakScore float64,
) float64 {
	confidence := 0.0

	if topChange > 0 {
		confidence = engine.AlignConfidence(breadth, change/topChange)
	}

	if confidence <= 0 {
		confidence = engine.ConfidenceFromScore(peakScore)
	}

	if confidence <= 0 {
		confidence = engine.ConfidenceFromScore(breadth * math.Abs(change))
	}

	if confidence <= 0 {
		confidence = engine.ConfidenceFromScore(breadth)
	}

	return confidence
}

func leaderPeers(leaders map[string]float64, skip string) []float64 {
	peers := make([]float64, 0, len(leaders)-1)

	for symbol, value := range leaders {
		if symbol == skip {
			continue
		}

		peers = append(peers, value)
	}

	return peers
}
