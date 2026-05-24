package trader

import "fmt"

/*
CreditBase adds spot inventory received from one long entry fill.
*/
func (wallet *Wallet) CreditBase(symbol string, baseQty float64) error {
	if wallet == nil {
		return fmt.Errorf("wallet is required")
	}

	if symbol == "" || baseQty <= 0 {
		return fmt.Errorf("invalid base credit")
	}

	if wallet.Inventory == nil {
		wallet.Inventory = make(map[string]float64)
	}

	wallet.Inventory[symbol] += baseQty

	return nil
}

/*
DebitBase removes spot inventory spent on one exit fill.
*/
func (wallet *Wallet) DebitBase(symbol string, baseQty float64) error {
	if wallet == nil {
		return fmt.Errorf("wallet is required")
	}

	if symbol == "" || baseQty <= 0 {
		return fmt.Errorf("invalid base debit")
	}

	if wallet.AvailableBase(symbol) < baseQty {
		return fmt.Errorf("insufficient %s inventory", symbol)
	}

	wallet.Inventory[symbol] -= baseQty

	if wallet.Inventory[symbol] <= 0 {
		delete(wallet.Inventory, symbol)
	}

	return nil
}

/*
AvailableBase returns free base asset quantity for one symbol.
*/
func (wallet *Wallet) AvailableBase(symbol string) float64 {
	if wallet == nil || symbol == "" || wallet.Inventory == nil {
		return 0
	}

	return wallet.Inventory[symbol]
}
