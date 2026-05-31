package market

import (
	"fmt"
	"sync"

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
var (
	perspectiveRegistryLock sync.RWMutex
	perspectiveRegistry     = defaultPerspectiveRegistry()
)

func defaultPerspectiveRegistry() []registeredPerspective {
	return []registeredPerspective{
		{name: string(perspectives.PlaybookTrend), perspective: perspectives.NewTrendPerspective()},
		{name: string(perspectives.PlaybookDrive), perspective: perspectives.NewDrivePerspective()},
		{name: string(perspectives.PlaybookLeadLag), perspective: perspectives.NewLeadLagPerspective()},
		{name: string(perspectives.PlaybookScarcity), perspective: perspectives.NewScarcityPerspective()},
		{name: string(perspectives.PlaybookPump), perspective: perspectives.NewPumpPerspective()},
	}
}

func snapshotPerspectiveRegistry() []registeredPerspective {
	perspectiveRegistryLock.RLock()
	defer perspectiveRegistryLock.RUnlock()

	return perspectiveRegistry
}

/*
RestoreDefaultPerspectiveRegistry resets the active registry to the Go builtin
playbooks. Tests and tooling use this after loading YAML overrides.
*/
func RestoreDefaultPerspectiveRegistry() {
	perspectiveRegistryLock.Lock()
	perspectiveRegistry = defaultPerspectiveRegistry()
	perspectiveRegistryLock.Unlock()
}

func LoadPerspectiveRegistry(path string) error {
	document, err := perspectives.LoadDocumentFile(path)

	if err != nil {
		return err
	}

	strategies, err := perspectives.BuildStrategies(document)

	if err != nil {
		return err
	}

	return SetPerspectiveRegistry(strategies)
}

func SetPerspectiveRegistry(strategies []perspectives.Perspective) error {
	if len(strategies) == 0 {
		return fmt.Errorf("perspective registry requires at least one strategy")
	}

	registry := make([]registeredPerspective, 0, len(strategies))

	for _, strategy := range strategies {
		if strategy == nil {
			return fmt.Errorf("nil perspective strategy")
		}

		registry = append(registry, registeredPerspective{
			name:        string(strategy.Name()),
			perspective: strategy,
		})
	}

	perspectiveRegistryLock.Lock()
	defer perspectiveRegistryLock.Unlock()
	perspectiveRegistry = registry

	return nil
}

/*
NewPerspective returns the highest-priority traversable entry playbook, or nil.
*/
func NewPerspective(measurements []perspectives.Measurement) perspectives.Perspective {
	for _, entry := range snapshotPerspectiveRegistry() {
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
	return DecisionsWithContext(measurements, observations, nil)
}

func DecisionsWithContext(
	measurements []perspectives.Measurement,
	observations []perspectives.ObservationType,
	context func(string) perspectives.DecisionContext,
) []Decision {
	registry := snapshotPerspectiveRegistry()
	decisions := make([]Decision, 0, len(registry))

	for _, entry := range registry {
		action := decideWithContext(entry.perspective, measurements, observations, contextFor(context, entry.name))

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

type contextDecider interface {
	DecideWithContext(
		measurements []perspectives.Measurement,
		observations []perspectives.ObservationType,
		context perspectives.DecisionContext,
	) *perspectives.ActionType
}

func decideWithContext(
	perspective perspectives.Perspective,
	measurements []perspectives.Measurement,
	observations []perspectives.ObservationType,
	context perspectives.DecisionContext,
) *perspectives.ActionType {
	if decider, ok := perspective.(contextDecider); ok {
		return decider.DecideWithContext(measurements, observations, context)
	}

	return perspective.Decide(measurements, observations)
}

func contextFor(
	provider func(string) perspectives.DecisionContext,
	name string,
) perspectives.DecisionContext {
	if provider == nil {
		return perspectives.DecisionContext{}
	}

	return provider(name)
}

type entryCategoryProvider interface {
	EntryCategories() []perspectives.CategoryType
}

func EntryCategoriesForPlaybook(name string) []perspectives.CategoryType {
	for _, entry := range snapshotPerspectiveRegistry() {
		if entry.name != name {
			continue
		}

		provider, ok := entry.perspective.(entryCategoryProvider)

		if !ok {
			return nil
		}

		return provider.EntryCategories()
	}

	return nil
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
	registry := snapshotPerspectiveRegistry()
	decisions := make([]Decision, 0, len(registry)+1)

	if action := universalExitTree.Walk(measurements, observations); action != nil {
		if perspectives.IsExitAction(*action) && exitAllowed(*action, softExitsAllowed) {
			decisions = append(decisions, Decision{
				Name:   string(perspectives.PlaybookUniversal),
				Action: *action,
			})
		}
	}

	for _, entry := range registry {
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
