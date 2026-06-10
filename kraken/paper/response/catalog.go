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
PairCatalog holds Kraken AssetPairs metadata for paper fill simulation.
Fee rates always come from the published tier tables, never from config guesses.
*/
type PairCatalog struct {
	pairs   market.AssetPairs
	volumes sync.Map
}

/*
LoadPairCatalog fetches tradable asset pairs from Kraken REST.
*/
func LoadPairCatalog(ctx context.Context) (*PairCatalog, error) {
	rest := public.NewRest(ctx, public.EndpointTypeAssetPairs)

	var pairs market.AssetPairs

	if err := rest.Get(ctx, fiber.Map{}, &pairs); err != nil {
		return nil, fmt.Errorf("paper pair catalog: %w", err)
	}

	return NewPairCatalog(pairs)
}

/*
NewPairCatalog builds a catalog from an in-memory AssetPairs snapshot.
*/
func NewPairCatalog(pairs market.AssetPairs) (*PairCatalog, error) {
	if len(pairs) == 0 {
		return nil, fmt.Errorf("paper pair catalog: empty asset pairs")
	}

	return &PairCatalog{
		pairs: pairs,
	}, nil
}

func (catalog *PairCatalog) pair(symbol string) (*market.Pair, error) {
	if catalog == nil {
		return nil, fmt.Errorf("paper pair catalog: nil catalog")
	}

	return catalog.pairs.PairByWsname(symbol)
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

	pair, err := catalog.pair(symbol)

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
	pair, err := catalog.pair(symbol)

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
	pair, err := catalog.pair(symbol)

	if err != nil {
		return 0, err
	}

	return pair.TickSizeFloat()
}
