package trader

import (
	"context"
	"fmt"
	"sync"

	"github.com/theapemachine/datura"
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
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:      ctx,
		cancel:   cancel,
		status:   types.READY,
		thesis:   types.NewThesis(),
		dataPath: utils.ResolveDataPath(),
		ui:       ui,
		recorder: recorder,
		planner:  planner,
		desk:     desk,
		subscriptions: map[string]*types.Subscription[any]{
			"ticker": api.Subscribe(
				"ticker", types.NewSubscription[any](),
			),
			"trades": api.Subscribe(
				"trades", types.NewSubscription[any](),
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
	errnie.Info(fmt.Sprintf("websocket: new subscriber %s", key))

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
			case trades := <-crypto.subscriptions["trades"].Channel:
				crypto.onTrades(trades)
			case decisions := <-crypto.subscriptions["decisions"].Channel:
				if !crypto.decisionsReady(decisions) {
					continue
				}

				crypto.thesis.Tick = crypto.thesis.Tick + 1

				out := datura.NewMap()
				out["decisions"] = decisions
				utils.Publish(crypto.ui, out)

				tickOut := datura.NewMap()
				tickOut["tick"] = datura.NewMap()
				tickOut["tick"].(datura.Map[any])["count"] = crypto.thesis.Tick
				utils.Publish(crypto.ui, tickOut)
			}
		}
	}()
}

func (crypto *Crypto) onTicker(data any) {
	typedTickers, ok := data.([]*kraken.TickerData)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: unexpected ticker payload type",
			nil,
		))

		return
	}

	for _, typedData := range typedTickers {
		found, ok := crypto.thesis.Tickers.LoadOrStore(
			typedData.Symbol, []*kraken.TickerData{typedData},
		)

		if ok {
			typedSlice, ok := found.([]*kraken.TickerData)

			if !ok {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"crypto: unexpected ticker slice type",
					nil,
				))

				continue
			}

			typedSlice = append(typedSlice, typedData)
			crypto.thesis.Tickers.Store(typedData.Symbol, typedSlice)
		}
	}

	for symbol, book := range crypto.api.Books() {
		crypto.thesis.Books.Store(symbol, book)
	}

	found, ok := crypto.subscribers.Load("ticker")

	if ok {
		subscribers := found.([]*types.Subscription[any])

		for _, subscriber := range subscribers {
			subscriber.Send(data)
		}
	}
}

func (crypto *Crypto) onTrades(data any) {
	typedTrades, ok := data.([]*kraken.TradeData)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: unexpected trades payload type",
			nil,
		))

		return
	}

	for _, trade := range typedTrades {
		found, ok := crypto.thesis.Trades.LoadOrStore(
			trade.Symbol, []*kraken.TradeData{trade},
		)

		if ok {
			typedSlice, ok := found.([]*kraken.TradeData)

			if !ok {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"crypto: unexpected trades slice type",
					nil,
				))

				continue
			}

			typedSlice = append(typedSlice, trade)
			crypto.thesis.Trades.Store(trade.Symbol, typedSlice)
		}
	}

	for symbol, book := range crypto.api.Books() {
		crypto.thesis.Books.Store(symbol, book)
	}

	found, ok := crypto.subscribers.Load("trades")

	if ok {
		subscribers := found.([]*types.Subscription[any])

		for _, subscriber := range subscribers {
			subscriber.Send(data)
		}
	}
}

func (crypto *Crypto) decisionsReady(decisions any) bool {
	typedDecisions, ok := decisions.([]types.Decision)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: unexpected decisions payload type",
			nil,
		))

		return false
	}

	return len(typedDecisions) > 0
}

func (crypto *Crypto) Close() (err error) {
	crypto.cancel()
	return nil
}
