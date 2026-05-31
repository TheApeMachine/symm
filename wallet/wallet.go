package wallet

import (
	"fmt"
	"log"
	"maps"
	"sync"
	"time"

	"github.com/theapemachine/symm/config"
)

const seenFillCap = 4096

/*
WalletType is the type of wallet.
*/
type WalletType uint8

/*
WalletType constants.
*/
const (
	PaperWallet WalletType = iota
	CryptoWallet
)

/*
Wallet is spot cash for the trading engine: available balance plus entry reservations.

mu guards every mutable field. Direct field access from outside the wallet
package is unsafe across goroutines; use the methods on this type instead.
*/
type Wallet struct {
	mu           sync.Mutex
	Type         WalletType
	Currency     string
	Balance      float64
	ReservedEUR  float64
	FeePct       float64
	Inventory    map[string]float64
	AvgEntry     map[string]float64
	Marks        map[string]float64
	Positions    map[string]PositionBinding
	seenFills    map[string]struct{}
	seenFillRing []string
	seenFillHead int
}

/*
PositionBinding records the prediction maturity that authorized one open
position. Observational predictions for the same base must not close it.
*/
type PositionBinding struct {
	Source         string
	Playbook       string // perspective that authorized entry (trend, drive, pump, …)
	Regime         string // optional pump-regime tag (pump_fast / pump_slow); empty otherwise
	EntryScore     float64
	PredictedAt    time.Time
	DueAt          time.Time
	HasLotDecimals bool
	LotDecimals    int
	EntryFeePct    float64 // real per-pair fee charged on the entry fill
	ExitFeePct     float64 // real per-pair fee expected on the exit sell
	TakerFeePct    float64 // legacy exit-fee field for existing bindings
	Exploratory    bool    // opened by exploration (cold bucket), not the disciplined edge gate
}

/*
NewWallet creates a new wallet.
*/
func NewWallet(
	walletType WalletType,
	currency string,
	balance float64,
	feePct float64,
) *Wallet {
	return &Wallet{
		Type:         walletType,
		Currency:     currency,
		Balance:      balance,
		FeePct:       feePct,
		Inventory:    make(map[string]float64),
		AvgEntry:     make(map[string]float64),
		Positions:    make(map[string]PositionBinding),
		seenFills:    make(map[string]struct{}, seenFillCap),
		seenFillRing: make([]string, seenFillCap),
	}
}

