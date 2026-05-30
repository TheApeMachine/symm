package market

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
PairCatalog indexes Kraken AssetPairs metadata by dashboard symbol (BTC/EUR).
*/
type PairCatalog struct {
	bySymbol map[string]*Pair
}

var catalogStore atomic.Pointer[PairCatalog]

type catalogSettings struct {
	feeVolume30d  float64
	fallbackTaker float64
}

var catalogConfig atomic.Value // catalogSettings

/*
SetCatalog installs the global pair catalog used by the paper desk for fees and
lot sizing. Nil clears the catalog.
*/
func SetCatalog(catalog *PairCatalog) {
	catalogStore.Store(catalog)
}

/*
Catalog returns the loaded pair catalog, or nil before boot completes.
*/
func Catalog() *PairCatalog {
	return catalogStore.Load()
}

/*
LoadPairCatalog fetches tradable pair metadata from Kraken REST AssetPairs.
*/
func LoadPairCatalog(ctx context.Context) (*PairCatalog, error) {
	client := public.NewRest(ctx, public.EndpointTypeAssetPairs)
	pairs, err := NewAssetPairs(ctx, client)

	if err != nil {
		return nil, err
	}

	catalog := &PairCatalog{bySymbol: make(map[string]*Pair, len(pairs))}

	for _, pair := range pairs {
		if pair == nil {
			continue
		}

		if pair.Wsname != "" {
			catalog.bySymbol[pair.Wsname] = pair
		}

		if pair.Altname != "" {
			catalog.bySymbol[normalizePairSymbol(pair.Altname)] = pair
		}
	}

	return catalog, nil
}

/*
Lookup returns REST metadata for a dashboard symbol such as BTC/EUR.
*/
func (catalog *PairCatalog) Lookup(symbol string) *Pair {
	if catalog == nil {
		return nil
	}

	if pair, ok := catalog.bySymbol[symbol]; ok {
		return pair
	}

	return catalog.bySymbol[normalizePairSymbol(symbol)]
}

/*
ConfigureCatalogFees sets the 30d volume tier and fallback taker fee used by
TakerFeePercent. Call from cmd after config is loaded.
*/
func ConfigureCatalogFees(feeVolume30d, fallbackTakerPct float64) {
	catalogConfig.Store(catalogSettings{
		feeVolume30d:  feeVolume30d,
		fallbackTaker: fallbackTakerPct,
	})
}

func catalogFees() catalogSettings {
	settings, ok := catalogConfig.Load().(catalogSettings)

	if !ok {
		return catalogSettings{fallbackTaker: defaultTakerFeePct}
	}

	return settings
}

/*
TakerFeePercent resolves the taker fee for symbol at the configured 30d volume tier.
*/
func (catalog *PairCatalog) TakerFeePercent(symbol string) float64 {
	pair := catalog.Lookup(symbol)
	settings := catalogFees()

	if pair == nil {
		return settings.fallbackTaker
	}

	return pair.TakerFeePercent(settings.feeVolume30d, settings.fallbackTaker)
}

func normalizePairSymbol(symbol string) string {
	if strings.Contains(symbol, "/") {
		return symbol
	}

	if len(symbol) >= 6 {
		return symbol[0:3] + "/" + symbol[3:]
	}

	return symbol
}

/*
BootPairCatalog loads the catalog and logs failures without aborting startup.
*/
func BootPairCatalog(ctx context.Context, feeVolume30d, fallbackTakerPct float64) {
	ConfigureCatalogFees(feeVolume30d, fallbackTakerPct)

	catalog, err := LoadPairCatalog(ctx)

	if err != nil {
		errnie.Error(err)

		return
	}

	SetCatalog(catalog)
}
