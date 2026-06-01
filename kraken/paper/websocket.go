package paper

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/user"
)

const (
	methodAddOrder = "add_order"

	// Kraken starter-tier fees (0-volume bracket). Loaded from AssetPairs on start;
	// these conservative values are the fallback if the REST call fails.
	defaultTakerFeePct = 0.40 // 0.40%
	defaultMakerFeePct = 0.25 // 0.25%

	// Slippage/spread constants for simulated market impact (basis points).
	halfSpreadBps      = 2.5 // half bid-ask spread on market fills
	marketSlippageBps  = 1.0 // additional market-order impact
	triggerSlippageBps = 0.5 // impact on stop/take-profit trigger fills
)

// pairMeta caches fee rates and tick size for one trading pair.
type pairMeta struct {
	takerPct float64 // e.g. 0.26 means 0.26 %
	makerPct float64
	tickSize float64
	quote    string // e.g. "USD" from "BTC/USD"
}

type WebSocket struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber

	priceMu   sync.RWMutex
	lastPrice map[string]float64 // symbol → last mid price (from ticker feed)

	metaMu   sync.RWMutex
	pairMeta map[string]*pairMeta // symbol → fee + tick metadata

	wallet   *Wallet
	sequence int
}

func NewWebSocket(ctx context.Context, pool *qpool.Q) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	ws := &WebSocket{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		lastPrice:   make(map[string]float64),
		pairMeta:    make(map[string]*pairMeta),
		wallet: NewWallet(
			resolvedQuote(),
			viper.GetFloat64("trading.paper.wallet_eur"),
		),
	}

	for _, channel := range []string{
		"raw", "orders", "ticker", "balances", "kraken:public",
	} {
		ws.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		ws.subscribers[channel] = ws.broadcasts[channel].Subscribe(channel, 128)
	}

	// Load real fee schedules and tick sizes asynchronously; defaults are used
	// until the REST call completes.
	go ws.loadPairMeta()

	return ws
}

// loadPairMeta fetches the Kraken AssetPairs REST endpoint and caches the
// base-tier maker/taker fees and tick size for every trading pair.
func (ws *WebSocket) loadPairMeta() {
	rest := public.NewRest(ws.ctx, public.EndpointTypeAssetPairs)

	// Minimal struct — only the fields we need.
	var pairs map[string]*struct {
		Wsname    string      `json:"wsname"`
		Fees      [][]float64 `json:"fees"`
		FeesMaker [][]float64 `json:"fees_maker"`
		TickSize  string      `json:"tick_size"`
	}

	if err := rest.Get(ws.ctx, fiber.Map{}, &pairs); err != nil {
		errnie.Error(err)
		return
	}

	ws.metaMu.Lock()
	defer ws.metaMu.Unlock()

	for _, pair := range pairs {
		if pair == nil || pair.Wsname == "" {
			continue
		}

		pm := &pairMeta{
			takerPct: defaultTakerFeePct,
			makerPct: defaultMakerFeePct,
			tickSize: 0.01,
			quote:    quoteAsset(pair.Wsname),
		}

		if len(pair.Fees) > 0 && len(pair.Fees[0]) >= 2 {
			pm.takerPct = pair.Fees[0][1]
		}
		if len(pair.FeesMaker) > 0 && len(pair.FeesMaker[0]) >= 2 {
			pm.makerPct = pair.FeesMaker[0][1]
		}
		if pair.TickSize != "" {
			if ts, err := strconv.ParseFloat(pair.TickSize, 64); err == nil && ts > 0 {
				pm.tickSize = ts
			}
		}

		ws.pairMeta[pair.Wsname] = pm
	}
}

func (ws *WebSocket) Connect(endpoint public.EndpointType, channel string) error {
	return nil
}

