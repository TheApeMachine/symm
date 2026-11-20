package broker

import (
	"math"

	"github.com/theapemachine/symm/system"
)

/*
SizeHint steers Allocate: conviction scales the risk budget on the keep-back
lane. Matched paths, higher board scores, and lower resistance size up;
explore stays smaller. A zero-value hint keeps the full MaxFraction budget.
*/
type SizeHint struct {
	Score      float64
	Resistance float64
	PathFound  bool
}

/*
Conviction is the fraction of the MaxFraction lane budget to spend (0.35–1.0).
*/
func (hint SizeHint) Conviction() float64 {
	if hint.Score == 0 && hint.Resistance == 0 {
		return 1
	}

	floor := system.Cfg.Selection.MinBoardScore
	if floor <= 0 {
		floor = 0.35
	}

	conviction := 0.55

	if hint.PathFound {
		conviction += 0.20
	}

	if hint.Score > 0 {
		conviction += 0.25 * math.Tanh((hint.Score-floor)/floor)
	}

	if hint.Resistance > 0 {
		conviction -= 0.20 * math.Min(hint.Resistance, 1)
	}

	if conviction < 0.35 {
		return 0.35
	}

	if conviction > 1 {
		return 1
	}

	return conviction
}
