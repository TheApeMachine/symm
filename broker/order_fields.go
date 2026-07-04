package broker

import (
	"encoding/hex"
	"strings"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

func actionSymbol(action *datura.Artifact) string {
	if action == nil {
		return ""
	}

	if symbol, err := action.Scope(); err == nil && strings.TrimSpace(symbol) != "" {
		return strings.TrimSpace(symbol)
	}

	return strings.TrimSpace(actionString(action, "symbol"))
}

func actionString(action *datura.Artifact, path ...any) string {
	if value, ok, err := datura.LookupPayload[string](action, path...); err == nil && ok {
		return strings.TrimSpace(value)
	}

	if value, ok, err := datura.LookupAttribute[string](action, path...); err == nil && ok {
		return strings.TrimSpace(value)
	}

	return strings.TrimSpace(datura.Peek[string](action, path...))
}

func actionStringFirst(action *datura.Artifact, paths ...[]any) string {
	for _, path := range paths {
		if value := actionString(action, path...); value != "" {
			return value
		}
	}

	return ""
}

func actionFloat(action *datura.Artifact, path ...any) float64 {
	if value, ok, err := datura.LookupPayload[float64](action, path...); err == nil && ok {
		return value
	}

	if value, ok, err := datura.LookupAttribute[float64](action, path...); err == nil && ok {
		return value
	}

	return datura.Peek[float64](action, path...)
}

func setupKey(action *datura.Artifact) string {
	for _, path := range [][]any{{"setup_key"}, {"branch_key"}, {"journey", "story", "terminal_branch_id"}} {
		if value := actionString(action, path...); value != "" {
			return normalizeKey(value)
		}
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

func artifactOrderID(order *datura.Artifact) (string, error) {
	if order == nil {
		return "", errnie.Error(errnie.Err(errnie.Validation, "broker: nil order id source", nil))
	}

	uuidBytes, err := order.Uuid()
	if err != nil {
		return "", errnie.Error(errnie.Err(errnie.Validation, "broker: order id missing uuid", err))
	}

	id := strings.TrimSpace(string(uuidBytes))
	if id != "" {
		return id, nil
	}

	if len(uuidBytes) > 0 {
		return hex.EncodeToString(uuidBytes), nil
	}

	return "", errnie.Error(errnie.Err(errnie.Validation, "broker: order id empty", nil))
}