func (ws *WebSocket) Tick() error {
	orders := ws.subscribers["orders"].Incoming
	ticker := ws.subscribers["ticker"].Incoming
	outbound := ws.subscribers["kraken:public"].Incoming

	for {
		select {
		case <-ws.ctx.Done():
			return ws.ctx.Err()

		case msg, ok := <-ticker:
			if !ok {
				continue
			}
			if msg != nil && msg.Value != nil {
				ws.updatePrice(msg.Value)
			}

		case msg, ok := <-outbound:
			if !ok {
				continue
			}

			if msg == nil || msg.Value == nil {
				continue
			}

			ws.handleOutbound(msg.Value)

		case msg, ok := <-orders:
			if !ok {
				return ws.ctx.Err()
			}
			if msg == nil || msg.Value == nil {
				continue
			}

			payload, err := sonic.Marshal(msg.Value)
			if err != nil {
				errnie.Error(err)
				continue
			}

			var frame struct {
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if err := sonic.Unmarshal(payload, &frame); err != nil {
				errnie.Error(err)
				continue
			}

			if frame.Method == methodAddOrder {
				ws.simulateAddOrder(frame.Params)
			}
		}
	}
}

// updatePrice extracts bid/ask/last from a ticker broadcast message and stores
// the mid price per symbol so market orders have a reference price.
func (ws *WebSocket) updatePrice(value any) {
	var sm public.SocketMessage

	switch v := value.(type) {
	case public.SocketMessage:
		sm = v
	case *public.SocketMessage:
		if v == nil {
			return
		}
		sm = *v
	default:
		return
	}

	var rows []struct {
		Symbol string  `json:"symbol"`
		Bid    float64 `json:"bid"`
		Ask    float64 `json:"ask"`
		Last   float64 `json:"last"`
	}

	if err := sonic.Unmarshal(sm.Data, &rows); err != nil {
		return
	}

	ws.priceMu.Lock()
	defer ws.priceMu.Unlock()

	for _, row := range rows {
		if row.Symbol == "" {
			continue
		}

		mid := row.Last
		if row.Bid > 0 && row.Ask > 0 {
			mid = (row.Bid + row.Ask) / 2.0
		}

		if mid > 0 {
			ws.lastPrice[row.Symbol] = mid
		}
	}
}

func (ws *WebSocket) Close() error {
	ws.cancel()
	return nil
}

// simulateAddOrder converts an add_order frame into a realistic execution report
// with fees, cost, slippage, and liquidity indicator — mirroring what Kraken's
// executions channel emits for a real filled order.
func (ws *WebSocket) simulateAddOrder(params map[string]any) {
	symbol, _ := params["symbol"].(string)
	orderQty, _ := params["order_qty"].(float64)
	orderType, _ := params["order_type"].(string)
	side, _ := params["side"].(string)

	if symbol == "" || orderQty <= 0 {
		return
	}

	clOrdID, _ := params["cl_ord_id"].(string)
	if clOrdID == "" {
		clOrdID = nextPaperClOrdID()
	}

	pm := ws.getPairMeta(symbol)

	// Limit orders rest on the book → maker.
	// Market / stop / take-profit → taker.
	isMaker := orderType == "limit"

	// Determine raw fill price and applicable slippage in basis points.
	var fillPrice, slipBps float64

	switch orderType {
	case "limit":
		fillPrice, _ = params["limit_price"].(float64)

	case "market":
		fillPrice = ws.getLastPrice(symbol)
		slipBps = halfSpreadBps + marketSlippageBps

	default:
		// stop-loss, stop-loss-limit, take-profit, take-profit-limit
		if triggers, ok := params["triggers"].(map[string]any); ok {
			fillPrice, _ = triggers["price"].(float64)
		}
		if fillPrice <= 0 {
			fillPrice, _ = params["limit_price"].(float64)
		}
		slipBps = triggerSlippageBps
	}

	if fillPrice <= 0 {
		errnie.Debug("paper.WebSocket.simulateAddOrder: no reference price for", symbol, orderType)
		return
	}

	// Apply slippage: buys fill higher, sells fill lower.
	if slipBps > 0 {
		factor := slipBps / 10_000.0
		if side == "buy" {
			fillPrice *= 1 + factor
		} else {
			fillPrice *= 1 - factor
		}
	}

	fillPrice = roundToTick(fillPrice, pm.tickSize)

	// Cost = notional value of the fill in quote currency.
	cost := orderQty * fillPrice

	var feeRate float64
	if isMaker {
		feeRate = pm.makerPct / 100.0
	} else {
		feeRate = pm.takerPct / 100.0
	}

	feeCost := roundFee(cost * feeRate)

	liquidityInd := "t"
	if isMaker {
		liquidityInd = "m"
	}

	// fee_usd_equiv is only meaningful when the quote currency is USD.
	feeUSD := feeCost
	if pm.quote != "USD" && !strings.HasSuffix(pm.quote, "USD") {
		feeUSD = 0
	}

	execPayload, err := sonic.Marshal(map[string]any{
		"channel": "executions",
		"type":    "update",
		"data": []map[string]any{{
			"exec_type":     "trade",
			"order_id":      nextPaperOrderID(),
			"cl_ord_id":     clOrdID,
			"symbol":        symbol,
			"side":          side,
			"order_type":    orderType,
			"order_qty":     orderQty,
			"last_qty":      orderQty,
			"last_price":    fillPrice,
			"avg_price":     fillPrice,
			"cum_qty":       orderQty,
			"cum_cost":      cost,
			"cost":          cost,
			"liquidity_ind": liquidityInd,
			"order_status":  "filled",
			"timestamp":     time.Now().UTC().Format(time.RFC3339Nano),
			"exec_id":       nextPaperExecID(),
			"fee_usd_equiv": feeUSD,
			"fee_ccy_pref":  pm.quote,
			"fees": []map[string]any{{
				"asset": pm.quote,
				"qty":   feeCost,
			}},
		}},
	})

	if err != nil {
		errnie.Error(err)
		return
	}

	var msg public.SocketMessage
	if err := sonic.Unmarshal(execPayload, &msg); err != nil {
		errnie.Error(err)
		return
	}

	ws.broadcasts["raw"].Send(&qpool.QValue[any]{Value: msg})

	updates := ws.wallet.ApplyFill(symbol, side, orderQty, fillPrice, feeCost, nextPaperExecID())

	for _, update := range updates {
		ws.publishBalances("update", []user.Balance{update})
	}
}

func (ws *WebSocket) getLastPrice(symbol string) float64 {
	ws.priceMu.RLock()
	defer ws.priceMu.RUnlock()
	return ws.lastPrice[symbol]
}

func (ws *WebSocket) getPairMeta(symbol string) *pairMeta {
	ws.metaMu.RLock()
	pm := ws.pairMeta[symbol]
	ws.metaMu.RUnlock()

	if pm != nil {
		return pm
	}

	return &pairMeta{
		takerPct: defaultTakerFeePct,
		makerPct: defaultMakerFeePct,
		tickSize: 0.01,
		quote:    quoteAsset(symbol),
	}
}

// quoteAsset extracts the quote currency from a "BASE/QUOTE" symbol string.
func quoteAsset(symbol string) string {
	if index := strings.IndexByte(symbol, '/'); index >= 0 {
		return symbol[index+1:]
	}

	return "USD"
}

func resolvedQuote() string {
	quote := viper.GetString("market.quote_currency")

	if quote == "" {
		return "EUR"
	}

	return quote
}

// roundToTick rounds price to the nearest multiple of tick.
func roundToTick(price, tick float64) float64 {
	if tick <= 0 {
		return price
	}
	inv := 1.0 / tick
	return math.Round(price*inv) / inv
}

// roundFee rounds a fee amount to 8 decimal places (satoshi precision).
func roundFee(f float64) float64 {
	return math.Round(f*1e8) / 1e8
}

func nextPaperOrderID() string {
	return "PAPER-" + randomHex(8)
}

func nextPaperExecID() string {
	return randomHex(16)
}

func nextPaperClOrdID() string {
	return "p" + randomHex(8)
}

func randomHex(byteCount int) string {
	buf := make([]byte, byteCount)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
