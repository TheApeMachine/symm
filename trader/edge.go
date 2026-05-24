package trader

import (
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

/*
requiredEdgeReturn estimates round-trip cost plus minimum edge for one entry size.
*/
func requiredEdgeReturn(
	quotes QuoteReader,
	market engine.MarketReader,
	symbol string,
	notionalEUR float64,
	now time.Time,
) float64 {
	fee := 2 * config.System.TakerFeePct / 100
	spread := liveSpreadCost(quotes, symbol)
	depth := depthSlippageCost(quotes, symbol, notionalEUR)
	stale := staleDataPenalty(market, symbol, now)
	minEdge := config.System.MinEdgeReturn

	if minEdge <= 0 {
		minEdge = 0
	}

	return fee + spread + depth + stale + minEdge
}

func liveSpreadCost(quotes QuoteReader, symbol string) float64 {
	if quotes == nil || symbol == "" {
		return 0
	}

	last, bid, ask, _, ok := quotes.Quote(symbol)

	if !ok || last <= 0 || bid <= 0 || ask <= 0 || ask < bid {
		return 0
	}

	return (ask - bid) / last
}

func depthSlippageCost(
	quotes QuoteReader,
	symbol string,
	notionalEUR float64,
) float64 {
	if quotes == nil || notionalEUR <= 0 {
		return 0
	}

	fillReader, ok := quotes.(FillReader)

	if !ok {
		return 0
	}

	last, bid, ask, _, quoteOK := quotes.Quote(symbol)

	if !quoteOK || last <= 0 {
		return 0
	}

	bids, asks, depthOK := fillReader.BookDepth(symbol)

	if !depthOK {
		return 0
	}

	mid := config.System.SlippageFill(
		last, bid, ask, "buy", 0, notionalEUR, bids, asks,
	)
	reference := config.System.SlippagePrice(last, bid, ask, "buy", 0)

	if mid <= 0 || reference <= 0 || mid <= reference {
		return 0
	}

	return (mid - reference) / reference
}

func staleDataPenalty(
	market engine.MarketReader,
	symbol string,
	now time.Time,
) float64 {
	if market == nil || symbol == "" {
		return 0
	}

	ttl := config.System.SnapshotFreshnessTTL

	if ttl <= 0 {
		return 0
	}

	snapshot := market.ReadFresh(symbol, now, ttl)

	if snapshot.LastOK && snapshot.BatchOK && snapshot.SpreadOK {
		return 0
	}

	penalty := config.System.MinEdgeReturn

	if penalty <= 0 {
		penalty = 0.0005
	}

	return penalty
}

func slotNotionalEstimate(cashEUR float64) float64 {
	slotPct := config.System.MaxSlotPct

	if slotPct <= 0 {
		slotPct = 5
	}

	if cashEUR <= 0 {
		cashEUR = config.System.WalletEUR
	}

	notional := cashEUR * slotPct / 100

	if notional > cashEUR {
		return cashEUR
	}

	return notional
}
