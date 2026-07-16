package trader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
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
	go func() {
		errnie.Info("crypto runtime started")

		for crypto.Status() != types.ERROR {
			select {
			case <-crypto.ctx.Done():
				return
			default:
			}

			if crypto.booter.Ready(system.StageWarmup) {
				thesis := crypto.planner.Update()
				crypto.uiHub.SetThesis(thesis)

				thesis.Tick = crypto.tick.Add(1)
				crypto.trade(thesis)

				encoded, err := sonic.Marshal(thesis)

				if err != nil {
					crypto.status = types.ERROR
					errnie.Error(err)
					continue
				}

				// Store the thesis to a file in the data directory
				filePath := filepath.Join(crypto.dataPath, "thesis.json")
				err = os.WriteFile(filePath, encoded, 0644)

				if err != nil {
					crypto.status = types.ERROR
					errnie.Error(err)
					continue
				}
			}
		}
	}()

	return nil
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

	if err := crypto.desk.Close(); err != nil {
		return err
	}

	if crypto.analyzer != nil {
		crypto.analyzer.Close()
	}

	return nil
}
