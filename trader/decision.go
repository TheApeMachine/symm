package trader

import (
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type Decision struct {
	baseFraction float64
	readiness    *Readiness
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

	readiness, err := NewReadiness()
	if err != nil {
		return nil, err
	}

	return &Decision{
		baseFraction: baseFraction,
		readiness:    readiness,
	}, nil
}

func (decision *Decision) Choose(
	actions []*logic.Action,
	story *market.Story,
	now time.Time,
) ([]*logic.Action, error) {
	ready := make([]*logic.Action, 0, len(actions))
	selected := make([]*logic.Action, 0, len(actions))

	for _, action := range actions {
		if action == nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"trader: nil candidate action",
				nil,
			))
		}

		reason, err := decision.readiness.Reason(story, action, now)
		if err != nil {
			return nil, err
		}

		if reason != "" {
			decision.block(action, reason)
			continue
		}

		ready = append(ready, action)
	}

	entry := decision.entry(ready)

	for _, action := range ready {
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

func (decision *Decision) entry(actions []*logic.Action) *logic.Action {
	var selected *logic.Action
	score := 0.0

	for _, action := range actions {
		if action == nil || decision.exit(action) {
			continue
		}

		if selected != nil && action.EntryScore <= score {
			continue
		}

		selected = action
		score = action.EntryScore
	}

	return selected
}

func (decision *Decision) allow(action *logic.Action) error {
	return action.Allow(decision.baseFraction)
}

func (decision *Decision) block(action *logic.Action, reason string) {
	action.Block(reason)
}

func (decision *Decision) exit(action *logic.Action) bool {
	return action.Type.IsExit() || action.Type.Protective()
}
