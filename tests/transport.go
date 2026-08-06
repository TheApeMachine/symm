package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/tests/fixtures/balances"
	"github.com/theapemachine/symm/tests/fixtures/tradevolume"
	testtypes "github.com/theapemachine/symm/tests/types"
)

const (
	// simulatedTakerFeePercent is the fixture venue's declared taker tier.
	simulatedTakerFeePercent = 0.26
	percentDenominator       = 100.0
)

/*
mockTransport intercepts all HTTP calls from the SDK REST client and responds
with fixture-generated payloads. Every REST endpoint the production Live boot
sequence touches is handled in-memory — no network traffic escapes.

Symbols are configured lazily via configure() so that the transport can be
constructed before the symbol list is known (matching NewConn's signature).
*/
type mockTransport struct {
	mu          sync.RWMutex
	symbols     []*testtypes.Symbol
	tradeVolume *tradevolume.Fixture
	pending     []simulatedOrder
	balances    map[string]string
	trades      map[string]spot.Trade
	openOrders  map[string]spot.Order
	addOrderErr error
}

/*
simulatedOrder is one REST-accepted market order waiting for the simulated
book to publish the next executable price for its own symbol.
*/
type simulatedOrder struct {
	ID       string
	Request  spot.AddOrderRequest
	Quantity float64
}

func newMockTransport() *mockTransport {
	return &mockTransport{}
}

func (transport *mockTransport) configure(symbols []*testtypes.Symbol) {
	transport.mu.Lock()
	defer transport.mu.Unlock()

	transport.symbols = symbols
	pairs := make([]string, len(symbols))

	for index, symbol := range symbols {
		pairs[index] = symbol.Pair
	}

	transport.tradeVolume = tradevolume.NewMarket(pairs)
}

func (transport *mockTransport) configureAccount(
	balances map[string]string,
	trades map[string]spot.Trade,
) {
	transport.mu.Lock()
	defer transport.mu.Unlock()

	transport.balances = balances
	transport.trades = trades
}

func (transport *mockTransport) configureOpenOrders(orders map[string]spot.Order) {
	transport.mu.Lock()
	defer transport.mu.Unlock()

	transport.openOrders = orders
}

func (transport *mockTransport) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	path := request.URL.Path

	transport.mu.RLock()
	addOrderErr := transport.addOrderErr
	transport.mu.RUnlock()

	if path == "/0/private/AddOrder" && addOrderErr != nil {
		return nil, addOrderErr
	}

	var body []byte

	switch {
	case path == "/0/public/Assets":
		body = transport.assets()
	case path == "/0/public/AssetPairs":
		body = transport.assetPairs()
	case path == "/0/public/Time":
		body = transport.serverTime()
	case path == "/0/private/GetWebSocketsToken":
		body = transport.wsToken()
	case path == "/0/private/Balance":
		body = transport.balance()
	case path == "/0/private/TradesHistory":
		body = transport.tradesHistory()
	case path == "/0/private/OpenOrders":
		body = transport.openOrdersResponse()
	case path == "/0/private/CancelOrder":
		body = envelope(map[string]any{"count": 1, "pending": false})
	case path == "/0/private/TradeVolume":
		body = transport.tradeVolumeResponse()
	case path == "/0/private/AddOrder":
		order := spot.AddOrderRequest{}

		if err := json.NewDecoder(request.Body).Decode(&order); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"tests: simulated AddOrder request could not be decoded",
				err,
			))
		}

		orderBody, err := transport.addOrder(order)

		if err != nil {
			return nil, err
		}

		body = orderBody
	default:
		body = envelope(map[string]any{})
	}

	response := &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}

	response.Header.Set("Content-Type", "application/json")

	return response, nil
}

func (transport *mockTransport) getSymbols() []*testtypes.Symbol {
	transport.mu.RLock()
	defer transport.mu.RUnlock()

	return transport.symbols
}

/*
assets builds a Kraken Assets REST response from the simulated symbol list.
Each side of every pair becomes an asset entry.
*/
func (transport *mockTransport) assets() []byte {
	result := map[string]any{}

	for _, symbol := range transport.getSymbols() {
		parts := strings.Split(symbol.Pair, "/")

		for _, asset := range parts {
			if _, exists := result[asset]; exists {
				continue
			}

			result[asset] = map[string]any{
				"aclass":           "currency",
				"altname":          asset,
				"decimals":         8,
				"display_decimals": 2,
				"status":           "enabled",
			}
		}
	}

	return envelope(result)
}

