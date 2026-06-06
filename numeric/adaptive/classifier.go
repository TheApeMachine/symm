package adaptive

import (
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
)

/*
Classifier maps one scalar into a class using ascending upper bounds.
*/
type Classifier struct {
	upper  []float64
	codes  []float64
	labels []string
}

/*
NewClassifier builds a classifier with len(upper) classes. codes and labels must
match len(upper). The last class applies above the final upper bound.
*/
func NewClassifier(upper, codes []float64, labels []string) *Classifier {
	if len(upper) == 0 || len(codes) != len(upper)+1 || len(labels) != len(upper)+1 {
		errnie.Error(fmt.Errorf("adaptive: Classifier needs upper bounds and one more code/label each"))
	}

	return &Classifier{
		upper:  append([]float64(nil), upper...),
		codes:  append([]float64(nil), codes...),
		labels: append([]string(nil), labels...),
	}
}

/*
Next classifies one observation from values[0].
*/
func (classifier *Classifier) Next(_ float64, values ...float64) (float64, error) {
	if classifier == nil {
		return 0, fmt.Errorf("adaptive: Classifier.Next nil receiver")
	}

	if len(values) != 1 {
		return 0, fmt.Errorf("adaptive: Classifier.Next expects one value, got %d", len(values))
	}

	observation := values[0]

	for index, bound := range classifier.upper {
		if observation <= bound {
			return classifier.codes[index], nil
		}
	}

	return classifier.codes[len(classifier.upper)], nil
}

/*
Code classifies one observation without variadic boxing.
*/
func (classifier *Classifier) Code(observation float64) (float64, error) {
	if classifier == nil {
		return 0, fmt.Errorf("adaptive: Classifier.Code nil receiver")
	}

	for index, bound := range classifier.upper {
		if observation <= bound {
			return classifier.codes[index], nil
		}
	}

	return classifier.codes[len(classifier.upper)], nil
}

/*
Confidence returns selection confidence in [1/N, 1): 1/N — a uniform guess among the
classifier's N categories — right on a band boundary, rising toward (but never
reaching) full certainty deep inside a band. The floor is derived from the category
count, not a hard-coded constant: a selection cannot be less certain than a coin
flip among its options.
*/
func (classifier *Classifier) Confidence(observation float64) float64 {
	if classifier == nil || len(classifier.upper) == 0 {
		return 0
	}

	floor := classifier.uniformConfidence()
	classIndex := classifier.classIndex(observation)
	inBandMargin := classifier.margin(observation, classIndex)
	halfWidth := classifier.localHalfWidth(classIndex)

	if halfWidth <= snrEpsilon {
		return floor
	}

	// Continuous, monotonic band depth in [0, 1): 0 right on the band boundary, ~0.86
	// a half-width inside (a closed band's centre), saturating toward (but never
	// reaching) 1 deeper into an open-ended band.
	depth := 0.0

	if inBandMargin > 0 {
		depth = 1 - math.Exp(-2*inBandMargin/halfWidth)
	}

	// Map band depth [0, 1) onto [1/N, 1): a boundary reading is no more certain than
	// a uniform guess among the N categories; deeper inside, certainty approaches 1.
	return floor + depth*(1-floor)
}

// uniformConfidence is 1/N, the confidence of a uniform guess among the
// classifier's N categories — the derived floor below which a selection cannot fall.
func (classifier *Classifier) uniformConfidence() float64 {
	return 1 / float64(len(classifier.codes))
}

/*
Standout is winner clarity minus the strongest adjacent-category clarity at the
current observation — a unit competition margin in [0, 1].
*/
func (classifier *Classifier) Standout(observation float64) float64 {
	if classifier == nil || len(classifier.upper) == 0 {
		return 0
	}

	winIndex := classifier.classIndex(observation)
	win := classifier.Confidence(observation)

	if win <= 0 {
		return 0
	}

	runner := 0.0
	upperCount := len(classifier.upper)

	if winIndex > 0 {
		runner = math.Max(runner, classifier.confidenceForClass(observation, winIndex-1))
	}

	if winIndex < upperCount {
		runner = math.Max(runner, classifier.confidenceForClass(observation, winIndex+1))
	}

	margin := win - runner

	if margin <= 0 {
		return standoutTieFloor
	}

	return margin
}

func (classifier *Classifier) confidenceForClass(observation float64, classIndex int) float64 {
	floor := classifier.uniformConfidence()
	inBandMargin := classifier.margin(observation, classIndex)
	halfWidth := classifier.localHalfWidth(classIndex)

	if halfWidth <= snrEpsilon || inBandMargin <= 0 {
		return floor
	}

	depth := 1 - math.Exp(-2*inBandMargin/halfWidth)

	return floor + depth*(1-floor)
}

/*
Upper returns a copy of the band edges (ascending). SetUpper replaces them in
place, used by online self-calibration to retune the bands to the live
distribution; a length mismatch is ignored so the classifier never breaks.
*/
func (classifier *Classifier) Labels() []string {
	if classifier == nil {
		return nil
	}

	return append([]string(nil), classifier.labels...)
}

func (classifier *Classifier) Upper() []float64 {
	if classifier == nil {
		return nil
	}

	return append([]float64(nil), classifier.upper...)
}

func (classifier *Classifier) SetUpper(upper []float64) {
	if classifier == nil || len(upper) != len(classifier.codes)-1 {
		return
	}

	classifier.upper = append([]float64(nil), upper...)
}

func (classifier *Classifier) classIndex(observation float64) int {
	for index, bound := range classifier.upper {
		if observation <= bound {
			return index
		}
	}

	return len(classifier.upper)
}

func (classifier *Classifier) margin(observation float64, classIndex int) float64 {
	upperCount := len(classifier.upper)

	if classIndex == 0 {
		return classifier.upper[0] - observation
	}

	if classIndex >= upperCount {
		return observation - classifier.upper[upperCount-1]
	}

	lower := classifier.upper[classIndex-1]
	upper := classifier.upper[classIndex]

	return math.Min(observation-lower, upper-observation)
}

func (classifier *Classifier) localHalfWidth(classIndex int) float64 {
	upperCount := len(classifier.upper)

	if upperCount == 1 {
		return math.Abs(classifier.upper[0]) / 2
	}

	if classIndex == 0 {
		return (classifier.upper[1] - classifier.upper[0]) / 2
	}

	if classIndex >= upperCount {
		return (classifier.upper[upperCount-1] - classifier.upper[upperCount-2]) / 2
	}

	return (classifier.upper[classIndex] - classifier.upper[classIndex-1]) / 2
}

/*
Label returns the label for the class code.
*/
func (classifier *Classifier) Label(code float64) string {
	for index, classCode := range classifier.codes {
		if classCode == code {
			return classifier.labels[index]
		}
	}

	return ""
}

/*
Reset is a no-op for Classifier.
*/
func (classifier *Classifier) Reset() error {
	return nil
}
