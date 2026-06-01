package trader

import (
	"context"
	"os"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/private"
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
	actions     *qpool.Subscriber
	desk        *broker.Desk
	balance     market.Feed
	paperWallet float64
}

func NewCrypto(ctx context.Context, pool *qpool.Q) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	balance := market.Feed{}

	if viper.GetString("trading.model") == "live" {
		apiKey := os.Getenv("SYMM_KRAKEN_API_KEY")
		apiSecret := os.Getenv("SYMM_KRAKEN_API_SECRET")

		if apiKey != "" && apiSecret != "" {
			provider := errnie.Does(func() (*private.TokenProvider, error) {
				return private.NewTokenProvider(ctx, apiKey, apiSecret)
			}).Or(func(err error) {
				errnie.Error(err)
			}).Value()

			balance = user.NewBalanceSubscription(ctx, provider)
		}
	}

	crypto := &Crypto{
		ctx:    ctx,
		cancel: cancel,
		ui:     pool.CreateBroadcastGroup("ui", 10*time.Millisecond),
		desk: errnie.Does(func() (*broker.Desk, error) {
			return broker.NewDesk(ctx, pool)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value(),
		balance:     balance,
		paperWallet: viper.GetFloat64("trading.paper.wallet_eur"),
	}

	actions := pool.CreateBroadcastGroup("actions", 10*time.Millisecond)
	crypto.actions = actions.Subscribe("trader:actions", 128)

	return crypto
}

const paperWalletInterval = time.Second

func (crypto *Crypto) Tick() error {
	if crypto.balance.Stream == nil {
		return crypto.tickPaper()
	}

	cash := 0.0
	inventory := make(map[string]float64)
	quote := quoteCurrency()

	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		case row, ok := <-crypto.actions.Incoming:
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
		case message, ok := <-crypto.balance.Stream:
			if !ok {
				crypto.balance.Stream = nil

				continue
			}

			if message == nil {
				continue
			}

			var rows []user.Balance

			if err := sonic.Unmarshal(message.Data, &rows); err != nil {
				errnie.Error(err)
				continue
			}

			for index := range rows {
				rows[index].SetEnvelopeType(message.Type)

				if isQuoteAsset(rows[index].Asset, quote) {
					cash = rows[index].Holdings
				} else {
					inventory[rows[index].Asset] = rows[index].Holdings
				}
			}

			crypto.sendWallet(cash, inventory)
		}
	}
}

func (crypto *Crypto) tickPaper() error {
	ticker := time.NewTicker(paperWalletInterval)
	defer ticker.Stop()

	crypto.sendWallet(crypto.paperWallet, map[string]float64{})

	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		case <-ticker.C:
			crypto.sendWallet(crypto.paperWallet, map[string]float64{})
		case row, ok := <-crypto.actions.Incoming:
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
		}
	}
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
