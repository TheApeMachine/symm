package response

import (
	"encoding/json"
	"strings"
	"time"

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

	b.publishUI()

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

	balances.publishUI()
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

func (balances *Balances) publishUI() {
	if balances.ui == nil {
		return
	}

	total := 0.0
	for _, a := range balances.model.Asset {
		if a.Asset == balances.quote {
			total = a.Balance
			break
		}
	}

	balances.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":   "wallet",
		"balance": total,
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
