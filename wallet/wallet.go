package wallet

import (
	"fmt"
	"maps"
	"sync"

	"github.com/theapemachine/symm/config"
)

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
	mu          sync.Mutex
	Type        WalletType
	Currency    string
	Balance     float64
	ReservedEUR float64
	FeePct      float64
	Inventory   map[string]float64
	AvgEntry    map[string]float64
	Marks       map[string]float64
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
		Type:      walletType,
		Currency:  currency,
		Balance:   balance,
		FeePct:    feePct,
		Inventory: make(map[string]float64),
		AvgEntry:  make(map[string]float64),
	}
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
	}
}

/*
AddInventory atomically credits base inventory and records the fill economics.
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
	wallet.mu.Unlock()
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
