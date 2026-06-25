package manifold

import (
	"fmt"
	"strings"
	"time"
)

/*
InstrumentLane distinguishes spot, perpetual, and dated futures projections.
*/
type InstrumentLane int

const (
	InstrumentLaneSpot InstrumentLane = iota
	InstrumentLanePerpetual
	InstrumentLaneDatedFuture

	// LaneCount is the cardinality of the lane enum. The manifold's Y axis is
	// the lane projection, so gridY is exactly this — not the symbol count.
	LaneCount = iota
)

/*
InstrumentIdentity keys one instrument lane in the manifold universe.
*/
type InstrumentIdentity struct {
	Symbol string
	Base   string
	Lane   InstrumentLane
}

/*
BookLevel is one price/qty level in a book update payload.
*/
type BookLevel struct {
	Price float64 `json:"price"`
	Qty   float64 `json:"qty"`
}

/*
BookUpdate is the decoded book frame used by manifold field state.
*/
type BookUpdate struct {
	Symbol    string      `json:"symbol"`
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Bids      []BookLevel `json:"bids"`
	Asks      []BookLevel `json:"asks"`
}

/*
TickerUpdate is the decoded ticker frame used by manifold field state.
*/
type TickerUpdate struct {
	Symbol    string    `json:"symbol"`
	Last      float64   `json:"last"`
	Bid       float64   `json:"bid"`
	Ask       float64   `json:"ask"`
	BidQty    float64   `json:"bid_qty"`
	AskQty    float64   `json:"ask_qty"`
	Timestamp time.Time `json:"timestamp"`
}

/*
TradeUpdate is the decoded trade print used by manifold field state.
*/
type TradeUpdate struct {
	Symbol    string    `json:"symbol"`
	Side      string    `json:"side"`
	Price     float64   `json:"price"`
	Qty       float64   `json:"qty"`
	Timestamp time.Time `json:"timestamp"`
}

/*
SpotIdentityFromPair maps a Kraken spot pair into manifold identity.
*/
func SpotIdentityFromPair(pair string) (InstrumentIdentity, error) {
	if pair == "" {
		return InstrumentIdentity{}, fmt.Errorf("manifold: empty spot pair")
	}

	base := pair

	if parts := strings.Split(pair, "/"); len(parts) > 0 && parts[0] != "" {
		base = parts[0]
	}

	return InstrumentIdentity{
		Symbol: pair,
		Base:   base,
		Lane:   InstrumentLaneSpot,
	}, nil
}

/*
FuturesIdentityFromProduct maps a Kraken futures product id into manifold identity.
*/
func FuturesIdentityFromProduct(productID string) (InstrumentIdentity, error) {
	if productID == "" {
		return InstrumentIdentity{}, fmt.Errorf("manifold: empty futures product")
	}

	lane := InstrumentLanePerpetual
	trimmed := productID

	switch {
	case strings.HasPrefix(productID, "FI_"):
		lane = InstrumentLaneDatedFuture
		trimmed = strings.TrimPrefix(productID, "FI_")
	case strings.HasPrefix(productID, "PF_"):
		trimmed = strings.TrimPrefix(productID, "PF_")
	}

	base := trimmed

	if underscore := strings.Index(trimmed, "_"); underscore > 0 {
		base = trimmed[:underscore]
	}

	if base == "" {
		return InstrumentIdentity{}, fmt.Errorf("manifold: futures base for %q", productID)
	}

	return InstrumentIdentity{
		Symbol: productID,
		Base:   base,
		Lane:   lane,
	}, nil
}
