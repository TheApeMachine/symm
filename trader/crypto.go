package trader

import (
	"context"
	"fmt"
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
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	measurements  *Measurements
	thesis        *types.Thesis
	dataPath      string
	ui            chan []byte
	recorder      *audit.Recorder
	desk          *broker.Desk
	analyzer      *logic.Analyzer
	planner       *strategy.Planner
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
	analyzer *logic.Analyzer,
	planner *strategy.Planner,
	thesis *types.Thesis,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:          ctx,
		cancel:       cancel,
		status:       types.READY,
		api:          api,
		measurements: NewMeasurements(ctx, api, desk.Instrument(), ui),
		thesis:       thesis,
		dataPath:     utils.ResolveDataPath(),
		ui:           ui,
		recorder:     recorder,
		desk:         desk,
		analyzer:     analyzer,
		planner:      planner,
		bookUpdates:  api.BookUpdates(),
		subscriptions: map[string]*types.Subscription[any]{
			"ticker": api.Subscribe(
				"ticker", types.NewSubscription[any](),
			),
			"trade": api.Subscribe(
				"trade", types.NewSubscription[any](),
			),
		},
	}

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
			}
		}
	}()
}

/*
Update is the main control loop for the signals receiving the triggering data.
*/
func (crypto *Crypto) Update(receivers []types.SourceType) error {
	crypto.thesis.At = time.Now().UTC()

	resonanceReady, err := crypto.measurements.Update(crypto.thesis, receivers)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("crypto: measurements update failed [%s]", err.Error()),
			err,
		))
	}

	if err := crypto.analyzer.Process(crypto.thesis, resonanceReady); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("crypto: analyzer process failed [%s]", err.Error()),
			err,
		))
	}

	if err := crypto.planner.Update(crypto.thesis); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("crypto: planner update failed [%s]", err.Error()),
			err,
		))
	}

	return nil
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

	errnie.Error(crypto.Update(types.BookReceivers))
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
		crypto.thesis.Symbol(ticker.Symbol).AppendTicker(ticker)
		crypto.desk.Price().Update(&ticker)
	}

	errnie.Error(crypto.Update(types.TickerReceivers))
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
		crypto.thesis.Symbol(trade.Symbol).AppendTrade(trade)
	}

	errnie.Error(crypto.Update(types.TradeReceivers))
}

func (crypto *Crypto) Close() (err error) {
	crypto.cancel()
	return nil
}
