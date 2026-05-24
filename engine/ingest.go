package engine

import (
	"time"

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
	LastAt          time.Time
	LastOK          bool
	VolumeBase      float64
	VolumeOK        bool
	BatchVolume     float64
	TradesAt        time.Time
	BatchOK         bool
	BuyPressure     float64
	PressureOK      bool
	SpreadBPS       float64
	BookAt          time.Time
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

			if timestamp, ok := ingest.ticker.Timestamp(symbol); ok {
				snapshot.LastAt = parseExchangeTime(timestamp)
			}
		}

		snapshot.VolumeBase, snapshot.VolumeOK = ingest.ticker.VolumeBase(symbol)
	}

	if ingest.trades != nil {
		snapshot.BatchVolume, snapshot.BatchOK = ingest.trades.BatchVolume(symbol)
		snapshot.BuyPressure, snapshot.PressureOK = ingest.trades.BuyPressure(symbol)

		if updated, ok := ingest.trades.UpdatedAt(symbol); ok {
			snapshot.TradesAt = updated
		}
	}

	if ingest.book != nil {
		snapshot.SpreadBPS, snapshot.SpreadOK = ingest.book.SpreadBPS(symbol)
		snapshot.Imbalance, snapshot.ImbalanceOK = ingest.book.Imbalance(symbol)
		snapshot.Density, snapshot.DensityOK = ingest.book.Density(symbol)

		if updated, ok := ingest.book.UpdatedAt(symbol); ok {
			snapshot.BookAt = updated
		}
	}

	return snapshot
}

/*
Ticker exposes the shared ticker observer for UI snapshots.
*/
func (ingest *Ingest) Ticker() *kticker.Ticker {
	return ingest.ticker
}

func parseExchangeTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}

	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}

	return parsed
}
