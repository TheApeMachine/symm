package logic

/*
Balances holds portfolio inventory used during playbook evaluation.
*/
type Balances struct {
	Inventory map[string]float64 `json:"inventory"`
	Asset     []BalanceAsset     `json:"asset"`
}

/*
BalanceAsset is one asset row in a balances snapshot.
*/
type BalanceAsset struct {
	Asset   string  `json:"asset"`
	Balance float64 `json:"balance"`
}

/*
Held reports whether the ledger holds a positive balance for symbol, checking
both the inventory map and the asset rows.
*/
func (balances *Balances) Held(symbol string) bool {
	return symbolHeld(balances, symbol)
}
