package derivatives

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

/*
Signal converts real-time Kraken Futures streams (tickers, trades, and
order books) into multi-dimensional derivatives measurements via nomagique.
*/
type Signal struct {
	ctx            context.Context
	cancel         context.CancelFunc
	err            error
	thesis         *types.Thesis
	oi             *nomagique.Number[string]
	oiAcceleration *nomagique.Number[string]
	basis          *nomagique.Number[string]
	basisVelocity  *nomagique.Number[string]
	indexBasis     *nomagique.Number[string]
	flow           *nomagique.Number[string]
	liquidations   *nomagique.Number[string]
	tickerPrice    *nomagique.Number[string]
	tradePrice     *nomagique.Number[string]
	work           *transport.Consumer[*types.Symbol]
	pool           *types.SymbolPool
}

/*
NewSignal constructs a new Derivatives Signal.
*/
func NewSignal(ctx context.Context, thesis *types.Thesis) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:            ctx,
		cancel:         cancel,
		thesis:         thesis,
		oi:             nomagique.NewNumber[string](equation.RelativeChange(nomagique.SampleValue)),
		oiAcceleration: nomagique.NewNumber[string](oiAccelerationPipeline()),
		basis:          nomagique.NewNumber[string](basisPipeline()),
		basisVelocity:  nomagique.NewNumber[string](basisVelocityPipeline()),
		indexBasis:     nomagique.NewNumber[string](indexBasisPipeline()),
		flow:           nomagique.NewNumber[string](flowPipeline()),
		liquidations:   nomagique.NewNumber[string](liqPipeline()),
		tickerPrice:    nomagique.NewNumber[string](equation.RelativeChange(nomagique.SampleValue)),
		tradePrice:     nomagique.NewNumber[string](equation.RelativeChange(nomagique.SampleValue)),
		pool:           types.NewSymbolPool(types.ShardWorkers()),
	}

	signal.work = transport.NewConsumer[*types.Symbol](signal.Name(), signal.consume)
	thesis.Work(types.SourceDerivatives).Register(signal.work)

	return signal
}

func (signal *Signal) Name() string           { return string(types.SourceDerivatives) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceDerivatives }

func (signal *Signal) consume() {
	go func() {
		defer func() {
			if err := signal.pool.Error(); err != nil {
				signal.err = err
			}

			signal.thesis.Fail(signal.err)
		}()

		for symbol := range signal.thesis.Work(types.SourceDerivatives).Drain(signal.work, nil) {
			select {
			case <-signal.ctx.Done():
				signal.pool.CaptureError(signal.ctx.Err())
				return
			default:
			}

			if symbol == nil {
				continue
			}

			symbolName := symbol.Symbol

			signal.pool.Submit(symbolName, func() {
				if err := signal.consumeSymbol(symbol); err != nil {
					signal.pool.CaptureError(errnie.Error(errnie.Err(
						errnie.Validation,
						"derivatives: condition "+symbolName,
						err,
					)))
				}
			})
		}
	}()
}

func (signal *Signal) consumeSymbol(symbol *types.Symbol) error {
	updated := false
	latestTime := time.Time{}
	data := DerivativesData{}

	for ticker := range symbol.MarketFuturesTickers(
		symbol.FuturesTickerConsumers[types.FuturesTickerConsumerDerivatives],
	) {
		updated = true
		latestTime = ticker.Timestamp

		if err := signal.consumeTicker(symbol, ticker, &data); err != nil {
			return err
		}
	}

	for trade := range symbol.MarketFuturesTrades(
		symbol.FuturesTradeConsumers[types.FuturesTradeConsumerDerivatives],
	) {
		updated = true

		if trade.Timestamp.After(latestTime) {
			latestTime = trade.Timestamp
		}

		if err := signal.consumeTrade(symbol, trade, &data); err != nil {
			return err
		}
	}

	for book := range symbol.MarketFuturesBooks(
		symbol.FuturesBookConsumers[types.FuturesBookConsumerDerivatives],
	) {
		updated = true

		if book.Timestamp.After(latestTime) {
			latestTime = book.Timestamp
		}
	}

	if !updated {
		return nil
	}

	measurement, err := BuildMeasurement(signal.Name(), symbol.Symbol, latestTime, data)

	if err != nil {
		return err
	}

	symbol.AppendMeasurement(measurement)
	return nil
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	if signal.pool != nil {
		signal.pool.Close()
	}

	return nil
}
