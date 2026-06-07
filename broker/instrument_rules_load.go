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
