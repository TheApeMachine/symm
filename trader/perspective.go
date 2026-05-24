package trader

import (
	"sort"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

/*
PerspectiveScore is one market-angle aggregate for a symbol.
*/
type PerspectiveScore struct {
	Perspective engine.MarketPerspective
	Score       float64
	Sources     int
}

/*
scorePerspectives aggregates weighted candidates into perspective frames.
Returns active perspectives sorted by score descending.
*/
func scorePerspectives(
	sources map[string]SignalCandidate,
	ensemble EnsembleContext,
) []PerspectiveScore {
	byPerspective := make(map[engine.MarketPerspective]PerspectiveScore)

	for _, candidate := range sources {
		regime := RegimeWeight(ensemble.Regime, candidate.Source)

		if regime <= 0 {
			continue
		}

		if !regimeAllowsSource(ensemble.Regime, candidate.Source) {
			continue
		}

		effective := ensembleWeight(ensemble, candidate) * regimeGateScale(ensemble.Regime, candidate.Source)

		if effective <= 0 {
			continue
		}

		perspective := engine.SourcePerspective(candidate.Source)
		current := byPerspective[perspective]
		current.Perspective = perspective
		current.Score += effective
		current.Sources++
		byPerspective[perspective] = current
	}

	scores := make([]PerspectiveScore, 0, len(byPerspective))

	for _, score := range byPerspective {
		if score.Score <= 0 || score.Sources <= 0 {
			continue
		}

		scores = append(scores, score)
	}

	sort.Slice(scores, func(left, right int) bool {
		if scores[left].Score != scores[right].Score {
			return scores[left].Score > scores[right].Score
		}

		return scores[left].Perspective < scores[right].Perspective
	})

	return scores
}

/*
combinePerspectives selects the strongest angles instead of summing every source.
*/
func combinePerspectives(scores []PerspectiveScore) (combined float64, active int) {
	if len(scores) == 0 {
		return 0, 0
	}

	limit := config.System.MaxActivePerspectives

	if limit <= 0 {
		limit = 2
	}

	threshold := perspectiveThreshold(scores)

	for index, score := range scores {
		if index >= limit {
			break
		}

		if score.Score < threshold {
			continue
		}

		weight := perspectiveBlendWeight(index)
		combined += score.Score * weight
		active++
	}

	return combined, active
}

func perspectiveThreshold(scores []PerspectiveScore) float64 {
	if len(scores) == 0 {
		return 0
	}

	values := make([]float64, len(scores))

	for index, score := range scores {
		values[index] = score.Score
	}

	sort.Float64s(values)
	median := values[len(values)/2]

	if median <= 0 {
		return 0
	}

	return median * 0.5
}

func perspectiveBlendWeight(rank int) float64 {
	if rank == 0 {
		return 1
	}

	if rank == 1 {
		return 0.55
	}

	return 0.35
}

func regimeAllowsSource(regime MarketRegime, source string) bool {
	if regime != RegimeDead {
		return true
	}

	weight := RegimeWeight(regime, source)

	return weight >= 0.5
}

func regimeGateScale(regime MarketRegime, source string) float64 {
	if regime != RegimeTrending {
		return 1
	}

	switch source {
	case "pumpdump", "sentiment":
		return 0.85
	default:
		return 1
	}
}
