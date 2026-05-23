package trader

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
Wallet is a wallet that holds the funds for the trader.
*/
type Wallet struct {
	Type     WalletType
	Currency string
	Balance  float64
	FeePct   float64
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
		Type:     walletType,
		Currency: currency,
		Balance:  balance,
		FeePct:   feePct,
	}
}
