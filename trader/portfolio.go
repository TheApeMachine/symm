package trader

type Portfolio struct {
	trades map[string]*Trade
}

func NewPortfolio(wallet *Wallet) *Portfolio {
	return &Portfolio{}
}
