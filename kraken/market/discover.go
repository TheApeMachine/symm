package market

import (
	"context"
	"sort"
	"strings"
	"time"
)

// discoveryTimeout bounds how long DiscoverSymbols waits for the instrument
// snapshot before giving up and letting the caller fall back to a default list.
const discoveryTimeout = 8 * time.Second

// pairOnline is the Kraken instrument status for a pair that can be traded now.
const pairOnline = "online"

/*
DiscoverSymbols returns every online pair quoted in quote (e.g. "EUR") from
Kraken's instrument snapshot. This is the universe the signals watch: all
tradable coins, not a hand-picked list. It blocks until the snapshot arrives or
discoveryTimeout elapses; on timeout or error it returns nil so the caller keeps
its fallback. quote == "" returns every online pair.
*/
func DiscoverSymbols(parent context.Context, quote string) []string {
	ctx, cancel := context.WithTimeout(parent, discoveryTimeout)
	defer cancel()

	updates := NewInstrumentSubscription(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil
		case update, ok := <-updates:
			if !ok {
				return nil
			}

			if update == nil {
				continue
			}

			return onlinePairs(update.Pairs, quote)
		}
	}
}

// onlinePairs filters the instrument catalog to online pairs in quote, sorted so
// the universe is stable across restarts.
func onlinePairs(pairs []InstrumentPair, quote string) []string {
	symbols := make([]string, 0, len(pairs))
	suffix := "/" + quote

	for _, pair := range pairs {
		if pair.Status != pairOnline || pair.Symbol == "" {
			continue
		}

		if quote != "" && !strings.HasSuffix(pair.Symbol, suffix) {
			continue
		}

		symbols = append(symbols, pair.Symbol)
	}

	sort.Strings(symbols)

	return symbols
}
