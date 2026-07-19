package trader

import (
	"context"
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
	lastThesis     atomic.Pointer[types.Thesis]
	checkpointAt   atomic.Int64
	checkpointSlot atomic.Pointer[types.Thesis]
	snapshot       *types.Recovery
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
		snapshot:   snapshot,
		recorder:   recorder,
	}

	crypto.tick.Store(thesis.Tick)
	crypto.lastThesis.Store(thesis)

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

func (crypto *Crypto) Run() error {
	auditPath := filepath.Join(crypto.dataPath, "runtime-audit.jsonl")

	if viper.GetBool("system.audit.rotate_on_boot") {
		if err := audit.Rotate(auditPath); err != nil {
			return err
		}
	}

	go crypto.checkpointLoop()

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

			if crypto.market != nil {
				crypto.market.WaitDirty(budget)
			}

			_, err := crypto.Tick(time.Now().UTC())

			if err != nil {
				return
			}
		}
	}()

	return nil
}

/*
Tick cuts the market at at, runs one planner update and trade pass, then
publishes the tick projection. An empty cut returns a nil Thesis without error
so callers can advance virtual time without busy-spinning.
*/
func (crypto *Crypto) Tick(at time.Time) (*types.Thesis, error) {
	frame, err := crypto.market.Cut(at)

	if err != nil {
		crypto.status = types.ERROR
		errnie.Error(errnie.Err(
			errnie.Internal,
			"crypto: market cut failed",
			err,
		))

		return nil, err
	}

	if frame.IsEmpty() {
		return nil, nil
	}

	tick := crypto.tick.Add(1)

	errnie.Error(audit.Phase(crypto.recorder, tick, "cut", map[string]any{
		"tickers": len(frame.Tickers),
		"trades":  len(frame.Trades),
		"books":   len(frame.Books),
	}))

	thesis := crypto.planner.Update(crypto.lastThesis.Load(), frame, tick)

	if err := crypto.Plan(thesis); err != nil {
		crypto.status = types.ERROR

		return nil, err
	}

	crypto.trade(thesis)
	crypto.publishStrategy(thesis)
	crypto.lastThesis.Store(thesis)
	crypto.checkpoint(thesis)
	crypto.publishTick(thesis)

	errnie.Error(audit.Record(crypto.recorder, "tick", map[string]any{
		"tick":         thesis.Tick,
		"measurements": types.ObservationCount(thesis.Measurements),
		"decisions":    len(thesis.Decisions),
		"forecasts":    len(thesis.Forecasts),
		"completed":    true,
	}))

	return thesis, nil
}

/*
publishTick sends the frontend one compact factual runtime projection after a
completed Thesis tick so engine health reflects actual planner progress and
latency rather than focus-scoped kernel standby.
*/
func (crypto *Crypto) publishTick(thesis *types.Thesis) {
	if crypto.uiHub == nil {
		return
	}

	positions := 0

	thesis.Positions.Range(func(key, value any) bool {
		positions++
		return true
	})

	tick := datura.Map[any]{
		"count":        thesis.Tick,
		"measurements": types.ObservationCount(thesis.Measurements),
		"candidates":   positions,
		"completed":    true,
		"phase":        "complete",
	}

	if crypto.desk != nil {
		tick["open"] = crypto.desk.OpenPositions()
	}

	select {
	case crypto.uiHub.Messages <- datura.Map[any]{"tick": tick}.Marshal():
	default:
	}
}

/*
Close flushes the durable Thesis checkpoint then stops composed resources.
*/
func (crypto *Crypto) Close() error {
	crypto.cancel()

	if latest := crypto.lastThesis.Load(); latest != nil {
		if err := latest.Save(crypto.dataPath); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"crypto: thesis checkpoint on close failed",
				err,
			))
		}
	}

	if crypto.market != nil {
		crypto.market.Close()
	}

	if crypto.planner != nil {
		crypto.planner.Close()
	}

	if err := crypto.desk.Close(); err != nil {
		return err
	}

	if crypto.analyzer != nil {
		crypto.analyzer.Close()
	}

	return nil
}
