package trader

import (
	"context"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
Crypto routes perspective actions through the broker desk and publishes wallet
snapshots to the ui broadcast.
*/
type Crypto struct {
	ctx         context.Context
	cancel      context.CancelFunc
	ui          *qpool.BroadcastGroup
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	desk        *broker.Desk
}

func NewCrypto(ctx context.Context, pool *qpool.Q) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:         ctx,
		cancel:      cancel,
		ui:          pool.CreateBroadcastGroup("ui", 10*time.Millisecond),
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		desk: errnie.Does(func() (*broker.Desk, error) {
			return broker.NewDesk(ctx, pool)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value(),
	}

	for _, channel := range []string{"raw", "actions"} {
		crypto.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		crypto.subscribers[channel] = crypto.broadcasts[channel].Subscribe(channel, 128)
	}

	return crypto
}

func (crypto *Crypto) Tick() error {
	cash := 0.0
	inventory := make(map[string]float64)
	quote := quoteCurrency()

	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		case row, ok := <-crypto.subscribers["actions"].Incoming:
			if !ok {
				return nil
			}

			if row == nil {
				continue
			}

			action, actionOK := row.Value.(perspectives.Action)

			if !actionOK {
				continue
			}

			errnie.Error(crypto.desk.AddOrder(action))
		case message := <-crypto.subscribers["raw"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(public.SocketMessage)

			if !ok {
				continue
			}

			for _, row := range errnie.Does(func() ([]*public.SocketMessage, error) {
				return envelope.SplitDataRows()
			}).Or(func(err error) {
				errnie.Error(err)
			}).Value() {
				switch row.Channel {
				case "executions":
					execution := errnie.Does(func() (user.Execution, error) {
						return user.DecodeExecution(row)
					}).Or(func(err error) {
						errnie.Error(err)
					}).Value()

					crypto.publishFill(execution)
				case "balances":
					for _, row := range errnie.Does(func() ([]user.Balance, error) {
						return user.DecodeBalances(row)
					}).Or(func(err error) {
						errnie.Error(err)
					}).Value() {
						if isQuoteAsset(row.Asset, quote) {
							cash = row.Balance
							continue
						}

						if row.Balance > 0 {
							inventory[row.Asset] = row.Balance
							continue
						}

						delete(inventory, row.Asset)
					}

					crypto.sendWallet(cash, inventory)
				}
			}
		}
	}
}

func (crypto *Crypto) publishFill(execution user.Execution) {
	if execution.Symbol == "" || execution.LastQty <= 0 {
		return
	}

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"OrderID": execution.OrderID,
		"Symbol":  execution.Symbol,
		"Side":    execution.Side,
		"Qty":     execution.LastQty,
		"Price":   execution.LastPrice,
	}})
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
