package toxicity

import (
	"context"
	"sort"
	"time"

	"github.com/theapemachine/errnie"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Signal tracks whether near-touch liquidity is sincere, retreating, or bluffing
from level3 order events corroborated by the public trade tape.
*/
type Signal struct {
	tickerIn     chan []kraken.TickerData
	bookIn       chan []kraken.BookData
	tradeIn      chan []kraken.TradeData
	ack     chan struct{}
	ctx          context.Context
	cancel       context.CancelFunc
	level3       *Level3
	priorTouch   map[string]touchSnapshot
	pendingTouch map[string]touchSnapshot
	evidence     map[string]*symbolEvidence
	increments   map[string]*decimal.Decimal
	ui           chan []byte
}

/*
NewSignal creates the Level3 honesty calculator against the production Kraken
API so tests can replace only its connections, never its market mechanics.
*/
func NewSignal(ctx context.Context, api *websocket.API, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		tickerIn:     make(chan []kraken.TickerData, 64),
		bookIn:       make(chan []kraken.BookData, 64),
		tradeIn:      make(chan []kraken.TradeData, 64),
		ack:     make(chan struct{}, 256),
		ctx:          ctx,
		cancel:       cancel,
		level3:       NewLevel3(api),
		priorTouch:   map[string]touchSnapshot{},
		pendingTouch: map[string]touchSnapshot{},
		evidence:     map[string]*symbolEvidence{},
		increments:   map[string]*decimal.Decimal{},
		ui:           ui,
	}

	return signal
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
touchSnapshot retains prior best-level quantities so toxicity can distinguish
withdrawal from execution.
*/
type touchSnapshot struct {
	bidPrice    decimal.Decimal
	askPrice    decimal.Decimal
	bidQuantity float64
	askQuantity float64
	observedAt  time.Time
}

/*
Calculate converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) ([]*types.Measurement, error) {
	signal.ensureScratch()

	if err := signal.ingestIncrements(books); err != nil {
		return nil, err
	}

	// A public trade and the book update that reflects it share one market
	// timestamp but arrive as two separate cuts. Attribution must compare each
	// trade against the touch that existed strictly before this instant, so a
	// pending touch is only promoted to the authoritative prior once the cut
	// clock advances past the moment it was observed.
	cutAt := cutTimestamp(trades, books)
	signal.promotePrior(cutAt)

	if err := signal.accumulateEvidence(trades); err != nil {
		return nil, err
	}

	if err := signal.observeBooks(books); err != nil {
		return nil, err
	}

	out := make([]*types.Measurement, 0, len(signal.evidence)*8)
	symbols := make([]string, 0, len(signal.evidence))

	for symbol := range signal.evidence {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)

	for _, symbol := range symbols {
		if err := signal.emitSymbolMeasurements(
			symbol,
			signal.evidence[symbol],
			&out,
			signal.pendingTouch,
		); err != nil {
			return nil, err
		}
	}

	return out, nil
}

/*
cutTimestamp returns the latest source event time in this cut so pending touch
snapshots are only promoted to prior once the observation clock advances.
*/
func cutTimestamp(trades []kraken.TradeData, books []kraken.BookData) time.Time {
	cutAt := time.Time{}

	for _, trade := range trades {
		if at := trade.Timestamp.UTC(); at.After(cutAt) {
			cutAt = at
		}
	}

	for _, bookRow := range books {
		if at := bookRow.Timestamp.UTC(); at.After(cutAt) {
			cutAt = at
		}
	}

	return cutAt
}

/*
promotePrior advances each symbol's authoritative prior touch to its pending
snapshot once the cut clock has moved strictly past when it was observed. A
trade and the book update at the same instant therefore both attribute against
the touch that preceded that instant rather than its own post-event book.
*/
func (signal *Signal) promotePrior(cutAt time.Time) {
	if cutAt.IsZero() {
		return
	}

	for symbol, snapshot := range signal.pendingTouch {
		if snapshot.observedAt.Before(cutAt) {
			signal.priorTouch[symbol] = snapshot
			delete(signal.pendingTouch, symbol)
		}
	}
}

/*
ensureScratch allocates reusable tick maps when tests construct Signal by hand.
*/
func (signal *Signal) ensureScratch() {
	if signal.priorTouch == nil {
		signal.priorTouch = map[string]touchSnapshot{}
	}

	if signal.pendingTouch == nil {
		signal.pendingTouch = map[string]touchSnapshot{}
	}

	if signal.evidence == nil {
		signal.evidence = map[string]*symbolEvidence{}
	}

	if signal.increments == nil {
		signal.increments = map[string]*decimal.Decimal{}
	}
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}

/*
Tickers returns the ticker ingress channel.
*/
func (signal *Signal) Tickers() chan []kraken.TickerData {
	return signal.tickerIn
}

/*
Books returns the book ingress channel.
*/
func (signal *Signal) Books() chan []kraken.BookData {
	return signal.bookIn
}

/*
Trades returns the trade ingress channel.
*/
func (signal *Signal) Trades() chan []kraken.TradeData {
	return signal.tradeIn
}


/*
Ack signals that one ingress frame finished Calculate so Crypto can barrier
before draining outs.
*/
func (signal *Signal) Ack() <-chan struct{} {
	return signal.ack
}

/*
Measure consumes ingress channels and sends measurements on out.
*/
func (signal *Signal) Measure() chan []*types.Measurement {
	out := make(chan []*types.Measurement, 64)

	go func() {
		defer close(out)

		for {
			select {
			case <-signal.ctx.Done():
				return
			case rows := <-signal.tickerIn:
				measured, err := signal.Calculate(rows, nil, nil)

				if err != nil {
					errnie.Error(err)
					signal.ack <- struct{}{}
					continue
				}

				if len(measured) == 0 {
					signal.ack <- struct{}{}
					continue
				}

				out <- measured
				signal.Publish(measured)
				signal.ack <- struct{}{}
			case rows := <-signal.bookIn:
				measured, err := signal.Calculate(nil, nil, rows)

				if err != nil {
					errnie.Error(err)
					signal.ack <- struct{}{}
					continue
				}

				if len(measured) == 0 {
					signal.ack <- struct{}{}
					continue
				}

				out <- measured
				signal.Publish(measured)
				signal.ack <- struct{}{}
			case rows := <-signal.tradeIn:
				measured, err := signal.Calculate(nil, rows, nil)

				if err != nil {
					errnie.Error(err)
					signal.ack <- struct{}{}
					continue
				}

				if len(measured) == 0 {
					signal.ack <- struct{}{}
					continue
				}

				out <- measured
				signal.Publish(measured)
				signal.ack <- struct{}{}
			}
		}
	}()

	return out
}
