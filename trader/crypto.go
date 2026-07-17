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
	market     *Market
	recorder   *audit.Recorder
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

	market, err := NewMarket(api, instrument)

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
	}
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

func (crypto *Crypto) Run() error {
	auditPath := filepath.Join(crypto.dataPath, "runtime-audit.jsonl")

	if viper.GetBool("system.audit.rotate_on_boot") {
		if err := audit.Rotate(auditPath); err != nil {
			return err
		}
	}

	recorder, recorderErr := audit.NewRecorder(auditPath)

	if recorderErr != nil {
		return recorderErr
	}

	crypto.planner.SetRecorder(recorder)
	crypto.recorder = recorder

	go func() {
		errnie.Info("crypto runtime started")

		if recorder != nil {
			defer func() {
				errnie.Error(recorder.Close())
			}()
		}

		for crypto.Status() != types.ERROR {
			select {
			case <-crypto.ctx.Done():
				return
			default:
			}

			if !crypto.booter.Ready(system.StageWarmup) {
				continue
			}

			if _, err := crypto.Tick(time.Now().UTC()); err != nil {
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
	started := time.Now()
	recorder := crypto.recorder

	errnie.Error(audit.Phase(recorder, tick, "cut", map[string]any{
		"tickers": len(frame.Tickers),
		"trades":  len(frame.Trades),
		"books":   len(frame.Books),
	}))

	thesis := crypto.planner.Update(frame, tick)
	elapsed := time.Since(started).Nanoseconds()

	errnie.Error(audit.Phase(recorder, tick, "update_end", map[string]any{
		"ns":        elapsed,
		"completed": true,
	}))

	crypto.trade(thesis)
	crypto.publishTick(thesis, elapsed)

	errnie.Error(audit.Record(recorder, "tick", map[string]any{
		"tick":         thesis.Tick,
		"measurements": types.ObservationCount(thesis.Measurements),
		"decisions":    len(thesis.Decisions),
		"forecasts":    len(thesis.Forecasts),
		"ns":           elapsed,
		"completed":    true,
	}))

	return thesis, nil
}

/*
publishTick sends the frontend one compact factual runtime projection after a
completed Thesis tick so engine health reflects actual planner progress and
latency rather than focus-scoped kernel standby.
*/
func (crypto *Crypto) publishTick(thesis *types.Thesis, elapsedNs int64) {
	if crypto.uiHub == nil {
		return
	}

	tick := datura.Map[any]{
		"count":        thesis.Tick,
		"measurements": types.ObservationCount(thesis.Measurements),
		"candidates":   len(thesis.Positions),
		"ns":           elapsedNs,
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
trade supplies current broker constraints to strategy, then submits each
selected order through Desk for exchange validation and execution.
*/
func (crypto *Crypto) trade(thesis *types.Thesis) {
	for _, holding := range thesis.Positions {
		if !crypto.desk.HasSlot(holding.IsOpportunity) {
			return
		}

		if err := crypto.desk.Buy(holding, holding.IsOpportunity); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"failed to buy holding "+holding.Symbol,
				err,
			))
			continue
		}
	}
}

/*
Close stops the trader and its composed resources.
*/
func (crypto *Crypto) Close() error {
	crypto.cancel()

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
