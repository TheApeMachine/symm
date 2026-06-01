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
Confidence returns how decisively observation lands in its assigned class:
distance to the nearest class boundary, normalised by half the local band width.
High when deep inside a band (a clear StochasticNoise reads high); low on a
boundary where another category is plausible. Category-agnostic — it says how
clearly we measured, not what we measured.
*/
func (classifier *Classifier) Confidence(observation float64) float64 {
	if classifier == nil || len(classifier.upper) == 0 {
		return 0
	}

	classIndex := classifier.classIndex(observation)
	margin := classifier.margin(observation, classIndex)

	if margin <= 0 {
		return 0
	}

	halfWidth := classifier.localHalfWidth(classIndex)

	if halfWidth <= snrEpsilon {
		return 0
	}

	return margin / halfWidth
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
Label returns the label for one class code.
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
