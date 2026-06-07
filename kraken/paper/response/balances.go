package response

import (
	"errors"
	"math/big"
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
	mu    sync.Mutex
	quote string
	model user.Balances
	raw   *qpool.BroadcastGroup
	ui    *qpool.BroadcastGroup
	ids   *Identifier
	// The ledger of record is exact rational arithmetic (math/big): float
	// accumulation across fills is how a wallet ends up holding
	// 19999.99999000 of a 20000-minimum asset. Floats exist only at the
	// edges — wire frames and UI — never in the book.
	holdings  map[string]*big.Rat
	costBasis map[string]*big.Rat // fee-inclusive average cost per base asset
	realized  *big.Rat            // running net realized P&L over the session
}

func NewBalances(raw, ui *qpool.BroadcastGroup, ids *Identifier) *Balances {
	quote := viper.GetViper().GetString("market.quote_currency")
	balance := viper.GetViper().GetFloat64("trading.paper.wallet_" + strings.ToLower(quote))

	balances := &Balances{
		quote:     quote,
		raw:       raw,
		ui:        ui,
		ids:       ids,
		holdings:  map[string]*big.Rat{quote: new(big.Rat).SetFloat64(balance)},
		costBasis: make(map[string]*big.Rat),
		realized:  new(big.Rat),
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
	qtyRat := new(big.Rat).SetFloat64(qty)
	feeRat := new(big.Rat).SetFloat64(fee)
	priceRat := new(big.Rat).SetFloat64(price)
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	outcome := FillOutcome{}

	var ledgerRows []user.Balance

	if side == "buy" {
		costRat := new(big.Rat).Mul(qtyRat, priceRat)
		spentRat := new(big.Rat).Add(costRat, feeRat)

		if balances.availableRat(quote).Cmp(spentRat) < 0 {
			return FillOutcome{}, ErrInsufficientFunds
		}

		// Fold the lot into the fee-inclusive average cost before crediting it, so a
		// later sell can realise the true round-trip P&L.
		balances.foldCostBasis(base, balances.availableRat(base), qtyRat, spentRat)
		balances.adjust(quote, new(big.Rat).Neg(spentRat))
		balances.adjust(base, qtyRat)

		spent, _ := spentRat.Float64()
		ledgerRows = []user.Balance{
			balances.ledgerRow(base, qty, 0, refID, stamp),
			balances.ledgerRow(quote, -spent, fee, refID, stamp),
		}
	} else {
		held := balances.availableRat(base)

		if held.Cmp(qtyRat) < 0 {
			// A seller quoting the float echo of its exact holdings can sit one
			// ulp above the ledger value — rejecting that would make every full
			// close one ulp impossible. Sub-ppb overshoot clamps to "sell all";
			// a genuine oversell still rejects.
			overshoot := new(big.Rat).Sub(qtyRat, held)
			tolerance := new(big.Rat).Mul(qtyRat, big.NewRat(1, 1_000_000_000))

			if held.Sign() <= 0 || overshoot.Cmp(tolerance) > 0 {
				return FillOutcome{}, ErrInsufficientFunds
			}

			qtyRat = new(big.Rat).Set(held)
		}

		costRat := new(big.Rat).Mul(qtyRat, priceRat)

		// Realise P&L for the sold quantity against its average cost: proceeds net of
		// the sell fee, minus the fee-inclusive cost it was bought at.
		basis := balances.basisRat(base)
		proceeds := new(big.Rat).Sub(costRat, feeRat)
		realizedRat := new(big.Rat).Sub(proceeds, new(big.Rat).Mul(qtyRat, basis))
		balances.realized.Add(balances.realized, realizedRat)

		realized, _ := realizedRat.Float64()
		entryBasis, _ := basis.Float64()
		outcome = FillOutcome{Settled: true, Realized: realized, EntryBasis: entryBasis}

		balances.adjust(base, new(big.Rat).Neg(qtyRat))
		balances.adjust(quote, proceeds)

		// Exact arithmetic means a full sell lands on EXACT zero — the float
		// ledger could strand 1e-15 dust here, keeping a stale basis alive.
		if balances.availableRat(base).Sign() <= 0 {
			delete(balances.costBasis, base) // position flat — drop its basis
		}

		netProceeds, _ := proceeds.Float64()
		ledgerRows = []user.Balance{
			balances.ledgerRow(base, -qty, 0, refID, stamp),
			balances.ledgerRow(quote, netProceeds, fee, refID, stamp),
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

// foldCostBasis updates the fee-inclusive average cost of base after buying qty
// for spent (cost + fee), weighting the prior holding against the new lot —
// exactly.
func (balances *Balances) foldCostBasis(base string, prevQty, qty, spent *big.Rat) {
	newQty := new(big.Rat).Add(prevQty, qty)

	if newQty.Sign() <= 0 {
		return
	}

	weighted := new(big.Rat).Mul(prevQty, balances.basisRat(base))
	weighted.Add(weighted, spent)

	balances.costBasis[base] = weighted.Quo(weighted, newQty)
}

func (balances *Balances) basisRat(asset string) *big.Rat {
	if basis := balances.costBasis[asset]; basis != nil {
		return basis
	}

	return new(big.Rat)
}

func (balances *Balances) availableRat(asset string) *big.Rat {
	if held := balances.holdings[asset]; held != nil {
		return held
	}

	return new(big.Rat)
}

func (balances *Balances) available(asset string) float64 {
	value, _ := balances.availableRat(asset).Float64()

	return value
}

func (balances *Balances) adjust(asset string, delta *big.Rat) {
	held := balances.holdings[asset]

	if held == nil {
		if delta.Sign() <= 0 {
			return
		}

		held = new(big.Rat)
		balances.holdings[asset] = held
	}

	held.Add(held, delta)
	balance, _ := held.Float64()

	for index, bal := range balances.model.Asset {
		if bal.Asset != asset {
			continue
		}

		balances.model.Asset[index].Balance = balance

		if len(balances.model.Asset[index].Wallets) > 0 {
			balances.model.Asset[index].Wallets[0].Balance = balance
		}

		return
	}

	balances.model.Asset = append(balances.model.Asset, user.Balance{
		Asset:      asset,
		AssetClass: "currency",
		Balance:    balance,
		Wallets:    []user.BalanceWallet{{Balance: balance, Type: "spot", ID: "main"}},
	})
}

/*
RealizedPnL returns the session net realized P&L in the quote currency.
*/
func (balances *Balances) RealizedPnL() float64 {
	balances.mu.Lock()
	defer balances.mu.Unlock()

	return balances.realizedLocked()
}

func (balances *Balances) realizedLocked() float64 {
	value, _ := balances.realized.Float64()

	return value
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
		"realized":  balances.realizedLocked(),
		"Balance":   cash,
		"Inventory": inventory,
	}})
}
