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
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/tests/fixtures/tradevolume"
	testtypes "github.com/theapemachine/symm/tests/types"
)

const (
	// simulatedTakerFeePercent is the fixture venue's declared taker tier.
	simulatedTakerFeePercent = testtypes.DefaultTakerFeePercent
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
	mu           sync.RWMutex
	symbols      []*testtypes.Symbol
	tradeVolume  *tradevolume.Fixture
	pending      []simulatedOrder
	balances     map[string]string
	prices       map[string]float64
	basis        map[string]float64
	trades       map[string]spot.Trade
	openOrders   map[string]spot.Order
	addOrderErr  error
	faults       *faultInjector
	clock        time.Time
	orderCounter atomic.Uint64
}

/*
simulatedOrder is one REST-accepted market order waiting for the simulated
book to publish the next executable price for its own symbol.
*/
type simulatedOrder struct {
	ID       string
	Request  spot.AddOrderRequest
	Quantity float64
	Price    float64
	Price2   float64
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		clock:  testtypes.DefaultScenarioStart,
		prices: map[string]float64{},
		basis:  map[string]float64{},
	}
}

func (transport *mockTransport) configureTime(start time.Time) {
	transport.mu.Lock()
	transport.clock = start
	transport.mu.Unlock()
}

func (transport *mockTransport) configure(symbols []*testtypes.Symbol) {
	transport.mu.Lock()
	defer transport.mu.Unlock()

	transport.symbols = symbols
	transport.tradeVolume = tradevolume.NewProfiles(symbols)
}

func (transport *mockTransport) configureFaults(faults *faultInjector) {
	transport.mu.Lock()
	transport.faults = faults
	transport.mu.Unlock()
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
	faults := transport.faults
	transport.mu.RUnlock()

	if faults != nil {
		delay := faults.RESTDelay(path)

		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()

			select {
			case <-timer.C:
			case <-request.Context().Done():
				return nil, request.Context().Err()
			}
		}
	}

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
	case path == "/0/private/TradeBalance":
		body = transport.tradeBalance()
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
			"lot_decimals":        symbol.QuantityPrecision,
			"lot_multiplier":      1,
			"leverage_buy":        []int{},
			"leverage_sell":       []int{},
			"fees":                [][]float64{{0, symbol.TakerFeePercent}},
			"fees_maker":          [][]float64{{0, symbol.MakerFeePercent}},
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

func (transport *mockTransport) orderFee(
	symbol string,
	cost float64,
	maker bool,
) (float64, error) {
	transport.mu.RLock()
	defer transport.mu.RUnlock()

	for _, profile := range transport.symbols {
		if profile.Pair != symbol {
			continue
		}

		feePercent := profile.TakerFeePercent

		if maker {
			feePercent = profile.MakerFeePercent
		}

		return cost * feePercent / percentDenominator, nil
	}

	return 0, fmt.Errorf("tests: cannot price a fee for unknown symbol %q", symbol)
}

func (transport *mockTransport) serverTime() []byte {
	transport.mu.RLock()
	now := transport.clock
	transport.mu.RUnlock()

	return envelope(map[string]any{
		"unixtime": now.Unix(),
		"rfc1123":  now.Format(time.RFC1123),
	})
}

func (transport *mockTransport) wsToken() []byte {
	transport.mu.RLock()
	clock := transport.clock
	transport.mu.RUnlock()

	return envelope(map[string]any{
		"token":   fmt.Sprintf("sim_token_%d", clock.UnixNano()),
		"expires": 900,
	})
}

func (transport *mockTransport) nextOrderIdentity() (uint64, string) {
	sequence := transport.orderCounter.Add(1)

	return sequence, fmt.Sprintf("SIM-ORD-%06d", sequence)
}

func envelope(result any) []byte {
	payload, err := json.Marshal(map[string]any{
		"error":  []string{},
		"result": result,
	})

	if err != nil {
		panic(fmt.Errorf("tests: encode fixture REST envelope: %w", err))
	}

	return payload
}
