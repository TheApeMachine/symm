package trader

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Crypto is the trading runtime. Handlers push rows to signals; Tick drains
measurements into the planner and submits desk orders.
*/
type Crypto struct {
	booter     *system.Booter
	status     types.Status
	ctx        context.Context
	cancel     context.CancelFunc
	desk       *broker.Desk
	price      *broker.Price
	balance    *broker.Balance
	api        *websocket.API
	instrument *broker.Instrument
	tree       *dmt.Tree
	tick       *atomic.Int64
	planner    *strategy.Planner
	postMortem *strategy.PostMortem
	analyzer   *logic.Analyzer
	dataPath   string
	uiHub      *ui.Hub
	recorder   *audit.Recorder
	signals    []types.Signal
	outs       []chan []*types.Measurement
	lastThesis atomic.Pointer[types.Thesis]
}

/*
NewCrypto wires handlers onto the API and retains signals for Drain.
*/
func NewCrypto(
	ctx context.Context,
	booter *system.Booter,
	api *websocket.API,
	price *broker.Price,
	balance *broker.Balance,
	desk *broker.Desk,
	instrument *broker.Instrument,
	analyzer *logic.Analyzer,
	planner *strategy.Planner,
	tree *dmt.Tree,
	uiHub *ui.Hub,
	recorder *audit.Recorder,
	signals []types.Signal,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	dataPath := strings.TrimSpace(viper.GetViper().GetString("system.data_path"))

	if strings.HasPrefix(dataPath, "~/") {
		home, err := os.UserHomeDir()

		if err != nil {
			cancel()
			return nil, errnie.Error(errnie.Err(
				errnie.IO, "failed to resolve system.data_path", err,
			))
		}

		dataPath = filepath.Join(home, strings.TrimPrefix(dataPath, "~/"))
	}

	crypto := &Crypto{
		ctx:        ctx,
		cancel:     cancel,
		booter:     booter,
		status:     types.INITIALIZING,
		api:        api,
		desk:       desk,
		price:      price,
		balance:    balance,
		instrument: instrument,
		tree:       tree,
		tick:       &atomic.Int64{},
		analyzer:   analyzer,
		planner:    planner,
		postMortem: &strategy.PostMortem{},
		dataPath:   dataPath,
		uiHub:      uiHub,
		recorder:   recorder,
		signals:    signals,
	}

	for _, signal := range signals {
		crypto.outs = append(crypto.outs, signal.Measure())
	}

	if api != nil {
		api.On("ticker", crypto.OnTicker)
		api.On("book", crypto.OnBook)
		api.On("trade", crypto.OnTrade)
	}

	return crypto, nil
}

func (crypto *Crypto) Initialize() error {
	errnie.Info("initializing crypto")
	crypto.status = types.READY
	return nil
}

func (crypto *Crypto) Status() types.Status {
	return crypto.status
}

func (crypto *Crypto) OnTicker(data []byte) {
	ticker := kraken.NewTicker(data)

	if !ticker.IsSuccess() {
		return
	}

	for _, signal := range crypto.signals {
		select {
		case <-crypto.ctx.Done():
			return
		case signal.Tickers() <- ticker.Data:
		}
	}
}

func (crypto *Crypto) OnBook(data []byte) {
	book := kraken.NewBook(data)

	if !book.IsSuccess() {
		return
	}

	if crypto.instrument != nil {
		for index := range book.Data {
			pair, err := crypto.instrument.Pair(book.Data[index].Symbol)

			if err != nil {
				errnie.Error(err)
				return
			}

			book.Data[index].PriceIncrement = &pair.PriceIncrement
		}
	}

	for _, signal := range crypto.signals {
		select {
		case <-crypto.ctx.Done():
			return
		case signal.Books() <- book.Data:
		}
	}
}

func (crypto *Crypto) OnTrade(data []byte) {
	trade := kraken.NewTrade(data)

	if !trade.IsSuccess() {
		return
	}

	for _, signal := range crypto.signals {
		select {
		case <-crypto.ctx.Done():
			return
		case signal.Trades() <- trade.Data:
		}
	}
}

