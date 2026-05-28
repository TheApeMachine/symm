package engine

const (
	ExitReasonImbalanceFlip = "imbalance_flip"
	ExitReasonPressureFade  = "pressure_fade"
	ExitReasonRunwayExpired = "runway_expired"
)

/*
Exit is exhaust-driven urgency to close an open long.
*/
type Exit struct {
	Symbol  string
	Urgency float64
	Reason  string
}

/*
ValidExit reports whether an exit signal has the fields required to act on.
*/
func ValidExit(exit Exit) bool {
	if exit.Symbol == "" || exit.Reason == "" {
		return false
	}

	if exit.Urgency <= 0 {
		return false
	}

	return true
}
