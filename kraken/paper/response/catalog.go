package response

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
	"golang.org/x/sync/singleflight"
)

const assetPairsCacheKey = "asset_pairs"

type depthBook struct {
	Bids [][]any `json:"bids"`
	Asks [][]any `json:"asks"`
}

type cachedDepthBook struct {
	book      depthBook
	expiresAt time.Time
}

/*
PairCatalog resolves Kraken AssetPairs metadata through REST when paper fills need it.

The only session state kept locally is the simulated fee-tier volume ledger.
*/
type PairCatalog struct {
	ctx              context.Context
	volumes          sync.Map
	pairCache        sync.Map
	depthCache       sync.Map
	liveBooks        sync.Map
	depthTTL         atomic.Int64
	assetPairsAPI    public.EndpointType
	depthAPI         public.EndpointType
	assetPairsFlight singleflight.Group
	depthFlight      singleflight.Group
}

/*
NewPairCatalog builds a REST-backed pair resolver for paper simulation.
*/
func NewPairCatalog(ctx context.Context) *PairCatalog {
	return &PairCatalog{
		ctx: ctx,
	}
}

/*
RestPair returns the Kraken REST pair identifier for one ws symbol.
*/
func (catalog *PairCatalog) RestPair(symbol string) (string, error) {
	pair, err := catalog.fetchPair(symbol)

	if err != nil {
		return "", err
	}

	if pair.Altname == "" {
		return "", fmt.Errorf("paper pair catalog: altname missing for %s", symbol)
	}

	return pair.Altname, nil
}

func (catalog *PairCatalog) fetchPair(symbol string) (*market.Pair, error) {
	if catalog == nil {
		return nil, fmt.Errorf("paper pair catalog: nil catalog")
	}

	if pair, found := catalog.cachedPair(symbol); found {
		return pair, nil
	}

	pairs, pairErr := catalog.assetPairs()

	if pairErr != nil {
		return nil, pairErr
	}

	pair, pairErr := pairs.PairByWsname(symbol)

	if pairErr != nil {
		return nil, pairErr
	}

	catalog.pairCache.Store(symbol, pair)

	return pair, nil
}

func (catalog *PairCatalog) cachedPair(symbol string) (*market.Pair, bool) {
	cached, ok := catalog.pairCache.Load(symbol)

	if !ok {
		return nil, false
	}

	pair, ok := cached.(*market.Pair)

	return pair, ok && pair != nil
}

func (catalog *PairCatalog) assetPairs() (market.AssetPairs, error) {
	cached, ok := catalog.pairCache.Load(assetPairsCacheKey)

	if ok {
		pairs, pairsOK := cached.(market.AssetPairs)

		if pairsOK && len(pairs) > 0 {
			return pairs, nil
		}
	}

	result, err, _ := catalog.assetPairsFlight.Do(assetPairsCacheKey, func() (any, error) {
		if cached, ok := catalog.pairCache.Load(assetPairsCacheKey); ok {
			pairs, pairsOK := cached.(market.AssetPairs)

			if pairsOK && len(pairs) > 0 {
				return pairs, nil
			}
		}

		endpoint := catalog.assetPairsAPI

		if endpoint == "" {
			endpoint = public.EndpointTypeAssetPairs
		}

		rest := public.NewRest(catalog.ctx, endpoint)
		pairs := market.AssetPairs{}

		if fetchErr := rest.Get(catalog.ctx, fiber.Map{}, &pairs); fetchErr != nil {
			return nil, fmt.Errorf("paper pair catalog: fetch asset pairs: %w", fetchErr)
		}

		if len(pairs) == 0 {
			return nil, fmt.Errorf("paper pair catalog: asset pairs response is empty")
		}

		catalog.pairCache.Store(assetPairsCacheKey, pairs)

		return pairs, nil
	})

	if err != nil {
		return nil, err
	}

	pairs, pairsOK := result.(market.AssetPairs)

	if !pairsOK || len(pairs) == 0 {
		return nil, fmt.Errorf("paper pair catalog: asset pairs response is invalid")
	}

	return pairs, nil
}

