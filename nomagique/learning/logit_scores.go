package learning

import "github.com/theapemachine/errnie"

/*
LogitScoresConfig configures typed feature-to-logit scoring.
*/
type LogitScoresConfig struct {
	Weights   ClassifierWeightsConfig
	Threshold float64
	Scales    map[string]float64
}

/*
LogitScoresInput carries named feature values.
*/
type LogitScoresInput struct {
	Features map[string]float64
}

/*
LogitScoresOutput carries scored logits in configured order.
*/
type LogitScoresOutput struct {
	Outputs []string
	Scores  []float64
	ByName  map[string]float64
}

/*
LogitScores maps typed feature values to classifier logits.
*/
type LogitScores struct {
	config  LogitScoresConfig
	weights ClassifierWeights
}

/*
NewLogitScores returns a typed classifier logit calculator.
*/
func NewLogitScores(config LogitScoresConfig) (*LogitScores, error) {
	weights, err := NewClassifierWeights(
		config.Weights,
		config.Threshold,
		config.Scales,
	)

	if err != nil {
		return nil, err
	}

	return &LogitScores{
		config:  config,
		weights: weights,
	}, nil
}

/*
Measure returns configured logits for the input features.
*/
func (logitScores *LogitScores) Measure(input LogitScoresInput) (LogitScoresOutput, error) {
	if logitScores == nil {
		return LogitScoresOutput{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"logit-scores: config required",
			nil,
		))
	}

	for key, value := range input.Features {
		if !finite(value) {
			return LogitScoresOutput{}, errnie.Error(errnie.Err(
				errnie.Validation,
				"logit-scores: feature must be finite",
				nil,
			).With("feature", key))
		}
	}

	scores := logitScores.weights.Scores(input.Features)
	byName := make(map[string]float64, len(scores))

	for index, output := range logitScores.weights.outputs {
		byName[output] = scores[index]
	}

	return LogitScoresOutput{
		Outputs: append([]string(nil), logitScores.weights.outputs...),
		Scores:  scores,
		ByName:  byName,
	}, nil
}
