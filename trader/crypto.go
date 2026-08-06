package trader

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
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
	dataPath      string
	ui            chan []byte
	recorder      *audit.Recorder
	planner       *strategy.Planner
	desk          *broker.Desk
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
}

/*
NewCrypto constructs Crypto; Boot Initialize attaches planner and desk.
*/
func NewCrypto(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	recorder *audit.Recorder,
	planner *strategy.Planner,
	desk *broker.Desk,
	thesis *types.Thesis,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:      ctx,
		cancel:   cancel,
		status:   types.READY,
		api:      api,
		thesis:   thesis,
		dataPath: utils.ResolveDataPath(),
		ui:       ui,
		recorder: recorder,
		planner:  planner,
		desk:     desk,
		subscriptions: map[string]*types.Subscription[any]{
			"ticker": api.Subscribe(
				"ticker", types.NewSubscription[any](),
			),
			"trade": api.Subscribe(
				"trade", types.NewSubscription[any](),
			),
			"decisions": planner.Subscribe(
				"decisions", types.NewSubscription[any](),
			),
		},
		subscribers: &sync.Map{},
	}

	crypto.run()
	return crypto
}

func (crypto *Crypto) Status() types.Status {
	return crypto.status
}

func (crypto *Crypto) Subscribe(
	key string, subscription *types.Subscription[any],
) *types.Subscription[any] {
	subscribers, ok := crypto.subscribers.LoadOrStore(
		key, []*types.Subscription[any]{subscription},
	)

	if ok {
		subscribers = append(
			subscribers.([]*types.Subscription[any]), subscription,
		)

		crypto.subscribers.Store(key, subscribers)
	}

	return subscription
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
			case in := <-crypto.subscriptions["planner"].Channel:
				var ok bool
				crypto.thesis, ok = in.(*types.Thesis)

				if !ok {
					continue
				}

				crypto.thesis.Decisions.Range(func(key, value any) bool {
					decision, ok := value.(*types.Decision)

					if !ok {
						return true
					}

					go func() {
						if err := crypto.desk.Execute(*decision); err != nil {
							errnie.Error(errnie.Err(
								errnie.Internal,
								"crypto: failed to execute decision round",
								err,
							))
						}
					}()

					return true
				})
			}
		}
	}()
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
		crypto.thesis.AppendTicker(ticker)
		crypto.desk.Price().Update(&ticker)
	}

	utils.Fanout(crypto.subscribers, "thesis", crypto.thesis)
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

	utils.Fanout(crypto.subscribers, "thesis", crypto.thesis)
}

func (crypto *Crypto) Close() (err error) {
	crypto.cancel()
	return nil
}
