package pumpdump

import (
	"context"
	"math"
	"slices"
	"sort"

	"github.com/theapemachine/datura"
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
	ctx        context.Context
	cancel     context.CancelFunc
	ignition   *equation.Ignition
	volume     map[string]float64
	books      map[string]*flow.Book
	increments map[string]float64
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

	return &Signal{
		ctx:        ctx,
		cancel:     cancel,
		ignition:   equation.NewIgnition(baselineCapacity),
		volume:     make(map[string]float64),
		books:      make(map[string]*flow.Book),
		increments: make(map[string]float64),
		ui:         ui,
	}
}

/*
Publish sends one small datura frame to the UI the moment this signal has
measured its evidence, mirroring broker.Balance.Publish.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	select {
	case signal.ui <- datura.Map[any]{
		"measurements": types.ForPublish(measurements),
	}.Marshal():
	default:
	}
}

/*
Interest requires executed trades for lift, the reconstructed book for price
displacement and spread, and ticker observations to close each market sample.
*/
func (signal *Signal) Interest() types.StreamInterest {
	return types.StreamAll
}

/*
Measure returns typed measurements for the cut, or an error when the
cut cannot be measured honestly.
*/
func (signal *Signal) Measure(thesis *types.Thesis) ([]*types.Measurement, error) {
	return signal.Calculate(thesis.Market())
}

/*
Calculate converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Calculate(
	frame *types.MarketFrame,
) ([]*types.Measurement, error) {
	if types.FrameInterest(frame)&types.StreamTicker == 0 {
		if err := signal.ingest(frame.Trades, frame.Books); err != nil {
			return nil, err
		}

		return nil, nil
	}

	tickers := slices.Clone(frame.Tickers)
	trades := slices.Clone(frame.Trades)
	books := slices.Clone(frame.Books)
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

	return out, nil
}

/*
measure derives one ticker observation from the causally preceding tape state.
*/
func (signal *Signal) measure(row kraken.TickerData) ([]*types.Measurement, error) {
	if row.Symbol == "" || row.Last == nil || row.Last.Sign() <= 0 {
		return nil, nil
	}

	book := signal.books[row.Symbol]
	volume := signal.volume[row.Symbol]

	if book == nil || volume <= 0 {
		return nil, nil
	}

	mid := book.Mid()
	increment := signal.increments[row.Symbol]

	if mid <= 0 || increment <= 0 {
		return nil, errnie.Err(
			errnie.Validation,
			"pumpdump: positive midpoint and price increment required",
			nil,
		)
	}

	spread := math.Round(book.Spread()/increment) * increment

	if spread <= 0 {
		return nil, errnie.Err(
			errnie.Validation,
			"pumpdump: positive executable spread required",
			nil,
		)
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

		signal.volume[trade.Symbol] += trade.Qty
	}

	for _, row := range books {
		if row.Symbol == "" || row.PriceIncrement == nil || row.PriceIncrement.Sign() <= 0 {
			return errnie.Err(
				errnie.Validation,
				"pumpdump: symbol and price increment required for book update",
				nil,
			)
		}

		book := signal.books[row.Symbol]
		signal.increments[row.Symbol] = row.PriceIncrement.Float64()

		if book == nil || row.Type == "snapshot" {
			book = flow.NewBook()
			signal.books[row.Symbol] = book
		}

		bids, asks, err := types.BookLevels(row)

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
