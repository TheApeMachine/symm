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
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Crypto is the simple trading runtime.
It consumes market and private frames, publishes UI frames,
and delegates measurement to Signal.
*/
type Crypto struct {
	booter         *system.Booter
	status         types.Status
	ctx            context.Context
	cancel         context.CancelFunc
	desk           *broker.Desk
	price          *broker.Price
	balance        *broker.Balance
	api            *websocket.API
	instrument     *broker.Instrument
	tree           *dmt.Tree
	tick           *atomic.Int64
	planner        *strategy.Planner
	postMortem     *strategy.PostMortem
	analyzer       *logic.Analyzer
	dataPath       string
	uiHub          *ui.Hub
	market         *Market
	recorder       *audit.Recorder
	checkpointAt   atomic.Int64
	checkpointSlot atomic.Pointer[types.Recovery]
	snapshot       atomic.Pointer[types.Recovery]
	thesis         atomic.Pointer[types.Thesis]
	trading        atomic.Bool
}

/*
NewCrypto wires the trading runtime around shared infrastructure.
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
	thesis *types.Thesis,
	uiHub *ui.Hub,
	recorder *audit.Recorder,
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

	market, err := NewMarket(ctx, api, instrument)

	if err != nil {
		cancel()
		return nil, err
	}

	snapshot, err := types.LoadRecovery(dataPath)

	if err != nil {
		cancel()
		return nil, err
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
		market:     market,
		recorder:   recorder,
	}

	crypto.snapshot.Store(snapshot)
	crypto.thesis.Store(thesis)
	crypto.tick.Store(thesis.Tick)

	return crypto, nil
}

func (crypto *Crypto) Initialize() error {
	errnie.Info("initializing crypto")
	crypto.status = types.READY
	return nil
}

/*
Status returns the current status of the crypto runtime.
*/
func (crypto *Crypto) Status() types.Status {
	return crypto.status
}

/*
Run starts the checkpoint loop and the tick pump. Audit rotation is owned by the
boot path before the recorder opens its file, so Run never rotates a live
handle.
*/
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

			err := crypto.Tick()

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

/*
Tick processes one complete ingress cut through the production planner. Run and
the deterministic market decorator share this exact system transition.
*/
func (crypto *Crypto) Tick() error {
	frame, err := crypto.market.Cut()

	if err != nil {
		return err
	}

	tick := crypto.tick.Add(1)
	errnie.Error(audit.Phase(crypto.recorder, tick, "cut", map[string]any{
		"tickers": len(frame.Tickers),
		"trades":  len(frame.Trades),
		"books":   len(frame.Books),
	}))

	thesis := crypto.planner.Update(nil, frame, tick)
	crypto.thesis.Store(thesis)
	candidates := 0

	thesis.Positions.Range(func(key, value any) bool {
		candidates++
		return true
	})

	select {
	case crypto.uiHub.Messages <- datura.Map[any]{"tick": datura.Map[any]{
		"count":        thesis.Tick,
		"measurements": types.ObservationCount(thesis.Measurements),
		"candidates":   candidates,
		"open":         crypto.desk.HoldingCount(),
		"completed":    true,
		"phase":        "complete",
	}}.Marshal():
	default:
	}

	errnie.Error(audit.Record(crypto.recorder, "tick", map[string]any{
		"tick":         thesis.Tick,
		"measurements": types.ObservationCount(thesis.Measurements),
		"decisions":    len(thesis.Decisions),
		"forecasts":    len(thesis.Forecasts),
		"completed":    true,
	}))

	return nil
}

/*
Thesis returns the latest completed planner result published by Tick.
*/
func (crypto *Crypto) Thesis() *types.Thesis {
	return crypto.thesis.Load()
}

/*
Close flushes the durable Thesis checkpoint then stops composed resources.
*/
func (crypto *Crypto) Close() (err error) {
	crypto.cancel()

	for _, closer := range []io.Closer{
		crypto.market, crypto.planner, crypto.desk, crypto.analyzer,
	} {
		if closer != nil {
			if closerErr := closer.Close(); closerErr != nil {
				err = errors.Join(err, closerErr)
			}
		}
	}

	return nil
}