func (crypto *Crypto) Run() error {
	go func() {
		errnie.Info("crypto runtime started")

		if crypto.recorder != nil {
			defer func() {
				errnie.Error(crypto.recorder.Close())
			}()
		}

		budget := viper.GetDuration("cognitive.tick_budget")

		if budget <= 0 {
			budget = 10 * time.Millisecond
		}

		for crypto.Status() != types.ERROR {
			select {
			case <-crypto.ctx.Done():
				return
			default:
			}

			if !crypto.booter.Ready(system.StageWarmup) {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			_, err := crypto.Tick()

			if errnie.IsPreconditionFailed(err) {
				time.Sleep(budget)
				continue
			}

			if err != nil {
				errnie.Error(err)
				crypto.status = types.ERROR
				return
			}
		}
	}()

	return nil
}

func (crypto *Crypto) Tick() (*types.Thesis, error) {
	rows := make([]*types.Measurement, 0)

	for _, out := range crypto.outs {
		for draining := true; draining; {
			select {
			case batch := <-out:
				rows = append(rows, batch...)
			default:
				draining = false
			}
		}
	}

	if len(rows) == 0 {
		return nil, errnie.Err(
			errnie.PreconditionFailed,
			"crypto: no signal measurements",
			nil,
		)
	}

	tick := crypto.tick.Add(1)
	at := time.Time{}

	for _, row := range rows {
		if row != nil && row.At.After(at) {
			at = row.At
		}
	}

	thesis := crypto.planner.Update(crypto.lastThesis.Load(), at, tick, rows)

	if crypto.balance != nil {
		for holding := range crypto.balance.Holdings() {
			if _, found := thesis.Lifecycle.Load(holding.Symbol); !found {
				thesis.NoteLifecycle(holding.Symbol, types.LifecycleManaging, thesis.At)
			}
		}
	}

	crypto.planner.Decide(thesis)

	if crypto.desk != nil && crypto.balance != nil &&
		crypto.balance.Status() == types.READY {
		for _, decision := range thesis.Decisions {
			switch decision.Action {
			case types.ActionExit:
				if phase, found := thesis.Lifecycle.Load(decision.Symbol); found {
					if phase == types.LifecycleExitSubmitted {
						continue
					}
				}

				if err := crypto.desk.Sell(decision.Symbol); err != nil {
					errnie.Error(err)
					continue
				}

				thesis.NoteLifecycle(
					decision.Symbol, types.LifecycleExitSubmitted, thesis.At,
				)
			case types.ActionEnter:
				raw, ok := thesis.Holdings.Load(decision.Symbol)

				if !ok {
					continue
				}

				holding, ok := raw.(*types.Holding)

				if !ok || holding == nil || !crypto.desk.HasSlot(holding.IsOpportunity) {
					continue
				}

				position, err := crypto.desk.Buy(holding, holding.IsOpportunity)

				if err != nil {
					errnie.Error(err)
					thesis.Holdings.Delete(decision.Symbol)
					continue
				}

				thesis.Positions.Store(decision.Symbol, position)
				thesis.NoteLifecycle(
					decision.Symbol, types.LifecycleEntrySubmitted, thesis.At,
				)
			}
		}
	}

	if len(thesis.Decisions) > 0 {
		select {
		case crypto.uiHub.Messages <- datura.Map[any]{
			"decisions": thesis.Decisions,
		}.Marshal():
		default:
		}
	}

	crypto.lastThesis.Store(thesis)

	select {
	case crypto.uiHub.Messages <- datura.Map[any]{"tick": datura.Map[any]{
		"count":        thesis.Tick,
		"measurements": types.ObservationCount(thesis.Measurements),
		"open":         crypto.desk.HoldingCount(),
		"completed":    true,
		"phase":        "complete",
	}}.Marshal():
	default:
	}

	return thesis, nil
}

func (crypto *Crypto) Close() (err error) {
	crypto.cancel()

	for _, closer := range []io.Closer{
		crypto.planner, crypto.desk, crypto.analyzer,
	} {
		if closer != nil {
			if closerErr := closer.Close(); closerErr != nil {
				err = errors.Join(err, closerErr)
			}
		}
	}

	return err
}
