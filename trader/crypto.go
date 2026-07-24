package trader

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
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
	desk     *broker.Desk
}

/*
NewCrypto constructs Crypto; Boot Initialize attaches planner and desk.
*/
func NewCrypto(
	ctx context.Context,
	uiHub *ui.Hub,
	recorder *audit.Recorder,
	desk *broker.Desk,
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
		desk:     desk,
	}

	crypto.Actor = types.NewActor(ctx, map[string]types.Handler{
		"ticker": {Topic: "ticker", Fn: crypto.thesis},
		"trade":  {Topic: "trade", Fn: crypto.thesis},
	})

	return crypto, nil
}

func (crypto *Crypto) Initialize(
	planner *strategy.Planner,
) error {
	errnie.Info("initializing crypto")

	crypto.Actor.InitializeSize(
		1,
		types.Topic{Name: "ticker", Actor: planner.Actor},
		types.Topic{Name: "trade", Actor: planner.Actor},
	)

	crypto.status = types.READY
	return nil
}

func (crypto *Crypto) Status() types.Status {
	return crypto.status
}

func (crypto *Crypto) thesis(message any) any {
	thesis := message.(*types.Thesis)
	thesis.Tick = crypto.NextTick()
	crypto.Apply(thesis)

	return thesis
}

/*
NextTick advances the observation counter for a new market cut.
*/
func (crypto *Crypto) NextTick() int64 {
	return crypto.tick.Add(1)
}

/*
Apply submits sized enter/exit decisions to the desk and records lifecycle.
*/
func (crypto *Crypto) Apply(thesis *types.Thesis) {
	if thesis == nil || crypto.desk == nil {
		return
	}

	started := time.Now()
	// Copy so a concurrent Decide retain/replace cannot empty the slice mid-apply.
	decisions := append([]types.Decision(nil), thesis.Decisions...)

	for index := range decisions {
		decision := &decisions[index]

		switch decision.Action {
		case types.ActionEnter:
			crypto.enter(thesis, decision)
		case types.ActionExit:
			crypto.exit(thesis, decision)
		}
	}

	elapsed := time.Since(started)
	crypto.publish(thesis, elapsed)

	errnie.Error(audit.Phase(crypto.recorder, thesis.Tick, "desk", map[string]any{
		"ns":        elapsed.Nanoseconds(),
		"decisions": len(decisions),
	}))
}

/*
publish forwards the engine tick plus decisions, lifecycle, and findings so the
terminal pulse and rails leave their waiting states as soon as a cut completes.
*/
func (crypto *Crypto) publish(thesis *types.Thesis, elapsed time.Duration) {
	if crypto.uiHub == nil || crypto.uiHub.Messages == nil || thesis == nil {
		return
	}

	crypto.enqueue(datura.Map[any]{"tick": datura.Map[any]{
		"count":        thesis.Tick,
		"measurements": types.ObservationCount(thesis.Measurements),
		"candidates":   len(thesis.Forecasts),
		"open":         crypto.desk.HoldingCount(),
		"ns":           elapsed.Nanoseconds(),
		"completed":    true,
		"phase":        "complete",
	}})

	if len(thesis.Decisions) > 0 {
		crypto.enqueue(datura.Map[any]{"decisions": thesis.Decisions})
	}

	lifecycle := make([]datura.Map[any], 0)

	thesis.Lifecycle.Range(func(key, value any) bool {
		lifecycle = append(lifecycle, datura.Map[any]{
			"symbol": key.(string),
			"state":  value.(string),
		})

		return true
	})

	if len(lifecycle) > 0 {
		crypto.enqueue(datura.Map[any]{"lifecycle": lifecycle})
	}

	if len(thesis.Findings) > 0 {
		crypto.enqueue(datura.Map[any]{"findings": thesis.Findings})
	}
}

/*
enqueue non-blocking-publishes one UI frame; a full hub channel drops the frame
rather than stalling the desk path.
*/
func (crypto *Crypto) enqueue(frame datura.Map[any]) {
	select {
	case crypto.uiHub.Messages <- frame.Marshal():
	default:
	}
}

func (crypto *Crypto) enter(thesis *types.Thesis, decision *types.Decision) {
	if decision.ProposedQuantity == nil || decision.ProposedQuantity.Sign() <= 0 {
		return
	}

	value, ok := thesis.Holdings.Load(decision.Symbol)
	var holding *types.Holding

	if ok {
		holding = value.(*types.Holding)
	}

	if holding == nil {
		holding = types.NewHolding(crypto.ctx, decision.Symbol, decision.ProposedQuantity)
		thesis.Holdings.Store(decision.Symbol, holding)
	}

	if holding.Qty == nil || holding.Qty.Sign() <= 0 {
		holding.Qty = decision.ProposedQuantity.Copy()
	}

	if _, err := crypto.desk.Buy(holding, holding.IsOpportunity); err != nil {
		errnie.Error(err)
		return
	}

	thesis.NoteLifecycle(decision.Symbol, types.LifecycleEntrySubmitted, thesis.At)
}

func (crypto *Crypto) exit(thesis *types.Thesis, decision *types.Decision) {
	if err := crypto.desk.Sell(decision.Symbol); err != nil {
		errnie.Error(err)
		return
	}

	thesis.NoteLifecycle(decision.Symbol, types.LifecycleExitSubmitted, thesis.At)
}

func (crypto *Crypto) Close() (err error) {
	crypto.cancel()
	return nil
}
