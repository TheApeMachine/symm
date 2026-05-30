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
*/
type Decision struct {
	Name        string
	Action      perspectives.ActionType
	Perspective perspectives.Perspective
}

var universalExitTree = &perspectives.Tree{Branches: perspectives.UniversalExitBranches()}

/*
perspectiveRegistry is conviction-first: more confirming categories rank earlier.
*/
var perspectiveRegistry = []registeredPerspective{
	{name: string(perspectives.PlaybookTrend), perspective: perspectives.NewTrendPerspective()},
	{name: string(perspectives.PlaybookDrive), perspective: perspectives.NewDrivePerspective()},
	{name: string(perspectives.PlaybookLeadLag), perspective: perspectives.NewLeadLagPerspective()},
	{name: string(perspectives.PlaybookScarcity), perspective: perspectives.NewScarcityPerspective()},
	{name: string(perspectives.PlaybookPump), perspective: perspectives.NewPumpPerspective()},
}

/*
NewPerspective returns the highest-priority traversable entry playbook, or nil.
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
Decisions returns every flat-entry playbook that authorizes ActionEnter.
Deny and wait verdicts are omitted.
*/
func Decisions(
	measurements []perspectives.Measurement,
	observations []perspectives.ObservationType,
) []Decision {
	decisions := make([]Decision, 0, len(perspectiveRegistry))

	for _, entry := range perspectiveRegistry {
		action := entry.perspective.Decide(measurements, observations)

		if action == nil || *action != perspectives.ActionEnter {
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
Decide returns the first actionable entry verdict in registry order.
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

/*
ExitDecisions collects exit verdicts from the universal overlay and from the
opening playbook (or every playbook when opener is empty). Soft take-profit
leaves are suppressed until MinExhaustHold when the trader passes hold timing.
*/
func ExitDecisions(
	measurements []perspectives.Measurement,
	observations []perspectives.ObservationType,
	openerPlaybook string,
	softExitsAllowed bool,
) []Decision {
	decisions := make([]Decision, 0, len(perspectiveRegistry)+1)

	if action := universalExitTree.Walk(measurements, observations); action != nil {
		if perspectives.IsExitAction(*action) && exitAllowed(*action, softExitsAllowed) {
			decisions = append(decisions, Decision{
				Name:   string(perspectives.PlaybookUniversal),
				Action: *action,
			})
		}
	}

	for _, entry := range perspectiveRegistry {
		if openerPlaybook != "" && entry.name != openerPlaybook {
			continue
		}

		action := entry.perspective.DecideExit(measurements, observations)

		if action == nil || !perspectives.IsExitAction(*action) {
			continue
		}

		if !exitAllowed(*action, softExitsAllowed) {
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

func exitAllowed(action perspectives.ActionType, softExitsAllowed bool) bool {
	if action == perspectives.ActionTakeProfit {
		return softExitsAllowed
	}

	return true
}
