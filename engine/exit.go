package engine

const (
	ExitReasonImbalanceFlip = "imbalance_flip"
	ExitReasonPressureFade  = "pressure_fade"
	ExitReasonRunwayExpired = "runway_expired"
	ExitReasonStopHit       = "stop_hit"
)

/*
Exit is exhaust-driven urgency to close an open long.
*/
type Exit struct {
	Symbol     string
	Urgency    float64
	Reason     string
	LimitPrice float64 // stop trigger; the fill must not be credited above this
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
