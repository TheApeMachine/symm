package market

import (
	"github.com/theapemachine/datura"
)

/*
PayloadSymbols extracts unique symbols from book, trade, or ticker artifacts.
*/
func PayloadSymbols(artifact *datura.Artifact) []string {
	if artifact == nil {
		return nil
	}

	role := datura.Peek[string](artifact, "role")

	switch role {
	case "book":
		return symbolsFromBookUpdates(datura.As[BookUpdates](artifact))
	case "trade":
		return symbolsFromTradeUpdates(datura.As[TradeUpdates](artifact))
	case "ticker":
		return symbolsFromTickerUpdates(datura.As[TickerUpdates](artifact))
	default:
		return nil
	}
}

/*
VisitTickers invokes visit for each ticker row with a positive last price.
*/
func VisitTickers(
	artifact *datura.Artifact,
	visit func(symbol string, last float64) bool,
) {
	if artifact == nil || visit == nil {
		return
	}

	if datura.Peek[string](artifact, "role") != "ticker" {
		return
	}

	for _, update := range datura.As[TickerUpdates](artifact) {
		if update == nil || update.Symbol == "" || update.Last <= 0 {
			continue
		}

		if !visit(update.Symbol, update.Last) {
			return
		}
	}
}

/*
TickerFeedArtifact wraps ticker updates in a feed artifact for tests.
*/
func TickerFeedArtifact(updates TickerUpdates) *datura.Artifact {
	return feedArtifact("ticker", updates)
}

/*
TradeFeedArtifact wraps trade updates in a feed artifact for tests.
*/
func TradeFeedArtifact(updates TradeUpdates) *datura.Artifact {
	return feedArtifact("trade", updates)
}

/*
BookFeedArtifact wraps book updates in a feed artifact for tests.
*/
func BookFeedArtifact(updates BookUpdates) *datura.Artifact {
	return feedArtifact("book", updates)
}

func feedArtifact(role string, payload any) *datura.Artifact {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)

	if artifact == nil {
		return nil
	}

	artifact.WithRole(role)
	_ = artifact.From(payload)

	return artifact
}

func symbolsFromBookUpdates(updates BookUpdates) []string {
	symbols := make([]string, 0, len(updates))
	seen := make(map[string]struct{}, len(updates))

	for _, update := range updates {
		if update == nil || update.Symbol == "" {
			continue
		}

		if _, exists := seen[update.Symbol]; exists {
			continue
		}

		seen[update.Symbol] = struct{}{}
		symbols = append(symbols, update.Symbol)
	}

	return symbols
}

func symbolsFromTradeUpdates(updates TradeUpdates) []string {
	symbols := make([]string, 0, len(updates))
	seen := make(map[string]struct{}, len(updates))

	for _, update := range updates {
		if update == nil || update.Symbol == "" {
			continue
		}

		if _, exists := seen[update.Symbol]; exists {
			continue
		}

		seen[update.Symbol] = struct{}{}
		symbols = append(symbols, update.Symbol)
	}

	return symbols
}

func symbolsFromTickerUpdates(updates TickerUpdates) []string {
	symbols := make([]string, 0, len(updates))
	seen := make(map[string]struct{}, len(updates))

	for _, update := range updates {
		if update == nil || update.Symbol == "" {
			continue
		}

		if _, exists := seen[update.Symbol]; exists {
			continue
		}

		seen[update.Symbol] = struct{}{}
		symbols = append(symbols, update.Symbol)
	}

	return symbols
}
