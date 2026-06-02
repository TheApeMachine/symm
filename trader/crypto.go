package trader

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/user"
)

const cryptoRawSubscriberID = "trader/crypto:raw"

/*
Crypto publishes wallet snapshots to the ui broadcast from Kraken balance frames.
*/
type Crypto struct {
	ctx         context.Context
	cancel      context.CancelFunc
	ui          *qpool.BroadcastGroup
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	auditSeq    atomic.Int64
	cash        float64
	inventory   map[string]float64
}

func NewCrypto(ctx context.Context, pool *qpool.Q) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:         ctx,
		cancel:      cancel,
		ui:          pool.CreateBroadcastGroup("ui", 10*time.Millisecond),
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		inventory:   make(map[string]float64),
	}

	crypto.broadcasts["raw"] = pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
	crypto.subscribers["raw"] = crypto.broadcasts["raw"].Subscribe(
		cryptoRawSubscriberID, 128,
	)

	crypto.subscribers["ui:resync"] = pool.CreateBroadcastGroup(
		"ui:resync", 10*time.Millisecond,
	).Subscribe("trader/crypto:resync", 128)

	activate.Boot("trader/crypto ready")

	return crypto
}

func (crypto *Crypto) Tick() error {
	quote := quoteCurrency()

	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		case _, ok := <-crypto.subscribers["ui:resync"].Incoming:
			if !ok {
				return crypto.ctx.Err()
			}

			crypto.resendWallet()
		case message := <-crypto.subscribers["raw"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(public.SocketMessage)

			if !ok {
				continue
			}

			switch envelope.Channel {
			case public.ExecutionsChannel:
				activate.Once("trader/crypto:executions-channel")

				for _, execution := range errnie.Does(func() ([]user.Execution, error) {
					return user.DecodeExecutions(&envelope)
				}).Or(func(err error) {
					errnie.Error(err)
				}).Value() {
					crypto.publishFill(execution)
				}
			case public.BalancesChannel:
				activate.Once("trader/crypto:balances-channel")

				for _, balance := range errnie.Does(func() ([]user.Balance, error) {
					return user.DecodeBalances(&envelope)
				}).Or(func(err error) {
					errnie.Error(err)
				}).Value() {
					if isQuoteAsset(balance.Asset, quote) {
						crypto.cash = balance.Balance
					} else if balance.Balance > 0 {
						crypto.inventory[balance.Asset] = balance.Balance
					} else {
						delete(crypto.inventory, balance.Asset)
					}
				}

				crypto.sendWallet(crypto.cash, walletInventorySnapshot(crypto.inventory))
			}
		}
	}
}

func walletInventorySnapshot(inventory map[string]float64) map[string]float64 {
	snapshot := make(map[string]float64, len(inventory))

	for asset, amount := range inventory {
		snapshot[asset] = amount
	}

	return snapshot
}

func (crypto *Crypto) publishFill(execution user.Execution) {
	if execution.Symbol == "" || execution.LastQty <= 0 {
		return
	}

	activate.Once("trader/crypto:fill")

	ts := time.Now().UTC().Format(time.RFC3339Nano)

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"OrderID": execution.OrderID,
		"Symbol":  execution.Symbol,
		"Side":    execution.Side,
		"Qty":     execution.LastQty,
		"Price":   execution.LastPrice,
	}})

	auditEvent := "entry"
	if execution.Side == "sell" {
		auditEvent = "exit"
	}

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":       "audit",
		"ts":          ts,
		"audit_event": auditEvent,
		"seq":         crypto.auditSeq.Add(1),
		"symbol":      execution.Symbol,
		"source":      "trader",
		"reason":      execution.OrderID,
	}})
}

func (crypto *Crypto) resendWallet() {
	crypto.sendWallet(crypto.cash, walletInventorySnapshot(crypto.inventory))
}

func (crypto *Crypto) sendWallet(cash float64, inventory map[string]float64) {
	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":     "wallet",
		"ts":        time.Now().UTC().Format(time.RFC3339Nano),
		"Currency":  quoteCurrency(),
		"Balance":   cash,
		"Inventory": inventory,
	}})
}

func isQuoteAsset(asset, quote string) bool {
	return asset == quote || asset == "Z"+quote
}

func quoteCurrency() string {
	quote := viper.GetString("market.quote_currency")

	if quote == "" {
		return "EUR"
	}

	return quote
}

func (crypto *Crypto) Close() error {
	crypto.cancel()

	return nil
}
