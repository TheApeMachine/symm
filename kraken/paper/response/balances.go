package response

import (
	"encoding/json"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/paper/types"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/user"
)

/*
Balances simulates the Kraken balances channel on the shared raw bus.
*/
type Balances struct {
	quote     string
	model     user.Balances
	observers []types.Socket
}

func NewBalances() *Balances {
	quote := viper.GetViper().GetString("market.quote_currency")
	balance := viper.GetViper().GetFloat64("trading.paper.wallet_" + strings.ToLower(quote))

	return &Balances{
		quote: quote,
		model: user.Balances{
			Asset: []user.Balance{
				{
					Asset:   quote,
					Balance: balance,
					Wallets: []user.BalanceWallet{{
						Balance: balance,
						Type:    "spot",
						ID:      "main",
					}},
				},
			},
		},
	}
}

func (balances *Balances) Send(message *qpool.QValue[any]) map[string]any {
	if _, ok := message.Value.(user.SubscribeFrame); ok {
		return balances.snapshot()
	}

	return nil
}

func (balances *Balances) Observe(socket types.Socket) {
	balances.observers = append(balances.observers, socket)
}

/*
ApplyFill updates wallet balances after a simulated fill and notifies observers.
*/
func (balances *Balances) ApplyFill(symbol, side string, qty, price, fee float64, _ string) {
	base := symbol[:strings.IndexByte(symbol, '/')]
	cost := qty * price

	if side == "buy" {
		balances.adjust(base, qty)
		balances.adjust(balances.quote, -(cost + fee))
	} else {
		balances.adjust(base, -qty)
		balances.adjust(balances.quote, cost-fee)
	}

	for _, observer := range balances.observers {
		observer.Send(&qpool.QValue[any]{
			Type:  public.BalancesChannel,
			Value: balances.snapshot(),
		})
	}
}

func (balances *Balances) adjust(asset string, delta float64) {
	for index, bal := range balances.model.Asset {
		if bal.Asset != asset {
			continue
		}

		balances.model.Asset[index].Balance += delta

		if len(balances.model.Asset[index].Wallets) > 0 {
			balances.model.Asset[index].Wallets[0].Balance += delta
		}

		return
	}

	if delta <= 0 {
		return
	}

	balances.model.Asset = append(balances.model.Asset, user.Balance{
		Asset:   asset,
		Balance: delta,
		Wallets: []user.BalanceWallet{{Balance: delta, Type: "spot", ID: "main"}},
	})
}

func (balances *Balances) snapshot() map[string]any {
	data, err := sonic.Marshal(balances.model.Asset)

	if err != nil {
		return nil
	}

	return map[string]any{
		"channel": public.BalancesChannel,
		"type":    "snapshot",
		"data":    json.RawMessage(data),
	}
}
