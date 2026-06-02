package numeric

import "github.com/theapemachine/symm/numeric/adaptive"

type Classed struct {
	derived    *Derived
	classifier *adaptive.Classifier
	classify   *Classify
}

func NewClassed(classifier *adaptive.Classifier, stages ...Dynamic) *Classed {
	classify := NewClassify(classifier)

	return &Classed{
		derived:    NewDerived(WithDynamics(append(stages, classify)...)),
		classifier: classifier,
		classify:   classify,
	}
}

func (classed *Classed) Push(values ...float64) (float64, error) {
	return classed.derived.Push(values...)
}

func (classed *Classed) Label(code float64) string {
	return classed.classifier.Label(code)
}

/*
Confidence returns how clearly the last Push landed in its category — see
adaptive.Classifier.Confidence.
*/
func (classed *Classed) Confidence() float64 {
	if classed == nil || classed.classifier == nil {
		return 0
	}

	return classed.classifier.Confidence(classed.classify.lastObservation)
}

func (classed *Classed) Standout() float64 {
	if classed == nil || classed.classifier == nil {
		return 0
	}

	return classed.classifier.Standout(classed.classify.lastObservation)
}

func (classed *Classed) Reset() error {
	return classed.derived.Reset()
}

type Classify struct {
	classifier      *adaptive.Classifier
	lastObservation float64
}

func NewClassify(classifier *adaptive.Classifier) *Classify {
	return &Classify{classifier: classifier}
}

func (classify *Classify) Next(out float64, values ...float64) (float64, error) {
	_ = values
	classify.lastObservation = out

	return classify.classifier.Code(out)
}

func (classify *Classify) Reset() error {
	return nil
}
