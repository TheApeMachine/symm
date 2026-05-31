package market

import (
	"maps"
	"slices"
)

/*
subscriptionSpec is one consumer's requested symbol set and optional book depth.
Depth is ignored for ticker and trade feeds; book feeds take the max depth across
all active subscribers.
*/
type subscriptionSpec struct {
	symbols []string
	depth   int
}

func mergeSubscriptionSpecs(specs []subscriptionSpec) subscriptionSpec {
	seen := make(map[string]struct{})
	merged := subscriptionSpec{}

	for _, spec := range specs {
		for _, symbol := range spec.symbols {
			if symbol == "" {
				continue
			}

			if _, ok := seen[symbol]; ok {
				continue
			}

			seen[symbol] = struct{}{}
			merged.symbols = append(merged.symbols, symbol)
		}

		if spec.depth > merged.depth {
			merged.depth = spec.depth
		}
	}

	slices.Sort(merged.symbols)

	return merged
}

func subscriptionSpecEqual(left, right subscriptionSpec) bool {
	if left.depth != right.depth {
		return false
	}

	if len(left.symbols) != len(right.symbols) {
		return false
	}

	leftSet := make(map[string]struct{}, len(left.symbols))

	for _, symbol := range left.symbols {
		leftSet[symbol] = struct{}{}
	}

	for _, symbol := range right.symbols {
		if _, ok := leftSet[symbol]; !ok {
			return false
		}
	}

	return true
}

func symbolSet(symbols []string) map[string]struct{} {
	set := make(map[string]struct{}, len(symbols))

	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		set[symbol] = struct{}{}
	}

	return set
}

func mergeSymbolMaps(into, from map[string]struct{}) {
	maps.Copy(into, from)
}
