package toxicity

import (
	"context"
	"math"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal tracks whether near-touch liquidity is sincere, retreating, or bluffing
from Level3 order events corroborated by the public trade tape.
*/
type Signal struct {
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	books         websocket.BookSource
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	pool          pond.Pool
	group         pond.TaskGroup
}

/*
NewSignal creates the Level3 honesty calculator against the production Kraken
API so tests can replace only its connections, never its market mechanics.
*/
func NewSignal(
	ctx context.Context,
	books websocket.BookSource,
	ui chan []byte,
	subscriptions map[string]*types.Subscription[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		status:        types.INITIALIZING,
		ctx:           ctx,
		cancel:        cancel,
		books:         books,
		ui:            ui,
		subscriptions: subscriptions,
		subscribers:   &sync.Map{},
		pool:          pond.NewPool(runtime.GOMAXPROCS(0)),
	}
	signal.group = signal.pool.NewGroup()

	signal.status = types.READY
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
	return signal.status
}

func (signal *Signal) Subscribe(
	channel string,
	subscription *types.Subscription[any],
) *types.Subscription[any] {
	return utils.Subscribe(
		signal.subscribers,
		channel,
		subscription,
	)
}

func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case message := <-signal.subscriptions["thesis"].Channel:
				if thesis, ok := message.(*types.Thesis); ok {
					measurements := signal.Measure(thesis)

					if len(measurements) > 0 {
						thesis.AppendMeasurements(measurements, true)
					}

					utils.Fanout(signal.subscribers, signal.Name(), thesis)
				}
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

	trades := thesis.MarketTrades()
	toxicity := utils.Measurements(thesis, types.SourceToxicity)
	symbols := thesis.MarketSymbols()
	sort.Strings(symbols)
	results := make([][]*types.Measurement, len(symbols))

	if signal.pool == nil {
		signal.pool = pond.NewPool(runtime.GOMAXPROCS(0))
	}

	if signal.group == nil {
		signal.group = signal.pool.NewGroup()
	}

	for index, symbol := range symbols {
		resultIndex := index

		signal.group.Submit(func() {
			current, ok := observedTouch(signal.books.Book(symbol))

			if !ok {
				return
			}

			previous, ok := latestTouch(toxicity, symbol)

			if !ok {
				results[resultIndex] = []*types.Measurement{touchMeasurement(symbol, current)}
				return
			}

			if !current.asOf.After(previous.asOf) {
				return
			}

			bracketed := bracketedTrades(
				trades,
				symbol,
				previous.asOf,
				current.asOf,
			)

			if len(bracketed) == 0 {
				results[resultIndex] = []*types.Measurement{touchMeasurement(symbol, current)}
				return
			}

			measurement := toxicityMeasurement(symbol, previous, current, bracketed)
			results[resultIndex] = []*types.Measurement{
				measurement,
				touchMeasurement(symbol, current),
			}
		})
	}

	if err := signal.group.Wait(); err != nil {
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

func completed(measurements []*types.Measurement) bool {
	for _, measurement := range measurements {
		if measurement != nil {
			return true
		}
	}

	return false
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

	if signal.pool != nil {
		signal.pool.StopAndWait()
	}

	return nil
}
