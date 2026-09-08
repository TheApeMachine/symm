package types

func (envelope *Envelope) Symbol() string {
	if envelope == nil {
		return ""
	}

	if envelope.Key != "" {
		return envelope.Key
	}

	if envelope.TickerData.Symbol != "" {
		return envelope.TickerData.Symbol
	}

	if envelope.TradeData.Symbol != "" {
		return envelope.TradeData.Symbol
	}

	if envelope.Level3Data.Symbol != "" {
		return envelope.Level3Data.Symbol
	}

	if envelope.StrategyRound != nil && envelope.StrategyRound.Symbol != "" {
		return envelope.StrategyRound.Symbol
	}

	if len(envelope.Opportunities) > 0 && envelope.Opportunities[0] != nil && envelope.Opportunities[0].Symbol != "" {
		return envelope.Opportunities[0].Symbol
	}

	return ""
}
