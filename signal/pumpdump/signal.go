package pumpdump

import (
	"context"
	"math"
	"slices"
	"sort"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Signal owns pump-cycle measurements derived from executed trade lift and the
reconstructed book's midpoint and spread. It reads each market
fact from its authoritative stream without treating them as independent
corroborating signals.
*/
type Signal struct {
	*types.Actor
	thesis     *types.Thesis
	ctx        context.Context
	cancel     context.CancelFunc
	ignition   *equation.Ignition
	volume     *sync.Map
	orderBooks *sync.Map
	increments *sync.Map
	ui         chan []byte
}

/*
NewSignal creates an empty per-symbol pump state whose baseline capacity is the
same explicit retention bound used by the production market feed.
*/
func NewSignal(
	ctx context.Context,
	ui chan []byte,
	baselineCapacity int,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:        ctx,
		cancel:     cancel,
		ignition:   equation.NewIgnition(baselineCapacity),
		volume:     &sync.Map{},
		orderBooks: &sync.Map{},
		increments: &sync.Map{},
		ui:         ui,
	}

	signal.Actor = types.NewActor(ctx, map[string]types.Handler{
		"ticker": {Topic: "thesis", Fn: signal.onTicker},
		"book":   {Topic: "thesis", Fn: signal.onBook},
		"trade":  {Topic: "thesis", Fn: signal.onTrade},
	})

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourcePumpDump)
}

/*
Initialize wires ticker, book, and trade ingress from Live.
*/
func (signal *Signal) Initialize(live *types.Actor, thesis *types.Thesis) {
	signal.thesis = thesis
	signal.Actor.Initialize(
		types.Topic{Name: "ticker", Actor: live},
		types.Topic{Name: "book", Actor: live},
		types.Topic{Name: "trade", Actor: live},
	)
}

func (signal *Signal) onTicker(message any) any {
	rows := message.(*kraken.Ticker).Data
	measurements, err := signal.Calculate(rows, nil, nil)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if len(measurements) == 0 {
		return nil
	}

	signal.thesis.Publish(types.SourcePumpDump, measurements)

	return signal.thesis
}

func (signal *Signal) onBook(message any) any {
	rows := message.(*kraken.Book).Data
	measurements, err := signal.Calculate(nil, nil, rows)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if len(measurements) == 0 {
		return nil
	}

	signal.thesis.Publish(types.SourcePumpDump, measurements)

	return signal.thesis
}

func (signal *Signal) onTrade(message any) any {
	rows := message.(*kraken.Trade).Data
	measurements, err := signal.Calculate(nil, rows, nil)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if len(measurements) == 0 {
		return nil
	}

	signal.thesis.Publish(types.SourcePumpDump, measurements)

	return signal.thesis
}

func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) ([]*types.Measurement, error) {
	if len(tickers) == 0 {
		if err := signal.ingest(trades, books); err != nil {
			return nil, err
		}

		return nil, nil
	}

	tickers = slices.Clone(tickers)
	trades = slices.Clone(trades)
	books = slices.Clone(books)
	sort.SliceStable(tickers, func(left, right int) bool {
		return tickers[left].Timestamp.Before(tickers[right].Timestamp)
	})
	sort.SliceStable(trades, func(left, right int) bool {
		return trades[left].Timestamp.Before(trades[right].Timestamp)
	})
	sort.SliceStable(books, func(left, right int) bool {
		return books[left].Timestamp.Before(books[right].Timestamp)
	})

	out := make([]*types.Measurement, 0, len(tickers))
	tradeIndex := 0
	bookIndex := 0

	for _, row := range tickers {
		for tradeIndex < len(trades) &&
			!trades[tradeIndex].Timestamp.After(row.Timestamp) {
			if err := signal.ingest(trades[tradeIndex:tradeIndex+1], nil); err != nil {
				return nil, err
			}

			tradeIndex++
		}

		for bookIndex < len(books) &&
			!books[bookIndex].Timestamp.After(row.Timestamp) {
			if err := signal.ingest(nil, books[bookIndex:bookIndex+1]); err != nil {
				return nil, err
			}

			bookIndex++
		}

		measurements, err := signal.measure(row)

		if err != nil {
			return nil, err
		}

		out = append(out, measurements...)
	}

	if err := signal.ingest(trades[tradeIndex:], books[bookIndex:]); err != nil {
		return nil, err
	}

	types.WireMeasurements(out, signal.ui)

	return out, nil
}

