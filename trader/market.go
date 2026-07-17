package trader

import (
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
	tickers    *types.MarketFeed[kraken.TickerData]
	trades     *types.MarketFeed[kraken.TradeData]
	books      *types.MarketFeed[kraken.BookData]
}

/*
NewMarket creates the central public feed and registers exactly one handler per
Kraken stream so decoding and retention are not repeated by every signal.
*/
func NewMarket(
	api *websocket.API,
	instrument *broker.Instrument,
) (*Market, error) {
	timelineCapacity := viper.GetInt("signals.feed_timeline_capacity")
	trackCapacity := viper.GetInt("signals.feed_track_capacity")
	market := &Market{
		instrument: instrument,
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
	}

	if _, err := market.tickers.Pending(time.Now().UTC()); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"market: invalid feed configuration",
			err,
		))
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
			return
		}
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
			return
		}
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

		if !pair.HasIncrement() {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"market: positive price increment required for "+row.Symbol,
				nil,
			))
			continue
		}

		row.PriceIncrement = pair.Increment()

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
			return
		}
	}
}

/*
Cut captures each public stream at the same measurement time, reads every
retained row without advancing any cursor early, and commits the three batches
only after the immutable shared frame is complete.
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

	tickers, err := market.tickers.Batch(at)

	if err != nil {
		return nil, errnie.Err(errnie.Internal, "market: read ticker cut", err)
	}

	trades, err := market.trades.Batch(at)

	if err != nil {
		return nil, errnie.Err(errnie.Internal, "market: read trade cut", err)
	}

	books, err := market.books.Batch(at)

	if err != nil {
		return nil, errnie.Err(errnie.Internal, "market: read book cut", err)
	}

	crossSection := types.NewCrossSection()
	crossSection.Measure(tickers.Rows)
	frame := &types.MarketFrame{
		Tickers:      tickers.Rows,
		Trades:       trades.Rows,
		Books:        books.Rows,
		CrossSection: crossSection,
	}

	if err := market.tickers.Commit(tickers); err != nil {
		return nil, errnie.Err(errnie.Internal, "market: commit ticker cut", err)
	}

	if err := market.trades.Commit(trades); err != nil {
		return nil, errnie.Err(errnie.Internal, "market: commit trade cut", err)
	}

	if err := market.books.Commit(books); err != nil {
		return nil, errnie.Err(errnie.Internal, "market: commit book cut", err)
	}

	return frame, nil
}
