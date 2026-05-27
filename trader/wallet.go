package trader

import "fmt"

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
Wallet is spot cash for the trader: available balance plus entry reservations.
*/
type Wallet struct {
	Type        WalletType
	Currency    string
	Balance     float64
	ReservedEUR float64
	FeePct      float64
	Inventory   map[string]float64
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
	}
}

/*
AvailableEUR returns cash not reserved for in-flight entry orders.
*/
func (wallet *Wallet) AvailableEUR() float64 {
	if wallet == nil {
		return 0
	}

	return wallet.Balance
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