/*
measure derives one ticker observation from the causally preceding tape state.
*/
func (signal *Signal) measure(row kraken.TickerData) ([]*types.Measurement, error) {
	if row.Symbol == "" || row.Last == nil || row.Last.Sign() <= 0 {
		return nil, nil
	}

	bookFound, ok := signal.orderBooks.Load(row.Symbol)

	if !ok {
		return nil, nil
	}

	book := bookFound.(*flow.Book)
	found, ok := signal.volume.Load(row.Symbol)

	if !ok {
		return nil, nil
	}

	volume := found.(float64)

	if book == nil || volume <= 0 {
		return nil, nil
	}

	mid := book.Mid()
	incrementFound, ok := signal.increments.Load(row.Symbol)

	if !ok {
		return nil, nil
	}

	increment := incrementFound.(float64)

	if mid <= 0 || increment <= 0 {
		return nil, nil
	}

	spread := math.Round(book.Spread()/increment) * increment

	if spread <= 0 {
		return nil, nil
	}

	bid := mid - spread/2
	ask := mid + spread/2

	output, ready, maturity, err := signal.ignition.Measure(equation.IgnitionInput{
		Symbol: row.Symbol,
		Volume: volume,
		Last:   mid,
		Bid:    bid,
		Ask:    ask,
		At:     row.Timestamp,
	})

	if err != nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)
	}

	return ignitionMeasurements(
		row.Symbol,
		row.Timestamp,
		output,
		maturity,
		ready,
		bid,
		ask,
	)
}

/*
ingest accumulates executed quantity and applies incremental book updates before
the next ticker price closes the observation. Ticker-reported volume is not
used as a substitute for trades when the subscribed trade stream is absent.
*/
func (signal *Signal) ingest(
	trades []kraken.TradeData,
	books []kraken.BookData,
) error {
	for _, trade := range trades {
		if trade.Symbol == "" || trade.Price.Sign() <= 0 || trade.Qty <= 0 {
			return errnie.Err(
				errnie.Validation,
				"pumpdump: valid executed trade required",
				nil,
			)
		}

		found, ok := signal.volume.Load(trade.Symbol)
		volume := 0.0

		if ok {
			volume = found.(float64)
		}

		signal.volume.Store(trade.Symbol, volume+trade.Qty)
	}

	if len(books) == 0 {
		return nil
	}

	groups := types.ChunkRowsBySymbol(books, func(row kraken.BookData) string {
		return row.Symbol
	})

	return types.RunSymbolGroupsParallel(groups, func(index int, rows []kraken.BookData) error {
		for _, row := range rows {
			if err := signal.ingestBook(row); err != nil {
				return err
			}
		}

		return nil
	})
}

/*
ingestBook applies one incremental or snapshot book row to the symbol-local
order book state pump measurements consume.
*/
func (signal *Signal) ingestBook(row kraken.BookData) error {
	if row.Symbol == "" || row.PriceIncrement == nil || row.PriceIncrement.Sign() <= 0 {
		return errnie.Err(
			errnie.Validation,
			"pumpdump: symbol and price increment required for book update",
			nil,
		)
	}

	bookFound, ok := signal.orderBooks.Load(row.Symbol)
	var book *flow.Book

	if ok {
		book = bookFound.(*flow.Book)
	}

	signal.increments.Store(row.Symbol, row.PriceIncrement.Float64())

	if book == nil || row.Type == "snapshot" {
		book = flow.NewBook()
		signal.orderBooks.Store(row.Symbol, book)
	}

	bids, asks, err := kraken.BookLevels(row)

	if err != nil {
		return errnie.Err(errnie.Validation, "pumpdump: decode book levels", err)
	}

	if err := book.Configure(flow.BookInput{
		Symbol:   row.Symbol,
		TickSize: row.PriceIncrement.Float64(),
	}); err != nil {
		return errnie.Err(errnie.Validation, "pumpdump: configure book", err)
	}

	if _, err := book.ApplyLevels(bids, flow.SideBid); err != nil {
		return errnie.Err(errnie.Validation, "pumpdump: apply bid levels", err)
	}

	if _, err := book.ApplyLevels(asks, flow.SideAsk); err != nil {
		return errnie.Err(errnie.Validation, "pumpdump: apply ask levels", err)
	}

	return nil
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
