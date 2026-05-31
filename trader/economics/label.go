package economics

import (
	"time"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/market"
)

/*
Label is one execution-economics observation (entry, forward, or exit).
*/
type Label struct {
	Event                string
	Symbol               string
	Playbook             string
	Side                 string
	FillPrice            float64
	DecisionPrice        float64
	FeePct               float64
	EntryFeePct          float64
	ExitFeePct           float64
	RoundTripCostPct     float64
	SpreadBPS            float64
	QuoteAgeMS           int64
	ProjectedSlippageBPS float64
	DepthCoverage        float64
	ForwardReturn        float64
	NetReturn            float64
	HeldMS               int64
	At                   time.Time
}

/*
EntryLabel builds the label recorded at a paper entry fill.
*/
func EntryLabel(
	symbol, playbook, side string,
	quote broker.Quote,
	notional, fillPrice, feePct, spreadBPS float64,
	decisionAt time.Time,
) Label {
	return EntryLabelWithFees(
		symbol, playbook, side, quote, notional, fillPrice, feePct, feePct, spreadBPS, decisionAt,
	)
}

/*
EntryLabelWithFees builds an entry label with explicit entry and exit fee rates.
*/
func EntryLabelWithFees(
	symbol, playbook, side string,
	quote broker.Quote,
	notional, fillPrice, entryFeePct, exitFeePct, spreadBPS float64,
	decisionAt time.Time,
) Label {
	decisionPrice := quote.Last
	depthLevels := quote.AskDepth

	if side == "sell" {
		depthLevels = quote.BidDepth
	}

	roundTrip := RoundTripCostPctForFees(entryFeePct, exitFeePct, spreadBPS)
	quoteAgeMS := int64(0)

	if !quote.At.IsZero() {
		quoteAgeMS = decisionAt.Sub(quote.At).Milliseconds()
	}

	projectedSlippageBPS := 0.0

	if decisionPrice > 0 {
		projectedSlippageBPS = (fillPrice - decisionPrice) / decisionPrice * 10000

		if side == "sell" {
			projectedSlippageBPS = (decisionPrice - fillPrice) / decisionPrice * 10000
		}
	}

	return Label{
		Event:                "entry",
		Symbol:               symbol,
		Playbook:             playbook,
		Side:                 side,
		FillPrice:            fillPrice,
		DecisionPrice:        decisionPrice,
		FeePct:               entryFeePct,
		EntryFeePct:          entryFeePct,
		ExitFeePct:           exitFeePct,
		RoundTripCostPct:     roundTrip,
		SpreadBPS:            spreadBPS,
		QuoteAgeMS:           quoteAgeMS,
		ProjectedSlippageBPS: projectedSlippageBPS,
		DepthCoverage:        market.DepthVisibleNotionalFraction(depthLevels, notional),
		At:                   decisionAt,
	}
}

/*
ForwardLabel labels a matured forward return after entry.
*/
func ForwardLabel(
	symbol, playbook string,
	entryPrice, lastPrice, roundTripCost float64,
	openedAt time.Time,
) Label {
	forward := 0.0

	if entryPrice > 0 && lastPrice > 0 {
		forward = (lastPrice - entryPrice) / entryPrice
	}

	return Label{
		Event:            "forward",
		Symbol:           symbol,
		Playbook:         playbook,
		Side:             "buy",
		DecisionPrice:    entryPrice,
		FillPrice:        lastPrice,
		RoundTripCostPct: roundTripCost,
		ForwardReturn:    forward,
		NetReturn:        NetForwardReturn(forward, roundTripCost),
		At:               openedAt,
	}
}

/*
ExitLabel labels a closed round-trip.
*/
func ExitLabel(
	symbol, playbook string,
	entryPrice, exitPrice, feePct, spreadBPS float64,
	openedAt time.Time,
	closedAt time.Time,
) Label {
	return ExitLabelWithFees(
		symbol, playbook, entryPrice, exitPrice, feePct, feePct, spreadBPS, openedAt, closedAt,
	)
}

/*
ExitLabelWithFees labels a closed round-trip with explicit entry and exit fee rates.
*/
func ExitLabelWithFees(
	symbol, playbook string,
	entryPrice, exitPrice, entryFeePct, exitFeePct, spreadBPS float64,
	openedAt time.Time,
	closedAt time.Time,
) Label {
	roundTrip := RoundTripCostPctForFees(entryFeePct, exitFeePct, spreadBPS)
	forward := 0.0
	heldMS := int64(0)

	if entryPrice > 0 {
		forward = (exitPrice - entryPrice) / entryPrice
	}

	if !openedAt.IsZero() {
		heldMS = closedAt.Sub(openedAt).Milliseconds()
	}

	return Label{
		Event:            "exit",
		Symbol:           symbol,
		Playbook:         playbook,
		Side:             "sell",
		DecisionPrice:    entryPrice,
		FillPrice:        exitPrice,
		FeePct:           exitFeePct,
		EntryFeePct:      entryFeePct,
		ExitFeePct:       exitFeePct,
		RoundTripCostPct: roundTrip,
		SpreadBPS:        spreadBPS,
		ForwardReturn:    forward,
		NetReturn:        NetExitReturn(entryPrice, exitPrice, roundTrip),
		HeldMS:           heldMS,
		At:               closedAt,
	}
}
