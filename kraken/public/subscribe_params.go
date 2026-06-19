package public

func instrumentSubscribeParams() map[string]any {
	return map[string]any{
		"channel":  "instrument",
		"snapshot": true,
	}
}

func bookSubscribeParams(symbols []string, depth int) map[string]any {
	return map[string]any{
		"channel":  "book",
		"symbol":   symbols,
		"depth":    depth,
		"snapshot": true,
	}
}

func tradeSubscribeParams(symbols []string) map[string]any {
	return map[string]any{
		"channel":  "trade",
		"symbol":   symbols,
		"snapshot": true,
	}
}

func tickerSubscribeParams(symbols []string) map[string]any {
	return map[string]any{
		"channel":  "ticker",
		"symbol":   symbols,
		"snapshot": true,
	}
}
