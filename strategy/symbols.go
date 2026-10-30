package strategy

import (
	"slices"
	"strings"

	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

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

func isBookDependent(source string) bool {
	return source == "liquidity" || source == "depthflow"
}

/*
SignalSourcesBySymbol groups signal metrics on value by symbol, source, and label.
Optional bookStale function prevents book-dependent aggregate signals from fanning
onto symbols whose book is stale or absent.
*/
func SignalSourcesBySymbol(
	value *data.Value,
	bookStale ...func(string) bool,
) map[string]map[string]map[string]float64 {
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

			mergeLayer(out, symbol, source, parts[3:], values)
		}
	}

	var isStale func(string) bool
	if len(bookStale) > 0 {
		isStale = bookStale[0]
	}

	mergeAggregateLayers(value, out, isStale)

	return out
}

func mergeLayer(
	out map[string]map[string]map[string]float64,
	symbol, source string,
	labels []string,
	values []float64,
) {
	bySource := out[symbol]
	if bySource == nil {
		bySource = make(map[string]map[string]float64)
		out[symbol] = bySource
	}

	byLabel := bySource[source]
	if byLabel == nil {
		byLabel = make(map[string]float64)
		bySource[source] = byLabel
	}

	for i, label := range labels {
		if i >= len(values) {
			break
		}
		byLabel[label] = values[i]
	}
}

/*
mergeAggregateLayers fans source/aggregate/<label> readings onto every
symbol the tick touches so the grid can score enter support without a
focal pair.
*/
func mergeAggregateLayers(
	value *data.Value,
	out map[string]map[string]map[string]float64,
	bookStale func(string) bool,
) {
	targets := map[string]struct{}{}
	for symbol := range out {
		targets[symbol] = struct{}{}
	}
	for _, symbol := range SymbolsOnTick(value) {
		targets[symbol] = struct{}{}
	}

	for _, source := range types.SignalSourceStrings {
		for _, layer := range value.LookupPrefix(source) {
			parts := strings.Split(layer.Key, "/")
			if len(parts) != 3 || parts[1] != "aggregate" {
				continue
			}

			var values []float64
			if err := layer.Decode(&values); err != nil || len(values) == 0 {
				continue
			}

			labels := []string{parts[2]}
			for symbol := range targets {
				if isBookDependent(source) && bookStale != nil && bookStale(symbol) {
					continue
				}
				mergeLayer(out, symbol, source, labels, values)
			}
		}
	}
}
