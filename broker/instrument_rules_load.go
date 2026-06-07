package broker

import (
	"context"
	"fmt"

	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
)

/*
LoadInstrumentRulesFromKraken fetches AssetPairs and seeds one InstrumentRulesCache.
*/
func LoadInstrumentRulesFromKraken(ctx context.Context) (*InstrumentRulesCache, int, error) {
	rest := public.NewRest(ctx, public.EndpointTypeAssetPairs)

	pairs, err := market.NewAssetPairs(ctx, rest)

	rest.Close()

	if err != nil {
		return nil, 0, fmt.Errorf("broker: load instrument rules: %w", err)
	}

	cache := NewInstrumentRulesCache(ctx)
	loaded := cache.LoadFromAssetPairs(pairs)

	if loaded == 0 {
		return nil, 0, fmt.Errorf("broker: load instrument rules: empty AssetPairs catalog")
	}

	return cache, loaded, nil
}

/*
SeedFromKraken fills an EXISTING cache from the REST AssetPairs catalog — the
same source `symm tune` loads 1,500+ pairs from. The live engine previously
relied solely on the websocket instrument snapshot; when that was missed or
partial, the desk rejected every entry with "missing instrument rules", forever.
REST seeds the floor; the websocket snapshot keeps it fresh.
*/
func (cache *InstrumentRulesCache) SeedFromKraken(ctx context.Context) (int, error) {
	if cache == nil {
		return 0, fmt.Errorf("broker: instrument rules cache is required")
	}

	rest := public.NewRest(ctx, public.EndpointTypeAssetPairs)

	pairs, err := market.NewAssetPairs(ctx, rest)

	rest.Close()

	if err != nil {
		return 0, fmt.Errorf("broker: seed instrument rules: %w", err)
	}

	loaded := cache.LoadFromAssetPairs(pairs)

	if loaded == 0 {
		return 0, fmt.Errorf("broker: seed instrument rules: empty AssetPairs catalog")
	}

	return loaded, nil
}

/*
Size reports how many pairs the cache currently holds, so a watchdog can tell
"market is quiet" apart from "the desk cannot validate any order".
*/
func (cache *InstrumentRulesCache) Size() int {
	if cache == nil {
		return 0
	}

	count := 0

	cache.pairs.Range(func(_, _ any) bool {
		count++

		return true
	})

	return count
}

/*
LoadFromAssetPairs installs every tradable pair from REST metadata into the cache.
*/
func (cache *InstrumentRulesCache) LoadFromAssetPairs(pairs market.AssetPairs) int {
	if cache == nil {
		return 0
	}

	loaded := 0

	for _, pair := range pairs {
		instrument, err := market.InstrumentPairFromREST(pair)

		if err != nil {
			continue
		}

		cache.InstallPair(instrument)
		loaded++
	}

	return loaded
}
