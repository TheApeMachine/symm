package response

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/frame"
	"github.com/theapemachine/symm/kraken/types"
)

const (
	balanceSnapshotScope = "snapshot"
	balanceUpdateScope   = "update"
)

/*
Balances simulates the Kraken balances channel on the shared raw bus.
*/
type Balances struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	pool          *qpool.Q[any]
	tree          *dmt.Tree
	isActive      atomic.Bool
	observers     []types.Socket
	quoteCurrency string
	model         *datura.Artifact
}

func NewBalances(
	ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree,
) *Balances {
	ctx, cancel := context.WithCancel(ctx)
	quote := strings.ToUpper(viper.GetString("market.quote_currency"))
	seed := datura.Map[any]{
		"asset": []map[string]any{{
			"asset":       viper.GetString("market.quote_currency"),
			"asset_class": "currency",
			"balance": viper.GetFloat64(
				"trading.paper.wallet." + strings.ToLower(quote),
			),
			"wallets": []map[string]any{{
				"balance": viper.GetFloat64(
					"trading.paper.wallet." + strings.ToLower(quote),
				),
				"type": "spot",
				"id":   "main",
			}},
		}},
	}

	return &Balances{
		ctx:           ctx,
		cancel:        cancel,
		pool:          pool,
		tree:          tree,
		observers:     make([]types.Socket, 0),
		quoteCurrency: quote,
		model: datura.Acquire("kraken:balances", datura.APPJSON).
			WithPayload(seed.Marshal()),
	}
}

func (balances *Balances) Send(message []byte) *types.SocketMessage {
	var request types.KrakenMessage

	if err := sonic.Unmarshal(message, &request); err != nil {
		return nil
	}

	switch request.Method {
	case "subscribe":
		alreadyActive := balances.isActive.Load()
		balances.isActive.Store(true)

		if alreadyActive {
			return balances.snapshotMessage(balanceSnapshotScope)
		}
	case "unsubscribe":
		balances.isActive.Store(false)

		return nil
	default:
		return nil
	}

	return balances.snapshotMessage(balanceSnapshotScope)
}

func (balances *Balances) snapshotMessage(messageType string) *types.SocketMessage {
	data, err := balances.channelData()

	if err != nil {
		return nil
	}

	for _, socket := range balances.observers {
		socket.Send(data)
	}

	return &types.SocketMessage{
		Channel: "balances",
		Type:    messageType,
		Success: true,
		Data:    data,
	}
}

func (balances *Balances) channelData() (json.RawMessage, error) {
	wire, err := balances.balanceWire()

	if err != nil {
		return nil, err
	}

	rows, ok := wire["asset"]

	if !ok {
		return nil, errnie.Err(errnie.Validation, "paper balances: missing asset rows", nil)
	}

	return sonic.Marshal(rows)
}

func (balances *Balances) PublishUpdate() {
	if !balances.isActive.Load() || balances.pool == nil {
		return
	}

	message := balances.snapshotMessage(balanceUpdateScope)

	if message == nil {
		return
	}

	ui := balances.pool.CreateBroadcastGroup("ui")

	errnie.Error(frame.Publish(balances.tree, ui, nil, message))
}

func (balances *Balances) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		balances.observers = append(balances.observers, socket)
	}
}

func (balances *Balances) symbolParts(symbol string) (baseAsset, quoteAsset string) {
	parts := strings.Split(symbol, "/")

	if len(parts) != 2 {
		return "", ""
	}

	return strings.ToUpper(parts[0]), strings.ToUpper(parts[1])
}

func (balances *Balances) ApplyFill(fill *datura.Artifact) {
	if fill == nil {
		return
	}

	symbol := datura.Peek[string](fill, "symbol")
	side := datura.Peek[string](fill, "side")
	price := datura.Peek[float64](fill, "last_price")
	qty := datura.Peek[float64](fill, "order_qty")
	fee := datura.Peek[float64](fill, "fee")
	base, quote := balances.symbolParts(symbol)

	if base == "" || quote == "" || price <= 0 || qty <= 0 {
		return
	}

	// Kraken charges the spot fee in the quote currency: it raises the quote we
	// pay on a buy and lowers the quote we receive on a sell.
	cost := price * qty

	switch side {
	case "buy":
		balances.adjustAsset(quote, -(cost + fee))
		balances.adjustAsset(base, qty)
	case "sell":
		balances.adjustAsset(base, -qty)
		balances.adjustAsset(quote, cost-fee)
	}
}

func (balances *Balances) adjustAsset(asset string, delta float64) {
	wire, err := balances.balanceWire()

	if err != nil {
		return
	}

	rows, _ := wire["asset"].([]any)

	for index, rowAny := range rows {
		row, ok := rowAny.(map[string]any)

		if !ok || row["asset"] != asset {
			continue
		}

		balance, _ := row["balance"].(float64)
		row["balance"] = balance + delta

		if wallets, walletOK := row["wallets"].([]any); walletOK && len(wallets) > 0 {
			if wallet, mapOK := wallets[0].(map[string]any); mapOK {
				walletBalance, _ := wallet["balance"].(float64)
				wallet["balance"] = walletBalance + delta
				wallets[0] = wallet
				row["wallets"] = wallets
			}
		}

		rows[index] = row
		balances.setBalanceWire(wire)

		return
	}

	rows = append(rows, map[string]any{
		"asset":       asset,
		"asset_class": "currency",
		"balance":     delta,
		"wallets": []map[string]any{{
			"balance": delta,
			"type":    "spot",
			"id":      "main",
		}},
	})
	wire["asset"] = rows
	balances.setBalanceWire(wire)
}

func (balances *Balances) balanceWire() (map[string]any, error) {
	if !balances.model.HasEncryptedPayload() {
		return nil, errnie.Err(errnie.Validation, "paper balances: empty payload", nil)
	}

	payload := balances.model.DecryptPayload()

	if len(payload) == 0 {
		return nil, errnie.Err(errnie.Validation, "paper balances: empty payload", nil)
	}

	var wire map[string]any

	if err := sonic.Unmarshal(payload, &wire); err != nil {
		return nil, errnie.Err(errnie.Validation, "paper balances: invalid payload", err)
	}

	return wire, nil
}

func (balances *Balances) setBalanceWire(wire map[string]any) {
	encoded, err := sonic.Marshal(wire)

	if err != nil {
		return
	}

	balances.model.WithPayload(encoded)
}

func (balances *Balances) Clone() *Balances {
	return &Balances{
		ctx:           balances.ctx,
		cancel:        balances.cancel,
		pool:          balances.pool,
		tree:          balances.tree,
		observers:     balances.observers,
		quoteCurrency: balances.quoteCurrency,
		model:         balances.model,
	}
}

func assetBalance(balances *Balances, asset string) float64 {
	wire, err := balances.balanceWire()

	if err != nil {
		return 0
	}

	rows, _ := wire["asset"].([]any)

	for _, rowAny := range rows {
		row, ok := rowAny.(map[string]any)

		if !ok || row["asset"] != asset {
			continue
		}

		balance, _ := row["balance"].(float64)

		return balance
	}

	return 0
}
