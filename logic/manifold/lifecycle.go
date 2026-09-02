package manifold

import (
	"fmt"
	"sort"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

type orderIdentity struct {
	symbol  string
	orderID string
}

/* orderLifecycle owns the ContentIDs of orders currently resident in Sensorium. */
type orderLifecycle struct {
	byContent map[int64]orderIdentity
	bySymbol  map[string]map[int64]struct{}
}

func newOrderLifecycle() *orderLifecycle {
	return &orderLifecycle{
		byContent: make(map[int64]orderIdentity),
		bySymbol:  make(map[string]map[int64]struct{}),
	}
}

/*
Apply validates one authoritative Level3 lifecycle message and returns the
ContentIDs that explicitly departed before its resting orders are projected.
*/
func (lifecycle *orderLifecycle) Apply(message kraken.Level3Data) ([]int64, error) {
	if lifecycle == nil || message.Symbol == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: Level3 lifecycle requires a symbol",
			nil,
		))
	}

	departures := lifecycle.resetSnapshot(message)

	for _, orders := range [][]kraken.Level3Order{message.Bids, message.Asks} {
		for _, order := range orders {
			departure, err := lifecycle.applyOrder(message.Symbol, order)

			if err != nil {
				return nil, err
			}

			if departure != 0 {
				departures = append(departures, departure)
			}
		}
	}

	return departures, nil
}

func (lifecycle *orderLifecycle) resetSnapshot(message kraken.Level3Data) []int64 {
	if message.Type != "snapshot" {
		return nil
	}

	residents := lifecycle.bySymbol[message.Symbol]
	departures := make([]int64, 0, len(residents))

	for contentID := range residents {
		departures = append(departures, contentID)
		delete(lifecycle.byContent, contentID)
	}

	delete(lifecycle.bySymbol, message.Symbol)
	sort.Slice(departures, func(left, right int) bool {
		return departures[left] < departures[right]
	})

	return departures
}

func (lifecycle *orderLifecycle) applyOrder(
	symbol string,
	order kraken.Level3Order,
) (int64, error) {
	if order.OrderID == "" {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("manifold: %s Level3 order has no identity", symbol),
			nil,
		))
	}

	identity := orderIdentity{symbol: symbol, orderID: order.OrderID}
	contentID := orderContentID(identity)
	resident, found := lifecycle.byContent[contentID]

	if found && resident != identity {
		return 0, errnie.Error(errnie.Err(
			errnie.Conflict,
			fmt.Sprintf(
				"manifold: content identity collision id=%d resident=%s/%s incoming=%s/%s",
				contentID,
				resident.symbol,
				resident.orderID,
				identity.symbol,
				identity.orderID,
			),
			nil,
		))
	}

	if order.Event == "delete" {
		return lifecycle.delete(identity, contentID, found)
	}

	if !order.Resting() {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("manifold: %s/%s does not describe a resting order", symbol, order.OrderID),
			nil,
		))
	}

	if found {
		return 0, nil
	}

	lifecycle.byContent[contentID] = identity

	if lifecycle.bySymbol[symbol] == nil {
		lifecycle.bySymbol[symbol] = make(map[int64]struct{})
	}

	lifecycle.bySymbol[symbol][contentID] = struct{}{}

	return 0, nil
}

func (lifecycle *orderLifecycle) delete(
	identity orderIdentity,
	contentID int64,
	found bool,
) (int64, error) {
	if !found {
		return 0, errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf(
				"manifold: delete names non-resident order %s/%s",
				identity.symbol,
				identity.orderID,
			),
			nil,
		))
	}

	delete(lifecycle.byContent, contentID)
	delete(lifecycle.bySymbol[identity.symbol], contentID)

	if len(lifecycle.bySymbol[identity.symbol]) == 0 {
		delete(lifecycle.bySymbol, identity.symbol)
	}

	return contentID, nil
}

/* orderContentID is a stable 63-bit FNV-1a identity over symbol and order ID. */
func orderContentID(identity orderIdentity) int64 {
	const (
		offsetBasis = uint64(14695981039346656037)
		prime       = uint64(1099511628211)
		positive    = ^uint64(0) >> 1
	)

	hash := offsetBasis
	fold := func(value string) {
		length := uint64(len(value))

		for shift := 0; shift < 64; shift += 8 {
			hash ^= uint64(byte(length >> shift))
			hash *= prime
		}

		for index := 0; index < len(value); index++ {
			hash ^= uint64(value[index])
			hash *= prime
		}
	}

	fold(identity.symbol)
	fold(identity.orderID)

	return int64(hash & positive)
}
