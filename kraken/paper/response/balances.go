package response

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/paper/types"
	"github.com/theapemachine/symm/kraken/user"
)

// ErrInsufficientFunds rejects a fill that the wallet cannot fund in the spent currency.
var ErrInsufficientFunds = errors.New("paper balances: insufficient funds")

/*
Balances simulates the Kraken balances channel on the shared raw bus.
*/
type Balances struct {
	quote string
	model user.Balances
	raw   *qpool.BroadcastGroup
	ui    *qpool.BroadcastGroup
	ids   *Identifier
}

func NewBalances(raw, ui *qpool.BroadcastGroup, ids *Identifier) *Balances {
	quote := viper.GetViper().GetString("market.quote_currency")
	balance := viper.GetViper().GetFloat64("trading.paper.wallet_" + strings.ToLower(quote))

	balances := &Balances{
		quote: quote,
		raw:   raw,
		ui:    ui,
		ids:   ids,
		model: user.Balances{
			Asset: []user.Balance{
				{
					Asset:      quote,
					AssetClass: "currency",
					Balance:    balance,
					Wallets: []user.BalanceWallet{{
						Balance: balance,
						Type:    "spot",
						ID:      "main",
					}},
				},
			},
		},
	}

	balances.PublishUI()

	return balances
}

func (balances *Balances) Send(message *qpool.QValue[any]) map[string]any {
	switch frame := message.Value.(type) {
	case user.SubscribeFrame:
		return balances.subscribeAck(frame)
	case map[string]any:
		if frame["method"] == "subscribe" {
			return balances.subscribeAckMap(frame)
		}
	}

	return nil
}

func (balances *Balances) Observe(_ types.Socket) {}

func (balances *Balances) subscribeAck(frame user.SubscribeFrame) map[string]any {
	balances.publishSnapshot()

	return map[string]any{
		"method":   frame.Method,
		"success":  true,
		"result":   map[string]any{"channel": "balances", "snapshot": frame.Params.Snapshot},
		"time_out": time.Now(),
	}
}

func (balances *Balances) subscribeAckMap(frame map[string]any) map[string]any {
	balances.publishSnapshot()

	return map[string]any{
		"method":   frame["method"],
		"req_id":   frame["req_id"],
		"success":  true,
		"time_in":  frame["time_in"],
		"time_out": time.Now(),
		"result":   map[string]any{"channel": "balances", "snapshot": true},
	}
}

func (balances *Balances) publishSnapshot() {
	user.PublishBalancesRaw(balances.raw, user.BalanceSnapshot, balances.model.Asset)
	balances.PublishUI()
}

/*
ApplyFill settles a simulated fill against the multi-currency wallet, faithful to
Kraken: a buy spends the pair's quote currency, a sell spends its base. The fill
is rejected (ErrInsufficientFunds, wallet untouched) when the spent currency is
short — exactly as the exchange would reject an order you cannot fund.
*/
func (balances *Balances) ApplyFill(
	symbol, side string,
	qty, price, fee float64,
	refID string,
) error {
	slash := strings.IndexByte(symbol, '/')

	if slash <= 0 {
		return fmt.Errorf("paper balances: malformed symbol %q", symbol)
	}

	base := symbol[:slash]
	quote := symbol[slash+1:]
	cost := qty * price
	stamp := time.Now().UTC().Format(time.RFC3339Nano)

	var ledgerRows []user.Balance

	if side == "buy" {
		if balances.available(quote) < cost+fee {
			return ErrInsufficientFunds
		}

		balances.adjust(quote, -(cost + fee))
		balances.adjust(base, qty)

		ledgerRows = []user.Balance{
			balances.ledgerRow(base, qty, 0, refID, stamp),
			balances.ledgerRow(quote, -(cost + fee), fee, refID, stamp),
		}
	} else {
		if balances.available(base) < qty {
			return ErrInsufficientFunds
		}

		balances.adjust(base, -qty)
		balances.adjust(quote, cost-fee)

		ledgerRows = []user.Balance{
			balances.ledgerRow(base, -qty, 0, refID, stamp),
			balances.ledgerRow(quote, cost-fee, fee, refID, stamp),
		}
	}

	for _, row := range ledgerRows {
		user.PublishBalancesRaw(balances.raw, user.BalanceUpdate, []user.Balance{row})
	}

	balances.PublishUI()

	return nil
}

func (balances *Balances) ledgerRow(
	asset string,
	amount, fee float64,
	refID, stamp string,
) user.Balance {
	return user.Balance{
		Asset:      asset,
		AssetClass: "currency",
		Amount:     amount,
		Balance:    balances.available(asset),
		Fee:        fee,
		LedgerID:   balances.ids.LedgerID(),
		RefID:      refID,
		Timestamp:  stamp,
		Type:       "trade",
		Category:   "trade",
		WalletType: "spot",
		WalletID:   "main",
	}
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
		Asset:      asset,
		AssetClass: "currency",
		Balance:    delta,
		Wallets:    []user.BalanceWallet{{Balance: delta, Type: "spot", ID: "main"}},
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
