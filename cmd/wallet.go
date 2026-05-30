package cmd

import (
	"github.com/theapemachine/errnie"
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

	if config.System.LiveTradingEnabled &&
		(config.System.KrakenAPIKey == "" || config.System.KrakenAPISecret == "") {
		errnie.Warn(
			"SYMM_LIVE set but Kraken credentials missing; falling back to paper wallet",
			map[string]any{
				"live_trading_enabled": config.System.LiveTradingEnabled,
				"has_api_key":          config.System.KrakenAPIKey != "",
				"has_api_secret":       config.System.KrakenAPISecret != "",
			},
		)
	}

	return wallet.NewWallet(
		walletType,
		config.System.QuoteCurrency,
		config.System.WalletEUR,
		config.System.TakerFeePct,
	)
}
