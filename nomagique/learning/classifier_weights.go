package learning

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
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
ClassifierReading is one scored observation: one logit per configured output,
the first output's strength, and the configured threshold and scales.
*/
type ClassifierReading struct {
	Scores    []float64
	Strength  float64
	Threshold float64
	Scales    map[string]float64
}

/*
ClassifierWeights holds dynamically derived coefficients for configured outputs
and scores feature maps against them.
*/
type ClassifierWeights struct {
	err         error
	threshold   float64
	scales      map[string]float64
	outputs     []string
	specs       map[string]LogitSpec
	termWeights map[string]map[string]float64
	out         ClassifierReading
}

/*
NewClassifierWeights builds balanced logits from typed recipes and feature
scales. A rejected configuration is recorded as the primitive's error state
and every stream over it yields nothing.
*/
func NewClassifierWeights(
	config ClassifierWeightsConfig,
	threshold float64,
	scales map[string]float64,
) core.Primitive {
	weights, err := buildClassifierWeights(config, threshold, scales)

	if err != nil {
		return &ClassifierWeights{err: err}
	}

	return weights
}

/*
Next receives feature maps and yields a *ClassifierReading per arrival with
one logit per configured output and the first output's strength.
*/
func (op *ClassifierWeights) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	if op.err != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			features := (*map[string]float64)(arriving)

			op.out = ClassifierReading{
				Scores:    op.scores(*features),
				Threshold: op.threshold,
				Scales:    cloneScales(op.scales),
			}
			op.out.Strength = op.out.Scores[0]

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *ClassifierWeights) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

func buildClassifierWeights(
	config ClassifierWeightsConfig,
	threshold float64,
	scales map[string]float64,
) (*ClassifierWeights, error) {
	if threshold <= 0 {
		return nil, fmt.Errorf(
			"%w: classifier weights threshold must be positive, got %v",
			core.ErrDomain,
			threshold,
		)
	}

	if len(config.Outputs) == 0 {
		return nil, fmt.Errorf(
			"%w: classifier weights require outputs",
			core.ErrShape,
		)
	}

	specs := make(map[string]LogitSpec, len(config.Outputs))
	termWeights := make(map[string]map[string]float64, len(config.Outputs))

	for _, outputKey := range config.Outputs {
		spec := config.Specs[outputKey]

		if len(spec.Terms) == 0 {
			return nil, fmt.Errorf(
				"%w: output %q requires terms",
				core.ErrShape,
				outputKey,
			)
		}

		weights, err := balancedTermWeights(spec.Terms, scales)

		if err != nil {
			return nil, err
		}

		specs[outputKey] = spec
		termWeights[outputKey] = weights
	}

	return &ClassifierWeights{
		threshold:   threshold,
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
		return nil, fmt.Errorf(
			"%w: balanced term weights require positive scales",
			core.ErrDomain,
		)
	}

	weights := make(map[string]float64, len(terms))

	for featureKey, weight := range rawWeights {
		weights[featureKey] = weight / total
	}

	return weights, nil
}

func positiveScale(scale float64, name string) (float64, error) {
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return 0, fmt.Errorf(
			"%w: feature scale for %s must be finite and positive, got %v",
			core.ErrDomain,
			name,
			scale,
		)
	}

	return scale, nil
}

/*
scores returns one logit per configured output.
*/
func (op *ClassifierWeights) scores(features map[string]float64) []float64 {
	scores := make([]float64, len(op.outputs))

	for index, outputKey := range op.outputs {
		scores[index] = op.outputScore(outputKey, features)
	}

	return scores
}

func (op *ClassifierWeights) outputScore(
	outputKey string,
	features map[string]float64,
) float64 {
	spec, ok := op.specs[outputKey]

	if !ok {
		return 0
	}

	termWeights := op.termWeights[outputKey]
	score := 0.0

	for _, featureKey := range spec.Terms {
		normalized := normalizeFeature(features[featureKey], op.scales[featureKey])

		if spec.Inverts[featureKey] {
			normalized = 1.0 - normalized
		}

		score += normalized * termWeights[featureKey]
	}

	return score
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

func cloneScales(scales map[string]float64) map[string]float64 {
	clone := make(map[string]float64, len(scales))

	for key, value := range scales {
		clone[key] = value
	}

	return clone
}
