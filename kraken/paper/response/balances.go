package response

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/paper/types"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/user"
)

// ErrInsufficientFunds rejects a fill that the wallet cannot fund in the spent currency.
var ErrInsufficientFunds = errors.New("paper balances: insufficient funds")

/*
Balances simulates the Kraken balances channel on the shared raw bus.
*/
type Balances struct {
	quote     string
	model     user.Balances
	observers []types.Socket
	ui        *qpool.BroadcastGroup
}

func NewBalances(ui *qpool.BroadcastGroup) *Balances {
	quote := viper.GetViper().GetString("market.quote_currency")
	balance := viper.GetViper().GetFloat64("trading.paper.wallet_" + strings.ToLower(quote))

	b := &Balances{
		quote: quote,
		ui:    ui,
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

	b.PublishUI()

	return b
}

func (balances *Balances) Send(message *qpool.QValue[any]) map[string]any {
	frame, ok := message.Value.(map[string]any)

	if !ok {
		return nil
	}

	out := map[string]any{
		"method":   frame["method"],
		"req_id":   frame["req_id"],
		"success":  true,
		"time_in":  frame["time_in"],
		"time_out": time.Now(),
	}

	switch frame["method"] {
	case "subscribe":
		out["result"] = balances.model
		balances.PublishUI()
	}

	for _, observer := range balances.observers {
		observer.Send(&qpool.QValue[any]{
			Type:  "kraken:private",
			Value: out,
		})
	}

	return out

}

func (balances *Balances) Observe(socket types.Socket) {
	balances.observers = append(balances.observers, socket)
}

/*
ApplyFill settles a simulated fill against the multi-currency wallet, faithful to
Kraken: a buy spends the pair's quote currency, a sell spends its base. The fill
is rejected (ErrInsufficientFunds, wallet untouched) when the spent currency is
short — exactly as the exchange would reject an order you cannot fund.
*/
func (balances *Balances) ApplyFill(symbol, side string, qty, price, fee float64, _ string) error {
	slash := strings.IndexByte(symbol, '/')

	if slash <= 0 {
		return fmt.Errorf("paper balances: malformed symbol %q", symbol)
	}

	base := symbol[:slash]
	quote := symbol[slash+1:]
	cost := qty * price

	if side == "buy" {
		if balances.available(quote) < cost+fee {
			return ErrInsufficientFunds
		}

		balances.adjust(quote, -(cost + fee))
		balances.adjust(base, qty)
	} else {
		if balances.available(base) < qty {
			return ErrInsufficientFunds
		}

		balances.adjust(base, -qty)
		balances.adjust(quote, cost-fee)
	}

	for _, observer := range balances.observers {
		observer.Send(&qpool.QValue[any]{
			Type:  public.BalancesChannel,
			Value: balances.snapshot(),
		})
	}

	balances.PublishUI()

	return nil
}

func (balances *Balances) available(asset string) float64 {
	for _, row := range balances.model.Asset {
		if row.Asset == asset {
			return row.Balance
		}
	}

	return 0
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

func (balances *Balances) PublishUI() {
	if balances.ui == nil {
		return
	}

	cash := 0.0
	open := 0
	inventory := make(map[string]float64)

	for _, asset := range balances.model.Asset {
		if asset.Asset == balances.quote {
			cash = asset.Balance
			continue
		}

		if asset.Balance > 0 {
			open++
			inventory[asset.Asset] = asset.Balance
		}
	}

	balances.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":     "wallet",
		"balance":   cash,
		"open":      open,
		"Balance":   cash,
		"Inventory": inventory,
	}})
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
