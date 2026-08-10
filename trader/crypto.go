package trader

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Crypto submits desk work from thesis messages delivered by the Actor cascade.
*/
type Crypto struct {
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	thesis        *types.Thesis
	semaphore     chan struct{}
	dataPath      string
	ui            chan []byte
	recorder      *audit.Recorder
	desk          *broker.Desk
	subscriptions map[string]*types.Subscription[any]
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
	thesis *types.Thesis,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:       ctx,
		cancel:    cancel,
		status:    types.READY,
		api:       api,
		thesis:    thesis,
		semaphore: make(chan struct{}, 1),
		dataPath:  utils.ResolveDataPath(),
		ui:        ui,
		recorder:  recorder,
		desk:      desk,
		subscriptions: map[string]*types.Subscription[any]{
			"ticker": api.Subscribe(
				"ticker", types.NewSubscription[any](),
			),
			"trade": api.Subscribe(
				"trade", types.NewSubscription[any](),
			),
		},
	}

	crypto.thesis.Subscribe(types.SourceTrader, crypto.semaphore)
	crypto.run()

	return crypto
}

func (crypto *Crypto) Status() types.Status {
	return crypto.status
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
			case <-crypto.semaphore:
				completed := make([]string, 0)
				crypto.thesis.Symbols.Range(func(key, _ any) bool {
					symbol, ok := key.(string)

					if !ok || !crypto.thesis.Stamped(symbol, types.SourcePlanner) {
						return true
					}

					if value, found := crypto.thesis.Decisions.Load(symbol); found {
						decision, valid := value.(*types.Decision)

						if valid {
							go func() {
								if err := crypto.desk.Execute(*decision); err != nil {
									errnie.Error(errnie.Err(
										errnie.Internal,
										"crypto: failed to execute decision round",
										err,
									))
								}
							}()
						}
					}

					completed = append(completed, symbol)
					return true
				})

				if len(completed) > 0 {
					crypto.thesis.Reset(completed...)
				}
			}
		}
	}()
}

func (crypto *Crypto) onTicker(data any) {
	crypto.thesis.Tick++

	utils.Publish(crypto.ui, datura.NewMap(
		"tick", datura.NewMap(
			"count", crypto.thesis.Tick,
		),
	))

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
		crypto.thesis.AppendTicker(ticker)
		crypto.desk.Price().Update(&ticker)
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
		crypto.thesis.AppendTrade(trade)
	}
}

func (crypto *Crypto) Close() (err error) {
	crypto.cancel()
	return nil
}
