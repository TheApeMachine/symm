package toxicity

import (
	"context"
	"math"
	"sort"
	"sync/atomic"
	"time"

	spotbook "github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
	"golang.org/x/sync/errgroup"
)

/*
Signal tracks whether near-touch liquidity is sincere, retreating, or bluffing
from Level3 order events corroborated by the public trade tape.
*/
type Signal struct {
	status    atomic.Value
	ctx       context.Context
	cancel    context.CancelFunc
	books     websocket.BookSource
	ui        chan []byte
	thesis    *types.Thesis
	semaphore chan struct{}
}

/*
NewSignal creates the Level3 honesty calculator against the production Kraken
API so tests can replace only its connections, never its market mechanics.
*/
func NewSignal(
	ctx context.Context,
	books websocket.BookSource,
	ui chan []byte,
	thesis *types.Thesis,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:       ctx,
		cancel:    cancel,
		books:     books,
		ui:        ui,
		thesis:    thesis,
		semaphore: make(chan struct{}, 1),
	}

	signal.status.Store(types.INITIALIZING)
	signal.thesis.Subscribe(types.SourceToxicity, signal.semaphore)
	signal.status.Store(types.READY)
	signal.run()

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceToxicity)
}

func (signal *Signal) Status() types.Status {
	return signal.status.Load().(types.Status)
}

func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case <-signal.semaphore:
				signal.status.Store(types.BUSY)
				errnie.Error(signal.thesis.AppendMeasurements(
					types.SourceToxicity,
					signal.Measure(signal.thesis), true,
				))

				signal.status.Store(types.READY)
			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	measurements := make([]*types.Measurement, 0)
	out := make([]*types.Measurement, 0)

	if thesis == nil || signal.books == nil {
		return measurements
	}

	tickers := thesis.MarketTickers(types.SourceToxicity)
	toxicity := utils.Measurements(thesis, types.SourceToxicity)
	symbolSet := make(map[string]struct{})

	for _, ticker := range tickers {
		if ticker.Symbol != "" {
			symbolSet[ticker.Symbol] = struct{}{}
		}
	}

	thesis.Measurements.Range(func(_, value any) bool {
		rows, ok := value.([]*types.Measurement)

		if !ok {
			return true
		}

		for _, measurement := range rows {
			if measurement != nil && measurement.Symbol != "" {
				symbolSet[measurement.Symbol] = struct{}{}
			}
		}

		return true
	})

	thesis.Trades.Range(func(key, _ any) bool {
		if symbol, ok := key.(string); ok {
			symbolSet[symbol] = struct{}{}
		}
		return true
	})

	symbols := make([]string, 0, len(symbolSet))

	for symbol := range symbolSet {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)
	results := make([][]*types.Measurement, len(symbols))

	group, _ := errgroup.WithContext(signal.ctx)

	for index, symbol := range symbols {
		resultIndex := index

		group.Go(func() error {
			var current touchSnapshot
			var ok bool
			signal.books.Book(symbol, func(book *spotbook.Book) {
				current, ok = observedTouch(book)
			})

			if !ok {
				return nil
			}

			previous, ok := latestTouch(toxicity, symbol)

			if !ok {
				results[resultIndex] = []*types.Measurement{
					toxicityMeasurement(symbol, current, current, nil),
				}
				return nil
			}

			if !current.asOf.After(previous.asOf) {
				return nil
			}

			stored, _ := thesis.Trades.Load(symbol)
			trades, _ := stored.([]kraken.TradeData)

			bracketed := bracketedTrades(
				trades,
				symbol,
				previous.asOf,
				current.asOf,
			)

			if len(bracketed) == 0 {
				results[resultIndex] = []*types.Measurement{
					toxicityMeasurement(symbol, previous, current, nil),
				}
				return nil
			}

			measurement := toxicityMeasurement(symbol, previous, current, bracketed)
			results[resultIndex] = []*types.Measurement{measurement}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"toxicity: parallel measurement failed",
			err,
		))
		return measurements
	}

	for _, symbolMeasurements := range results {
		measurements = append(measurements, symbolMeasurements...)

		for _, measurement := range symbolMeasurements {
			_, complete := measurement.Metrics[types.MetricKey(
				types.MetricTradeVolume,
				types.SideNone,
			)]

			if measurement.Symbol == types.Focus() && complete {
				out = append(out, measurement)
			}
		}
	}

	if len(out) > 0 {
		utils.Publish(signal.ui, datura.NewMap(
			"measurements", out,
		))
	}

	return measurements
}

func bracketedTrades(
	trades []kraken.TradeData,
	symbol string,
	from time.Time,
	through time.Time,
) []kraken.TradeData {
	bracketed := make([]kraken.TradeData, 0)

	for _, trade := range trades {
		if trade.Symbol != symbol || !validTrade(trade) ||
			!trade.Timestamp.After(from) || trade.Timestamp.After(through) {
			continue
		}

		bracketed = append(bracketed, trade)
	}

	return bracketed
}

func validTrade(row kraken.TradeData) bool {
	price := row.Price.Float64()

	return row.Symbol != "" && !row.Timestamp.IsZero() && price > 0 && row.Qty > 0 &&
		!math.IsNaN(price) && !math.IsInf(price, 0) && !math.IsNaN(row.Qty) &&
		!math.IsInf(row.Qty, 0) && (row.Side == "buy" || row.Side == "sell")
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
