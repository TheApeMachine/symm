package trader

import (
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
)

type Decision struct {
	baseFraction float64
}

func NewDecision() (*Decision, error) {
	baseFraction := viper.GetFloat64("trading.sizing.base_fraction")
	if baseFraction <= 0 || math.IsNaN(baseFraction) || math.IsInf(baseFraction, 0) {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: trading.sizing.base_fraction must be positive",
			nil,
		))
	}

	return &Decision{baseFraction: baseFraction}, nil
}

func (decision *Decision) Choose(actions []*datura.Artifact) ([]*datura.Artifact, error) {
	selected := make([]*datura.Artifact, 0, len(actions))
	entry := decision.entry(actions)

	for _, action := range actions {
		if action == nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"trader: nil candidate action",
				nil,
			))
		}

		action.WithRole("decision").WithDestination("ui")

		if decision.exit(action) || action == entry {
			if err := decision.allow(action); err != nil {
				return nil, err
			}

			selected = append(selected, action)
			continue
		}

		decision.block(action, "lower-ranked candidate")
	}

	return selected, nil
}

func (decision *Decision) entry(actions []*datura.Artifact) *datura.Artifact {
	var selected *datura.Artifact
	score := 0.0

	for _, action := range actions {
		if action == nil || decision.exit(action) {
			continue
		}

		confidence := datura.Peek[float64](action, "entry_confidence")
		if confidence <= score {
			continue
		}

		selected = action
		score = confidence
	}

	return selected
}

func (decision *Decision) allow(action *datura.Artifact) error {
	actionType := logic.ActionType(datura.Peek[string](action, "type"))
	if actionType == "" || actionType == logic.ActionNone {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: candidate action type required",
			nil,
		).With(action.Log()...))
	}

	if datura.Peek[float64](action, "fraction") <= 0 &&
		datura.Peek[float64](action, "notional") <= 0 &&
		datura.Peek[float64](action, "quantity") <= 0 &&
		!actionType.IsExit() {
		action.Poke(decision.baseFraction, "risk", "fraction")
	}

	action.Poke(true, "allowed")
	action.Poke("allowed", "decision", "verdict")
	action.Poke(time.Now().UTC().Format(time.RFC3339Nano), "decision", "timestamp")
	action.Poke(true, "risk", "stamped")

	return nil
}

func (decision *Decision) block(action *datura.Artifact, reason string) {
	action.Poke(false, "allowed")
	action.Poke("blocked", "decision", "verdict")
	action.Poke(reason, "decision", "reason")
	action.Poke(time.Now().UTC().Format(time.RFC3339Nano), "decision", "timestamp")
}

func (decision *Decision) exit(action *datura.Artifact) bool {
	actionType := logic.ActionType(datura.Peek[string](action, "type"))
	return actionType.IsExit() || actionType.Protective()
}
