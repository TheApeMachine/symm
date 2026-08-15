package trader

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"sync/atomic"
	"time"

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
	status             atomic.Value
	ctx                context.Context
	cancel             context.CancelFunc
	api                *websocket.API
	ui                 chan []byte
	thesis             *types.Thesis
	recorder           *audit.Recorder
	analyzer           *logic.Analyzer
	planner            *strategy.Planner
	desk               *broker.Desk
	bookUpdates        <-chan string
	level3Events       <-chan kraken.Level3Data
	subscriptions      map[string]*types.Subscription[any]
	measurements       *Measurements
	clocks             clockBank
	startedAt          time.Time
	diagnosticInterval time.Duration
	tickers            atomic.Uint64
	trades             atomic.Uint64
	level3             atomic.Uint64
	busy               atomic.Int32
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
		api:          api,
		ui:           ui,
		thesis:       thesis,
		recorder:     recorder,
		analyzer:     analyzer,
		planner:      planner,
		desk:         desk,
		bookUpdates:  api.BookUpdates(),
		level3Events: api.Level3Events(),
		subscriptions: map[string]*types.Subscription[any]{
			"ticker": api.Subscribe(
				"ticker", types.NewSubscription[any](),
			),
			"trade": api.Subscribe(
				"trade", types.NewSubscription[any](),
			),
		},
		measurements: NewMeasurements(ctx, api, desk.Instrument(), ui),
	}

	crypto.status.Store(types.READY)
	crypto.bindDiagnostics()
	crypto.run()

	return crypto, nil
}

func (crypto *Crypto) Status() types.Status {
	return crypto.status.Load().(types.Status)
}

/*
Sync waits until every market frame already delivered has been measured,
analyzed, planned, and the manifold has finished the relaxation those frames
queued. Replay uses this so the next captured arrival cannot overtake the
decision that belongs to the current one.
*/
func (crypto *Crypto) Sync(ctx context.Context, _ time.Time) error {
	if crypto == nil {
		return fmt.Errorf("crypto: trader required")
	}

	for {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}

		if crypto.idle() {
			return nil
		}

		runtime.Gosched()
	}
}

func (crypto *Crypto) idle() bool {
	if crypto.busy.Load() != 0 {
		return false
	}

	if crypto.queued() > 0 {
		return false
	}

	if crypto.analyzer != nil && crypto.analyzer.Settling() {
		return false
	}

	if crypto.desk != nil && crypto.desk.Queued() > 0 {
		return false
	}

	return crypto.busy.Load() == 0
}

func (crypto *Crypto) queued() int {
	queued := 0

	if crypto.subscriptions != nil {
		if ticker := crypto.subscriptions["ticker"]; ticker != nil {
			queued += len(ticker.Channel)
		}

		if trade := crypto.subscriptions["trade"]; trade != nil {
			queued += len(trade.Channel)
		}
	}

	if crypto.level3Events != nil {
		queued += len(crypto.level3Events)
	}

	return queued
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
			case level3 := <-crypto.level3Events:
				crypto.onLevel3(level3)
			}
		}
	}()
}

func (crypto *Crypto) Update(receivers []types.SourceType, symbols ...string) {
	started := time.Now()

	if crypto.measurements != nil {
		if !crypto.measurements.dispatchedAt.IsZero() {
			crypto.clocks.observeHop(
				"crypto",
				"measurements",
				started.Sub(crypto.measurements.dispatchedAt),
			)
		}

		crypto.measurements.dispatchedAt = started
		ready, err := crypto.measurements.Generate(crypto.thesis, receivers, symbols...)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"crypto: measurements failed",
				err,
			))
			return
		}

		crypto.clocks.observe("measurements", time.Since(started))

		if !ready {
			return
		}
	}

	analyzedAt := time.Now()

	if crypto.analyzer != nil {
		crypto.clocks.observeHop(
			"measurements",
			"category",
			analyzedAt.Sub(started),
		)

		if err := crypto.analyzer.Process(crypto.thesis); err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"crypto: analyzer failed",
				err,
			))
		}
	}

	plannedAt := time.Now()

	if crypto.planner != nil {
		crypto.clocks.observeHop(
			"category",
			"planner",
			plannedAt.Sub(analyzedAt),
		)

		if err := crypto.planner.Update(crypto.thesis); err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"crypto: planner failed",
				err,
			))
		}
	}

	crypto.clocks.observe("crypto", time.Since(started))
}

