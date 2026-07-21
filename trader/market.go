package trader

import (
	"context"
	"slices"
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
	ctx          context.Context
	cancel       context.CancelFunc
	api          *websocket.API
	instrument   *broker.Instrument
	crossSection *types.CrossSection
	tickers      []kraken.TickerData
	trades       []kraken.TradeData
	books        []kraken.BookData
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
	ctx, cancel := context.WithCancel(ctx)

	market := &Market{
		ctx:          ctx,
		cancel:       cancel,
		api:          api,
		instrument:   instrument,
		crossSection: types.NewCrossSection(),
		tickers:      make([]kraken.TickerData, 0, timelineCapacity),
		trades:       make([]kraken.TradeData, 0, timelineCapacity),
		books:        make([]kraken.BookData, 0, timelineCapacity),
	}

	if api != nil {
		api.On("ticker", market.OnTicker)
		api.On("trade", market.OnTrade)
		api.On("book", market.OnBook)
	}

	return market, errnie.Error(errnie.Require(map[string]any{
		"market":       market,
		"ctx":          ctx,
		"cancel":       cancel,
		"crossSection": market.crossSection,
	}))
}

/*
OnTicker decodes one ticker message, retains every row once, and updates the
latest cross-sectional state under the same cut boundary.
*/
func (market *Market) OnTicker(data []byte) {
	frame := kraken.NewTicker(data)

	if kraken.Validate(frame) != nil {
		return
	}

	market.tickers = append(market.tickers, frame.Data...)
}

/*
OnTrade decodes one trade message, retains every row once, and updates the
latest cross-sectional state under the same cut boundary.
*/
func (market *Market) OnTrade(data []byte) {
	frame := kraken.NewTrade(data)

	if kraken.Validate(frame) != nil {
		return
	}

	market.trades = append(market.trades, frame.Data...)
}

/*
OnBook decodes one public book message, attaches exchange tick size centrally,
and retains the enriched rows once for every signal.
*/
func (market *Market) OnBook(data []byte) {
	frame := kraken.NewBook(data)

	if kraken.Validate(frame) != nil {
		return
	}

	market.books = append(market.books, frame.Data...)
}

/*
Close cancels ingress workers.
*/
func (market *Market) Close() error {
	market.cancel()
	market.cancel = nil
	return nil
}

/*
Cut captures each public stream at the same measurement time. When the ticker
stream advanced, the full retained quote surface is attached so quote signals
remeasure the universe. When only trades or books advanced, only tickers for
those active symbols are attached so quote signals measure what changed.
Trades and books remain unseen-event batches. A cut with no new ingress on any
stream is empty so the planner does not busy-spin.
*/
func (market *Market) Cut() (*types.MarketFrame, error) {
	tickerProgress := len(market.tickers) > 0
	tradeProgress := len(market.trades) > 0
	bookProgress := len(market.books) > 0

	if !tickerProgress && !tradeProgress && !bookProgress {
		return nil, errnie.Err(
			errnie.PreconditionFailed,
			"market: no progress",
			nil,
		)
	}

	frame := &types.MarketFrame{
		At:           time.Now().UTC(),
		Tickers:      slices.Clone(market.tickers),
		Trades:       slices.Clone(market.trades),
		Books:        slices.Clone(market.books),
		CrossSection: market.crossSection,
	}

	market.tickers = market.tickers[:0]
	market.trades = market.trades[:0]
	market.books = market.books[:0]

	if len(frame.Tickers) > 0 {
		frame.CrossSection.Measure(frame.Tickers)
	}

	return frame, nil
}
