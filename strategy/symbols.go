package strategy

import (
	"slices"
	"strings"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
symbolScope is the desk and dashboard scope for a pair (BTC/USD → BTC-USD).
Signal and resonance layers on the workspace tick use this form.
*/
func symbolScope(symbol string) string {
	return strings.ReplaceAll(symbol, "/", "-")
}

/*
SymbolsOnTick lists every pair touched on this workspace tick: trade prints
first, then any signal layer keyed under source/symbol/….
*/
func SymbolsOnTick(value *data.Value) []string {
	if value == nil {
		return nil
	}

	seen := map[string]struct{}{}

	var trades []kraken.TradeData
	value.Lookup(&trades, "spot:trade")

	for _, trade := range trades {
		if trade.Symbol == "" {
			continue
		}

		seen[trade.Symbol] = struct{}{}
	}

	for _, source := range types.SignalSourceStrings {
		for _, layer := range value.LookupPrefix(source) {
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

/*
SignalSourcesBySymbol groups signal metrics on value by symbol, source, and label.
*/
func SignalSourcesBySymbol(value *data.Value) map[string]map[string]map[string]float64 {
	if value == nil {
		return nil
	}

	out := make(map[string]map[string]map[string]float64)

	for _, source := range types.SignalSourceStrings {
		for _, layer := range value.LookupPrefix(source) {
			parts := strings.Split(layer.Key, "/")

			if len(parts) < 4 {
				continue
			}

			symbol := parts[1] + "/" + parts[2]

			var values []float64

			if err := layer.Decode(&values); err != nil {
				continue
			}

			bySource := out[symbol]

			if bySource == nil {
				bySource = make(map[string]map[string]float64)
				out[symbol] = bySource
			}

			labels := bySource[source]

			if labels == nil {
				labels = make(map[string]float64)
				bySource[source] = labels
			}

			for i, label := range parts[3:] {
				if i >= len(values) {
					break
				}

				labels[label] = values[i]
			}
		}
	}

	return out
}
