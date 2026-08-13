package trader

import (
	"context"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

/*
Crypto submits desk work from thesis messages delivered by the Actor cascade.
*/
type Crypto struct {
	statusMu      sync.RWMutex
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	pipeline      *streamPipeline
	thesis        *types.Thesis
	recorder      *audit.Recorder
	desk          *broker.Desk
	bookUpdates   <-chan string
	level3Events  <-chan kraken.Level3Data
	subscriptions map[string]*types.Subscription[any]
	sequence      uint64
	nextTick      int64
	epochs        map[string]symbolEpoch
}

type symbolEpoch struct {
	at   time.Time
	tick int64
}

/*
NewCrypto constructs Crypto; Boot Initialize attaches planner and desk.
*/
func NewCrypto(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	recorder *audit.Recorder,
	desk *broker.Desk,
	analyzer *logic.Analyzer,
	planner *strategy.Planner,
	thesis *types.Thesis,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:          ctx,
		cancel:       cancel,
		status:       types.INITIALIZING,
		api:          api,
		thesis:       thesis,
		recorder:     recorder,
		desk:         desk,
		bookUpdates:  api.BookUpdates(),
		level3Events: api.Level3Events(),
		epochs:       make(map[string]symbolEpoch),
		subscriptions: map[string]*types.Subscription[any]{
			"ticker": api.Subscribe(
				"ticker", types.NewSubscription[any](),
			),
			"trade": api.Subscribe(
				"trade", types.NewSubscription[any](),
			),
		},
	}
	pipeline, err := newStreamPipeline(
		ctx,
		api,
		desk.Instrument(),
		ui,
		thesis,
		analyzer,
		planner,
		crypto.fail,
	)

	if err != nil {
		cancel()
		return nil, err
	}

	crypto.pipeline = pipeline
	crypto.setStatus(types.READY)

	crypto.run()

	return crypto, nil
}

func (crypto *Crypto) Status() types.Status {
	crypto.statusMu.RLock()
	defer crypto.statusMu.RUnlock()

	return crypto.status
}

func (crypto *Crypto) setStatus(status types.Status) {
	crypto.statusMu.Lock()
	crypto.status = status
	crypto.statusMu.Unlock()
}

func (crypto *Crypto) fail(err error) {
	if err == nil {
		return
	}

	crypto.setStatus(types.ERROR)
	errnie.Error(err)
}

func (crypto *Crypto) run() {
	go func() {
		for {
			select {
			case <-crypto.ctx.Done():
				return
			case ticker := <-crypto.subscriptions["ticker"].Channel:
				crypto.onTicker(ticker)
			case trade := <-crypto.subscriptions["trade"].Channel:
				crypto.onTrade(trade)
			case symbol := <-crypto.bookUpdates:
				crypto.onBookUpdate(symbol)
			case level3 := <-crypto.level3Events:
				crypto.onLevel3(level3)
			}
		}
	}()
}

/*
onBookUpdate turns the authoritative manager's keyed update into the same
thesis semaphore fanout used by ticker and trade inputs.
*/
func (crypto *Crypto) onBookUpdate(symbol string) {
	if symbol == "" {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: book update requires a symbol",
			nil,
		))
		return
	}

	if _, observed := crypto.thesis.Symbols.Load(symbol); !observed {
		return
	}

	at := time.Time{}
	crypto.api.Book(symbol, func(managed *book.Book) {
		if managed == nil {
			return
		}

		for bid := managed.Bids.High; bid != nil; bid = bid.Lower {
			if bid.Timestamp.After(at) {
				at = bid.Timestamp
			}
		}

		for ask := managed.Asks.Low; ask != nil; ask = ask.Higher {
			if ask.Timestamp.After(at) {
				at = ask.Timestamp
			}
		}
	})

	crypto.sequence++
	errnie.Error(crypto.pipeline.Dispatch(marketEvent{
		sequence: crypto.sequence,
		tick:     crypto.eventTick(symbol),
		kind:     marketEventBook,
		symbol:   crypto.thesis.Symbol(symbol),
		at:       at,
	}))
}

func (crypto *Crypto) onTicker(data any) {
	typedTickers, ok := data.(*kraken.Ticker)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: unexpected ticker payload type",
			nil,
		))

		return
	}

	for _, ticker := range typedTickers.Data {
		crypto.sequence++
		crypto.nextTick++
		crypto.epochs[ticker.Symbol] = symbolEpoch{
			at: ticker.Timestamp, tick: crypto.nextTick,
		}
		crypto.desk.Price().Update(&ticker)
		errnie.Error(crypto.pipeline.Dispatch(marketEvent{
			sequence: crypto.sequence,
			tick:     crypto.nextTick,
			kind:     marketEventTicker,
			symbol:   crypto.thesis.Symbol(ticker.Symbol),
			at:       ticker.Timestamp,
			ticker:   ticker,
		}))
	}
}

func (crypto *Crypto) onTrade(data any) {
	typedTrades, ok := data.(*kraken.Trade)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: unexpected trades payload type",
			nil,
		))

		return
	}

	for _, trade := range typedTrades.Data {
		crypto.sequence++
		errnie.Error(crypto.pipeline.Dispatch(marketEvent{
			sequence: crypto.sequence,
			tick:     crypto.eventTick(trade.Symbol),
			kind:     marketEventTrade,
			symbol:   crypto.thesis.Symbol(trade.Symbol),
			at:       trade.Timestamp,
			trade:    trade,
		}))
	}
}

func (crypto *Crypto) onLevel3(level3 kraken.Level3Data) {
	if level3.Symbol == "" {
		crypto.fail(errnie.Err(
			errnie.Validation,
			"crypto: Level 3 event requires a symbol",
			nil,
		))
		return
	}

	at := level3.Timestamp

	for _, orders := range [][]kraken.Level3Order{level3.Bids, level3.Asks} {
		for _, order := range orders {
			if order.Timestamp.After(at) {
				at = order.Timestamp
			}
		}
	}

	crypto.sequence++
	errnie.Error(crypto.pipeline.Dispatch(marketEvent{
		sequence: crypto.sequence,
		tick:     crypto.eventTick(level3.Symbol),
		kind:     marketEventLevel3,
		symbol:   crypto.thesis.Symbol(level3.Symbol),
		at:       at,
		level3:   level3,
	}))
}

func (crypto *Crypto) eventTick(symbol string) int64 {
	epoch, found := crypto.epochs[symbol]

	if !found {
		return 0
	}

	return epoch.tick
}

/*
Sync waits for every analytical feed family to commit through an exchange
timestamp. Production ingress never calls it; deterministic replays use it as
an explicit boundary instead of assuming asynchronous work finished instantly.
*/
func (crypto *Crypto) Sync(ctx context.Context, at time.Time) error {
	return crypto.pipeline.Wait(ctx, at)
}

func (crypto *Crypto) Close() error {
	crypto.cancel()

	if crypto.pipeline == nil {
		return nil
	}

	return crypto.pipeline.Close()
}