/*
DepthBook returns one REST depth snapshot, cached for trading.max_quote_age.
*/
func (catalog *PairCatalog) DepthBook(restPair string, count int) (depthBook, error) {
	if catalog == nil {
		return depthBook{}, fmt.Errorf("paper pair catalog: nil catalog")
	}

	if restPair == "" {
		return depthBook{}, fmt.Errorf("paper pair catalog: rest pair missing")
	}

	if count <= 0 {
		return depthBook{}, fmt.Errorf("paper pair catalog: depth count must be positive")
	}

	cacheKey := fmt.Sprintf("%s:%d", restPair, count)
	cacheTTL, ttlErr := catalog.depthCacheTTL()

	if ttlErr != nil {
		return depthBook{}, ttlErr
	}

	if cacheTTL > 0 {
		if cached, found := catalog.cachedDepth(cacheKey); found {
			return cached, nil
		}
	}

	result, err, _ := catalog.depthFlight.Do(cacheKey, func() (any, error) {
		if cacheTTL > 0 {
			if cached, found := catalog.cachedDepth(cacheKey); found {
				return cached, nil
			}
		}

		book, bookErr := catalog.fetchDepthBook(restPair, count)

		if bookErr != nil {
			return depthBook{}, bookErr
		}

		if cacheTTL > 0 {
			catalog.depthCache.Store(cacheKey, cachedDepthBook{
				book:      book,
				expiresAt: time.Now().Add(cacheTTL),
			})
		}

		return book, nil
	})

	if err != nil {
		return depthBook{}, err
	}

	book, bookOK := result.(depthBook)

	if !bookOK {
		return depthBook{}, fmt.Errorf("paper pair catalog: depth response is invalid")
	}

	return book, nil
}

func (catalog *PairCatalog) depthCacheTTL() (time.Duration, error) {
	if cached := catalog.depthTTL.Load(); cached > 0 {
		return time.Duration(cached), nil
	}

	tradingConfig, err := config.LoadTradingConfig()

	if err != nil {
		return 0, fmt.Errorf("paper pair catalog: depth cache ttl: %w", err)
	}

	catalog.depthTTL.Store(int64(tradingConfig.MaxQuoteAge))

	return tradingConfig.MaxQuoteAge, nil
}

func (catalog *PairCatalog) cachedDepth(cacheKey string) (depthBook, bool) {
	cached, ok := catalog.depthCache.Load(cacheKey)

	if !ok {
		return depthBook{}, false
	}

	entry, ok := cached.(cachedDepthBook)

	if !ok || time.Now().After(entry.expiresAt) {
		catalog.depthCache.Delete(cacheKey)
		return depthBook{}, false
	}

	return entry.book, true
}

func (catalog *PairCatalog) fetchDepthBook(restPair string, count int) (depthBook, error) {
	endpoint := catalog.depthAPI

	if endpoint == "" {
		endpoint = public.EndpointTypeDepth
	}

	rest := public.NewRest(catalog.ctx, endpoint)
	books := map[string]depthBook{}

	if err := rest.Get(catalog.ctx, fiber.Map{
		"pair":  restPair,
		"count": count,
	}, &books); err != nil {
		return depthBook{}, fmt.Errorf("paper pair catalog: fetch depth for %s: %w", restPair, err)
	}

	for _, book := range books {
		return book, nil
	}

	return depthBook{}, fmt.Errorf("paper pair catalog: empty depth for %s", restPair)
}

func (catalog *PairCatalog) feeVolume(pair *market.Pair) float64 {
	if pair == nil {
		return 0
	}

	currency := pair.FeeVolumeCurrency

	if currency == "" {
		currency = pair.Quote
	}

	rawVolume, ok := catalog.volumes.Load(currency)

	if !ok {
		return 0
	}

	pointer, ok := rawVolume.(*atomic.Uint64)

	if !ok || pointer == nil {
		return 0
	}

	return float64(pointer.Load()) / 1_000_000
}

/*
RecordFill advances the fee tier volume ledger for one executed notional.
*/
func (catalog *PairCatalog) RecordFill(symbol string, notional float64) error {
	if notional <= 0 {
		return fmt.Errorf("paper pair catalog: fill notional must be positive")
	}

	pair, err := catalog.fetchPair(symbol)

	if err != nil {
		return err
	}

	currency := pair.FeeVolumeCurrency

	if currency == "" {
		currency = pair.Quote
	}

	rawVolume, _ := catalog.volumes.LoadOrStore(currency, &atomic.Uint64{})
	pointer := rawVolume.(*atomic.Uint64)
	micros := uint64(notional * 1_000_000)
	pointer.Add(micros)

	return nil
}

/*
FeeRate returns the decimal fee rate for one order on one symbol.
*/
func (catalog *PairCatalog) FeeRate(
	symbol string,
	orderType trading.OrderType,
) (float64, error) {
	pair, err := catalog.fetchPair(symbol)

	if err != nil {
		return 0, err
	}

	volume := catalog.feeVolume(pair)

	if orderType == trading.Limit {
		return pair.MakerFeeRate(volume)
	}

	return pair.TakerFeeRate(volume)
}

/*
TickSize returns the published tick size for one ws symbol.
*/
func (catalog *PairCatalog) TickSize(symbol string) (float64, error) {
	pair, err := catalog.fetchPair(symbol)

	if err != nil {
		return 0, err
	}

	return pair.TickSizeFloat()
}
