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
