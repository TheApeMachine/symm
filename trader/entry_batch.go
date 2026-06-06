package trader

import (
	"slices"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

/*
entryBatch collects concurrent entry verdicts, ranks them by conviction once the
hold-and-compare window closes, and only then deploys capital to the strongest
signals. Exits and protective triggers bypass the batch.
*/
type entryBatch struct {
	candidates []reasoning.Action
	deadline   time.Time
}

type preemptPlan struct {
	entry  reasoning.Action
	victim string
}

/*
entryBatchWindow defaults to market.subscribe_pace — the same cadence the live
feed already uses — so the hold delay matches the system's existing temporal grain.
*/
func entryBatchWindow() time.Duration {
	if window := viper.GetDuration("trading.entry.batch_window"); window > 0 {
		return window
	}

	if pace := viper.GetDuration("market.subscribe_pace"); pace > 0 {
		return pace
	}

	return 50 * time.Millisecond
}

func entryPreemptionEnabled() bool {
	if viper.IsSet("trading.entry.preemption_enabled") {
		return viper.GetBool("trading.entry.preemption_enabled")
	}

	return true
}

func actionConviction(action reasoning.Action) float64 {
	return action.SNR * action.Confidence
}

func (crypto *Crypto) routeAction(action reasoning.Action) {
	if reasoning.IsEntryAction(action.Type) {
		crypto.queueEntry(action)
		return
	}

	crypto.submit(action)
}

func (crypto *Crypto) queueEntry(action reasoning.Action) {
	window := entryBatchWindow()
	now := time.Now()

	if len(crypto.entryBatch.candidates) == 0 {
		crypto.entryBatch.deadline = now.Add(window)
	}

	replaced := false

	for index, existing := range crypto.entryBatch.candidates {
		if existing.Symbol != action.Symbol {
			continue
		}

		if actionConviction(action) >= actionConviction(existing) {
			crypto.entryBatch.candidates[index] = action
		}

		replaced = true

		break
	}

	if !replaced {
		crypto.entryBatch.candidates = append(crypto.entryBatch.candidates, action)
	}
}

func (crypto *Crypto) entryBatchDue(now time.Time) bool {
	if len(crypto.entryBatch.candidates) == 0 {
		return false
	}

	return !now.Before(crypto.entryBatch.deadline)
}

func (crypto *Crypto) flushEntryBatch() {
	if len(crypto.entryBatch.candidates) == 0 {
		return
	}

	for _, action := range rankEntryCandidates(crypto.entryBatch.candidates) {
		crypto.deployEntry(action)
	}

	crypto.entryBatch.candidates = crypto.entryBatch.candidates[:0]
	crypto.entryBatch.deadline = time.Time{}
}

func rankEntryCandidates(candidates []reasoning.Action) []reasoning.Action {
	ranked := append([]reasoning.Action(nil), candidates...)
	slices.SortFunc(ranked, func(left, right reasoning.Action) int {
		leftScore := actionConviction(left)
		rightScore := actionConviction(right)

		switch {
		case leftScore > rightScore:
			return -1
		case leftScore < rightScore:
			return 1
		default:
			return 0
		}
	})

	return ranked
}

func (crypto *Crypto) deployEntry(action reasoning.Action) {
	quantity, err := crypto.sizeEntry(action)

	if err != nil {
		crypto.publishDecision(action, "rejected", err.Error())
		return
	}

	if quantity > 0 {
		action.Quantity = quantity
		crypto.submit(action)
		return
	}

	if !entryPreemptionEnabled() {
		crypto.publishDecision(action, "rejected", "insufficient funds")
		return
	}

	victim, victimScore, ok := crypto.weakestHeldPosition()

	if !ok || actionConviction(action) <= victimScore {
		crypto.publishDecision(action, "rejected", "insufficient funds")
		return
	}

	crypto.preemptPlan = &preemptPlan{entry: action, victim: victim}
	crypto.submitPreemptExit(victim)
}

func (crypto *Crypto) weakestHeldPosition() (symbol string, score float64, ok bool) {
	ok = false
	minScore := 0.0

	for heldSymbol := range crypto.inventory {
		if crypto.inventory[heldSymbol] <= 0 {
			continue
		}

		heldScore := crypto.entryConviction[heldSymbol]

		if !ok || heldScore < minScore {
			symbol = heldSymbol
			minScore = heldScore
			ok = true
		}
	}

	for heldSymbol := range crypto.shortInventory {
		if crypto.shortInventory[heldSymbol] <= 0 {
			continue
		}

		heldScore := crypto.entryConviction[heldSymbol]

		if !ok || heldScore < minScore {
			symbol = heldSymbol
			minScore = heldScore
			ok = true
		}
	}

	return symbol, minScore, ok
}

func (crypto *Crypto) submitPreemptExit(symbol string) {
	exit := reasoning.Action{
		Type:   reasoning.ActionSettlePosition,
		Symbol: symbol,
	}

	if crypto.shortInventory[symbol] > 0 {
		exit.Side = trading.Buy
	} else {
		exit.Side = trading.Sell
	}

	crypto.submit(exit)
}

func (crypto *Crypto) tryFinishPreemption(closedSymbol string) {
	if crypto.preemptPlan == nil || crypto.preemptPlan.victim != closedSymbol {
		return
	}

	entry := crypto.preemptPlan.entry
	crypto.preemptPlan = nil

	crypto.deployEntry(entry)
}

func (crypto *Crypto) recordEntryConviction(symbol string, action reasoning.Action) {
	if actionConviction(action) <= 0 {
		return
	}

	crypto.entryConviction[symbol] = actionConviction(action)
}

func (crypto *Crypto) clearEntryConviction(symbol string) {
	delete(crypto.entryConviction, symbol)
}
