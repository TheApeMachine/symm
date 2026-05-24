package trader

type signalReading struct {
	source         string
	confidence     float64
	expectedReturn float64
}

type symbolReadings map[string]signalReading

func walletBalance(wallet *Wallet) float64 {
	if wallet == nil {
		return 0
	}

	return wallet.Balance
}

func walletReserved(wallet *Wallet) float64 {
	if wallet == nil {
		return 0
	}

	return wallet.ReservedEUR
}
