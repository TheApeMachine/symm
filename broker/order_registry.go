package broker

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
)

type OrderPhase string

const (
	OrderPhaseSubmitted OrderPhase = "submitted"
	OrderPhaseFilled    OrderPhase = "filled"
	OrderPhaseRejected  OrderPhase = "rejected"
	OrderPhaseCancelled OrderPhase = "cancelled"
)

/*
TrackedOrder is the desk lifecycle record for one client order id.
*/
type TrackedOrder struct {
	ClOrdID   string
	Phase     OrderPhase
	Frame     types.KrakenMessage
	QueuedAt  time.Time
	UpdatedAt time.Time
}

/*
OrderRegistry tracks open desk orders without mutexes.
*/
type OrderRegistry struct {
	orders sync.Map
}

func NewOrderRegistry() *OrderRegistry {
	return &OrderRegistry{}
}

func (registry *OrderRegistry) Store(clOrdID string, frame types.KrakenMessage) {
	if registry == nil || clOrdID == "" {
		return
	}

	now := time.Now().UTC()
	registry.orders.Store(clOrdID, &TrackedOrder{
		ClOrdID:   clOrdID,
		Phase:     OrderPhaseSubmitted,
		Frame:     frame,
		QueuedAt:  now,
		UpdatedAt: now,
	})
}

func (registry *OrderRegistry) Load(clOrdID string) (types.KrakenMessage, bool) {
	if registry == nil || clOrdID == "" {
		return types.KrakenMessage{}, false
	}

	raw, ok := registry.orders.Load(clOrdID)

	if !ok {
		return types.KrakenMessage{}, false
	}

	tracked, typed := raw.(*TrackedOrder)

	if !typed || tracked == nil {
		return types.KrakenMessage{}, false
	}

	return tracked.Frame, true
}

func (registry *OrderRegistry) MarkFilled(clOrdID string) {
	registry.transition(clOrdID, OrderPhaseFilled)
}

func (registry *OrderRegistry) MarkCancelled(clOrdID string) {
	registry.transition(clOrdID, OrderPhaseCancelled)
}

func (registry *OrderRegistry) MarkRejected(clOrdID string) {
	registry.transition(clOrdID, OrderPhaseRejected)
}

func (registry *OrderRegistry) Delete(clOrdID string) {
	if registry == nil || clOrdID == "" {
		return
	}

	registry.orders.Delete(clOrdID)
}

func (registry *OrderRegistry) transition(clOrdID string, phase OrderPhase) {
	if registry == nil || clOrdID == "" {
		return
	}

	raw, ok := registry.orders.Load(clOrdID)

	if !ok {
		return
	}

	tracked, typed := raw.(*TrackedOrder)

	if !typed || tracked == nil {
		return
	}

	tracked.Phase = phase
	tracked.UpdatedAt = time.Now().UTC()
	registry.orders.Store(clOrdID, tracked)
}

func (registry *OrderRegistry) RejectStaleEntries() []string {
	if registry == nil {
		return nil
	}

	ttl := trading.EntryTransitTTL()
	now := time.Now().UTC()
	rejected := make([]string, 0)

	registry.orders.Range(func(key any, value any) bool {
		clOrdID, _ := key.(string)
		tracked, ok := value.(*TrackedOrder)

		if !ok || tracked == nil || tracked.Phase != OrderPhaseSubmitted {
			return true
		}

		params, paramsOK := addParamsFromFrame(tracked.Frame)

		if !paramsOK || params.EntryQueuedAt.IsZero() {
			return true
		}

		if now.Sub(params.EntryQueuedAt) <= ttl {
			return true
		}

		tracked.Phase = OrderPhaseRejected
		tracked.UpdatedAt = now
		registry.orders.Store(clOrdID, tracked)
		rejected = append(rejected, clOrdID)

		return true
	})

	return rejected
}

func addParamsFromFrame(frame types.KrakenMessage) (trading.AddParams, bool) {
	switch typed := frame.Params.(type) {
	case trading.AddParams:
		return typed, true
	case *trading.AddParams:
		if typed == nil {
			return trading.AddParams{}, false
		}

		return *typed, true
	default:
		return trading.AddParams{}, false
	}
}

func RejectStaleEntry(params *trading.AddParams) error {
	if params == nil || params.EntryQueuedAt.IsZero() {
		return nil
	}

	return trading.RejectStaleEntry(params)
}
