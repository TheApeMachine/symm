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

	outB, outC := leadAdjustedIndices(a, b, c)

	touch := func(price, side float64) float64 {
		if side > 0 {
			return side
		}

		return price
	}

	bb, cc := trades[outB], trades[outC]

	return []float64{
		float64(a), float64(marks[0]), float64(outB), float64(bb.tick),
		float64(outC), float64(cc.tick), float64(d), float64(marks[3]),
		bb.price, cc.price, touch(bb.price, bb.ask), touch(cc.price, cc.bid),
		float64(bb.at), float64(cc.at),
		float64(outB), float64(bb.tick), float64(outC), float64(cc.tick),
		touch(bb.price, bb.ask), touch(cc.price, cc.bid), float64(bb.at), float64(cc.at),
	}, nil
}

func leadAdjustedIndices(a, b, c int) (outB, outC int) {
	outB, outC = b, c

	if system.Cfg == nil || system.Cfg.Detector == nil {
		return outB, outC
	}

	lead := system.Cfg.Detector.LeadSpan(int64(b - a))
	if lead <= 0 {
		return outB, outC
	}

	outB = b - int(lead)
	outC = c - int(lead)

	if outB < a {
		outB = a
	}

	if outC < outB {
		outC = outB
	}

	return outB, outC
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
