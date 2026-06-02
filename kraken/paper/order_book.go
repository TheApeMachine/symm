package paper

import (
	"github.com/theapemachine/symm/kraken/trading"
)

type openOrder struct {
	orderID    string
	clOrdID    string
	symbol     string
	side       trading.Side
	orderType  trading.OrderType
	orderQty   float64
	limitPrice float64
	postOnly   bool
}

func (orders *Orders) storeOrder(order *openOrder) {
	orders.mu.Lock()
	orders.open[order.orderID] = order
	orders.mu.Unlock()
}

func (orders *Orders) takeOrder(orderID string) (*openOrder, bool) {
	orders.mu.Lock()
	order, ok := orders.open[orderID]
	delete(orders.open, orderID)
	orders.mu.Unlock()

	return order, ok
}

func (orders *Orders) orderByID(orderID string) (*openOrder, bool) {
	orders.mu.RLock()
	order, ok := orders.open[orderID]
	orders.mu.RUnlock()

	return order, ok
}

func (orders *Orders) orderByClOrdID(clOrdID string) (*openOrder, bool) {
	orders.mu.RLock()
	defer orders.mu.RUnlock()

	for _, order := range orders.open {
		if order.clOrdID == clOrdID {
			return order, true
		}
	}

	return nil, false
}

func (orders *Orders) openOrderIDs() []string {
	orders.mu.RLock()
	defer orders.mu.RUnlock()

	ids := make([]string, 0, len(orders.open))

	for orderID := range orders.open {
		ids = append(ids, orderID)
	}

	return ids
}

func (orders *Orders) amendStored(order *openOrder, qty, limitPrice float64) {
	orders.mu.Lock()

	if qty > 0 {
		order.orderQty = qty
	}

	if limitPrice > 0 {
		order.limitPrice = limitPrice
	}

	orders.mu.Unlock()
}
