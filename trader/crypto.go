package trader

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic/category"
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

		if desk != nil {
			desk.SeedHoldings(crypto.savedHoldings())
		}

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
			crypto.exit(thesis, decision)
		}
	}

	for index := range decisions {
		decision := &decisions[index]

		if decision.Action != types.ActionEnter {
			continue
		}

		if decision.Displaces != "" {
			if _, open := crypto.desk.Position(decision.Displaces); open {
				// Wait for the displaced lot to leave before entering.
				continue
			}
		}

		crypto.enter(thesis, decision)
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

	journalChanged := crypto.captureJournal(thesis)

	if journalChanged && crypto.uiHub != nil && len(crypto.theses) > 0 {
		crypto.uiHub.Publish(datura.NewMap("journal", crypto.theses).MarshalAndFree())
	}

	if journalChanged && crypto.journal != nil {
		if err := crypto.journal.Save(crypto.theses); err != nil {
			errnie.Error(err)
		}
	}
}

func (crypto *Crypto) captureJournal(thesis *types.Thesis) bool {
	if thesis == nil {
		return false
	}

	changed := false

	thesis.Lifecycle.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok || symbol == "" {
			return true
		}

		lifecycle, _ := value.(string)

		if lifecycle == "" || crypto.hasJournalLifecycle(symbol, lifecycle) {
			return true
		}

		if snapshot := crypto.snapshotThesis(thesis, symbol); snapshot != nil {
			crypto.theses = append(crypto.theses, snapshot)
			changed = true
		}
		return true
	})

	return changed
}

func (crypto *Crypto) hasJournalLifecycle(symbol, lifecycle string) bool {
	for index := len(crypto.theses) - 1; index >= 0; index-- {
		entry := crypto.theses[index]

		if entry == nil {
			continue
		}

		value, ok := entry.Lifecycle.Load(symbol)

		if !ok {
			continue
		}

		return value == lifecycle
	}

	return false
}

func (crypto *Crypto) snapshotThesis(thesis *types.Thesis, symbol string) *types.Thesis {
	if thesis == nil || symbol == "" {
		return nil
	}

	snapshot := types.NewThesis()
	snapshot.Tick = thesis.Tick
	snapshot.At = thesis.At

	for _, decision := range thesis.Decisions {
		if decision.Symbol == symbol {
			snapshot.Decisions = append(snapshot.Decisions, decision)
		}
	}

	if value, ok := thesis.Holdings.Load(symbol); ok {
		if holding, ok := value.(*types.Holding); ok && holding != nil {
			snapshot.Holdings.Store(symbol, freezeHolding(holding))
		}
	}

	if value, ok := thesis.Lifecycle.Load(symbol); ok {
		snapshot.Lifecycle.Store(symbol, value)
	}

	for _, finding := range thesis.Findings {
		if finding.Symbol == symbol {
			snapshot.Findings = append(snapshot.Findings, finding)
		}
	}

	if thesis.Graphs != nil {
		thesis.Graphs.Range(func(key, value any) bool {
			name, ok := key.(string)

			if !ok {
				return true
			}

			if name != "categories" {
				return true
			}

			graph, ok := value.(*category.Graph)

			if ok && graph != nil {
				snapshot.Graphs.Store(name, freezeCategoryGraph(graph))
			}

			return true
		})
	}

	return snapshot
}

