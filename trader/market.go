package trader

import (
	"context"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
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
	api        *websocket.API
	tickers    *types.MarketFeed[kraken.TickerData]
	trades     *types.MarketFeed[kraken.TradeData]
	books      *types.MarketFeed[kraken.BookData]
	ctx        context.Context
	cancel     context.CancelFunc
	resyncIn   chan string
	dirty      chan struct{}
}

/*
NewMarket creates the central public feed and registers exactly one handler per
Kraken stream so decoding and retention are not repeated by every signal.
*/
func NewMarket(
	ctx context.Context,
	api *websocket.API,
	instrument *broker.Instrument,
) (*Market, error) {
	timelineCapacity := viper.GetInt("signals.feed_timeline_capacity")
	trackCapacity := viper.GetInt("signals.feed_track_capacity")
	ctx, cancel := context.WithCancel(ctx)

	market := &Market{
		instrument: instrument,
		api:        api,
		tickers: types.NewMarketFeed[kraken.TickerData](
			timelineCapacity,
			trackCapacity,
		),
		trades: types.NewMarketFeed[kraken.TradeData](
			timelineCapacity,
			trackCapacity,
		),
		books: types.NewMarketFeed[kraken.BookData](
			timelineCapacity,
			trackCapacity,
		),
		ctx:      ctx,
		cancel:   cancel,
		resyncIn: make(chan string, 64),
		dirty:    make(chan struct{}, 1),
	}

	if _, err := market.tickers.Pending(time.Now().UTC()); err != nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"market: invalid feed configuration",
			err,
		))
	}

	if api != nil {
		go market.resync()
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
		if err := market.tickers.Observe(
			row.Symbol,
			row.Timestamp,
			row,
		); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"market: retain ticker row for "+row.Symbol,
				err,
			))
			continue
		}
	}

	market.dirtyWake()
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
		if err := market.trades.Observe(
			row.Symbol,
			row.Timestamp,
			row,
		); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"market: retain trade row for "+row.Symbol,
				err,
			))
			continue
		}
	}

	market.dirtyWake()
}

/*
OnBook decodes one public book message, attaches exchange tick size centrally,
and retains the enriched rows once for every signal.
*/
func (market *Market) OnBook(data []byte) {
	if len(data) == 0 {
		return
	}

	if market.instrument == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"market: instrument registry required for book ingestion",
			nil,
		))
		return
	}

	frame := kraken.NewBook(data)

	if len(frame.Data) == 0 {
		return
	}

	for _, row := range frame.Data {
		pair, err := market.instrument.Pair(row.Symbol)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.NotFound,
				"market: instrument pair required for "+row.Symbol,
				err,
			))
			continue
		}

		if pair.PriceIncrement.Sign() <= 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"market: positive price increment required for "+row.Symbol,
				nil,
			))
			continue
		}

		row.PriceIncrement = &pair.PriceIncrement

		if err := market.books.Observe(
			row.Symbol,
			row.Timestamp,
			row,
		); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"market: retain book row for "+row.Symbol,
				err,
			))

			if market.api != nil {
				market.scheduleBookResync(row.Symbol)
			}

			continue
		}
	}

	market.dirtyWake()
}

/*
dirtyWake signals ingress so the tick loop can take one atomic Cut. The
single-slot channel naturally merges observations that arrive before the loop
can consume them without imposing a wall-clock delay on every market update.
*/
func (market *Market) dirtyWake() {
	if market == nil || market.dirty == nil {
		return
	}

	select {
	case market.dirty <- struct{}{}:
	default:
	}
}

/*
WaitDirty blocks until the first ingress arrives, the budget elapses, or Market
closes. It returns immediately for buffered ingress because MarketFeed.Capture
already forms the atomic cut and the dirty channel already coalesces updates
that outrun the consumer.
*/
func (market *Market) WaitDirty(budget time.Duration) {
	if market == nil {
		return
	}

	var done <-chan struct{}

	if market.ctx != nil {
		done = market.ctx.Done()
	}

	var dirty <-chan struct{}

	if market.dirty != nil {
		dirty = market.dirty
	}

	select {
	case <-done:
		return
	case <-dirty:
		return
	default:
	}

	if budget <= 0 {
		select {
		case <-done:
		case <-dirty:
		}

		return
	}

	deadline := time.NewTimer(budget)
	defer deadline.Stop()

	select {
	case <-done:
		return
	case <-deadline.C:
		return
	case <-dirty:
		return
	}
}

