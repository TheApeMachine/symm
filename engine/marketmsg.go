package engine

import (
	"time"

	"github.com/theapemachine/symm/kraken/market"
)

/*
TickUpdate is one ticker row from Kraken.
*/
type TickUpdate struct {
	Symbol     string
	Last       float64
	VolumeBase float64
	ChangePct  float64
	Timestamp  string
}

/*
TradeUpdate is one executed trade batch from Kraken.
*/
type TradeUpdate struct {
	Symbol      string
	BatchVolume float64
	BuyPressure float64
	UpdatedAt   time.Time
	Ticks       []market.TradeTick
}

/*
BookUpdate is one order-book delta from Kraken.
*/
type BookUpdate struct {
	Symbol     string
	SpreadBPS  float64
	Imbalance  float64
	Density    float64
	DepthSlope float64
	BidLevels  []market.BookLevel
	AskLevels  []market.BookLevel
	UpdatedAt  time.Time
}
