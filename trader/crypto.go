package trader

import (
	"context"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
	"github.com/theapemachine/symm/utils"
)

/*
Crypto submits desk work from thesis messages delivered by the Actor cascade.
*/
type Crypto struct {
	*types.Actor
	status   types.Status
	ctx      context.Context
	cancel   context.CancelFunc
	tick     *atomic.Int64
	dataPath string
	uiHub    *ui.Hub
	recorder *audit.Recorder
}

/*
NewCrypto constructs Crypto with market-topic handlers; Boot Initialize attaches
the planner.
*/
func NewCrypto(
	ctx context.Context,
	uiHub *ui.Hub,
	recorder *audit.Recorder,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:      ctx,
		cancel:   cancel,
		status:   types.INITIALIZING,
		tick:     &atomic.Int64{},
		dataPath: utils.ResolveDataPath(),
		uiHub:    uiHub,
		recorder: recorder,
	}

	crypto.Actor = types.NewActor(ctx, map[string]types.Handler{
		"ticker": {Topic: "ticker", Fn: crypto.thesis},
		"book":   {Topic: "book", Fn: crypto.thesis},
		"trade":  {Topic: "trade", Fn: crypto.thesis},
	})

	return crypto, nil
}

func (crypto *Crypto) Initialize(
	planner *strategy.Planner,
) error {
	errnie.Info("initializing crypto")

	crypto.Actor.Initialize(
		types.Topic{Name: "ticker", Actor: planner.Actor},
		types.Topic{Name: "book", Actor: planner.Actor},
		types.Topic{Name: "trade", Actor: planner.Actor},
	)

	crypto.status = types.READY
	return nil
}

func (crypto *Crypto) Status() types.Status {
	return crypto.status
}

/*
Run starts the Actor loop.
*/
func (crypto *Crypto) Run() {
	crypto.Actor.Run()
}

func (crypto *Crypto) thesis(message any) any {
	return message
}

func (crypto *Crypto) Close() (err error) {
	crypto.cancel()
	return nil
}
