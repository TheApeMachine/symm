package numeric

import "github.com/theapemachine/symm/numeric/adaptive"

/*
LabelTap classifies price move from two observations and passes confidence
through unchanged.
*/
type LabelTap struct {
	move       *adaptive.RelativeMove
	classifier *adaptive.Classifier
	code       float64
}

/*
NewLabelTap builds a side-channel classifier over current and anchor price.
*/
func NewLabelTap(classifier *adaptive.Classifier) *LabelTap {
	return &LabelTap{
		move:       adaptive.NewRelativeMove(),
		classifier: classifier,
	}
}

func (label *LabelTap) Next(out float64, values ...float64) (float64, error) {
	move, err := label.move.Next(0, values[len(values)-2], values[len(values)-1])

	if err != nil {
		return out, err
	}

	code, err := label.classifier.Next(0, move)

	if err != nil {
		return out, err
	}

	label.code = code

	return out, nil
}

func (label *LabelTap) Reset() error {
	label.code = 0

	return label.move.Reset()
}

func (label *LabelTap) ClassCode() float64 {
	return label.code
}

func (label *LabelTap) clone() *LabelTap {
	if label == nil {
		return nil
	}

	return &LabelTap{
		move:       adaptive.NewRelativeMove(),
		classifier: label.classifier,
		code:       label.code,
	}
}