/*
SeenFill reports whether execKey has already been applied. Empty keys are
never deduped — they are treated as un-keyed and applied unconditionally.
*/
func (wallet *Wallet) SeenFill(execKey string) bool {
	if wallet == nil || execKey == "" {
		return false
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	_, ok := wallet.seenFills[execKey]

	return ok
}

/*
MarkFill records execKey in a bounded LRU-style ring so subsequent SeenFill
returns true. Empty keys are ignored.
*/
func (wallet *Wallet) MarkFill(execKey string) {
	if wallet == nil || execKey == "" {
		return
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	wallet.markFillLocked(execKey)
}

func (wallet *Wallet) markFillLocked(execKey string) {
	if wallet.seenFills == nil {
		wallet.seenFills = make(map[string]struct{}, seenFillCap)
	}

	if wallet.seenFillRing == nil {
		wallet.seenFillRing = make([]string, seenFillCap)
	}

	if _, exists := wallet.seenFills[execKey]; exists {
		return
	}

	evicted := wallet.seenFillRing[wallet.seenFillHead]

	if evicted != "" {
		delete(wallet.seenFills, evicted)
	}

	wallet.seenFillRing[wallet.seenFillHead] = execKey
	wallet.seenFillHead = (wallet.seenFillHead + 1) % len(wallet.seenFillRing)
	wallet.seenFills[execKey] = struct{}{}
}

/*
Snapshot copies wallet state for immutable cross-goroutine publication.
*/
func (wallet *Wallet) Snapshot() *Wallet {
	if wallet == nil {
		return nil
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	return &Wallet{
		Type:        wallet.Type,
		Currency:    wallet.Currency,
		Balance:     wallet.Balance,
		ReservedEUR: wallet.ReservedEUR,
		FeePct:      wallet.FeePct,
		Inventory:   copyFloatMap(wallet.Inventory),
		AvgEntry:    copyFloatMap(wallet.AvgEntry),
		Marks:       copyFloatMap(wallet.Marks),
		Positions:   copyPositionMap(wallet.Positions),
	}
}

/*
BalanceCopy returns the balance under the wallet lock for safe external reads.
*/
func (wallet *Wallet) BalanceCopy() float64 {
	if wallet == nil {
		return 0
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	return wallet.Balance
}

/*
ReservedCopy returns reserved cash under the wallet lock.
*/
func (wallet *Wallet) ReservedCopy() float64 {
	if wallet == nil {
		return 0
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	return wallet.ReservedEUR
}

/*
AddInventory atomically credits base inventory and records the fill economics
using fillPrice as the per-unit cost basis. Prefer AddInventoryWithCost when
the fee is paid in the quote currency: fillPrice underestimates true cost by
the fee fraction, biasing realized PnL.
*/
func (wallet *Wallet) AddInventory(base string, qty, fillPrice float64) {
	if wallet == nil || base == "" || qty <= 0 {
		return
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	wallet.inventoryAddLocked(base, qty)

	if fillPrice > 0 {
		wallet.recordFillLocked(base, qty, fillPrice)
	}
}

/*
AddInventoryWithCost atomically credits base inventory and records cost basis
from the actual cash spent on the fill (including fees). The recorded average
entry is cashSpent/qty, which is what AvgEntryFor returns and what realized
PnL is computed against. Cash leaves the wallet through ReserveEntry /
SettleEntryReservation independently.
*/
func (wallet *Wallet) AddInventoryWithCost(base string, qty, cashSpent float64) {
	if wallet == nil || base == "" || qty <= 0 {
		return
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	wallet.inventoryAddLocked(base, qty)

	if cashSpent > 0 {
		wallet.recordFillLocked(base, qty, cashSpent/qty)
	}
}

/*
ApplyFill records a live fill atomically: dedupes against execKey, credits or
debits inventory, settles any matching reservation, and records cost basis from
cashDelta. side is "buy" or "sell". Returns true when the fill was applied,
false when execKey was already seen.
*/
func (wallet *Wallet) ApplyFill(
	execKey, side, base string,
	qty, fillPrice, cashDelta float64,
) bool {
	if wallet == nil || base == "" || qty <= 0 || execKey == "" {
		return false
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	if _, seen := wallet.seenFills[execKey]; seen {
		return false
	}

	wallet.markFillLocked(execKey)

	switch side {
	case "buy":
		wallet.inventoryAddLocked(base, qty)

		if cashDelta > 0 {
			wallet.recordFillLocked(base, qty, cashDelta/qty)
		} else if fillPrice > 0 {
			wallet.recordFillLocked(base, qty, fillPrice)
		}
	case "sell":
		current := wallet.Inventory[base]

		if qty > current {
			// Exchange reported a sell larger than our tracked inventory.
			// Most likely cause: the local wallet missed a prior buy fill
			// (reconnect snapshot gap), or two clients are sharing the
			// account. We cap inventory at 0 and still credit the cash so
			// the exchange-side truth wins, but we surface the anomaly so
			// the operator notices.
			log.Printf(
				"wallet: sell qty %.10f exceeds tracked inventory %.10f for %s "+
					"(execKey=%q cashDelta=%.4f); inventory capped at 0",
				qty, current, base, execKey, cashDelta,
			)
		}

		if qty >= current {
			wallet.Inventory[base] = 0
			delete(wallet.AvgEntry, base)
			delete(wallet.Positions, base)
		} else {
			wallet.Inventory[base] = current - qty
		}

		wallet.Balance += cashDelta
	}

	return true
}

/*
ZeroInventory atomically returns the held quantity for base, zeroes it, and
clears any tracked average entry. The returned quantity is the position prior
to clearing.
*/
func (wallet *Wallet) ZeroInventory(base string) float64 {
	if wallet == nil || base == "" {
		return 0
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	qty := wallet.Inventory[base]
	wallet.Inventory[base] = 0
	delete(wallet.AvgEntry, base)
	delete(wallet.Positions, base)

	return qty
}

/*
CreditBalance applies a signed delta to Balance under the wallet lock.
*/
func (wallet *Wallet) CreditBalance(delta float64) {
	if wallet == nil || delta == 0 {
		return
	}

	wallet.mu.Lock()
	wallet.Balance += delta
	wallet.mu.Unlock()
}

/*
SetMarks replaces the mark-to-market price map under the wallet lock.
*/
func (wallet *Wallet) SetMarks(marks map[string]float64) {
	if wallet == nil {
		return
	}

	wallet.mu.Lock()
	wallet.Marks = copyFloatMap(marks)
	wallet.mu.Unlock()
}

/*
InventoryQty returns the held quantity for one base asset under the wallet
lock. Returns 0 when the wallet or base is unknown.
*/
func (wallet *Wallet) InventoryQty(base string) float64 {
	if wallet == nil || base == "" {
		return 0
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	return wallet.Inventory[base]
}

/*
InventoryCopy returns a detached snapshot of the inventory map suitable for
iteration outside the wallet lock.
*/
func (wallet *Wallet) InventoryCopy() map[string]float64 {
	if wallet == nil {
		return nil
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	return copyFloatMap(wallet.Inventory)
}

/*
AvgEntryFor returns the volume-weighted entry price for one base asset under
the wallet lock. Returns 0 when none is tracked.
*/
func (wallet *Wallet) AvgEntryFor(base string) float64 {
	if wallet == nil || base == "" {
		return 0
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	return wallet.AvgEntry[base]
}

func (wallet *Wallet) inventoryAddLocked(base string, qty float64) {
	if wallet.Inventory == nil {
		wallet.Inventory = make(map[string]float64)
	}

	wallet.Inventory[base] += qty
}

func (wallet *Wallet) recordFillLocked(base string, qty, fillPrice float64) {
	if wallet.AvgEntry == nil {
		wallet.AvgEntry = make(map[string]float64)
	}

	priorQty := wallet.Inventory[base] - qty

	if priorQty <= config.System.LiveInventoryEpsilon {
		wallet.AvgEntry[base] = fillPrice

		return
	}

	priorEntry := wallet.AvgEntry[base]

	if priorEntry <= 0 {
		wallet.AvgEntry[base] = fillPrice

		return
	}

	newQty := wallet.Inventory[base]

	if newQty <= 0 {
		return
	}

	wallet.AvgEntry[base] = (priorQty*priorEntry + qty*fillPrice) / newQty
}

/*
RecordFill updates the volume-weighted average entry for one base asset.
*/
func (wallet *Wallet) RecordFill(base string, qty, fillPrice float64) {
	if wallet == nil || base == "" || qty <= 0 || fillPrice <= 0 {
		return
	}

	wallet.mu.Lock()
	wallet.recordFillLocked(base, qty, fillPrice)
	wallet.mu.Unlock()
}

/*
ClearPosition removes tracked entry economics for one base asset.
*/
func (wallet *Wallet) ClearPosition(base string) {
	if wallet == nil || base == "" {
		return
	}

	wallet.mu.Lock()
	delete(wallet.AvgEntry, base)
	delete(wallet.Positions, base)
	wallet.mu.Unlock()
}

/*
BindPosition attaches an open base inventory slot to the prediction that
authorized it.
*/
func (wallet *Wallet) BindPosition(base string, binding PositionBinding) {
	if wallet == nil || base == "" || binding.Source == "" ||
		binding.PredictedAt.IsZero() || binding.DueAt.IsZero() {
		return
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	if wallet.Positions == nil {
		wallet.Positions = make(map[string]PositionBinding)
	}

	wallet.Positions[base] = binding
}

/*
PositionBindingFor returns the prediction binding for one base asset.
*/
func (wallet *Wallet) PositionBindingFor(base string) (PositionBinding, bool) {
	if wallet == nil || base == "" {
		return PositionBinding{}, false
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	binding, ok := wallet.Positions[base]

	return binding, ok
}

/*
PositionMatches reports whether base is still bound to the exact source and
maturity that opened it.
*/
func (wallet *Wallet) PositionMatches(base string, source string, dueAt time.Time) bool {
	if wallet == nil || base == "" || source == "" || dueAt.IsZero() {
		return false
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	binding, ok := wallet.Positions[base]

	if !ok {
		return false
	}

	return binding.Source == source && binding.DueAt.Equal(dueAt)
}

/*
AvailableEUR returns cash not reserved for in-flight entry orders.
*/
func (wallet *Wallet) AvailableEUR() float64 {
	if wallet == nil {
		return 0
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	return wallet.Balance
}

/*
Put moves cash from the wallet's balance to the reserved balance.
*/
func (wallet *Wallet) Put(amount float64) error {
	return wallet.Reserve(amount)
}

/*
Take moves cash from the reserved balance to the wallet's balance.
*/
func (wallet *Wallet) Take(amount float64) error {
	if wallet == nil || amount <= 0 {
		return fmt.Errorf("invalid amount")
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	if amount > wallet.ReservedEUR {
		amount = wallet.ReservedEUR
	}

	wallet.ReservedEUR -= amount
	wallet.Balance += amount

	return nil
}

/*
Reserve moves cash from the wallet's balance to the reserved balance.
*/
func (wallet *Wallet) Reserve(amount float64) error {
	if wallet == nil || amount <= 0 {
		return fmt.Errorf("invalid amount")
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	if wallet.Balance < amount {
		return fmt.Errorf("insufficient available cash")
	}

	wallet.Balance -= amount
	wallet.ReservedEUR += amount

	return nil
}

/*
ReserveEntry locks cash for one pending entry order.
*/
func (wallet *Wallet) ReserveEntry(amount float64) error {
	if wallet == nil {
		return fmt.Errorf("wallet is required")
	}

	if amount <= 0 {
		return fmt.Errorf("reservation amount must be positive")
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	if wallet.Balance < amount {
		return fmt.Errorf("insufficient available cash")
	}

	wallet.Balance -= amount
	wallet.ReservedEUR += amount

	return nil
}

/*
ReleaseEntryReservation returns reserved cash after a failed entry.
*/
func (wallet *Wallet) ReleaseEntryReservation(amount float64) {
	if wallet == nil || amount <= 0 {
		return
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	if amount > wallet.ReservedEUR {
		amount = wallet.ReservedEUR
	}

	wallet.ReservedEUR -= amount
	wallet.Balance += amount
}

/*
SettleEntryReservation spends reserved cash on a confirmed entry fill.
*/
func (wallet *Wallet) SettleEntryReservation(reserved, actualCost float64) error {
	if wallet == nil {
		return fmt.Errorf("wallet is required")
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	if reserved <= 0 {
		if actualCost > wallet.Balance {
			return fmt.Errorf("insufficient cash for entry")
		}

		wallet.Balance -= actualCost

		return nil
	}

	if reserved > wallet.ReservedEUR {
		return fmt.Errorf("reservation exceeds held cash")
	}

	wallet.ReservedEUR -= reserved

	if actualCost > reserved {
		extra := actualCost - reserved

		if wallet.Balance < extra {
			return fmt.Errorf("insufficient cash for entry overage")
		}

		wallet.Balance -= extra

		return nil
	}

	wallet.Balance += reserved - actualCost

	return nil
}

/*
MarkEquity is cash plus reserved entry cash and mark-to-market inventory.
*/
func (wallet *Wallet) MarkEquity(lastPrices map[string]float64) float64 {
	if wallet == nil {
		return 0
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	equity := wallet.Balance + wallet.ReservedEUR

	for base, qty := range wallet.Inventory {
		if qty <= 0 {
			continue
		}

		symbol := base + "/" + wallet.Currency
		last := lastPrices[symbol]

		if last <= 0 {
			continue
		}

		equity += qty * last
	}

	return equity
}

func copyFloatMap(source map[string]float64) map[string]float64 {
	if source == nil {
		return nil
	}

	copied := make(map[string]float64, len(source))
	maps.Copy(copied, source)

	return copied
}

func copyPositionMap(source map[string]PositionBinding) map[string]PositionBinding {
	if source == nil {
		return nil
	}

	return maps.Clone(source)
}
