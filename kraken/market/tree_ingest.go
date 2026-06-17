package market

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

/*
MarketTree returns the process-wide DMT tree used for market ingest and signal replay.
*/
func MarketTree() *dmt.Tree {
	return dmt.NewTree("")
}

/*
InsertMarketArtifact indexes each symbol row from a batched book, trade, or ticker artifact.
*/
func InsertMarketArtifact(tree *dmt.Tree, batch *datura.Artifact) {
	if tree == nil || batch == nil {
		return
	}

	switch datura.Peek[string](batch, "role") {
	case "book":
		insertBookUpdates(tree, datura.As[BookUpdates](batch))
	case "trade":
		insertTradeUpdates(tree, datura.As[TradeUpdates](batch))
	case "ticker":
		insertTickerUpdates(tree, datura.As[TickerUpdates](batch))
	}
}

func insertBookUpdates(tree *dmt.Tree, updates BookUpdates) {
	for _, update := range updates {
		if update == nil || update.Symbol == "" {
			continue
		}

		insertScoped(tree, "book", update.Symbol, BookUpdates{update})
	}
}

func insertTradeUpdates(tree *dmt.Tree, updates TradeUpdates) {
	for _, update := range updates {
		if update == nil || update.Symbol == "" {
			continue
		}

		insertScoped(tree, "trade", update.Symbol, TradeUpdates{update})
	}
}

func insertTickerUpdates(tree *dmt.Tree, updates TickerUpdates) {
	for _, update := range updates {
		if update == nil || update.Symbol == "" {
			continue
		}

		insertScoped(tree, "ticker", update.Symbol, TickerUpdates{update})
	}
}

func insertScoped(tree *dmt.Tree, role string, scope string, payload any) {
	row := datura.Acquire("kraken", datura.Artifact_Type_json)

	if row == nil {
		return
	}

	row.WithRole(role)
	row.WithScope(scope)

	if err := row.From(payload); err != nil {
		row.Release()

		return
	}

	tree.Insert(row.Prefix(), row.Marshal())
	row.Release()
}
