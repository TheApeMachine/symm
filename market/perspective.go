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
Decision is one actionable verdict from a registered perspective.

Several playbooks can be true at the same time. The trader consumes the full set
so independent theses can reinforce each other, while Decide remains as the
backwards-compatible priority view for callers that need one deterministic answer.
*/
type Decision struct {
	Name        string
	Action      perspectives.ActionType
	Perspective perspectives.Perspective
}

/*
perspectiveRegistry is the priority-ordered list of playbooks the market layer
matches against. Order is significant and deterministic: when a caller asks for a
single verdict, the first actionable playbook wins.

Order is conviction-first: the more confirmations a playbook demands before it will
open a trade, the earlier it sits, so the best-supported thesis is preferred when
several apply to the same symbol. Callers that need concurrent theses use Decisions.
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
Decisions walks every registered perspective against the measurement set and live
observation state in fixed priority order, returning every actionable verdict.
With no observations it is the entry view; passing ObservationHolding asks the
same playbooks for the exit verdict on an open position.
*/
func Decisions(
	measurements []perspectives.Measurement,
	observations []perspectives.ObservationType,
) []Decision {
	decisions := make([]Decision, 0, len(perspectiveRegistry))

	for _, entry := range perspectiveRegistry {
		action := entry.perspective.Decide(measurements, observations)

		if action == nil {
			continue
		}

		decisions = append(decisions, Decision{
			Name:        entry.name,
			Action:      *action,
			Perspective: entry.perspective,
		})
	}

	return decisions
}

/*
Decide returns the first actionable verdict in deterministic priority order.
*/
func Decide(
	measurements []perspectives.Measurement,
	observations []perspectives.ObservationType,
) (*perspectives.ActionType, perspectives.Perspective) {
	for _, decision := range Decisions(measurements, observations) {
		action := decision.Action

		return &action, decision.Perspective
	}

	return nil, nil
}
