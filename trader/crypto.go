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
	bookUpdates   <-chan string
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
		ctx:         ctx,
		cancel:      cancel,
		status:      types.READY,
		api:         api,
		thesis:      thesis,
		semaphore:   make(chan struct{}, 1),
		dataPath:    utils.ResolveDataPath(),
		ui:          ui,
		recorder:    recorder,
		desk:        desk,
		bookUpdates: api.BookUpdates(),
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
			case symbol := <-crypto.bookUpdates:
				crypto.onBookUpdate(symbol)
			case <-crypto.semaphore:
				crypto.thesis.Symbols.Range(func(key, value any) bool {
					symbolName, nameOK := key.(string)
					symbol, symbolOK := value.(*types.Symbol)

					if !nameOK || !symbolOK || !symbol.Stamped(types.SourcePlanner) {
						return true
					}

					if value, found := symbol.Decisions.Load(symbolName); found {
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

					symbol.Reset()
					return true
				})
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

	crypto.thesis.Fanout(types.SourceTrader, types.BookReceivers...)
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

	crypto.thesis.Fanout(types.SourceTrader, types.TickerReceivers...)
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

	crypto.thesis.Fanout(types.SourceTrader, types.TradeReceivers...)
}

func (crypto *Crypto) Close() (err error) {
	crypto.cancel()
	return nil
}
