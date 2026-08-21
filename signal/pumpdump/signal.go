package pumpdump

import (
	"context"
	"fmt"
	"time"

	book "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
Signal conditions PumpDump's three authoritative streams through one keyed
Nomagique composition: trades advance Ignition, tickers advance Anchor, and
accepted Level 3 events read the already-committed resident book for Ladder.
*/
type Signal struct {
	ctx                  context.Context
	cancel               context.CancelFunc
	err                  error
	thesis               *types.Thesis
	books                websocket.BookSource
	number               *nomagique.Number[string]
	work                 *transport.Consumer[*types.Symbol]
	capacity             float64
	ladderHalflife       float64
	fastHalflife         float64
	slowHalflife         float64
	dispersionHalflife   float64
}

func NewSignal(
	ctx context.Context,
	thesis *types.Thesis,
	books websocket.BookSource,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	config := system.Cfg.PumpDump
	signal := &Signal{
		ctx:                ctx,
		cancel:             cancel,
		thesis:             thesis,
		books:              books,
		number:             nomagique.NewNumber[string](algo.PumpDump()),
		capacity:           float64(config.Capacity),
		ladderHalflife:     config.Halflife,
		fastHalflife:       config.FastHalflife,
		slowHalflife:       config.SlowHalflife,
		dispersionHalflife: config.DispersionHalflife,
	}

	signal.work = transport.NewConsumer[*types.Symbol](signal.Name(), signal.consume)
	thesis.Work(types.SourcePumpDump).Register(signal.work)

	return signal
}

func (signal *Signal) Name() string {
	return string(types.SourcePumpDump)
}

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Type() types.SourceType {
	return types.SourcePumpDump
}

func (signal *Signal) consume() {
	go func() {
		defer func() {
			signal.thesis.Fail(signal.err)
		}()

		for symbol := range signal.thesis.Work(types.SourcePumpDump).Drain(signal.work, nil) {
			select {
			case <-signal.ctx.Done():
				signal.err = signal.ctx.Err()
				return
			default:
			}

			if symbol == nil {
				continue
			}

			if err := signal.consumeSymbol(symbol); err != nil {
				signal.err = errnie.Error(errnie.Err(
					errnie.Validation,
					"pumpdump: condition "+symbol.Symbol,
					err,
				))
				return
			}
		}
	}()
}

func (signal *Signal) consumeSymbol(symbol *types.Symbol) error {
	for level3 := range symbol.MarketLevel3(
		symbol.Level3Consumers[types.Level3ConsumerPumpDump],
	) {
		input, err := signal.level3Frame(symbol.Symbol, level3)

		if err != nil {
			return err
		}

		if err = signal.step(symbol, level3.Timestamp, input); err != nil {
			return err
		}
	}

	for ticker := range symbol.MarketTickers(
		symbol.TickerConsumers[types.TickerConsumerPumpDump],
	) {
		if err := signal.step(symbol, ticker.Timestamp, signal.tickerFrame(ticker)); err != nil {
			return err
		}
	}

	for trade := range symbol.MarketTrades(
		symbol.TradeConsumers[types.TradeConsumerPumpDump],
	) {
		if err := signal.step(symbol, trade.Timestamp, signal.tradeFrame(trade)); err != nil {
			return err
		}
	}

	return nil
}

func (signal *Signal) step(
	symbol *types.Symbol,
	at time.Time,
	input nomagique.Frame,
) error {
	output, err := signal.number.Step(symbol.Symbol, input)

	if err != nil {
		return err
	}

	symbol.AppendMeasurement(signal.measurement(at, output))

	return nil
}

func (signal *Signal) tradeFrame(trade kraken.TradeData) nomagique.Frame {
	input := signal.eventFrame(algo.PumpDumpEventTrade, trade.Timestamp)
	input.Put(algo.SymbolVolume, trade.Qty)
	input.Put(algo.SymbolLast, trade.Price.Float64())
	input.Put(algo.SymbolTradeQuantity, trade.Qty)
	input.Put(algo.SymbolTradePrice, trade.Price.Float64())
	input.Put(algo.SymbolCapacity, signal.capacity)

	return input
}

func (signal *Signal) tickerFrame(ticker kraken.TickerData) nomagique.Frame {
	input := signal.eventFrame(algo.PumpDumpEventTicker, ticker.Timestamp)

	if ticker.Last != nil {
		input.Put(algo.SymbolLast, ticker.Last.Float64())
	}

	input.Put(algo.SymbolVWAP, ticker.Vwap)
	input.Put(algo.SymbolReportedVolume, ticker.Volume)
	input.Put(algo.SymbolAnchorFastHalflife, signal.fastHalflife)
	input.Put(algo.SymbolAnchorSlowHalflife, signal.slowHalflife)
	input.Put(algo.SymbolAnchorDispersionHalflife, signal.dispersionHalflife)

	return input
}

func (signal *Signal) level3Frame(
	symbol string,
	level3 kraken.Level3Data,
) (nomagique.Frame, error) {
	if signal.books == nil {
		return nomagique.Frame{}, fmt.Errorf(
			"pumpdump: authoritative Level 3 book source is required",
		)
	}

	input := signal.eventFrame(algo.PumpDumpEventLevel3, level3.Timestamp)
	found := false
	var err error
	signal.books.Book(symbol, func(resident *book.Book) {
		if resident == nil {
			return
		}

		found = true
		err = signal.readBook(resident, &input)
	})

	if err != nil {
		return nomagique.Frame{}, err
	}

	if !found {
		return nomagique.Frame{}, fmt.Errorf(
			"pumpdump: committed Level 3 book missing for %s",
			symbol,
		)
	}

	input.Put(algo.SymbolLadderHalflife, signal.ladderHalflife)

	return input, nil
}

func (signal *Signal) readBook(
	resident *book.Book,
	input *nomagique.Frame,
) error {
	bestBid := resident.BestBid()
	bestAsk := resident.BestAsk()

	if bestBid == nil || bestAsk == nil ||
		bestBid.Price == nil || bestAsk.Price == nil {
		return fmt.Errorf("pumpdump: resident Level 3 book has no executable touch")
	}

	input.Put(algo.SymbolBid, bestBid.Price.Float64())
	input.Put(algo.SymbolAsk, bestAsk.Price.Float64())
	input.Put(algo.SymbolLadderBidDepth, sideDepth(bestBid, false))
	input.Put(algo.SymbolLadderAskDepth, sideDepth(bestAsk, true))

	return nil
}

func sideDepth(touch *book.Level, higher bool) float64 {
	depth := 0.0

	for level := touch; level != nil; {
		if level.Quantity != nil {
			depth += level.Quantity.Float64()
		}

		if higher {
			level = level.Higher
			continue
		}

		level = level.Lower
	}

	return depth
}

func (signal *Signal) eventFrame(event int, at time.Time) nomagique.Frame {
	input := nomagique.Frame{}
	input.Put(algo.SymbolPumpDumpEvent, float64(event))
	input.Put(algo.SymbolUnixSec, float64(at.Unix()))
	input.Put(algo.SymbolUnixNsec, float64(at.Nanosecond()))

	return input
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
