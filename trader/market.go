package trader

import (
	"sync"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Market centrally decodes and retains public Kraken data. It is the sole owner
of ticker, trade, and book handlers and produces immutable cuts for all signals.
*/
type Market struct {
	instrument *broker.Instrument
	tickers    *sync.Map
	trades     *sync.Map
	books      *sync.Map
}

/*
NewMarket creates the central public feed and registers exactly one handler per
Kraken stream so decoding and retention are not repeated by every signal.
*/
func NewMarket(
	api *websocket.API,
	instrument *broker.Instrument,
) (*Market, error) {
	market := &Market{
		instrument: instrument,
		tickers:    new(sync.Map),
		trades:     new(sync.Map),
		books:      new(sync.Map),
	}

	if api != nil {
		api.On("ticker", market.OnTicker)
		api.On("trade", market.OnTrade)
		api.On("book", market.OnBook)
	}

	return market, nil
}

/*
OnTicker decodes one ticker message, retains every row once, and updates the
latest cross-sectional state under the same cut boundary.
*/
func (market *Market) OnTicker(data []byte) {
	if len(data) == 0 {
		return
	}

	frame := kraken.NewTicker(data)

	if len(frame.Data) == 0 {
		return
	}

	for _, row := range frame.Data {
		tickers, _ := market.tickers.LoadOrStore(row.Symbol, make([]kraken.TickerData, 0))
		tickers = append(tickers.([]kraken.TickerData), row)
		market.tickers.Store(row.Symbol, tickers)
	}
}

/*
OnTrade decodes one public trade message and retains its rows once for the next
shared market cut.
*/
func (market *Market) OnTrade(data []byte) {
	if len(data) == 0 {
		return
	}

	frame := kraken.NewTrade(data)

	if len(frame.Data) == 0 {
		return
	}

	for _, row := range frame.Data {
		trades, _ := market.trades.LoadOrStore(row.Symbol, make([]kraken.TradeData, 0))
		trades = append(trades.([]kraken.TradeData), row)
		market.trades.Store(row.Symbol, trades)
	}
}

/*
OnBook decodes one public book message, attaches exchange tick size centrally,
and retains the enriched rows once for every signal.
*/
func (market *Market) OnBook(data []byte) {
	if len(data) == 0 {
		return
	}

	frame := kraken.NewBook(data)

	if len(frame.Data) == 0 {
		return
	}

	for _, row := range frame.Data {
		books, _ := market.books.LoadOrStore(row.Symbol, make([]kraken.BookData, 0))
		books = append(books.([]kraken.BookData), row)
		market.books.Store(row.Symbol, books)
	}
}

/*
Cut freezes all three journals at one ingress boundary and returns a single
immutable state object that concurrent signal workers can safely share.
*/
func (market *Market) Cut() *types.MarketFrame {
	tickers := make([]kraken.TickerData, 0)
	trades := make([]kraken.TradeData, 0)
	books := make([]kraken.BookData, 0)

	market.tickers.Range(func(key, value interface{}) bool {
		tickers = append(tickers, value.([]kraken.TickerData)...)
		return true
	})

	market.trades.Range(func(key, value interface{}) bool {
		trades = append(trades, value.([]kraken.TradeData)...)
		return true
	})

	market.books.Range(func(key, value interface{}) bool {
		books = append(books, value.([]kraken.BookData)...)
		return true
	})

	return &types.MarketFrame{
		Tickers: tickers,
		Trades:  trades,
		Books:   books,
	}
}
