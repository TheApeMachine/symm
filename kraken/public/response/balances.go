package response

import (
	"context"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/types"
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

func (balances *Balances) Send(artifact *datura.Artifact) *datura.Artifact {
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
			"time_in":  time.Now(),
			"time_out": time.Now(),
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
	if balances == nil || artifact == nil {
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

func splitSymbol(symbol string) (string, string, bool) {
	base, quote, ok := strings.Cut(symbol, "/")
	if !ok {
		return "", "", false
	}

	base = strings.ToUpper(strings.TrimSpace(base))
	quote = strings.ToUpper(strings.TrimSpace(quote))

	return base, quote, base != "" && quote != ""
}

func (balances *Balances) adjustFee(asset string, fee float64) {
	if asset == "" || fee <= 0 {
		return
	}

	balances.adjustBalance(asset, -fee)
}

func (balances *Balances) adjustBalance(asset string, delta float64) {
	if balances == nil || balances.model == nil || asset == "" || delta == 0 {
		return
	}

	payload := balances.payload()
	rows, _ := payload["data"].([]any)
	index := balanceRowIndex(rows, asset)
	if index < 0 {
		rows = append(rows, newBalanceRow(asset))
		index = len(rows) - 1
	}

	row, _ := rows[index].(map[string]any)
	if row == nil {
		row = newBalanceRow(asset)
		rows[index] = row
	}

	current := floatValue(row["balance"])
	next := current + delta
	if math.Abs(next) < 1e-12 {
		next = 0
	}

	row["asset"] = strings.ToUpper(strings.TrimSpace(asset))
	row["asset_class"] = "currency"
	row["balance"] = next

	wallets, _ := row["wallets"].([]any)
	if len(wallets) == 0 {
		wallets = []any{map[string]any{
			"balance": 0.0,
			"type":    "spot",
			"id":      "main",
		}}
	}

	wallet, _ := wallets[0].(map[string]any)
	if wallet == nil {
		wallet = map[string]any{
			"type": "spot",
			"id":   "main",
		}
		wallets[0] = wallet
	}

	wallet["balance"] = next
	row["wallets"] = wallets
	payload["data"] = rows
	balances.storePayload(payload)
}

func balanceRowIndex(rows []any, asset string) int {
	target := strings.ToUpper(strings.TrimSpace(asset))

	for index, raw := range rows {
		row, _ := raw.(map[string]any)
		if row == nil {
			continue
		}

		current := strings.ToUpper(strings.TrimSpace(stringValue(row["asset"])))
		if current == target {
			return index
		}
	}

	return -1
}

func newBalanceRow(asset string) map[string]any {
	return map[string]any{
		"asset":       strings.ToUpper(strings.TrimSpace(asset)),
		"asset_class": "currency",
		"balance":     0.0,
		"wallets": []any{map[string]any{
			"balance": 0.0,
			"type":    "spot",
			"id":      "main",
		}},
	}
}

func (balances *Balances) snapshot(scope string) *datura.Artifact {
	payload := balances.payload()
	payload["channel"] = "balances"
	payload["type"] = scope
	balances.storePayload(payload)

	out := datura.Acquire(
		"kraken:private", datura.APPJSON,
	).WithRole(
		"balances",
	).WithScope(
		scope,
	)
	out.SetTimestamp(time.Now().UTC().UnixNano())
	out.WithPayload(balances.model.DecryptPayload())

	return out
}

func (balances *Balances) payload() map[string]any {
	payload := make(map[string]any)
	if balances == nil || balances.model == nil {
		return payload
	}

	if err := sonic.Unmarshal(balances.model.DecryptPayload(), &payload); err != nil {
		return map[string]any{
			"channel": "balances",
			"type":    "snapshot",
			"data":    []any{},
		}
	}

	if _, ok := payload["data"].([]any); !ok {
		payload["data"] = []any{}
	}

	return payload
}

func (balances *Balances) storePayload(payload map[string]any) {
	wire, err := sonic.Marshal(payload)
	if err != nil {
		return
	}

	balances.model.WithPayload(wire)
}

func floatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func stringValue(value any) string {
	typed, _ := value.(string)

	return typed
}

func (balances *Balances) publish(artifact *datura.Artifact, internal bool) {
	if artifact == nil {
		return
	}

	balances.observers.Range(func(_ any, value any) bool {
		value.(types.Socket).Send(artifact)
		return true
	})

	if !internal {
		return
	}

	if balances.tree != nil {
		balances.tree.InsertArtifact(artifact.Prefix(
			"role", "timestamp", "scope",
		), artifact)
	}

	if balances.pool != nil {
		balances.pool.CreateBroadcastGroup("balances").Send(artifact)
		balances.pool.CreateBroadcastGroup("ui").Send(uiBalanceArtifact(artifact))
	}
}

func uiBalanceArtifact(artifact *datura.Artifact) *datura.Artifact {
	if artifact == nil {
		return nil
	}

	scope, _ := artifact.Scope()
	uiArtifact := datura.Acquire(
		"kraken:private", datura.APPJSON,
	).WithRole(
		"balances",
	).WithScope(
		scope,
	).WithDestination(
		"ui",
	).WithPayload(
		artifact.DecryptPayload(),
	)

	if timestamp := artifact.Timestamp(); timestamp > 0 {
		uiArtifact.SetTimestamp(timestamp)
	}

	return uiArtifact
}

func (balances *Balances) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		balances.observers.Store(uuid.NewString(), socket)
	}
}
