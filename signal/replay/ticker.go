package replay

import (
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/signal/codec"
)

/*
IngestTickerBatch updates cross-section conviction features for every scope.
*/
func IngestTickerBatch(tree *dmt.Tree, batch *datura.Artifact) {
	if tree == nil || batch == nil {
		return
	}

	updates := datura.As[krakenmarket.TickerUpdates](batch)

	for _, update := range updates {
		if update == nil || update.Symbol == "" {
			continue
		}

		quoteState := quote(update.Symbol)
		quoteState.changePct = update.ChangePct
		quoteState.move = math.Abs(update.ChangePct)
	}

	breadth, surgeThreshold, leaderScope, leaderMove := convictionCrossSection()

	if breadth <= 0 && surgeThreshold <= 0 && leaderScope == "" {
		return
	}

	marketRegistry.quotes.Range(func(key, value any) bool {
		scope, ok := key.(string)

		if !ok || scope == "" {
			return true
		}

		quoteState, ok := value.(*quoteState)

		if !ok {
			return true
		}

		leader := 0.0

		if scope == leaderScope {
			leader = 1
		}

		payload := codec.EncodePayload(
			breadth,
			quoteState.changePct,
			surgeThreshold,
			leader,
			leaderMove,
		)

		if !codec.ValidFloatPayload(payload, codec.ConvictionMinFloats) {
			return true
		}

		insertScopedPayload(tree, "features", scope, payload)

		return true
	})
}

func convictionCrossSection() (breadth, surgeThreshold float64, leaderScope string, leaderMove float64) {
	changes := make([]float64, 0, 32)
	positive := 0
	total := 0
	leaderScope = ""
	leaderMove = 0

	marketRegistry.quotes.Range(func(key, value any) bool {
		scope, ok := key.(string)

		if !ok || scope == "" {
			return true
		}

		quoteState, ok := value.(*quoteState)

		if !ok {
			return true
		}

		total++

		if quoteState.changePct > 0 {
			positive++
			changes = append(changes, quoteState.changePct)
		}

		if quoteState.move >= leaderMove {
			leaderMove = quoteState.move
			leaderScope = scope
		}

		return true
	})

	if total == 0 {
		return 0, 0, "", 0
	}

	breadth = float64(positive) / float64(total)
	surgeThreshold = medianPositive(changes)

	if surgeThreshold <= 0 {
		surgeThreshold = breadth
	}

	if surgeThreshold > 1 {
		surgeThreshold = 1
	}

	return breadth, surgeThreshold, leaderScope, leaderMove
}
