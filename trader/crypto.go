package trader

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Crypto submits desk work from thesis messages delivered by the Actor cascade.
*/
type Crypto struct {
	status        atomic.Value
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	ui            chan []byte
	thesis        *types.Thesis
	recorder      *audit.Recorder
	analyzer      *logic.Analyzer
	planner       *strategy.Planner
	desk          *broker.Desk
	bookUpdates   <-chan string
	level3Events  <-chan kraken.Level3Data
	subscriptions map[string]*types.Subscription[any]
	measurements  *Measurements
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
	if crypto.queued() > 0 {
		return false
	}

	return crypto.desk == nil || crypto.desk.Queued() == 0
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
	theses := crypto.measurements.Generate(
		crypto.thesis, crypto.analyzer,
	)

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
			case thesis := <-theses:
				utils.Publish(crypto.ui, datura.NewMap(
					"tick", datura.NewMap("count", thesis.Tick),
				))

				if crypto.planner != nil {
					crypto.planner.Enqueue(thesis)
				}
			}
		}
	}()
}

func (crypto *Crypto) onTicker(data any) {
	typedTickers, ok := data.(*kraken.Ticker)

	if !ok || typedTickers == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: unexpected ticker payload type",
			nil,
		))

		return
	}

	for _, ticker := range typedTickers.Data {
		if ticker.Symbol == "" {
			continue
		}

		if crypto.desk != nil && crypto.desk.Price() != nil {
			crypto.desk.Price().Update(&ticker)
		}

		symbol := crypto.thesis.Symbol(ticker.Symbol)
		symbol.AppendTicker(ticker, types.TickerReceivers)
	}
}

func (crypto *Crypto) onTrade(data any) {
	typedTrades, ok := data.(*kraken.Trade)

	if !ok || typedTrades == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: unexpected trades payload type",
			nil,
		))

		return
	}

	for _, trade := range typedTrades.Data {
		if trade.Symbol == "" {
			continue
		}

		symbol := crypto.thesis.Symbol(trade.Symbol)
		symbol.AppendTrade(trade, types.TradeReceivers)
	}
}

func (crypto *Crypto) onLevel3(level3 kraken.Level3Data) {
	if level3.Symbol == "" {
		return
	}

	crypto.thesis.Symbol(level3.Symbol).AppendLevel3(level3, types.Level3Receivers)
}

func (crypto *Crypto) Close() error {
	crypto.cancel()

	if crypto.measurements != nil {
		return crypto.measurements.Close()
	}

	return nil
}
