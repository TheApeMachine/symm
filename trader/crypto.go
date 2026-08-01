package trader

import (
	"context"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
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
	tick          *atomic.Int64
	dataPath      string
	ui            chan []byte
	recorder      *audit.Recorder
	planner       *strategy.Planner
	desk          *broker.Desk
	subscriptions map[string]*types.Subscription[any]
}

/*
NewCrypto constructs Crypto; Boot Initialize attaches planner and desk.
*/
func NewCrypto(
	ctx context.Context,
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
		tick:     &atomic.Int64{},
		dataPath: utils.ResolveDataPath(),
		ui:       ui,
		recorder: recorder,
		planner:  planner,
		desk:     desk,
		subscriptions: map[string]*types.Subscription[any]{
			"decisions": planner.Subscribe(
				"decisions", types.NewSubscription[any](),
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
			case decisions := <-crypto.subscriptions["decisions"].Channel:
				if !crypto.decisionsReady(decisions) {
					continue
				}

				out := datura.NewMap()
				out["decisions"] = decisions
				utils.Publish(crypto.ui, out)

				tickOut := datura.NewMap()
				tickOut["tick"] = datura.NewMap()
				tickOut["tick"].(datura.Map[any])["count"] = crypto.tick.Add(1)
				utils.Publish(crypto.ui, tickOut)
			}
		}
	}()
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
