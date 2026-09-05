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
	// readyIndices names the groups scored by the last observation, in the
	// order their scores appear in the returned values. A consumer maps a
	// distribution position back to its declared class through it.
	readyIndices []int
	complete     bool
}

/*
MinimumComparableClasses is the number of classes an observation must be able
to score before it states anything. A distribution over a single class always
returns that class at probability one, which reads as certainty and carries no
evidence at all.
*/
const MinimumComparableClasses = 2

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
ReadyIndices names the declared classes the last observation scored, in the
order their scores were returned. It is how a caller recovers which class a
distribution position belongs to once unscored classes have been left out.
*/
func (logits *Logits) ReadyIndices() []int {
	if !logits.complete {
		return nil
	}

	return logits.readyIndices
}

/*
ready reports which declared classes an observation carries complete evidence
for. A class is ready only when every metric it declares is present: the
comparison this collection produces is between classes that were all measured,
never between one that was and one that was not.
*/
func (logits *Logits) ready(observation map[string]float64) []int {
	indices := make([]int, 0, len(logits.groups))

	for index, group := range logits.groups {
		complete := true

		for _, key := range group.Keys {
			if _, present := observation[key]; !present {
				complete = false

				break
			}
		}

		if complete {
			indices = append(indices, index)
		}
	}

	return indices
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
	logits.readyIndices = nil

	indices := logits.ready(observation)

	if len(indices) < MinimumComparableClasses {
		return false
	}

	// Only the metrics that will be scored advance their causal history, so a
	// class's standardizers see exactly the frames that class was measured on.
	scored := make(map[string]bool, len(logits.standardizers))

	for _, index := range indices {
		for _, key := range logits.groups[index].Keys {
			scored[key] = true
		}
	}

	for key := range scored {
		logits.standardized[key] = logits.standardizers[key].Step(
			types.Number(observation[key]),
		)
	}

	scores := make([]types.Number, 0, len(indices))

	for _, index := range indices {
		group := logits.groups[index]

		var score types.Number

		for _, key := range group.Keys {
			score += logits.standardized[key]
		}

		scores = append(scores, score/types.Number(len(group.Keys)))
	}

	// The distribution normalizes exactly what it is handed, so it is handed
	// the scored classes and nothing else. readyIndices carries the mapping
	// back to the declared class each position belongs to.
	logits.scores = scores
	logits.readyIndices = indices
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

/*
Complete reports whether an observation can state anything: whether at least
MinimumComparableClasses declared classes carry every metric they themselves
declare.

It does NOT require every declared metric of every class. An advisor mixing
evidence from several venues and clocks can wait forever for one metric that
this instrument will never produce, and a class whose own evidence is complete
has no reason to be silenced by a sibling's missing input. What must never
happen is a class scored on present evidence being compared against one scored
on absent evidence, and restricting the comparison to complete classes is what
holds that.
*/
func (classifier *Classifier) Complete(observation map[string]float64) bool {
	return len(classifier.logits.ready(observation)) >= MinimumComparableClasses
}

/*
ReadyClasses names the declared classes an observation carries complete
evidence for, and Unscored names the rest. A reader is owed both: a reading
drawn from two of five classes is a different statement from one drawn from
all five, and nothing else in the output distinguishes them.
*/
func (classifier *Classifier) ReadyClasses(
	observation map[string]float64,
) (ready, unscored []string) {
	scored := map[int]bool{}

	for _, index := range classifier.logits.ready(observation) {
		scored[index] = true
	}

	for index, group := range classifier.logits.groups {
		if scored[index] {
			ready = append(ready, group.Label)

			continue
		}

		unscored = append(unscored, group.Label)
	}

	return ready, unscored
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
