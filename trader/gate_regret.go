package trader

import (
	"time"

	"github.com/theapemachine/symm/config"
	decision "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/trader/economics"
)

/*
trackGateRejectRegret records counterfactual forward labels for blocked entries.
*/
func (crypto *Crypto) trackGateRejectRegret(symbol string, verdict decision.EntryVerdict) {
	if !config.System.Headless {
		return
	}

	quote := crypto.quotes.snapshot(symbol, 0)

	if quote.Last <= 0 {
		return
	}

	spreadBPS := crypto.quotes.spreadBPS(symbol)
	roundTrip := economics.RoundTripCostPctForFees(
		crypto.entryFeePct(symbol),
		crypto.exitFeePct(symbol),
		spreadBPS,
	)
	notional := crypto.scopedRuntime().Risk.MinCostEUR

	if notional <= 0 {
		return
	}

	crypto.economics.TrackGateReject(
		symbol,
		verdict.Name,
		traceWhy(verdict),
		quote.Last,
		roundTrip,
		notional,
		time.Now(),
	)
}

/*
FlushGateRejectRegret resolves pending gate rejects at replay end.
*/
func (crypto *Crypto) FlushGateRejectRegret() {
	crypto.economics.FlushGateReject(crypto.quotes.lastPrices())
}

/*
FlushOpenPositionPerformance records replay-end marks as exit labels for
headless evaluation. Wallet equity already marks open inventory to these prices;
this keeps tune eligibility on the same economic surface without touching live
positions.
*/
func (crypto *Crypto) FlushOpenPositionPerformance() {
	if crypto == nil || crypto.wallet == nil || crypto.quotes == nil || crypto.economics == nil {
		return
	}

	snapshot := crypto.wallet.Snapshot()

	if snapshot == nil {
		return
	}

	lastPrices := crypto.quotes.lastPrices()
	now := time.Now()

	for base, qty := range snapshot.Inventory {
		if qty <= 0 {
			continue
		}

		symbol := base + "/" + snapshot.Currency
		lastPrice := lastPrices[symbol]

		if lastPrice <= 0 {
			continue
		}

		binding, ok := snapshot.Positions[base]

		if !ok {
			continue
		}

		entryPrice := snapshot.AvgEntry[base]

		if entryPrice <= 0 {
			continue
		}

		entryFeePct := binding.EntryFeePct
		exitFeePct := binding.ExitFeePct

		if entryFeePct <= 0 {
			entryFeePct = binding.TakerFeePct
		}

		if exitFeePct <= 0 {
			exitFeePct = binding.TakerFeePct
		}

		label := economics.ExitLabelWithFees(
			symbol,
			binding.Playbook,
			entryPrice,
			lastPrice,
			entryFeePct,
			exitFeePct,
			crypto.quotes.spreadBPS(symbol),
			binding.PredictedAt,
			now,
		)
		crypto.economics.RecordExit(label)
	}
}

/*
GateRegretSummary returns aggregated counterfactual gate-reject outcomes.
*/
func (crypto *Crypto) GateRegretSummary() economics.RegretSummary {
	return crypto.economics.GateRegretSummary()
}

/*
PerformanceSummary returns closed-trade economics for this Crypto desk instance.
*/
func (crypto *Crypto) PerformanceSummary() economics.PerformanceSummary {
	return crypto.economics.PerformanceSummary()
}
