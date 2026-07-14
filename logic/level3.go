package logic

import (
	"context"
	"iter"
	"sort"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
level3Source supplies the SDK-managed books owned by the Kraken transport.
Keeping this boundary read-only lets Level3 consume the real book directly.
*/
type level3Source interface {
	Books() iter.Seq[*spot.BookManager]
}

/*
Level3 owns every book-derived analysis responsibility composed by Analyzer:
the SDK book source, field engine, persistent per-symbol models, and replay.
*/
type Level3 struct {
	ctx        context.Context
	cancel     context.CancelFunc
	source     level3Source
	engine     *manifold.Engine
	resonances map[string]*Resonance
	causals    map[string]*Causal
	replay     *manifold.ReplayRecorder
}

/*
level3Advance identifies one changed SDK book ready for field evolution.
*/
type level3Advance struct {
	symbol string
	slot   *manifold.Slot
}

/*
NewLevel3 constructs the complete Level3 analysis path around the transport's
managed books without creating another order-book representation in logic.
*/
func NewLevel3(ctx context.Context, source level3Source) *Level3 {
	ctx, cancel := context.WithCancel(ctx)

	return &Level3{
		ctx:        ctx,
		cancel:     cancel,
		source:     source,
		engine:     manifold.NewEngine(),
		resonances: make(map[string]*Resonance),
		causals:    make(map[string]*Causal),
		replay:     manifold.NewReplayRecorder(),
	}
}

/*
Close cancels Level3 ownership and releases every admitted field solver.
*/
func (level3 *Level3) Close() {
	level3.cancel()
	level3.engine.Close()
}

/*
Update processes each current SDK book once after signal measurements arrive.
Unchanged books do not advance their field slot, so trading ticks cannot invent
market epochs in the absence of new Level3 state.
*/
func (level3 *Level3) Update(thesis *types.Thesis) {
	if level3.source == nil {
		return
	}

	select {
	case <-level3.ctx.Done():
		return
	default:
	}

	advances := make([]level3Advance, 0)

	for manager := range level3.source.Books() {
		symbols := manager.GetBooks()
		sort.Strings(symbols)

		for _, symbol := range symbols {
			if advance := level3.observe(thesis, manager.GetBook(symbol)); advance != nil {
				advances = append(advances, *advance)
			}
		}
	}

	sort.Slice(advances, func(left, right int) bool {
		return advances[left].symbol < advances[right].symbol
	})

	for _, advance := range advances {
		level3.apply(thesis, advance.symbol, advance.slot.Advance())
	}
}

/*
observe reconciles one SDK book with the persistent solver carriers for its
symbol.
*/
func (level3 *Level3) observe(
	thesis *types.Thesis,
	managed *book.Book,
) *level3Advance {
	if managed == nil || strings.TrimSpace(managed.Name) == "" {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"logic level3: managed book symbol required",
			nil,
		))

		return nil
	}

	slot := level3.admit(managed.Name)

	if slot == nil {
		return nil
	}

	observed := slot.ObserveBook(managed)

	if observed.StateProduced {
		thesis.Manifold = append(thesis.Manifold, observed.State)
	}

	if !observed.AdvanceReady {
		return nil
	}

	return &level3Advance{symbol: managed.Name, slot: slot}
}

/*
admit obtains the persistent field slot and initializes the symbol models that
consume its states when a book first reaches Level3 analysis.
*/
func (level3 *Level3) admit(symbol string) *manifold.Slot {
	slot, err := level3.engine.Admit(symbol)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"logic level3: field admission failed",
			err,
		))

		return nil
	}

	if level3.resonances[symbol] == nil {
		level3.resonances[symbol] = NewResonance(
			symbol, level3.engine.Halflife(),
		)
	}

	if level3.causals[symbol] == nil {
		level3.causals[symbol] = NewCausal(symbol)
	}

	return slot
}

/*
apply records one field result and advances the symbol's persistent resonance
and causal models on the same Thesis consumed by the remaining logic stage.
*/
func (level3 *Level3) apply(
	thesis *types.Thesis,
	symbol string,
	result manifold.ProcessResult,
) {
	if result.StateProduced {
		thesis.Manifold = append(thesis.Manifold, result.State)
	}

	if result.Forecast != nil {
		thesis.Forecasts = append(thesis.Forecasts, *result.Forecast)

		if err := thesis.Transition(
			symbol, types.LifecycleShaped, result.Forecast.At,
		); err != nil {
			errnie.Error(err)
		}
	}

	if !result.GasReady {
		return
	}

	if !level3.replay.Record(
		symbol,
		result.Observation,
		result.State,
		result.Accounting,
		result.CohortCount,
		result.OrderCount,
		result.DepositCount,
	) {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"logic level3: replay record dropped",
			nil,
		))
	}

	level3.models(thesis, symbol, result.State)
}

/*
models advances the symbol's persistent resonance and causal interpretations
from one gas-ready field state and retains their durable evidence.
*/
func (level3 *Level3) models(
	thesis *types.Thesis,
	symbol string,
	state manifold.State,
) {
	measurements, resonance := level3.resonances[symbol].Update(state)
	thesis.Measurements = append(thesis.Measurements, measurements...)

	if resonance != nil {
		thesis.Resonance = append(thesis.Resonance, *resonance)
	}

	hypothesis, causal, err := level3.causals[symbol].Update(state)

	if err != nil {
		errnie.Error(err)
		return
	}

	if causal != nil {
		thesis.Hypotheses = append(thesis.Hypotheses, hypothesis)
		thesis.Causal = append(thesis.Causal, *causal)
	}
}
