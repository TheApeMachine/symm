package resonance

import "github.com/theapemachine/symm/logic"

/*
MeasureTargets maps a resonance attention mode to the specialist signals the
trader should Measure when that mode dominates.
*/
func MeasureTargets(category logic.CategoryType) []string {
	switch string(category) {
	case CategoryFlow:
		return []string{
			"fluid",
			"depthflow",
			"exhaust",
			"liquidity",
		}
	case CategoryStress:
		return []string{
			"toxicity",
			"hawkes",
			"pumpdump",
			"cvd",
		}
	case CategoryCoupling:
		return []string{
			"correlation",
			"leadlag",
			"causal",
			"sentiment",
			"manifold",
			"prediction",
		}
	default:
		return nil
	}
}
