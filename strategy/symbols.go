package strategy

import (
	"slices"
	"strings"

	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

// Symbols groups signal layers on one workspace trade tick for grid.Observe.
type Symbols struct {
	tick *data.Value
}

func NewSymbols(tick *data.Value) *Symbols {
	return &Symbols{tick: tick}
}

func (symbols *Symbols) Names() []string {
	if symbols == nil || symbols.tick == nil {
		return nil
	}

	list := []string{}
	
	var trades []kraken.TradeData
	symbols.tick.Lookup(&trades, "spot:trade")
	
	for _, trade := range trades {
		if trade.Symbol != "" {
			list = append(list, trade.Symbol)
		}
	}

	for _, source := range types.SignalSourceStrings {
		for _, layer := range symbols.tick.LookupPrefix(source) {
			parts := strings.Split(layer.Key, "/")
			
			if len(parts) >= 4 && parts[1] != "aggregate" {
				list = append(list, parts[1]+"/"+parts[2])
			}
		}
	}

	if len(list) == 0 {
		return nil
	}

	slices.Sort(list)
	return slices.Compact(list)
}

func (symbols *Symbols) GroupBySymbol(bookStale func(string) bool) map[string]map[string]map[string]float64 {
	if symbols == nil || symbols.tick == nil {
		return nil
	}

	grouped := make(map[string]map[string]map[string]float64)

	for _, source := range types.SignalSourceStrings {
		for _, layer := range symbols.tick.LookupPrefix(source) {
			parts := strings.Split(layer.Key, "/")
			
			if len(parts) < 4 || parts[1] == "aggregate" {
				continue
			}

			var metrics []float64
			
			if layer.Decode(&metrics) != nil {
				continue
			}

			symbol := parts[1] + "/" + parts[2]
			labels := parts[3:]

			for i, label := range labels {
				if i >= len(metrics) {
					break
				}
			
				if grouped[symbol] == nil {
					grouped[symbol] = make(map[string]map[string]float64)
				}
			
				if grouped[symbol][source] == nil {
					grouped[symbol][source] = make(map[string]float64)
				}
			
				grouped[symbol][source][label] = metrics[i]
			}
		}
	}

	for _, source := range types.SignalSourceStrings {
		for _, layer := range symbols.tick.LookupPrefix(source) {
			parts := strings.Split(layer.Key, "/")
			
			if len(parts) != 3 || parts[1] != "aggregate" {
				continue
			}

			var metrics []float64
			
			if layer.Decode(&metrics) != nil || len(metrics) == 0 {
				continue
			}

			label := parts[2]

			for symbol := range grouped {
				if bookStale != nil && bookStale(symbol) {
					switch source {
					case "liquidity", "depthflow", "cvd", "morphology", "toxicity", "pumpdump":
						continue
					}
				}

				if grouped[symbol][source] == nil {
					grouped[symbol][source] = make(map[string]float64)
				}

				grouped[symbol][source][label] = metrics[0]
			}
		}
	}

	if len(grouped) == 0 {
		return nil
	}

	return grouped
}
