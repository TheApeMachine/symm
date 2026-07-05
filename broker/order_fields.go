package broker

import (
	"strings"

	"github.com/theapemachine/symm/logic"
)

func actionSymbol(action *logic.Action) string {
	if action == nil {
		return ""
	}

	return strings.TrimSpace(action.Symbol)
}

func actionString(action *logic.Action, path ...any) string {
	if action == nil || len(path) == 0 {
		return ""
	}

	switch strings.TrimSpace(path[0].(string)) {
	case "symbol":
		return strings.TrimSpace(action.Symbol)
	case "side":
		return strings.TrimSpace(string(action.Side))
	case "type":
		return strings.TrimSpace(string(action.Type))
	case "decision_id":
		return strings.TrimSpace(action.DecisionID)
	case "action_id":
		return strings.TrimSpace(action.ActionID)
	case "setup_key", "branch_key":
		return strings.TrimSpace(action.BranchKey)
	case "reason_source":
		return strings.TrimSpace(string(action.ReasonSource))
	case "reason_category":
		return strings.TrimSpace(string(action.ReasonCategory))
	default:
		return ""
	}
}

func actionStringFirst(action *logic.Action, paths ...[]any) string {
	for _, path := range paths {
		if value := actionString(action, path...); value != "" {
			return value
		}
	}

	return ""
}

func actionFloat(action *logic.Action, path ...any) float64 {
	if action == nil || len(path) == 0 {
		return 0
	}

	switch strings.TrimSpace(path[0].(string)) {
	case "quantity":
		return action.Quantity
	case "fraction":
		return action.Fraction
	case "notional":
		return action.Notional
	case "limit_price", "price", "trigger_price", "stop":
		return action.Price
	case "trailing_stop", "offset":
		return action.Offset
	default:
		return 0
	}
}

func setupKey(action *logic.Action) string {
	if action == nil {
		return ""
	}

	for _, value := range []string{action.BranchKey, action.Story.TerminalBranchID} {
		if strings.TrimSpace(value) == "" {
			continue
		}

		return normalizeKey(value)
	}

	source := actionString(action, "reason_source")
	category := actionString(action, "reason_category")
	if source == "" || category == "" {
		return ""
	}

	return normalizeKey(source + "|" + category)
}

func normalizeKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.Join(strings.Fields(key), "_")
	return key
}

func baseAsset(symbol string, quote string) string {
	base, _, ok := strings.Cut(symbol, "/")
	if ok {
		return strings.ToUpper(strings.TrimSpace(base))
	}

	upper := strings.ToUpper(strings.TrimSpace(symbol))
	quote = strings.ToUpper(strings.TrimSpace(quote))
	if quote != "" && strings.HasSuffix(upper, quote) {
		return strings.TrimSuffix(upper, quote)
	}

	return upper
}
