package cmd

import (
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/wallet"
)

func newTradingWallet() *wallet.Wallet {
	walletType := wallet.PaperWallet

	if config.System.LiveTradingEnabled &&
		config.System.KrakenAPIKey != "" &&
		config.System.KrakenAPISecret != "" {
		walletType = wallet.CryptoWallet
	}

	return wallet.NewWallet(
		walletType,
		config.System.QuoteCurrency,
		config.System.WalletEUR,
		config.System.TakerFeePct,
	)
}
