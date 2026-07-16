package trader

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

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
	done       chan struct{}
	desk       *broker.Desk
	price      *broker.Price
	balance    *broker.Balance
	api        *websocket.API
	instrument *broker.Instrument
	tree       *dmt.Tree
	tick       *atomic.Int64
	tickBudget time.Duration
	planner    *strategy.Planner
	postMortem *strategy.PostMortem
	analyzer   *logic.Analyzer
	dataPath   string
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
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	dataPath := viper.GetViper().GetString("data.path")
	tickBudget := viper.GetViper().GetDuration("cognitive.tick_budget")

	if tickBudget <= 0 {
		tickBudget = 10 * time.Millisecond
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
		tickBudget: tickBudget,
		analyzer:   analyzer,
		planner:    planner,
		postMortem: &strategy.PostMortem{},
		dataPath:   dataPath,
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
	crypto.done = make(chan struct{})

	go func() {
		defer close(crypto.done)
		errnie.Info("crypto runtime started")

		for crypto.Status() != types.ERROR {
			select {
			case <-crypto.ctx.Done():
				return
			default:
			}

			if crypto.booter.Ready(system.StageWarmup) {
				thesis := crypto.planner.Update()
				thesis.Tick = crypto.tick.Add(1)
				crypto.trade(thesis)
				encoded, err := sonic.Marshal(thesis)

				if err != nil {
					crypto.status = types.ERROR
					errnie.Error(err)
					continue
				}

				// Store the thesis to a file in the data directory
				filePath := filepath.Join(os.Getenv(crypto.dataPath), "thesis.json")
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

	if crypto.done != nil {
		<-crypto.done
	}

	if err := crypto.desk.Close(); err != nil {
		return err
	}

	if crypto.analyzer != nil {
		crypto.analyzer.Close()
	}

	return nil
}
