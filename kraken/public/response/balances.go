package response

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

/*
Balances simulates the Kraken balances channel on the shared raw bus.
*/
type Balances struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	isActive      atomic.Bool
	observers     *sync.Map
	pool          *qpool.Q[any]
	tree          *dmt.Tree
	quoteCurrency string
	model         *datura.Artifact
	now           func() time.Time
}

func NewBalances(
	ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree,
) *Balances {
	ctx, cancel := context.WithCancel(ctx)
	quote := strings.ToUpper(viper.GetString("market.quote_currency"))
	balance := viper.GetFloat64("trading.paper.wallet." + strings.ToLower(quote))

	return &Balances{
		ctx:           ctx,
		cancel:        cancel,
		observers:     &sync.Map{},
		pool:          pool,
		tree:          tree,
		quoteCurrency: quote,
		now:           time.Now,
		model: datura.Acquire(
			"kraken:private", datura.APPJSON,
		).WithPayload(datura.Map[any]{
			"channel": "balances",
			"type":    "snapshot",
			"data": []map[string]any{{
				"asset":       quote,
				"asset_class": "currency",
				"balance":     balance,
				"wallets": []map[string]any{{
					"balance": balance,
					"type":    "spot",
					"id":      "main",
				}},
			}},
		}.Marshal()),
	}
}

func (balances *Balances) SetClock(clock func() time.Time) {
	if balances == nil {
		return
	}
	if clock == nil {
		balances.now = time.Now
		return
	}
	balances.now = clock
}

func (balances *Balances) currentTime() time.Time {
	if balances == nil || balances.now == nil {
		return time.Now().UTC()
	}

	return balances.now().UTC()
}

func (balances *Balances) Snapshot(scope string) *datura.Artifact {
	if balances == nil {
		return nil
	}
	if strings.TrimSpace(scope) == "" {
		scope = "snapshot"
	}

	return balances.snapshot(scope)
}

func (balances *Balances) Send(artifact *datura.Artifact) *datura.Artifact {
	if balances == nil || artifact == nil || !artifact.IsValid() {
		return nil
	}

	if out := balances.applyExecutionFill(artifact); out != nil {
		return out
	}

	method := datura.Peek[string](artifact, "method")
	var out *datura.Artifact
	publish := false

	switch method {
	case "subscribe":
		errnie.Info("subscribing to balances")
		balances.isActive.Store(true)
		publish = true
		out = balances.snapshot("snapshot")
	case "unsubscribe":
		errnie.Info("unsubscribing from balances")
		balances.isActive.Store(false)
		out = datura.Acquire(
			"kraken:private", datura.APPJSON,
		).WithRole(
			"balances",
		).WithScope(
			"unsubscribe",
		).WithPayload(datura.Map[any]{
			"method":   "unsubscribe",
			"success":  true,
			"time_in":  balances.currentTime(),
			"time_out": balances.currentTime(),
		}.Marshal())
	case "add_order":
		return nil
	default:
		return nil
	}

	if publish {
		balances.publish(out, false)
	}

	return out
}

func (balances *Balances) applyExecutionFill(artifact *datura.Artifact) *datura.Artifact {
	if balances == nil || artifact == nil || !artifact.IsValid() {
		return nil
	}

	role := datura.Peek[string](artifact, "channel")
	if role == "" {
		role = datura.Peek[string](artifact, "role")
	}
	if role != "executions" && role != "fill" {
		return nil
	}

	status := executionString(artifact, "order_status")
	if status != "filled" {
		return nil
	}

	symbol := executionString(artifact, "symbol")
	side := strings.ToLower(executionString(artifact, "side"))
	qty := executionNumber(artifact, "order_qty")
	if qty <= 0 {
		qty = executionNumber(artifact, "last_qty")
	}
	price := executionNumber(artifact, "avg_price")
	if price <= 0 {
		price = executionNumber(artifact, "last_price")
	}
	if price <= 0 {
		price = executionNumber(artifact, "price")
	}

	base, quote, ok := splitSymbol(symbol)
	if !ok || side == "" || qty <= 0 || price <= 0 {
		return nil
	}

	notional := qty * price
	fee := executionNumber(artifact, "fee")
	feeAsset := strings.ToUpper(executionString(artifact, "fee_ccy"))

	switch side {
	case "buy", "enter":
		balances.adjustBalance(quote, -notional)
		balances.adjustFee(feeAsset, fee)
		balances.adjustBalance(base, qty)
	case "sell", "exit":
		balances.adjustBalance(base, -qty)
		balances.adjustBalance(quote, notional)
		balances.adjustFee(feeAsset, fee)
	default:
		return nil
	}

	out := balances.snapshot("update")
	balances.publish(out, true)

	return out
}

func executionString(artifact *datura.Artifact, key string) string {
	if value := datura.Peek[string](artifact, key); value != "" {
		return value
	}
	if value := stringValue(executionPayloadValue(artifact, key)); value != "" {
		return value
	}

	return datura.Peek[string](artifact, "data", 0, key)
}

func executionNumber(artifact *datura.Artifact, key string) float64 {
	if value := datura.Peek[float64](artifact, key); value > 0 {
		return value
	}
	if value := floatValue(executionPayloadValue(artifact, key)); value > 0 {
		return value
	}

	return datura.Peek[float64](artifact, "data", 0, key)
}

func executionPayloadValue(artifact *datura.Artifact, key string) any {
	if artifact == nil {
		return nil
	}

	var payload map[string]any
	if err := sonic.Unmarshal(artifact.DecryptPayload(), &payload); err != nil {
		return nil
	}

	if value := payload[key]; value != nil {
		return value
	}

	rows, _ := payload["data"].([]any)
	if len(rows) == 0 {
		return nil
	}

	row, _ := rows[0].(map[string]any)
	if row == nil {
		return nil
	}

	return row[key]
}
