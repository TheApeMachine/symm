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
	feePct := crypto.takerFeePct(symbol)
	roundTrip := economics.RoundTripCostPct(feePct, spreadBPS)
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
GateRegretSummary returns aggregated counterfactual gate-reject outcomes.
*/
func (crypto *Crypto) GateRegretSummary() economics.RegretSummary {
	return crypto.economics.GateRegretSummary()
}