func (crypto *Crypto) onTicker(data any) {
	crypto.busy.Add(1)
	defer crypto.busy.Add(-1)

	typedTickers, ok := data.(*kraken.Ticker)

	if !ok || typedTickers == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: unexpected ticker payload type",
			nil,
		))

		return
	}

	pricedAt := time.Now()
	dirty := make(map[string]struct{}, len(typedTickers.Data))

	for _, ticker := range typedTickers.Data {
		if ticker.Symbol == "" {
			continue
		}

		if crypto.desk != nil && crypto.desk.Price() != nil {
			row := ticker
			crypto.desk.Price().Update(&row)
		}

		found, _ := crypto.thesis.Symbols.LoadOrStore(ticker.Symbol, types.NewSymbol(
			ticker.Symbol, crypto.ui,
		))

		symbol := found.(*types.Symbol)
		symbol.AppendTickerTo(ticker, types.TickerReceivers)
		dirty[ticker.Symbol] = struct{}{}
		crypto.advanceThesisAt(ticker.Timestamp)
	}

	crypto.tickers.Add(uint64(len(typedTickers.Data)))
	crypto.clocks.observe("price", time.Since(pricedAt))
	crypto.clocks.observeHop("price", "crypto", time.Since(pricedAt))

	if len(dirty) > 0 {
		crypto.Update(types.TickerReceivers, sortedSymbolSet(dirty)...)
	}
}

func (crypto *Crypto) onTrade(data any) {
	crypto.busy.Add(1)
	defer crypto.busy.Add(-1)

	typedTrades, ok := data.(*kraken.Trade)

	if !ok || typedTrades == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: unexpected trades payload type",
			nil,
		))

		return
	}

	dirty := make(map[string]struct{}, len(typedTrades.Data))

	for _, trade := range typedTrades.Data {
		if trade.Symbol == "" {
			continue
		}

		found, _ := crypto.thesis.Symbols.LoadOrStore(trade.Symbol, types.NewSymbol(
			trade.Symbol, crypto.ui,
		))

		symbol := found.(*types.Symbol)
		symbol.AppendTradeTo(trade, types.TradeReceivers)
		dirty[trade.Symbol] = struct{}{}
		crypto.advanceThesisAt(trade.Timestamp)
	}

	crypto.trades.Add(uint64(len(typedTrades.Data)))

	if len(dirty) > 0 {
		crypto.Update(types.TradeReceivers, sortedSymbolSet(dirty)...)
	}
}

func (crypto *Crypto) onLevel3(level3 kraken.Level3Data) {
	crypto.busy.Add(1)
	defer crypto.busy.Add(-1)

	if level3.Symbol == "" {
		return
	}

	crypto.thesis.Symbol(level3.Symbol).AppendLevel3(level3)
	crypto.advanceThesisAt(level3EventTime(level3))
	crypto.level3.Add(1)
	crypto.Update(types.AcceptedBookReceivers, level3.Symbol)
}

func (crypto *Crypto) advanceThesisAt(at time.Time) {
	if crypto == nil || crypto.thesis == nil || at.IsZero() {
		return
	}

	at = at.UTC()

	if at.After(crypto.thesis.At) {
		crypto.thesis.At = at
	}
}

func level3EventTime(level3 kraken.Level3Data) time.Time {
	at := level3.Timestamp

	for _, orders := range [][]kraken.Level3Order{level3.Bids, level3.Asks} {
		for _, order := range orders {
			if order.Timestamp.After(at) {
				at = order.Timestamp
			}
		}
	}

	return at
}

func sortedSymbolSet(symbols map[string]struct{}) []string {
	ordered := make([]string, 0, len(symbols))

	for symbol := range symbols {
		ordered = append(ordered, symbol)
	}

	sort.Strings(ordered)
	return ordered
}

func (crypto *Crypto) Close() error {
	crypto.cancel()

	if crypto.measurements != nil {
		return crypto.measurements.Close()
	}

	return nil
}