/*
assetPairs builds a Kraken AssetPairs REST response from the simulated symbols.
*/
func (transport *mockTransport) assetPairs() []byte {
	result := map[string]any{}

	for _, symbol := range transport.getSymbols() {
		parts := strings.Split(symbol.Pair, "/")

		if len(parts) != 2 {
			continue
		}

		krakenKey := parts[0] + parts[1]
		priceIncrement := strconv.FormatFloat(
			symbol.PriceIncrement, 'f', symbol.PricePrecision, 64,
		)

		result[krakenKey] = map[string]any{
			"altname":             krakenKey,
			"wsname":              symbol.Pair,
			"aclass_base":         "currency",
			"base":                parts[0],
			"aclass_quote":        "currency",
			"quote":               parts[1],
			"pair_decimals":       symbol.PricePrecision,
			"cost_decimals":       5,
			"lot_decimals":        8,
			"lot_multiplier":      1,
			"leverage_buy":        []int{},
			"leverage_sell":       []int{},
			"fees":                [][]float64{{0, simulatedTakerFeePercent}},
			"fees_maker":          [][]float64{{0, 0.16}},
			"fee_volume_currency": "ZUSD",
			"margin_call":         80,
			"margin_stop":         40,
			"ordermin":            "0.0001",
			"costmin":             "0.50",
			"tick_size":           priceIncrement,
			"status":              "online",
		}
	}

	return envelope(result)
}

/*
takerFee returns the USD-equivalent fee charged by the simulated venue.
Simulator pairs quote in USD, so their quote fee is already USD-equivalent.
*/
func (transport *mockTransport) takerFee(cost float64) float64 {
	return cost * simulatedTakerFeePercent / percentDenominator
}

func (transport *mockTransport) serverTime() []byte {
	now := time.Now()

	return envelope(map[string]any{
		"unixtime": now.Unix(),
		"rfc1123":  now.Format(time.RFC1123),
	})
}

func (transport *mockTransport) wsToken() []byte {
	return envelope(map[string]any{
		"token":   fmt.Sprintf("sim_token_%d", time.Now().UnixNano()),
		"expires": 900,
	})
}

/*
balance generates a REST Balance response using the balances fixture. The
initial wallet is the quote-currency balance from Market's paper config.
*/
func (transport *mockTransport) balance() []byte {
	transport.mu.RLock()
	configured := transport.balances
	transport.mu.RUnlock()

	if configured != nil {
		return envelope(configured)
	}

	fixture := balances.NewMarket("USD")
	var payload []byte

	for frame := range fixture.Generate() {
		payload = frame
	}

	var wsEnvelope map[string]any

	if err := json.Unmarshal(payload, &wsEnvelope); err != nil {
		return envelope(map[string]any{})
	}

	result := map[string]any{}
	data, _ := wsEnvelope["data"].([]any)

	for _, entry := range data {
		row, _ := entry.(map[string]any)
		asset, _ := row["asset"].(string)
		balance := row["balance"]

		if asset != "" {
			result[asset] = fmt.Sprintf("%v", balance)
		}
	}

	return envelope(result)
}

func (transport *mockTransport) tradesHistory() []byte {
	transport.mu.RLock()
	configured := transport.trades
	transport.mu.RUnlock()

	if configured != nil {
		return envelope(map[string]any{
			"count":  len(configured),
			"trades": configured,
		})
	}

	return envelope(map[string]any{
		"count":  0,
		"trades": map[string]any{},
	})
}

func (transport *mockTransport) openOrdersResponse() []byte {
	transport.mu.RLock()
	orders := transport.openOrders
	transport.mu.RUnlock()

	if orders == nil {
		orders = map[string]spot.Order{}
	}

	return envelope(map[string]any{"open": orders})
}

/*
tradeVolumeResponse uses the tradevolume fixture to generate a REST response
with the fee schedule for every simulated symbol.
*/
func (transport *mockTransport) tradeVolumeResponse() []byte {
	transport.mu.RLock()
	tradeVolume := transport.tradeVolume
	transport.mu.RUnlock()

	if tradeVolume == nil {
		return envelope(map[string]any{
			"currency": "ZUSD",
			"volume":   "0",
		})
	}

	var payload []byte

	for frame := range tradeVolume.Generate() {
		payload = frame
	}

	return payload
}

/*
addOrder validates and queues one simulated market order, then returns the
deterministic venue identity the later execution frame will carry.
*/
func (transport *mockTransport) addOrder(
	order spot.AddOrderRequest,
) ([]byte, error) {
	quantity, err := strconv.ParseFloat(order.Volume, 64)

	if order.Pair == "" || order.ClOrdId == "" ||
		order.OrderType != "market" ||
		order.Type != "buy" && order.Type != "sell" ||
		err != nil || quantity <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"tests: simulated market order is incomplete or invalid",
			err,
		))
	}

	sequence := orderCounter.Add(1)
	orderID := fmt.Sprintf("SIM-ORD-%06d", sequence)

	transport.mu.Lock()
	transport.pending = append(transport.pending, simulatedOrder{
		ID:       orderID,
		Request:  order,
		Quantity: quantity,
	})
	transport.mu.Unlock()

	return envelope(map[string]any{
		"descr": map[string]any{
			"order": "simulated order",
		},
		"txid": []string{orderID},
	}), nil
}

/*
takeOrders removes accepted orders for symbol while preserving orders waiting
for other books. A drained order can therefore produce exactly one fill.
*/
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

func envelope(result any) []byte {
	payload, _ := json.Marshal(map[string]any{
		"error":  []string{},
		"result": result,
	})

	return payload
}
