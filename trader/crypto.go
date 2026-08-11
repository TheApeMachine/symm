package trader

import (
	"context"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/regulator"
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
	regulator     *regulator.Solver
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
	regulatorSolver *regulator.Solver,
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
		regulator:    regulatorSolver,
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
Update is the main control loop.
*/
func (crypto *Crypto) Update() error {
	crypto.thesis.At = time.Now().UTC()

	if err := crypto.measurements.Update(crypto.thesis); err != nil {
		return err
	}

	if err := crypto.analyzer.Process(crypto.thesis); err != nil {
		return err
	}

	if err := crypto.planner.Update(crypto.thesis); err != nil {
		return err
	}

	decisions := make([]types.Decision, 0)

	crypto.thesis.Symbols.Range(func(key, value any) bool {
		symbolName := key.(string)
		symbol := value.(*types.Symbol)

		if value, found := symbol.Decisions.Load(symbolName); found {
			decisions = append(decisions, *value.(*types.Decision))
		}

		return true
	})

	if len(decisions) == 0 {
		return crypto.regulator.Update(crypto.thesis)
	}

	if crypto.desk == nil || crypto.desk.PositionStore == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: thesis checkpoint store required",
			nil,
		))
	}

	if err := crypto.desk.SaveThesis(crypto.thesis); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"crypto: save pre-execution thesis checkpoint",
			err,
		))
	}

	var err error

	for _, decision := range decisions {
		if err = crypto.desk.Execute(decision); err != nil {
			break
		}
	}

	if err != nil {
		return err
	}

	return crypto.regulator.Update(crypto.thesis)
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

	errnie.Error(crypto.Update())
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

	errnie.Error(crypto.Update())
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

	errnie.Error(crypto.Update())
}

func (crypto *Crypto) Close() (err error) {
	crypto.cancel()
	return nil
}
