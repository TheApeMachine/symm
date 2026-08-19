package trader

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
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
syncRest parks the replay sync poll between passes. Gosched spun the scheduler
through every poll (measured as the dominant profile entry during replay); the
queue depths change at tick pace, so a short rest is indistinguishable from
spinning to the pump and far cheaper to every other goroutine.
*/
const syncRest = time.Millisecond

/*
Crypto submits desk work from thesis messages delivered by the Actor cascade.
*/
type Crypto struct {
	status        atomic.Value
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	ui            chan []byte
	manifold      chan types.FluidFrame
	thesis        *types.Thesis
	recorder      *audit.Recorder
	analyzer      *logic.Analyzer
	planner       *strategy.Planner
	desk          *broker.Desk
	subscriptions map[string]*types.Subscription[any]
	measurements  *Measurements
	diagnostics   *Diagnostics
}

/*
NewCrypto constructs Crypto; Boot Initialize attaches planner and desk.
*/
func NewCrypto(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	manifold chan types.FluidFrame,
	recorder *audit.Recorder,
	desk *broker.Desk,
	analyzer *logic.Analyzer,
	planner *strategy.Planner,
	thesis *types.Thesis,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:      ctx,
		cancel:   cancel,
		api:      api,
		ui:       ui,
		manifold: manifold,
		thesis:   thesis,
		recorder: recorder,
		analyzer: analyzer,
		planner:  planner,
		desk:     desk,
		diagnostics: &Diagnostics{
			started: time.Now(),
		},
		subscriptions: map[string]*types.Subscription[any]{
			"ticker": api.Subscribe(
				"ticker", types.NewSubscription[any](),
			),
			"trade": api.Subscribe(
				"trade", types.NewSubscription[any](),
			),
			"level3": api.Subscribe(
				"level3", types.NewSubscription[any](),
			),
		},
		measurements: NewMeasurements(ctx, thesis, ui),
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
ObserveModule returns the diagnostics module clock hook so stages that live
outside the analyzer/measurements wiring (like the resonance solver) can still
report their per-step duration into the same clock bank.
*/
func (crypto *Crypto) ObserveModule() func(string, time.Duration) {
	if crypto == nil || crypto.diagnostics == nil {
		return nil
	}

	return crypto.diagnostics.applyModule
}

/*
Sync waits until every market frame already delivered has been measured,
analyzed, planned, and the manifold has finished the step those frames
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

		time.Sleep(syncRest)
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

	if level3 := crypto.subscriptions["level3"]; level3 != nil {
		queued += len(level3.Channel)
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
				crypto.onTicker(ticker.(*kraken.Ticker))
			case trade := <-crypto.subscriptions["trade"].Channel:
				crypto.onTrade(trade.(*kraken.Trade))
			case level3 := <-crypto.subscriptions["level3"].Channel:
				crypto.onLevel3(level3.(kraken.Level3Data))
			case thesis := <-theses:
				utils.Publish(crypto.ui, datura.NewMap(
					"tick", datura.NewMap("count", thesis.Tick),
				))
			}
		}
	}()
}

func (crypto *Crypto) onTicker(tickers *kraken.Ticker) {
	if tickers == nil {
		return
	}

	for _, ticker := range tickers.Data {
		if ticker.Symbol == "" {
			continue
		}

		crypto.desk.Price().Update(&ticker)
		symbol := crypto.thesis.Symbol(ticker.Symbol)
		symbol.AppendTicker(ticker)
	}
}

func (crypto *Crypto) onTrade(trades *kraken.Trade) {
	if trades == nil {
		return
	}

	for _, trade := range trades.Data {
		if trade.Symbol == "" {
			continue
		}

		symbol := crypto.thesis.Symbol(trade.Symbol)
		symbol.AppendTrade(trade)
	}
}

func (crypto *Crypto) onLevel3(level3 kraken.Level3Data) {
	if level3.Symbol == "" {
		return
	}

	crypto.thesis.Symbol(level3.Symbol).AppendLevel3(level3)
}

func (crypto *Crypto) Close() error {
	crypto.cancel()

	if crypto.measurements != nil {
		return crypto.measurements.Close()
	}

	return nil
}
