package numeric

import "github.com/theapemachine/symm/numeric/adaptive"

type Classed struct {
	derived    *Derived
	classifier *adaptive.Classifier
}

func NewClassed(classifier *adaptive.Classifier, stages ...Dynamic) *Classed {
	chain := append(stages, NewClassify(classifier))

	return &Classed{
		derived:    NewDerived(WithDynamics(chain...)),
		classifier: classifier,
	}
}

func (classed *Classed) Push(values ...float64) (float64, error) {
	return classed.derived.Push(values...)
}

func (classed *Classed) Label(code float64) string {
	return classed.classifier.Label(code)
}

func (classed *Classed) Reset() error {
	return classed.derived.Reset()
}

type Classify struct {
	classifier *adaptive.Classifier
}

func NewClassify(classifier *adaptive.Classifier) *Classify {
	return &Classify{classifier: classifier}
}

func (classify *Classify) Next(out float64, values ...float64) (float64, error) {
	return classify.classifier.Next(0, out)
}

func (classify *Classify) Reset() error {
	return nil
}
