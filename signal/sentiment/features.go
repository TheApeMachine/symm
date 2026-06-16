package sentiment

import (
	"context"
	"io"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken/market"
	feed "github.com/theapemachine/symm/signal"
	marketsection "github.com/theapemachine/symm/market"
)

type ScopeSnapshot struct {
	Price    float64
	Volume   float64
	Spread   float64
	Change   float64
	Move     float64
	Elapsed  float64
	Observed time.Time
}

type Features struct {
	ctx          context.Context
	cancel       context.CancelFunc
	scope        string
	crossSection *marketsection.CrossSection
	trade        *feed.Trade
	ticker       *feed.Ticker
	book         *feed.Book
}

func NewFeatures(
	ctx context.Context,
	crossSection *marketsection.CrossSection,
	trade *feed.Trade,
	ticker *feed.Ticker,
	book *feed.Book,
) *Features {
	ctx, cancel := context.WithCancel(ctx)

	return &Features{
		ctx:          ctx,
		cancel:       cancel,
		crossSection: crossSection,
		trade:        trade,
		ticker:       ticker,
		book:         book,
	}
}

func (features *Features) Snapshot() ScopeSnapshot {
	tickerSnap := features.ticker.Snapshot(features.scope)

	if tickerSnap.Last > 0 {
		change := tickerSnap.ChangePct
		move := change

		if change == 0 && tickerSnap.Change != 0 && tickerSnap.Last > 0 {
			change = tickerSnap.Change / tickerSnap.Last
			move = change
		}

		return ScopeSnapshot{
			Price:    tickerSnap.Last,
			Volume:   tickerSnap.Volume,
			Spread:   tickerSnap.Ask - tickerSnap.Bid,
			Change:   change,
			Move:     move,
			Elapsed:  tickerSnap.Elapsed,
			Observed: tickerSnap.Observed,
		}
	}

	window, ok := features.trade.Window(features.scope)

	if ok {
		volume := 0.0

		features.trade.Scan(features.scope, func(update *market.TradeUpdate) {
			if update != nil {
				volume += update.Price * update.Qty
			}
		})

		change, move, changeOK := resolvedChange(window.Prices)

		if changeOK && volume > 0 {
			row, rowErr := market.SymbolRowFromPrices(
				features.scope,
				window.Prices,
				volume,
				1,
				window.Latest.Timestamp,
			)

			if rowErr == nil {
				_ = features.crossSection.Observe(row)
			}

			spread, spreadOK := feed.TouchSpread(window.Prices)

			if spreadOK {
				return ScopeSnapshot{
					Price:    window.Latest.Price,
					Volume:   volume,
					Spread:   spread,
					Change:   change,
					Move:     move,
					Elapsed:  window.Elapsed,
					Observed: window.Latest.Timestamp,
				}
			}
		}
	}

	bookWindow, bookOK := features.book.Window(features.scope)

	if !bookOK {
		return ScopeSnapshot{}
	}

	change, move, changeOK := resolvedChange(bookWindow.Prices)

	if !changeOK {
		return ScopeSnapshot{}
	}

	volume := 0.0

	features.book.Scan(features.scope, func(update *market.BookUpdate) {
		if update == nil {
			return
		}

		for _, bid := range update.Bids {
			volume += bid.Qty
		}

		for _, ask := range update.Asks {
			volume += ask.Qty
		}
	})

	price := bookWindow.Prices[len(bookWindow.Prices)-1]
	spread := bookWindow.Spreads[len(bookWindow.Spreads)-1]

	row, rowErr := market.NewSymbolRow(
		features.scope,
		price,
		change,
		volume,
		1,
		bookWindow.Latest.Timestamp,
	)

	if rowErr == nil {
		_ = features.crossSection.Observe(row)
	}

	return ScopeSnapshot{
		Price:    price,
		Volume:   volume,
		Spread:   spread,
		Change:   change,
		Move:     move,
		Observed: bookWindow.Latest.Timestamp,
	}
}

func (features *Features) Read(p []byte) (int, error) {
	snapshot := features.Snapshot()

	if snapshot.Price <= 0 {
		return 0, io.EOF
	}

	at := snapshot.Observed

	if at.IsZero() {
		at = time.Now()
	}

	breadth := features.crossSection.Breadth(at)
	surgeThreshold := features.crossSection.MajorityThreshold(at)
	features.crossSection.RecordBreadth(breadth)
	leader := features.crossSection.IsLeader(features.scope, snapshot.Change, at)

	leaderFlag := 0.0

	if leader {
		leaderFlag = 1
	}

	artifact := datura.Acquire("conviction-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(features.scope)
	artifact.WithPayload(feed.EncodePayload(
		breadth,
		snapshot.Change,
		surgeThreshold,
		leaderFlag,
		snapshot.Move,
	))

	return artifact.Read(p)
}

func (features *Features) Close() error {
	features.cancel()

	return nil
}

func resolvedChange(prices []float64) (change, move float64, ok bool) {
	if len(prices) < 2 {
		return 0, 0, false
	}

	first := prices[0]
	last := prices[len(prices)-1]

	if first <= 0 || last <= 0 {
		return 0, 0, false
	}

	change = (last - first) / first
	move = change

	return change, move, true
}
