package toxicity

import (
	"context"
	"sort"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Signal tracks whether near-touch liquidity is sincere, retreating, or bluffing
from level3 order events corroborated by the public trade tape.
*/
type Signal struct {
	*types.Actor
	thesis       *types.Thesis
	ctx          context.Context
	cancel       context.CancelFunc
	level3       *Level3
	priorTouch   map[string]touchSnapshot
	pendingTouch map[string]touchSnapshot
	evidence     map[string]*symbolEvidence
	increments   map[string]*decimal.Decimal
	lastCutAt    time.Time
	ui           chan []byte
}

/*
NewSignal creates the Level3 honesty calculator against the production Kraken
API so tests can replace only its connections, never its market mechanics.
*/
func NewSignal(ctx context.Context, api *websocket.API, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		level3:       NewLevel3(api),
		priorTouch:   map[string]touchSnapshot{},
		pendingTouch: map[string]touchSnapshot{},
		evidence:     map[string]*symbolEvidence{},
		increments:   map[string]*decimal.Decimal{},
		ui:           ui,
	}

	signal.Actor = types.NewActor(ctx, map[string]types.Handler{
		"ticker": {
			Topic: "thesis",
			Fn:    signal.onTicker,
		},
		"book": {
			Topic: "thesis",
			Fn:    signal.onBook,
		},
		"trade": {
			Topic: "thesis",
			Fn:    signal.onTrade,
		},
	})

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceToxicity)
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

	signal.thesis.Publish(types.SourceToxicity, measurements)

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

	signal.thesis.Publish(types.SourceToxicity, measurements)

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

	signal.thesis.Publish(types.SourceToxicity, measurements)

	return signal.thesis
}

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

	if err := signal.accumulateEvidence(trades, cutAt); err != nil {
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

	types.WireMeasurements(out, signal.ui)

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
