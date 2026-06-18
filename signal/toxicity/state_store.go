package toxicity

import (
	"strconv"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/nomagique/statistic"
)

type symbolSlot struct {
	state atomic.Pointer[symbolState]
}

func copyObservationRing(
	from *statistic.ObservationRing,
	into *statistic.ObservationRing,
) {
	if from == nil || into == nil {
		return
	}

	for _, sample := range from.Samples() {
		into.Observe(sample)
	}
}

func cloneSymbolState(state *symbolState) *symbolState {
	if state == nil {
		return nil
	}

	next := newSymbolState(state.pair)
	next.flow = state.flow
	next.mid = state.mid
	next.lastPrice = state.lastPrice
	next.spreadPctEMA = state.spreadPctEMA
	next.peakVacuumRatio = state.peakVacuumRatio
	next.lastTradeAt = state.lastTradeAt
	next.hasLastTradeAt = state.hasLastTradeAt
	next.lastBookPulse = state.lastBookPulse
	next.lastEventAt = state.lastEventAt

	copyObservationRing(state.timing.TradeIntervals, next.timing.TradeIntervals)
	copyObservationRing(state.timing.LevelLifetimes, next.timing.LevelLifetimes)
	copyObservationRing(state.timing.BookPulseIntervals, next.timing.BookPulseIntervals)
	copyObservationRing(state.timing.ChurnDurations, next.timing.ChurnDurations)

	copyObservationRing(state.gates.ChurnRatios, next.gates.ChurnRatios)
	copyObservationRing(state.gates.FillMatchRatios, next.gates.FillMatchRatios)
	copyObservationRing(state.gates.CancelQtys, next.gates.CancelQtys)
	copyObservationRing(state.gates.LevelSizeFracs, next.gates.LevelSizeFracs)
	copyObservationRing(state.gates.VacuumRatios, next.gates.VacuumRatios)

	copyObservationRing(state.priceIncrements, next.priceIncrements)
	next.toxic = state.toxic.Clone()

	for key, order := range state.orders {
		orderCopy := *order
		next.orders[key] = &orderCopy
	}

	for key, level := range state.levels {
		levelCopy := *level
		next.levels[key] = &levelCopy
	}

	for key, window := range state.churn {
		windowCopy := *window
		next.churn[key] = &windowCopy
	}

	if len(state.trades) > 0 {
		next.trades = append([]tradePrint(nil), state.trades...)
	}

	return next
}

func (tracker *Tracker) loadSlot(symbol string, pair Pair) *symbolSlot {
	if raw, ok := tracker.symbols.Load(symbol); ok {
		slot, slotOK := raw.(*symbolSlot)

		if slotOK {
			return slot
		}
	}

	slot := &symbolSlot{}
	actual, _ := tracker.symbols.LoadOrStore(symbol, slot)
	loaded, _ := actual.(*symbolSlot)

	if loaded.state.Load() == nil {
		fresh := newSymbolState(pair)
		loaded.state.CompareAndSwap(nil, fresh)
	}

	return loaded
}

func (tracker *Tracker) mutateState(
	symbol string,
	pair Pair,
	at time.Time,
	bookPulse bool,
	role string,
	work func(*symbolState),
) {
	if tracker == nil || work == nil {
		return
	}

	slot := tracker.loadSlot(symbol, pair)

	for {
		current := slot.state.Load()

		if current == nil {
			fresh := newSymbolState(pair)
			slot.state.CompareAndSwap(nil, fresh)
			continue
		}

		next := cloneSymbolState(current)

		if bookPulse {
			next.recordBookPulse(at)
		}

		next.recordEventAt(at)
		work(next)

		if slot.state.CompareAndSwap(current, next) {
			tracker.recordObservation(symbol, role, at)
			return
		}
	}
}

func (tracker *Tracker) recordObservation(symbol, role string, eventAt time.Time) {
	if tracker == nil || tracker.observations == nil || eventAt.IsZero() {
		return
	}

	key := []byte(
		"toxicity/" + symbol + "/" + role + "/" +
			strconv.FormatInt(eventAt.UnixNano(), 36) + ".",
	)

	tracker.observations.Insert(key, []byte(role))
}

func newObservationTree() *dmt.Tree {
	return dmt.NewTree("")
}