/*
Close cancels ingress workers.
*/
func (market *Market) Close() {
	if market == nil || market.cancel == nil {
		return
	}

	market.cancel()
	market.cancel = nil
}

/*
Cut captures each public stream at the same measurement time. When the ticker
stream advanced, the full retained quote surface is attached so quote signals
remeasure the universe. When only trades or books advanced, only tickers for
those active symbols are attached so quote signals measure what changed.
Trades and books remain unseen-event batches. A cut with no new ingress on any
stream is empty so the planner does not busy-spin.
*/
func (market *Market) Cut(at time.Time) (*types.MarketFrame, error) {
	if err := market.tickers.Capture(at); err != nil {
		return nil, errnie.Err(errnie.Internal, "market: capture ticker cut", err)
	}

	if err := market.trades.Capture(at); err != nil {
		return nil, errnie.Err(errnie.Internal, "market: capture trade cut", err)
	}

	if err := market.books.Capture(at); err != nil {
		return nil, errnie.Err(errnie.Internal, "market: capture book cut", err)
	}

	tickerProgress := market.tickers.Progress()
	tradeProgress := market.trades.Progress()
	bookProgress := market.books.Progress()

	if !tickerProgress && !tradeProgress && !bookProgress {
		return &types.MarketFrame{
			At:           at,
			CrossSection: types.NewCrossSection(),
		}, nil
	}

	var tickerRows []kraken.TickerData
	var err error

	if tickerProgress {
		tickerRows, err = market.tickers.Frame(at)

		if err != nil {
			return nil, errnie.Err(errnie.Internal, "market: read ticker frame", err)
		}
	}

	trades, err := market.trades.Batch(at)

	if err != nil {
		return nil, errnie.Err(errnie.Internal, "market: read trade cut", err)
	}

	books, err := market.books.Batch(at)

	if err != nil {
		return nil, errnie.Err(errnie.Internal, "market: read book cut", err)
	}

	if err := market.trades.Commit(trades); err != nil {
		return nil, errnie.Err(errnie.Internal, "market: commit trade cut", err)
	}

	if err := market.books.Commit(books); err != nil {
		return nil, errnie.Err(errnie.Internal, "market: commit book cut", err)
	}

	if !tickerProgress {
		tickerRows, err = market.tickers.FrameSymbols(
			at, activeSymbols(trades.Rows, books.Rows),
		)

		if err != nil {
			return nil, errnie.Err(errnie.Internal, "market: read active ticker frame", err)
		}
	}

	crossSection := types.NewCrossSection()
	crossSection.Measure(tickerRows)

	var advanced types.StreamInterest

	if tickerProgress {
		advanced |= types.StreamTicker
	}

	if tradeProgress {
		advanced |= types.StreamTrade
	}

	if bookProgress {
		advanced |= types.StreamBook
	}

	return &types.MarketFrame{
		At:           at,
		Tickers:      tickerRows,
		Trades:       trades.Rows,
		Books:        books.Rows,
		CrossSection: crossSection,
		Advanced:     advanced,
	}, nil
}

/*
activeSymbols lists the symbols whose trade or book stream advanced this cut so a
book-only or trade-only tick materializes only those tickers instead of the whole
quote universe.
*/
func activeSymbols(
	trades []kraken.TradeData,
	books []kraken.BookData,
) []string {
	needed := make(map[string]struct{}, len(trades)+len(books))

	for _, trade := range trades {
		needed[trade.Symbol] = struct{}{}
	}

	for _, book := range books {
		needed[book.Symbol] = struct{}{}
	}

	symbols := make([]string, 0, len(needed))

	for symbol := range needed {
		symbols = append(symbols, symbol)
	}

	return symbols
}
