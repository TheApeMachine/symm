package response

import (
	"errors"
	"fmt"
	"strings"
	"sync"
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

mu guards model/costBasis/realized: fills arrive both from the paper matching
tick (resting triggers, pending takers) and from the quote cache's trade-listener
goroutine (maker queue fills), so wallet state is mutated from two goroutines.
*/
type Balances struct {
	mu        sync.Mutex
	quote     string
	model     user.Balances
	raw       *qpool.BroadcastGroup
	ui        *qpool.BroadcastGroup
	ids       *Identifier
	costBasis map[string]float64 // fee-inclusive average cost per base asset
	realized  float64            // running net realized P&L over the session
}

func NewBalances(raw, ui *qpool.BroadcastGroup, ids *Identifier) *Balances {
	quote := viper.GetViper().GetString("market.quote_currency")
	balance := viper.GetViper().GetFloat64("trading.paper.wallet_" + strings.ToLower(quote))

	balances := &Balances{
		quote:     quote,
		raw:       raw,
		ui:        ui,
		ids:       ids,
		costBasis: make(map[string]float64),
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
	balances.mu.Lock()
	defer balances.mu.Unlock()

	user.PublishBalancesRaw(balances.raw, user.BalanceSnapshot, balances.model.Asset)
	user.PublishHoldingsDerived(balances.raw, balances.model.Asset)
	balances.publishUILocked()
}

/*
PublishHoldingsSnapshot re-emits the wallet's holdings view so the trader can
reconcile its inventory on a cadence, not only at subscribe time — a lost
execution frame otherwise diverges trader and wallet until restart.
*/
func (balances *Balances) PublishHoldingsSnapshot() {
	balances.mu.Lock()
	defer balances.mu.Unlock()

	user.PublishHoldingsDerived(balances.raw, balances.model.Asset)
}

/*
FillOutcome reports what one settled fill did to the wallet. Settled is true for a
sell of a held base; Realized is the fee-inclusive round-trip P&L for the sold
quantity and EntryBasis the per-unit average cost it was bought at. The wallet is
the single source of truth for outcomes — the trader audits these instead of
recomputing an entry-fee-blind estimate of its own.
*/
type FillOutcome struct {
	Settled    bool
	Realized   float64
	EntryBasis float64
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
) (FillOutcome, error) {
	balances.mu.Lock()
	defer balances.mu.Unlock()

	slash := strings.IndexByte(symbol, '/')

	if slash <= 0 {
		return FillOutcome{}, fmt.Errorf("paper balances: malformed symbol %q", symbol)
	}

	base := symbol[:slash]
	quote := symbol[slash+1:]
	cost := qty * price
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	outcome := FillOutcome{}

	var ledgerRows []user.Balance

	if side == "buy" {
		if balances.available(quote) < cost+fee {
			return FillOutcome{}, ErrInsufficientFunds
		}

		// Fold the lot into the fee-inclusive average cost before crediting it, so a
		// later sell can realise the true round-trip P&L.
		balances.foldCostBasis(base, balances.available(base), qty, cost+fee)
		balances.adjust(quote, -(cost + fee))
		balances.adjust(base, qty)

		ledgerRows = []user.Balance{
			balances.ledgerRow(base, qty, 0, refID, stamp),
			balances.ledgerRow(quote, -(cost + fee), fee, refID, stamp),
		}
	} else {
		if balances.available(base) < qty {
			return FillOutcome{}, ErrInsufficientFunds
		}

		// Realise P&L for the sold quantity against its average cost: proceeds net of
		// the sell fee, minus the fee-inclusive cost it was bought at.
		basis := balances.costBasis[base]
		realized := (cost - fee) - qty*basis
		balances.realized += realized
		outcome = FillOutcome{Settled: true, Realized: realized, EntryBasis: basis}

		balances.adjust(base, -qty)
		balances.adjust(quote, cost-fee)

		if balances.available(base) <= 0 {
			delete(balances.costBasis, base) // position flat — drop its basis
		}

		ledgerRows = []user.Balance{
			balances.ledgerRow(base, -qty, 0, refID, stamp),
			balances.ledgerRow(quote, cost-fee, fee, refID, stamp),
		}
	}

	for _, row := range ledgerRows {
		user.PublishBalancesRaw(balances.raw, user.BalanceUpdate, []user.Balance{row})
	}

	balances.publishUILocked()

	return outcome, nil
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

// foldCostBasis updates the fee-inclusive average cost of base after buying qty for
// spent (cost + fee), weighting the prior holding (prevQty) against the new lot.
func (balances *Balances) foldCostBasis(base string, prevQty, qty, spent float64) {
	newQty := prevQty + qty

	if newQty <= 0 {
		return
	}

	balances.costBasis[base] = (prevQty*balances.costBasis[base] + spent) / newQty
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

/*
RealizedPnL returns the session net realized P&L in the quote currency.
*/
func (balances *Balances) RealizedPnL() float64 {
	return balances.realized
}

func (balances *Balances) PublishUI() {
	balances.mu.Lock()
	defer balances.mu.Unlock()

	balances.publishUILocked()
}

func (balances *Balances) publishUILocked() {
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
		"realized":  balances.realized,
		"Balance":   cash,
		"Inventory": inventory,
	}})
}
