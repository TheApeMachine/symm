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
	journal  *JournalStore
	theses   []*types.Thesis
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
	journalStore := NewJournalStore()

	crypto := &Crypto{
		ctx:      ctx,
		cancel:   cancel,
		status:   types.INITIALIZING,
		tick:     &atomic.Int64{},
		dataPath: utils.ResolveDataPath(),
		uiHub:    uiHub,
		recorder: recorder,
		desk:     desk,
		journal:  journalStore,
	}

	if savedTheses, err := journalStore.Load(); err == nil {
		crypto.theses = append(crypto.theses, savedTheses...)

		if uiHub != nil && len(savedTheses) > 0 {
			uiHub.Publish(datura.NewMap("journal", savedTheses).MarshalAndFree())
		}
	} else {
		return nil, errnie.Error(err)
	}

	crypto.Actor = types.NewActor(ctx, "crypto", map[string]types.Handler{
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
Exits run before enters so rotation sagas free capital before the challenger.
*/
func (crypto *Crypto) Apply(thesis *types.Thesis) {
	if thesis == nil || crypto.desk == nil {
		return
	}

	thesis.Tick = crypto.NextTick()
	started := time.Now()
	// Copy so a concurrent Decide retain/replace cannot empty the slice mid-apply.
	decisions := append([]types.Decision(nil), thesis.Decisions...)

	for index := range decisions {
		decision := &decisions[index]

		if decision.Action == types.ActionExit {
			if err := crypto.desk.Exit(decision.Symbol); err != nil {
				errnie.Error(err)
				continue
			}

			thesis.NoteLifecycle(decision.Symbol, types.LifecycleExitSubmitted, thesis.At)
		}
	}

	for index := range decisions {
		decision := &decisions[index]

		if decision.Action != types.ActionEnter {
			continue
		}

		if decision.Displaces != "" {
			if _, open := crypto.desk.Position(decision.Displaces); open {
				continue
			}
		}

		if _, err := crypto.desk.Enter(decision); err != nil {
			errnie.Error(err)
			continue
		}

		thesis.NoteLifecycle(decision.Symbol, types.LifecycleEntrySubmitted, thesis.At)
	}

	elapsed := time.Since(started)
	crypto.publish(thesis, elapsed)

	errnie.Error(audit.Phase(crypto.recorder, thesis.Tick, "desk", map[string]any{
		"ns":        elapsed.Nanoseconds(),
		"decisions": len(decisions),
	}))

	errnie.Error(audit.Phase(crypto.recorder, thesis.Tick, "tick_end", map[string]any{
		"ns":        elapsed.Nanoseconds(),
		"decisions": len(decisions),
	}))
}

/*
publish forwards the engine tick plus decisions, lifecycle, and findings so the
terminal pulse and rails leave their waiting states as soon as a cut completes.
*/
func (crypto *Crypto) publish(thesis *types.Thesis, elapsed time.Duration) {
	if crypto.uiHub == nil || thesis == nil {
		return
	}

	crypto.uiHub.Publish(datura.NewMap(
		"tick", datura.NewMap(
			"count", thesis.Tick,
			"measurements", types.ObservationCount(thesis.Measurements),
			"candidates", len(thesis.Forecasts),
			"open", crypto.desk.OpenPositions(),
			"ns", elapsed.Nanoseconds(),
			"completed", true,
			"phase", "complete",
		),
	).MarshalAndFree())

	if len(thesis.Decisions) > 0 {
		crypto.uiHub.Publish(datura.NewMap(
			"decisions", thesis.Decisions,
		).MarshalAndFree())
	}

	lifecycle := make([]datura.Map[any], 0)

	thesis.Lifecycle.Range(func(key, value any) bool {
		lifecycle = append(lifecycle, datura.NewMap(
			"symbol", key.(string),
			"state", value.(string),
		))

		return true
	})

	if len(lifecycle) > 0 {
		crypto.uiHub.Publish(datura.NewMap("lifecycle", lifecycle).MarshalAndFree())
	}

	if len(thesis.Findings) > 0 {
		crypto.uiHub.Publish(datura.NewMap("findings", thesis.Findings).MarshalAndFree())
	}

	journalChanged := false

	if journalChanged && crypto.uiHub != nil && len(crypto.theses) > 0 {
		crypto.uiHub.Publish(datura.NewMap("journal", crypto.theses).MarshalAndFree())
	}

	if journalChanged && crypto.journal != nil {
		if err := crypto.journal.Save(crypto.theses); err != nil {
			errnie.Error(err)
		}
	}
}

func (crypto *Crypto) Close() (err error) {
	crypto.cancel()
	return nil
}
