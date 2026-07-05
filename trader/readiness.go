package trader

import (
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

/*
Readiness owns the trader admission threshold for entry candidates.
It reads fresh signal origins from Story, so UI status and trade admission share
the same backend measurement state.
*/
type Readiness struct {
	maxAge     time.Duration
	minOrigins int
}

func NewReadiness() (*Readiness, error) {
	maxAge := viper.GetDuration("market.story.measurement_max_age")
	if maxAge <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: market.story.measurement_max_age must be positive",
			nil,
		))
	}

	minOrigins := viper.GetInt("trading.entry.min_active_origins")
	
	if minOrigins <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: trading.entry.min_active_origins must be positive",
			nil,
		))
	}

	return &Readiness{
		maxAge:     maxAge,
		minOrigins: minOrigins,
	}, nil
}

func (readiness *Readiness) Reason(
	story *market.Story,
	action *logic.Action,
	now time.Time,
) (string, error) {
	if story == nil {
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: story required for decision readiness",
			nil,
		))
	}

	if action == nil {
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: action required for decision readiness",
			nil,
		))
	}

	if action.Type.IsExit() || action.Type.Protective() {
		return "", nil
	}

	if action.Type == "" || action.Type == logic.ActionNone {
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: action type required for decision readiness",
			nil,
		))
	}

	if strings.TrimSpace(action.Symbol) == "" {
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: action symbol required for decision readiness",
			nil,
		))
	}

	origins := story.ActiveOrigins(action.Symbol, now, readiness.maxAge)
	if len(origins) >= readiness.minOrigins {
		return "", nil
	}

	return "insufficient active signals " +
		strconv.Itoa(len(origins)) + "/" +
		strconv.Itoa(readiness.minOrigins), nil
}
