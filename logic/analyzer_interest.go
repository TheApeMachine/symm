package logic

import (
	"sort"

	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/types"
)

/*
Interest returns open inventory first, then every Hawkes intensity leader.
L3 books for the quote universe are subscribed once with instruments; this
list is for analysis priority, not transport expansion.
*/
func (analyzer *Analyzer) Interest(thesis *types.Thesis) []string {
	if analyzer == nil || analyzer.hawkes == nil {
		return nil
	}

	type ranked struct {
		symbol    string
		intensity float64
	}

	rankedSymbols := make([]ranked, 0)
	selected := make([]string, 0)
	seen := make(map[string]struct{})

	if thesis != nil {
		thesis.Holdings.Range(func(key, value any) bool {
			symbol, ok := key.(string)

			if !ok || symbol == "" {
				return true
			}

			switch holding := value.(type) {
			case *types.Holding:
				if holding == nil || holding.Status == types.CLOSED {
					return true
				}
			case types.Holding:
				if holding.Status == types.CLOSED {
					return true
				}
			default:
				return true
			}

			if _, exists := seen[symbol]; exists {
				return true
			}

			seen[symbol] = struct{}{}
			selected = append(selected, symbol)

			return true
		})
	}

	for _, symbol := range analyzer.hawkes.Symbols() {
		outcome, ok := analyzer.hawkes.Outcome(symbol)

		if !ok || !outcome.Readiness.Intensity {
			continue
		}

		intensity := interestIntensity(outcome)

		if intensity <= 0 {
			continue
		}

		rankedSymbols = append(rankedSymbols, ranked{
			symbol:    symbol,
			intensity: intensity,
		})
	}

	sort.Slice(rankedSymbols, func(left, right int) bool {
		if rankedSymbols[left].intensity == rankedSymbols[right].intensity {
			return rankedSymbols[left].symbol < rankedSymbols[right].symbol
		}

		return rankedSymbols[left].intensity > rankedSymbols[right].intensity
	})

	for _, candidate := range rankedSymbols {
		if _, exists := seen[candidate.symbol]; exists {
			continue
		}

		seen[candidate.symbol] = struct{}{}
		selected = append(selected, candidate.symbol)
	}

	return selected
}

func interestIntensity(outcome excitation.Outcome) float64 {
	if outcome.Readiness.HawkesFit {
		return outcome.Fit.IntensityX + outcome.Fit.IntensityY
	}

	return outcome.BuyArrivalRate + outcome.SellArrivalRate
}
