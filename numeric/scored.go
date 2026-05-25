package numeric

import "github.com/theapemachine/symm/numeric/adaptive"

/*
Scored is a Derived pipeline ending in a LabelTap so callers read confidence
from Push and pump/dump class from ClassCode.
*/
type Scored struct {
	derived *Derived
	label   *LabelTap
}

/*
NewScored appends label to the stage list and wires the confidence pipeline.
*/
func NewScored(classifier *adaptive.Classifier, stages ...Dynamic) *Scored {
	label := NewLabelTap(classifier)
	chain := append(stages, label)

	return &Scored{
		derived: NewDerived(WithDynamics(chain...)),
		label:   label,
	}
}

func (scored *Scored) Push(values ...float64) (float64, error) {
	return scored.derived.Push(values...)
}

func (scored *Scored) ClassCode() float64 {
	return scored.label.ClassCode()
}

func (scored *Scored) Reset() error {
	return scored.derived.Reset()
}
