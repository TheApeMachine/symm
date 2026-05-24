package engine

import (
	kbook "github.com/theapemachine/symm/kraken/book"
	kticker "github.com/theapemachine/symm/kraken/ticker"
	"github.com/theapemachine/symm/kraken/trades"
)

/*
Ingest centralizes observer reads for one symbol per scan step.
*/
type Ingest struct {
	book   *kbook.Book
	trades *trades.Trades
	ticker *kticker.Ticker
}

/*
Snapshot holds the latest observer values for one symbol.
*/
type Snapshot struct {
	Last            float64
	VolumeBase      float64
	LastOK          bool
	VolumeOK        bool
	BatchVolume     float64
	BatchOK         bool
	BuyPressure     float64
	PressureOK      bool
	SpreadBPS       float64
	SpreadOK        bool
	Imbalance       float64
	ImbalanceOK     bool
	Density         float64
	DensityOK       bool
	ChangePct       float64
	ChangeOK        bool
}

/*
NewIngest wires shared market observers into one read facade.
*/
func NewIngest(
	book *kbook.Book,
	tradesObserver *trades.Trades,
	tickerObserver *kticker.Ticker,
) *Ingest {
	return &Ingest{
		book:   book,
		trades: tradesObserver,
		ticker: tickerObserver,
	}
}

/*
Read returns the latest book, trade, and ticker values for one symbol.
*/
func (ingest *Ingest) Read(symbol string) Snapshot {
	snapshot := Snapshot{}

	if ingest.ticker != nil {
		last, _, _, changePct, quoteOK := ingest.ticker.Quote(symbol)

		if quoteOK {
			snapshot.Last = last
			snapshot.LastOK = true
			snapshot.ChangePct = changePct
			snapshot.ChangeOK = true
		}

		snapshot.VolumeBase, snapshot.VolumeOK = ingest.ticker.VolumeBase(symbol)
	}

	if ingest.trades != nil {
		snapshot.BatchVolume, snapshot.BatchOK = ingest.trades.BatchVolume(symbol)
		snapshot.BuyPressure, snapshot.PressureOK = ingest.trades.BuyPressure(symbol)
	}

	if ingest.book != nil {
		snapshot.SpreadBPS, snapshot.SpreadOK = ingest.book.SpreadBPS(symbol)
		snapshot.Imbalance, snapshot.ImbalanceOK = ingest.book.Imbalance(symbol)
		snapshot.Density, snapshot.DensityOK = ingest.book.Density(symbol)
	}

	return snapshot
}

/*
Ticker exposes the shared ticker observer for UI snapshots.
*/
func (ingest *Ingest) Ticker() *kticker.Ticker {
	return ingest.ticker
}
