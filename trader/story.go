package trader

import (
	"github.com/theapemachine/symm/engine"
)

/*
recordStory folds one measurement's self-classified verdict into the symbol's
running MarketStory -- the per-signal snapshot the decision tree reasons over.
Each source keeps only its latest categorical reading, so the story is always
"what does each perspective say about this symbol right now". Measurements with
no category (a reading that did not commit to a labelled state) are ignored by
MarketStory.Observe, so they cannot erase a prior real verdict.

The calibrated confidence and the measurement's direction (Dump = bearish, all
else = bullish) travel with the reading so the decision tree can require bullish
confluence for a long entry and read decay/reversal on the way out.
*/
func (crypto *Crypto) recordStory(measurement engine.Measurement) {
	if measurement.Category == engine.CategoryNone {
		return
	}

	if len(measurement.Pairs) == 0 {
		return
	}

	symbol := measurement.Pairs[0].Wsname

	if symbol == "" {
		return
	}

	crypto.storiesMu.Lock()
	defer crypto.storiesMu.Unlock()

	story := crypto.stories[symbol]

	if story == nil {
		fresh := engine.NewMarketStory()
		story = &fresh
		crypto.stories[symbol] = story
	}

	story.Observe(measurement.Source, engine.CategoryReading{
		Category:   measurement.Category,
		Confidence: measurement.Confidence,
		Direction:  measurementDirection(measurement),
	})
}

/*
recordExhaustCategory injects an exhaust verdict into a symbol's story. Exhaust
is the one perspective that does not ride the measurements stream -- it feeds the
exits broadcast as an engine.Exit -- so its decay categories (mechanical
collapse, thermal exhaustion, ...) are mapped in here from the exit reason and
folded into the same story the held-position re-read consults. bookflow's
Source is used so the reading lands under the exhaust signal's slot.
*/
func (crypto *Crypto) recordExhaustCategory(symbol string, category engine.Category, urgency float64) {
	if category == engine.CategoryNone || symbol == "" {
		return
	}

	crypto.storiesMu.Lock()
	defer crypto.storiesMu.Unlock()

	story := crypto.stories[symbol]

	if story == nil {
		fresh := engine.NewMarketStory()
		story = &fresh
		crypto.stories[symbol] = story
	}

	story.Observe("exhaust", engine.CategoryReading{
		Category:   category,
		Confidence: urgency,
		Direction:  0,
	})
}

/*
storyVerdict snapshots a symbol's current MarketStory and asks the decision tree
for its verdict. holding selects the entry vs exit lens. An unknown symbol
yields an empty story, which Decide resolves to ActionNone (flat) or ActionHold
(holding) -- i.e. "no information, do nothing / let the runway manage it".
*/
func (crypto *Crypto) storyVerdict(symbol string, holding bool) engine.Verdict {
	crypto.storiesMu.Lock()
	story := crypto.stories[symbol]
	crypto.storiesMu.Unlock()

	if story == nil {
		return engine.Decide(engine.NewMarketStory(), holding)
	}

	return engine.Decide(*story, holding)
}

/*
routeExit sends an exit signal to the right authority. A thesis-decay pulse from
exhaust is folded into the symbol's story and handed to the decision tree (Stage
5), which decides whether the thesis has actually decayed enough to close -- the
tree is the authority, so e.g. a mere spread-widen (Fragile Expansion) holds. A
hard price-level trigger (stop hit, profit target, runway expiry) is a mechanical
event rather than a thesis decision, so it closes the position directly.
*/
func (crypto *Crypto) routeExit(exitSignal engine.Exit) error {
	category := exhaustReasonCategory(exitSignal.Reason)

	if category == engine.CategoryNone {
		return crypto.handleExit(exitSignal)
	}

	crypto.recordExhaustCategory(exitSignal.Symbol, category, exitSignal.Urgency)

	if !crypto.holdsSymbol(crypto.wallet, exitSignal.Symbol) {
		return nil
	}

	verdict := crypto.storyVerdict(exitSignal.Symbol, true)

	if verdict.Action != engine.ActionExit {
		return nil
	}

	return crypto.handleExit(engine.Exit{
		Symbol:  exitSignal.Symbol,
		Urgency: verdict.Urgency,
		Reason:  engine.ExitReasonEdgeFaded,
	})
}

/*
exhaustReasonCategory maps an exhaust exit reason onto its decay category so an
exhaust pulse can be folded into the decision story.
*/
func exhaustReasonCategory(reason string) engine.Category {
	switch reason {
	case "book_thinning":
		return engine.CatMechanicalCollapse
	case "imbalance_flip":
		return engine.CatActiveReversal
	case "pressure_fade":
		return engine.CatThermalExhaustion
	case "spread_widen":
		return engine.CatFragileExpansion
	default:
		return engine.CategoryNone
	}
}
