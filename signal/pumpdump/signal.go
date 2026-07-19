package pumpdump

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Signal owns pump-cycle measurements derived from executed trade lift,
reconstructed book spread, and ticker price movement. It reads each market
fact from its authoritative stream without treating them as independent
corroborating signals.
*/
type Signal struct {
	ctx      context.Context
	cancel   context.CancelFunc
	ignition *equation.Ignition
	volume   map[string]float64
	books    map[string]*flow.Book
	ui       chan []byte
}

/*
NewSignal creates an empty per-symbol pump state that will calibrate only from
the production trade, book, and ticker streams it observes.
*/
func NewSignal(ctx context.Context, api *websocket.API, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:      ctx,
		cancel:   cancel,
		ignition: equation.NewIgnition(),
		volume:   make(map[string]float64),
		books:    make(map[string]*flow.Book),
		ui:       ui,
	}
}

/*
Publish sends one small datura frame to the UI the moment this signal has
measured its evidence, mirroring broker.Balance.Publish.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	select {
	case signal.ui <- datura.Map[any]{
		"measurements": types.WireMeasurements(measurements),
	}.Marshal():
	default:
	}
}

/*
Interest requires executed trades for lift, the reconstructed book for spread,
and ticker prices for precursor and rejection.
*/
func (signal *Signal) Interest() types.StreamInterest {
	return types.StreamAll
}

/*
Measure applies the current immutable market cut and publishes no replacement
value when a required source is missing or invalid.
*/
func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	measurements, err := signal.Calculate(thesis.Market())

	if err != nil {
		errnie.Error(err)
		return nil
	}

	return measurements
}

/*
Calculate converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Calculate(
	frame *types.MarketFrame,
) ([]*types.Measurement, error) {
	if err := signal.ingest(frame.Trades, frame.Books); err != nil {
		return nil, err
	}

	if types.FrameInterest(frame)&types.StreamTicker == 0 {
		return nil, nil
	}

	rows := frame.Tickers
	out := make([]*types.Measurement, 0, len(rows))

	for _, row := range rows {
		if row.Symbol == "" || row.Last == nil || row.Last.Sign() <= 0 {
			continue
		}

		book := signal.books[row.Symbol]
		volume := signal.volume[row.Symbol]

		if book == nil || volume <= 0 {
			continue
		}

		mid := book.Mid()
		spread := book.Spread()

		if mid <= 0 || spread <= 0 {
			continue
		}

		bid := mid - spread/2
		ask := mid + spread/2

		output, ready, maturity, err := signal.ignition.Measure(equation.IgnitionInput{
			Symbol: row.Symbol,
			Volume: volume,
			Last:   row.Last.Float64(),
			Bid:    bid,
			Ask:    ask,
		})

		if err != nil {
			panic(errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			)))
		}

		measurements, err := ignitionMeasurements(
			row.Symbol, row.Timestamp, output, maturity, ready,
			bid, ask,
		)

		if err != nil {
			errnie.Error(errnie.Err(errnie.Validation, err.Error(), err))
			continue
		}

		out = append(out, measurements...)
	}

	return out, nil
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
func (signal *Signal) Close() (err error) {
	err = errnie.Error(errnie.Err(
		errnie.Internal,
		"signal: close failed",
		nil,
	))

	signal.cancel()
	return err
}
