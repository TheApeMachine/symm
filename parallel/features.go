package parallel

import (
	"strings"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

type resonanceFeatureScorer struct {
	moments      []core.Primitive
	residuals    []core.Primitive
	standardized []float64
}

func newResonanceFeatureScorer() *resonanceFeatureScorer {
	scorer := &resonanceFeatureScorer{
		moments:      make([]core.Primitive, len(resonance.Features)),
		residuals:    make([]core.Primitive, len(resonance.Features)),
		standardized: make([]float64, len(resonance.Features)),
	}

	for index := range scorer.moments {
		scorer.moments[index] = statistic.NewEstimator()
		scorer.residuals[index] = statistic.NewCausalResidual()
	}

	return scorer
}

func (scorer *resonanceFeatureScorer) standardize(tick *data.Value, symbol string) ([]float64, error) {
	sources := map[string]float64{}

	type sig struct {
		labels []string
		values []float64
	}

	bySource := map[string]map[string]sig{}

	for _, source := range types.SignalSourceStrings {
		for _, layer := range tick.LookupPrefix(source) {
			parts := strings.Split(layer.Key, "/")
			if len(parts) < 3 {
				continue
			}

			if parts[1]+"/"+parts[2] != symbol {
				continue
			}

			var values []float64
			if err := layer.Decode(&values); err != nil {
				return nil, err
			}

			if bySource[source] == nil {
				bySource[source] = map[string]sig{}
			}

			bySource[source][symbol] = sig{labels: parts[3:], values: values}
		}
	}

	for _, feature := range resonance.Features {
		symbolSigs, ok := bySource[feature.Source]
		if !ok {
			continue
		}

		s, ok := symbolSigs[symbol]
		if !ok {
			continue
		}

		for idx, label := range s.labels {
			if label == feature.Label && idx < len(s.values) {
				sources[feature.Source] = s.values[idx]
				break
			}
		}
	}

	for index, feature := range resonance.Features {
		val, carried := sources[feature.Source]
		if !carried {
			continue
		}

		var reading *statistic.MomentReading
		for out := range scorer.moments[index].Next(transport.NewValue(val).Next(nil)) {
			reading = (*statistic.MomentReading)(out)
		}

		if err := scorer.moments[index].Error(); err != nil || reading == nil {
			return nil, err
		}

		var residual *statistic.CausalResidualResult
		for out := range scorer.residuals[index].Next(transport.NewValue(*reading).Next(nil)) {
			residual = (*statistic.CausalResidualResult)(out)
		}

		if err := scorer.residuals[index].Error(); err != nil || residual == nil {
			return nil, err
		}

		scorer.standardized[index] = residual.ZScore
	}

	out := make([]float64, len(scorer.standardized))
	copy(out, scorer.standardized)

	return out, nil
}

func frictionHurdle(tick *data.Value, symbol string) float64 {
	if tick == nil {
		return 0
	}

	spread := layerScalar(tick, "liquidity", symbol, "relative_spread")
	fee := layerScalar(tick, "liquidity", symbol, "fee_rate")

	return spread + 2*fee
}

func layerScalar(tick *data.Value, source, symbol, label string) float64 {
	for _, layer := range tick.LookupPrefix(source) {
		parts := strings.Split(layer.Key, "/")
		if len(parts) < 4 {
			continue
		}

		if parts[1]+"/"+parts[2] != symbol {
			continue
		}

		for idx, name := range parts[3:] {
			if name != label {
				continue
			}

			var values []float64
			if err := layer.Decode(&values); err != nil || idx >= len(values) {
				return 0
			}

			return values[idx]
		}
	}

	return 0
}
