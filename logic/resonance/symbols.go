package resonance

import (
	"slices"
	"strings"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func symbolsOnTick(tick *data.Value) []string {
	if tick == nil {
		return nil
	}

	seen := map[string]struct{}{}

	var trades []kraken.TradeData
	tick.Lookup(&trades, "spot:trade")

	for _, trade := range trades {
		if trade.Symbol == "" {
			continue
		}

		seen[trade.Symbol] = struct{}{}
	}

	for _, source := range types.SignalSourceStrings {
		for _, layer := range tick.LookupPrefix(source) {
			parts := strings.Split(layer.Key, "/")

			if len(parts) < 4 {
				continue
			}

			seen[parts[1]+"/"+parts[2]] = struct{}{}
		}
	}

	out := make([]string, 0, len(seen))

	for symbol := range seen {
		out = append(out, symbol)
	}

	slices.Sort(out)
	return out
}

func attachReading(tick *data.Value, symbol string, reading *data.Value) {
	if tick == nil || reading == nil || symbol == "" {
		return
	}

	scope := strings.ReplaceAll(symbol, "/", "-")

	for label, val := range floatMetrics(reading) {
		tick.Append([]float64{val}, "resonance", scope, label)
	}
}
