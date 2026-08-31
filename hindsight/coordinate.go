package hindsight

import (
	"math"
	"time"
)

/*
MarketCoordinate is the declared external market coordinate an Episode selector
measures (§29). Every selector MUST declare one: a +20% excursion means the
named coordinate rose 20% and nothing more. It never implies realizable
profit, executable entry, or available capital.
*/
type MarketCoordinate string

const (
	// CoordinateMidpoint is (bid+ask)/2 of the touch quoted by the venue.
	CoordinateMidpoint MarketCoordinate = "midpoint"
	// CoordinateTrade is the price of an actually printed trade.
	CoordinateTrade MarketCoordinate = "trade"
	// CoordinateLast is the venue's reported last price on a ticker update.
	CoordinateLast MarketCoordinate = "last"
	// CoordinateBid is the best bid quoted by the venue.
	CoordinateBid MarketCoordinate = "bid"
	// CoordinateAsk is the best ask quoted by the venue.
	CoordinateAsk MarketCoordinate = "ask"
)

/*
Valid reports whether the coordinate is one this package can measure. An
undeclared coordinate is a validation failure, never a silent default (§29).
*/
func (coordinate MarketCoordinate) Valid() bool {
	switch coordinate {
	case CoordinateMidpoint, CoordinateTrade, CoordinateLast, CoordinateBid, CoordinateAsk:
		return true
	default:
		return false
	}
}

/*
Observation is one external market observation as it was captured, carrying the
exact CaptureIdentity that produced it (§4, §11) alongside the venue facts the
protocol supplied. It is read from the raw capture tape, never from a witness:
Episode discovery must operate on declared external market coordinates and must
never consume SYMM's own trading outputs as a selection criterion (§27).

Each quantity carries an explicit Has* presence flag, so an absent bid stays
absent instead of collapsing into a convenient zero (§43).
*/
type Observation struct {
	Capture    CaptureIdentity `json:"capture"`
	Ordinal    uint64          `json:"ordinal"`
	Symbol     string          `json:"symbol"`
	Kind       string          `json:"kind"`
	ReceivedAt time.Time       `json:"receivedAt"`
	VenueAt    time.Time       `json:"venueAt"`

	HasBid  bool    `json:"hasBid"`
	Bid     float64 `json:"bid"`
	BidQty  float64 `json:"bidQty"`
	HasAsk  bool    `json:"hasAsk"`
	Ask     float64 `json:"ask"`
	AskQty  float64 `json:"askQty"`
	HasLast bool    `json:"hasLast"`
	Last    float64 `json:"last"`

	HasTrade   bool    `json:"hasTrade"`
	TradePrice float64 `json:"tradePrice"`
	TradeQty   float64 `json:"tradeQty"`
	TradeSide  string  `json:"tradeSide,omitempty"`
}

/*
At returns the instant this observation should be positioned at on a time axis:
the venue event time where the protocol supplied one, otherwise the receive
time. Both remain separately retained on the record (§8) — this is a display
ordinate only, and never the ordering used for causal traversal, which is
always CaptureSequence (§6, §52).
*/
func (observation Observation) At() time.Time {
	if !observation.VenueAt.IsZero() {
		return observation.VenueAt
	}

	return observation.ReceivedAt
}

/*
Value returns this observation's value of the declared coordinate, and whether
that coordinate is defined here at all. An observation lacking the quotes a
coordinate needs is undefined — not zero (§43).
*/
func (observation Observation) Value(coordinate MarketCoordinate) (float64, bool) {
	switch coordinate {
	case CoordinateMidpoint:
		if !observation.HasBid || !observation.HasAsk {
			return 0, false
		}

		midpoint := (observation.Bid + observation.Ask) / 2

		return midpoint, positive(midpoint)
	case CoordinateBid:
		return observation.Bid, observation.HasBid && positive(observation.Bid)
	case CoordinateAsk:
		return observation.Ask, observation.HasAsk && positive(observation.Ask)
	case CoordinateLast:
		return observation.Last, observation.HasLast && positive(observation.Last)
	case CoordinateTrade:
		return observation.TradePrice, observation.HasTrade && positive(observation.TradePrice)
	default:
		return 0, false
	}
}

/*
SpreadFraction returns the quoted spread as a fraction of the midpoint, defined
only when both sides of the touch were quoted. It is market geometry: it says
what the venue quoted, never what an execution would have cost (§38).
*/
func (observation Observation) SpreadFraction() (float64, bool) {
	if !observation.HasBid || !observation.HasAsk {
		return 0, false
	}

	midpoint := (observation.Bid + observation.Ask) / 2

	if !positive(midpoint) {
		return 0, false
	}

	spread := observation.Ask - observation.Bid

	if !finite(spread) {
		return 0, false
	}

	return spread / midpoint, true
}

/*
TouchDepth returns the quoted size resting at the touch (bid size + ask size),
defined only when both sides quoted a size. It is the venue's quoted size, not
an executable quantity (§34, §36).
*/
func (observation Observation) TouchDepth() (float64, bool) {
	if !observation.HasBid || !observation.HasAsk {
		return 0, false
	}

	depth := observation.BidQty + observation.AskQty

	return depth, finite(depth) && depth > 0
}

func positive(value float64) bool {
	return finite(value) && value > 0
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
