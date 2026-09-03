/*
Package vector classifies an observation across competing declared classes.

A caller declares one Group per class, naming the metrics that evidence it.
Each metric is standardized against its own causal history, each class logit
is the mean standardized evidence for that class, and the logits normalize
into a probability distribution.

Metrics are addressed by plain string name. There is no interned symbol
table and no shared frame: each metric owns its standardizer privately.
*/
package vector

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Group declares one class and the metric names that evidence it.
*/
type Group struct {
	Label string
	Keys  []string
}

/*
NewGroup declares a class and its evidence metrics.
*/
func NewGroup(label string, keys ...string) Group {
	return Group{Label: label, Keys: append([]string(nil), keys...)}
}

/*
Logits folds standardized metric evidence into one score per declared class.

It satisfies probability.Collection, so a Distribution folds it directly.
Each class score is the mean of its metrics' standardized values — a mean
rather than a sum, so a class declaring more metrics does not win on breadth.
*/
type Logits struct {
	groups        []Group
	standardizers map[string]*equation.AdaptiveZScore
	standardized  map[string]types.Number
	scores        []types.Number
	complete      bool
}

/*
Values returns the current class scores, or nothing when the last observation
was incomplete.
*/
func (logits *Logits) Values() []types.Number {
	if !logits.complete {
		return nil
	}

	return logits.scores
}

/*
Observe standardizes a complete observation and folds it into class scores.

An incomplete observation is refused without advancing any standardizer: one
class scored on present evidence against another scored on absent evidence is
not a comparison, and a partial frame must not corrupt the causal history the
next complete one is measured against.
*/
func (logits *Logits) Observe(observation map[string]float64) bool {
	logits.complete = false

	for key := range logits.standardizers {
		if _, present := observation[key]; !present {
			return false
		}
	}

	for key, standardizer := range logits.standardizers {
		logits.standardized[key] = standardizer.Step(types.Number(observation[key]))
	}

	for index, group := range logits.groups {
		var score types.Number

		for _, key := range group.Keys {
			score += logits.standardized[key]
		}

		logits.scores[index] = score / types.Number(len(group.Keys))
	}

	logits.complete = true

	return true
}

/*
Missing names the declared metrics absent from an observation.
*/
func (logits *Logits) Missing(observation map[string]float64) []string {
	var missing []string

	for _, group := range logits.groups {
		for _, key := range group.Keys {
			if _, present := observation[key]; present {
				continue
			}

			if !contains(missing, key) {
				missing = append(missing, key)
			}
		}
	}

	return missing
}

/*
Standardized returns one metric's most recent causal z-score.
*/
func (logits *Logits) Standardized(key string) (types.Number, bool) {
	value, found := logits.standardized[key]

	return value, found
}

/*
Maturity returns the least mature standardizer's maturity: a classification
is only as established as its least-evidenced input.
*/
func (logits *Logits) Maturity() types.Number {
	maturity := types.Number(1)
	seen := false

	for _, standardizer := range logits.standardizers {
		if !seen || standardizer.Maturity() < maturity {
			maturity = standardizer.Maturity()
			seen = true
		}
	}

	if !seen {
		return 0
	}

	return maturity
}

/*
Classifier assigns an observation to one of several competing classes.

It is a Chain of two stages: Logits standardizes the evidence and folds it
into class scores, and Distribution normalizes those scores into
probabilities. Step returns the winning class's confidence.
*/
type Classifier struct {
	logits       Logits
	distribution probability.Distribution
	pipeline     types.Chain
}

/*
NewClassifier compiles the declared groups into a classifier, creating one
causal standardizer per distinct metric named across them.

It fails when fewer than two classes are declared — classification requires
something to choose between — or when a class declares no evidence.
*/
func NewClassifier(groups ...Group) (*Classifier, error) {
	if len(groups) < 2 {
		return nil, fmt.Errorf(
			"vector: classification requires at least two classes, got %d",
			len(groups),
		)
	}

	classifier := &Classifier{
		logits: Logits{
			groups:        append([]Group(nil), groups...),
			standardizers: make(map[string]*equation.AdaptiveZScore),
			standardized:  make(map[string]types.Number),
			scores:        make([]types.Number, len(groups)),
		},
	}

	labels := make(map[string]bool, len(groups))

	for _, group := range groups {
		if group.Label == "" {
			return nil, fmt.Errorf("vector: every class requires a label")
		}

		if labels[group.Label] {
			return nil, fmt.Errorf(
				"vector: class label %q is declared more than once", group.Label,
			)
		}

		labels[group.Label] = true

		if len(group.Keys) == 0 {
			return nil, fmt.Errorf(
				"vector: class %q declares no evidence metrics", group.Label,
			)
		}

		for _, key := range group.Keys {
			if key == "" {
				return nil, fmt.Errorf(
					"vector: class %q declares an unnamed metric", group.Label,
				)
			}

			if _, exists := classifier.logits.standardizers[key]; !exists {
				classifier.logits.standardizers[key] = &equation.AdaptiveZScore{}
			}
		}
	}

	classifier.distribution.Logits = &classifier.logits
	classifier.pipeline = types.Chain{A: &classifier.distribution}

	return classifier, nil
}

/*
Step normalizes the current class scores into a distribution and returns the
winning class's confidence.
*/
func (classifier *Classifier) Step(x types.Number) types.Number {
	return classifier.pipeline.Step(x)
}

/*
Observe standardizes one complete observation into class scores and steps the
distribution over them. It reports whether the observation was classified.
*/
func (classifier *Classifier) Observe(observation map[string]float64) bool {
	if !classifier.logits.Observe(observation) {
		return false
	}

	classifier.Step(0)

	return classifier.distribution.Ready()
}

// Groups exposes the declared classes in score order.
func (classifier *Classifier) Groups() []Group { return classifier.logits.groups }

// Complete reports whether an observation carries every declared metric.
func (classifier *Classifier) Complete(observation map[string]float64) bool {
	for key := range classifier.logits.standardizers {
		if _, present := observation[key]; !present {
			return false
		}
	}

	return true
}

// Missing names the declared metrics absent from an observation.
func (classifier *Classifier) Missing(observation map[string]float64) []string {
	return classifier.logits.Missing(observation)
}

// Standardized returns one metric's most recent causal z-score.
func (classifier *Classifier) Standardized(key string) (types.Number, bool) {
	return classifier.logits.Standardized(key)
}

// Maturity returns the least mature standardizer's maturity.
func (classifier *Classifier) Maturity() types.Number {
	return classifier.logits.Maturity()
}

// Distribution exposes the normalized simplex.
func (classifier *Classifier) Distribution() *probability.Distribution {
	return &classifier.distribution
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}

var (
	_ types.Node             = (*Classifier)(nil)
	_ probability.Collection = (*Logits)(nil)
)
