package adaptive

import (
	"fmt"

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