func freezeHolding(holding *types.Holding) *types.Holding {
	if holding == nil {
		return nil
	}

	frozen := *holding

	if holding.Qty != nil {
		frozen.Qty = holding.Qty.Copy()
	}

	if holding.SellableQty != nil {
		frozen.SellableQty = holding.SellableQty.Copy()
	}

	if holding.EntryPrice != nil {
		frozen.EntryPrice = holding.EntryPrice.Copy()
	}

	if holding.EntryFee != nil {
		frozen.EntryFee = holding.EntryFee.Copy()
	}

	if holding.ExitPrice != nil {
		frozen.ExitPrice = holding.ExitPrice.Copy()
	}

	if holding.ExitFee != nil {
		frozen.ExitFee = holding.ExitFee.Copy()
	}

	if holding.PnL != nil {
		frozen.PnL = holding.PnL.Copy()
	}

	if holding.Mark != nil {
		frozen.Mark = holding.Mark.Copy()
	}

	if holding.ReturnPct != nil {
		value := *holding.ReturnPct
		frozen.ReturnPct = &value
	}

	if holding.EntryAt != nil {
		value := *holding.EntryAt
		frozen.EntryAt = &value
	}

	if holding.ExitAt != nil {
		value := *holding.ExitAt
		frozen.ExitAt = &value
	}

	if holding.Stoploss != nil {
		stoploss := *holding.Stoploss

		if holding.Stoploss.Entry != nil {
			stoploss.Entry = holding.Stoploss.Entry.Copy()
		}

		if holding.Stoploss.Peak != nil {
			stoploss.Peak = holding.Stoploss.Peak.Copy()
		}

		if holding.Stoploss.Mark != nil {
			stoploss.Mark = holding.Stoploss.Mark.Copy()
		}

		if holding.Stoploss.Floor != nil {
			stoploss.Floor = holding.Stoploss.Floor.Copy()
		}

		stoploss.Actor = nil
		frozen.Stoploss = &stoploss
	}

	return &frozen
}

func freezeCategoryGraph(graph *category.Graph) *category.Graph {
	if graph == nil {
		return nil
	}

	frozen := &category.Graph{
		Nodes:  make([]*category.Node, 0, len(graph.Nodes)),
		Edges:  make([]*category.Relation, 0, len(graph.Edges)),
		Priors: make(map[string]types.CategoryType, len(graph.Priors)),
	}

	for _, node := range graph.Nodes {
		if node == nil {
			continue
		}

		copyNode := *node
		frozen.Nodes = append(frozen.Nodes, &copyNode)
	}

	for _, relation := range graph.Edges {
		if relation == nil {
			continue
		}

		copyRelation := *relation

		if relation.Evidence != nil {
			copyRelation.Evidence = append([]string(nil), relation.Evidence...)
		}

		frozen.Edges = append(frozen.Edges, &copyRelation)
	}

	for symbol, prior := range graph.Priors {
		frozen.Priors[symbol] = prior
	}

	return frozen
}

func (crypto *Crypto) savedHoldings() []*types.Holding {
	if len(crypto.theses) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	holdings := make([]*types.Holding, 0)

	for index := len(crypto.theses) - 1; index >= 0; index-- {
		thesis := crypto.theses[index]

		if thesis == nil || thesis.Holdings == nil {
			continue
		}

		thesis.Holdings.Range(func(key, value any) bool {
			symbol, ok := key.(string)

			if !ok || symbol == "" {
				return true
			}

			if _, exists := seen[symbol]; exists {
				return true
			}

			holding, ok := value.(*types.Holding)

			if !ok || holding == nil {
				return true
			}

			seen[symbol] = struct{}{}
			holdings = append(holdings, freezeHolding(holding))
			return true
		})
	}

	return holdings
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
		holding = types.NewHolding(crypto.ctx, decision.Symbol, decision.ProposedQuantity, decision.ReferencePrice, nil, nil, nil)
		thesis.Holdings.Store(decision.Symbol, holding)
	}

	if holding.Qty == nil || holding.Qty.Sign() <= 0 {
		holding.Qty = decision.ProposedQuantity.Copy()
	}

	if _, err := crypto.desk.BuyHolding(holding, holding.IsOpportunity); err != nil {
		errnie.Error(err)

		if holding.ReservationID != "" {
			if releaseErr := crypto.desk.Balance().Release(holding.ReservationID); releaseErr != nil {
				errnie.Error(releaseErr)
			}
		}

		thesis.Holdings.Delete(decision.Symbol)
		return
	}

	if holding.ReservationID != "" {
		if commitErr := crypto.desk.Balance().Commit(holding.ReservationID); commitErr != nil {
			errnie.Error(commitErr)
		}
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
