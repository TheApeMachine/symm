package hindsight

import (
	"fmt"
	"slices"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/system"
)

/*
annotationTrade is one print of the symbol inside the marked window.
*/
type annotationTrade struct {
	tick     uint64
	at       int64
	price    float64
	bid, ask float64
}

/*
annotationVector is the excursion a hand-placed A B C D stores. B and C are
pulled back from the clicked ticks by execution_lead_fraction of the A→B
precursor so train and paper price fills before ignition and exhaustion.
*/
func annotationVector(class string, marks [4]uint64, trades []annotationTrade) ([]float64, error) {
	if err := validateAnnotation(class, marks); err != nil {
		return nil, err
	}

	marks = leadAdjustedMarks(marks)

	indexOf := func(tick uint64, last bool) (int, error) {
		found := -1

		for idx, trade := range trades {
			if trade.tick != tick {
				continue
			}

			found = idx

			if !last {
				break
			}
		}

		if found < 0 {
			return 0, errnie.Err(errnie.NotFound, fmt.Sprintf("[hindsight] tick %d has no trade to mark", tick), nil)
		}

		return found, nil
	}

	a, err := indexOf(marks[0], false)
	if err != nil {
		return nil, err
	}

	b, err := indexOf(marks[1], true)
	if err != nil {
		return nil, err
	}

	c, err := indexOf(marks[2], true)
	if err != nil {
		return nil, err
	}

	d, err := indexOf(marks[3], true)
	if err != nil {
		return nil, err
	}

	touch := func(price, side float64) float64 {
		if side > 0 {
			return side
		}

		return price
	}

	bb, cc := trades[b], trades[c]

	return []float64{
		float64(a), float64(marks[0]), float64(b), float64(marks[1]),
		float64(c), float64(marks[2]), float64(d), float64(marks[3]),
		bb.price, cc.price, touch(bb.price, bb.ask), touch(cc.price, cc.bid),
		float64(bb.at), float64(cc.at),
		float64(b), float64(marks[1]), float64(c), float64(marks[2]),
		touch(bb.price, bb.ask), touch(cc.price, cc.bid), float64(bb.at), float64(cc.at),
	}, nil
}

func leadAdjustedMarks(marks [4]uint64) [4]uint64 {
	if system.Cfg == nil || system.Cfg.Detector == nil {
		return marks
	}

	precursor := int64(marks[1]) - int64(marks[0])
	lead := system.Cfg.Detector.LeadSpan(precursor)
	if lead <= 0 {
		return marks
	}

	out := marks
	out[1] = marks[1] - uint64(lead)
	out[2] = marks[2] - uint64(lead)

	if out[1] < out[0] {
		out[1] = out[0]
	}

	if out[2] < out[1] {
		out[2] = out[1]
	}

	return out
}

func validateAnnotation(class string, marks [4]uint64) error {
	if !slices.Contains(excursionClasses, class) {
		return errnie.Err(errnie.Validation, fmt.Sprintf("[hindsight] %q is not an excursion class", class), nil)
	}

	if !(marks[0] <= marks[1] && marks[1] <= marks[2] && marks[2] <= marks[3]) {
		return errnie.Err(errnie.Validation, "[hindsight] A B C D must be in tape order", nil)
	}

	return nil
}
