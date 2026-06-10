package response

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
)

/*
PairCatalog resolves Kraken AssetPairs metadata through REST when paper fills need it.

The only session state kept locally is the simulated fee-tier volume ledger.
*/
type PairCatalog struct {
	ctx           context.Context
	volumes       sync.Map
	assetPairsAPI public.EndpointType
	depthAPI      public.EndpointType
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

	endpoint := catalog.assetPairsAPI

	if endpoint == "" {
		endpoint = public.EndpointTypeAssetPairs
	}

	rest := public.NewRest(catalog.ctx, endpoint)
	pairs := market.AssetPairs{}

	if err := rest.Get(catalog.ctx, fiber.Map{}, &pairs); err != nil {
		return nil, fmt.Errorf("paper pair catalog: fetch asset pairs: %w", err)
	}

	return pairs.PairByWsname(symbol)
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
