package trader

import (
	"context"
	"os"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
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
	balance     <-chan *user.Balance
	paperWallet float64
}

func NewCrypto(ctx context.Context, pool *qpool.Q) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	var balance <-chan *user.Balance

	if viper.GetString("trading.model") == "live" {
		bindBalanceToken(ctx)
		balance = user.NewBalanceSubscription(ctx)
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

func bindBalanceToken(ctx context.Context) {
	apiKey := os.Getenv("SYMM_KRAKEN_API_KEY")
	apiSecret := os.Getenv("SYMM_KRAKEN_API_SECRET")

	if apiKey == "" || apiSecret == "" {
		return
	}

	provider, err := private.NewTokenProvider(ctx, apiKey, apiSecret)

	if err != nil {
		errnie.Error(err)

		return
	}

	user.SetBalanceTokenSource(provider)
}

func (crypto *Crypto) Tick() error {
	if crypto.balance == nil {
		crypto.sendWallet(crypto.paperWallet, map[string]float64{})

		return crypto.tickActions()
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
		case balanceRow, ok := <-crypto.balance:
			if !ok {
				crypto.balance = nil

				continue
			}

			if balanceRow == nil {
				continue
			}

			if isQuoteAsset(balanceRow.Asset, quote) {
				cash = balanceRow.Holdings
			} else {
				inventory[balanceRow.Asset] = balanceRow.Holdings
			}

			crypto.sendWallet(cash, inventory)
		}
	}
}

func (crypto *Crypto) tickActions() error {
	for row := range crypto.actions.Incoming {
		if row == nil {
			continue
		}

		action, actionOK := row.Value.(perspectives.Action)

		if !actionOK {
			continue
		}

		errnie.Error(crypto.desk.AddOrder(action))
	}

	return nil
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
