package learning

import (
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
)

/*
LogitSpec describes one classifier output as a weighted combination of features.
*/
type LogitSpec struct {
	Terms   []string
	Inverts map[string]bool
}

/*
ClassifierWeightsConfig describes output recipes and their order.
*/
type ClassifierWeightsConfig struct {
	Outputs []string
	Specs   map[string]LogitSpec
}

/*
ClassifierWeights holds dynamically derived coefficients for configured outputs.
*/
type ClassifierWeights struct {
	Threshold   float64
	scales      map[string]float64
	outputs     []string
	specs       map[string]LogitSpec
	termWeights map[string]map[string]float64
}

/*
NewClassifierWeights builds balanced logits from typed recipes and feature scales.
*/
func NewClassifierWeights(
	config ClassifierWeightsConfig,
	threshold float64,
	scales map[string]float64,
) (ClassifierWeights, error) {
	if threshold <= 0 {
		return ClassifierWeights{}, errnie.Error(fmt.Errorf(
			"learning: NewClassifierWeights threshold must be positive, got %v",
			threshold,
		))
	}

	if len(config.Outputs) == 0 {
		return ClassifierWeights{}, errnie.Error(fmt.Errorf(
			"learning: NewClassifierWeights requires outputs",
		))
	}

	specs := make(map[string]LogitSpec, len(config.Outputs))
	termWeights := make(map[string]map[string]float64, len(config.Outputs))

	for _, outputKey := range config.Outputs {
		spec := config.Specs[outputKey]

		if len(spec.Terms) == 0 {
			return ClassifierWeights{}, errnie.Error(fmt.Errorf(
				"learning: output %q requires terms",
				outputKey,
			))
		}

		weights, err := balancedTermWeights(spec.Terms, scales)

		if err != nil {
			return ClassifierWeights{}, err
		}

		specs[outputKey] = spec
		termWeights[outputKey] = weights
	}

	return ClassifierWeights{
		Threshold:   threshold,
		scales:      cloneScales(scales),
		outputs:     append([]string(nil), config.Outputs...),
		specs:       specs,
		termWeights: termWeights,
	}, nil
}

func balancedTermWeights(
	terms []string,
	scales map[string]float64,
) (map[string]float64, error) {
	rawWeights := make(map[string]float64, len(terms))
	total := 0.0

	for _, featureKey := range terms {
		scale, err := positiveScale(scales[featureKey], featureKey)

		if err != nil {
			return nil, err
		}

		rawWeights[featureKey] = 1.0 / scale
		total += rawWeights[featureKey]
	}

	if total <= 0 {
		return nil, errnie.Error(fmt.Errorf(
			"learning: balanced term weights require positive scales",
		))
	}

	weights := make(map[string]float64, len(terms))

	for featureKey, weight := range rawWeights {
		weights[featureKey] = weight / total
	}

	return weights, nil
}

func (weights *ClassifierWeights) FeatureScales() map[string]float64 {
	return cloneScales(weights.scales)
}

func positiveScale(scale float64, name string) (float64, error) {
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return 0, errnie.Error(fmt.Errorf(
			"learning: feature scale for %s must be finite and positive, got %v",
			name,
			scale,
		))
	}

	return scale, nil
}

/*
Scores returns one logit per configured output.
*/
func (weights *ClassifierWeights) Scores(features map[string]float64) []float64 {
	scores := make([]float64, len(weights.outputs))

	for index, outputKey := range weights.outputs {
		scores[index] = weights.outputScore(outputKey, features)
	}

	return scores
}

func (weights *ClassifierWeights) outputScore(
	outputKey string,
	features map[string]float64,
) float64 {
	spec, ok := weights.specs[outputKey]

	if !ok {
		return 0
	}

	termWeights := weights.termWeights[outputKey]
	score := 0.0

	for _, featureKey := range spec.Terms {
		normalized := normalizeFeature(features[featureKey], weights.scales[featureKey])

		if spec.Inverts[featureKey] {
			normalized = 1.0 - normalized
		}

		score += normalized * termWeights[featureKey]
	}

	return score
}

/*
Strength returns the first configured output logit from feature values.
*/
func (weights *ClassifierWeights) Strength(features map[string]float64) float64 {
	if len(weights.outputs) == 0 {
		return 0
	}

	return weights.outputScore(weights.outputs[0], features)
}

func normalizeFeature(value, scale float64) float64 {
	ratio := value

	if scale > 0 && !math.IsNaN(scale) && !math.IsInf(scale, 0) {
		ratio = value / scale
	}

	return squashFeature(ratio)
}

func squashFeature(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return math.NaN()
	}

	return value / (1.0 + math.Abs(value))
}

func (weights *ClassifierWeights) clamp() error {
	floor, ceiling, err := weightBounds(weights.scales)

	if err != nil {
		return err
	}

	if ceiling <= 0 {
		return nil
	}

	for outputKey, featureWeights := range weights.termWeights {
		for featureKey, weight := range featureWeights {
			weights.termWeights[outputKey][featureKey] = clamp(weight, floor, ceiling)
		}
	}

	return nil
}

func weightBounds(scales map[string]float64) (float64, float64, error) {
	minScale := math.MaxFloat64
	maxScale := 0.0

	for _, scale := range scales {
		if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
			continue
		}

		minScale = math.Min(minScale, scale)
		maxScale = math.Max(maxScale, scale)
	}

	if minScale == math.MaxFloat64 || maxScale <= 0 {
		return 0, 0, nil
	}

	spreadRatio := maxScale / minScale
	floor := 1.0 / (maxScale * spreadRatio)
	ceiling := spreadRatio / minScale

	return floor, ceiling, nil
}

func clamp(value, lower, upper float64) float64 {
	return math.Min(math.Max(value, lower), upper)
}

func cloneScales(scales map[string]float64) map[string]float64 {
	clone := make(map[string]float64, len(scales))

	for key, value := range scales {
		clone[key] = value
	}

	return clone
}
