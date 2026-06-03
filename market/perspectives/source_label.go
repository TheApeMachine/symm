package perspectives

import (
	"strings"
	"unicode"
)

var sourceDisplayLabels = map[string]string{
	"hawkes":      "Hawkes",
	"fluid":       "Fluid",
	"pumpdump":    "Pump",
	"causal":      "Causal",
	"depthflow":   "Depth",
	"leadlag":     "L-Lag",
	"liquidity":   "Liquidity",
	"sentiment":   "Sent",
	"toxicity":    "Toxic",
	"correlation": "Corr",
	"exhaustion":  "Exhaust",
	"prediction":  "Pred",
	"cvd":         "CVD",
}

/*
SourceDisplayLabel returns the short dashboard label for a telemetry source name.
Unknown sources are title-cased from the wire name.
*/
func SourceDisplayLabel(name string) string {
	trimmed := strings.TrimSpace(name)

	if trimmed == "" {
		return ""
	}

	if label, ok := sourceDisplayLabels[trimmed]; ok {
		return label
	}

	return titleSourceName(trimmed)
}

func titleSourceName(name string) string {
	if name == "" {
		return ""
	}

	runes := []rune(name)
	runes[0] = unicode.ToUpper(runes[0])

	return string(runes)
}
