package tests

import (
	"strconv"

	"github.com/theapemachine/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
)

func (transport *mockTransport) addOrder(
	order spot.AddOrderRequest,
) ([]byte, error) {
	quantity, err := strconv.ParseFloat(order.Volume, 64)

	if order.Pair == "" || order.ClOrdId == "" ||
		!supportedOrderType(order.OrderType) ||
		order.Type != "buy" && order.Type != "sell" ||
		err != nil || quantity <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"tests: simulated market order is incomplete or invalid",
			err,
		))
	}

	price, price2, err := parseOrderPrices(order)

	if err != nil {
		return nil, err
	}

	if !transport.knownSymbol(order.Pair) {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"tests: simulated order references an unknown symbol",
			nil,
		))
	}

	_, orderID := transport.nextOrderIdentity()

	transport.mu.Lock()
	transport.pending = append(transport.pending, simulatedOrder{
		ID: orderID, Request: order, Quantity: quantity, Price: price, Price2: price2,
	})
	transport.mu.Unlock()

	return envelope(map[string]any{
		"descr": map[string]any{"order": "simulated order"},
		"txid":  []string{orderID},
	}), nil
}

func supportedOrderType(orderType string) bool {
	switch orderType {
	case "market", "limit", "stop-loss", "stop-loss-limit":
		return true
	default:
		return false
	}
}

func parseOrderPrices(order spot.AddOrderRequest) (float64, float64, error) {
	if order.OrderType == "market" {
		return 0, 0, nil
	}

	price, err := strconv.ParseFloat(order.Price, 64)

	if err != nil || price <= 0 {
		return 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"tests: simulated priced order requires a positive trigger or limit price",
			err,
		))
	}

	if order.OrderType != "stop-loss-limit" {
		return price, 0, nil
	}

	price2, err := strconv.ParseFloat(order.SecondaryPrice, 64)

	if err != nil || price2 <= 0 {
		return 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"tests: simulated stop-loss-limit order requires a positive limit price",
			err,
		))
	}

	return price, price2, nil
}

func (transport *mockTransport) knownSymbol(pair string) bool {
	transport.mu.RLock()
	defer transport.mu.RUnlock()

	for _, symbol := range transport.symbols {
		if symbol.Pair == pair {
			return true
		}
	}

	return false
}

func (transport *mockTransport) takeOrders(symbol string) []simulatedOrder {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	ready := []simulatedOrder{}
	waiting := transport.pending[:0]

	for _, order := range transport.pending {
		if order.Request.Pair == symbol {
			ready = append(ready, order)
			continue
		}

		waiting = append(waiting, order)
	}

	transport.pending = waiting

	return ready
}
