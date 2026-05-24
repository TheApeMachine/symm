package ui

import "sync"

/*
ChartWatch tracks symbols whose ticker stream should be mirrored to dashboard
price_tick frames.
*/
type ChartWatch struct {
	mu      sync.RWMutex
	symbols map[string]struct{}
}

/*
NewChartWatch creates a ticker mirror watch set seeded with default symbols.
*/
func NewChartWatch(symbols ...string) *ChartWatch {
	chartWatch := &ChartWatch{
		symbols: make(map[string]struct{}, len(symbols)),
	}

	chartWatch.Subscribe(symbols)

	return chartWatch
}

/*
Has reports whether a symbol should be mirrored to the dashboard.
*/
func (chartWatch *ChartWatch) Has(symbol string) bool {
	if chartWatch == nil || symbol == "" {
		return false
	}

	chartWatch.mu.RLock()
	defer chartWatch.mu.RUnlock()

	_, ok := chartWatch.symbols[symbol]

	return ok
}

/*
Subscribe adds symbols to the dashboard ticker mirror set.
*/
func (chartWatch *ChartWatch) Subscribe(symbols []string) {
	if chartWatch == nil {
		return
	}

	chartWatch.mu.Lock()
	defer chartWatch.mu.Unlock()

	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		chartWatch.symbols[symbol] = struct{}{}
	}
}

/*
Unsubscribe removes symbols from the dashboard ticker mirror set.
*/
func (chartWatch *ChartWatch) Unsubscribe(symbols []string) {
	if chartWatch == nil {
		return
	}

	chartWatch.mu.Lock()
	defer chartWatch.mu.Unlock()

	for _, symbol := range symbols {
		delete(chartWatch.symbols, symbol)
	}
}

/*
Replace keeps only the supplied symbols in the dashboard ticker mirror set.
*/
func (chartWatch *ChartWatch) Replace(symbols []string) {
	if chartWatch == nil {
		return
	}

	chartWatch.mu.Lock()
	defer chartWatch.mu.Unlock()

	chartWatch.symbols = make(map[string]struct{}, len(symbols))

	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		chartWatch.symbols[symbol] = struct{}{}
	}
}
