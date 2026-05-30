package market

import (
	"github.com/theapemachine/symm/market/perspectives"
)

// registeredPerspective is one named playbook in the priority-ordered registry.
type registeredPerspective struct {
	name        string
	perspective perspectives.Perspective
}

/*
perspectiveRegistry is the priority-ordered list of playbooks the market layer
matches against. Order is significant and deterministic: when more than one
perspective is traversable for the same measurement set, the first entry wins.
A map was previously used here, which made the winning perspective depend on Go's
randomized map-iteration order — two identical measurement sets could pick
different playbooks run to run.

Order is conviction-first: the more confirmations a playbook demands before it will
open a trade, the earlier it sits, so the best-supported thesis is preferred when
several apply to the same symbol. "trend" requires an authentic driver confirmed by
the tape; "drive" enters on executed order flow; "leadlag" and "scarcity" act on a
single structural edge plus, for scarcity, an ignition; "pump" is the most
opportunistic microstructure entry. These run in parallel across symbols — one coin
can be a localized pump while another is a legitimate trend at the same time.
*/
var perspectiveRegistry = []registeredPerspective{
	{name: "trend", perspective: perspectives.NewTrendPerspective()},
	{name: "drive", perspective: perspectives.NewDrivePerspective()},
	{name: "leadlag", perspective: perspectives.NewLeadLagPerspective()},
	{name: "scarcity", perspective: perspectives.NewScarcityPerspective()},
	{name: "pump", perspective: perspectives.NewPumpPerspective()},
}

/*
NewPerspective returns the highest-priority perspective that is traversable for
the measurement set, or nil when none apply. Selection is deterministic: the
registry is scanned in fixed priority order.
*/
func NewPerspective(measurements []perspectives.Measurement) perspectives.Perspective {
	for _, entry := range perspectiveRegistry {
		if found := entry.perspective.Walk(measurements); found != nil {
			return found
		}
	}

	return nil
}

/*
Decide walks every registered perspective against the measurement set and live
observation state in fixed priority order and returns the first actionable verdict —
the playbook Action and the perspective that produced it. With no observations it is
the entry view; passing ObservationHolding asks the same playbooks for the exit
verdict on an open position. Returns (nil, nil) when no perspective is traversable.
Ordering is deterministic so the same readings always resolve to the same playbook.
*/
func Decide(
	measurements []perspectives.Measurement, observations []perspectives.ObservationType,
) (*perspectives.ActionType, perspectives.Perspective) {
	for _, entry := range perspectiveRegistry {
		if action := entry.perspective.Decide(measurements, observations); action != nil {
			return action, entry.perspective
		}
	}

	return nil, nil
}
