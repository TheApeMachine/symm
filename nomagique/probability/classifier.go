package probability

import "github.com/theapemachine/errnie"

/*
ClassifierSchema declares the category scores the classifier consumes.
*/
type ClassifierSchema struct {
	Categories      []string
	CategoryIndexes []float64
}

/*
CategoryScore is one named piece of category evidence.
*/
type CategoryScore struct {
	Category string
	Score    float64
}

/*
ClassifierInput is the typed observation consumed by Classifier.
*/
type ClassifierInput struct {
	Scores   []CategoryScore
	Strength float64
}

/*
Classifier selects a category from typed competing score observations.
*/
type Classifier struct {
	classifier *ScoreClassifier
}

/*
NewClassifier returns a typed classifier from an explicit schema.
*/
func NewClassifier(schema ClassifierSchema) *Classifier {
	return &Classifier{
		classifier: NewScoreClassifier(
			schema.Categories,
			schema.CategoryIndexes,
		),
	}
}

/*
Classify computes the selected category and confidence telemetry.
*/
func (classifier *Classifier) Classify(input ClassifierInput) (ScoreResult, error) {
	if classifier == nil || classifier.classifier == nil {
		return ScoreResult{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"classifier: config required",
			nil,
		))
	}

	scores, err := input.ScoreMap()
	if err != nil {
		return ScoreResult{}, err
	}

	return classifier.classifier.Classify(scores)
}

/*
ScoreMap converts typed observations into the score map used by ScoreClassifier.
*/
func (input ClassifierInput) ScoreMap() (map[string]float64, error) {
	scores := make(map[string]float64, len(input.Scores)+1)

	for _, score := range input.Scores {
		if score.Category == "" {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"classifier: empty input key",
				nil,
			))
		}

		if _, exists := scores[score.Category]; exists {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"classifier: duplicate input key",
				nil,
			))
		}

		scores[score.Category] = score.Score
	}

	scores["strength"] = input.Strength

	return scores, nil
}
