package response

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

const (
	defaultTakerFeePct = 0.40
	DefaultMakerFeePct = 0.25
	defaultTickSize    = 0.01
)

/*
pairMeta caches fee rates and tick size for one trading pair.
*/
type pairMeta struct {
	takerPct float64
	makerPct float64
	tickSize float64
	quote    string
}

/*
PairCatalog loads AssetPairs metadata for fill simulation.
*/
type PairCatalog struct {
	ctx   context.Context
	mu    sync.RWMutex
	pairs map[string]*pairMeta
}

func NewPairCatalog(ctx context.Context) *PairCatalog {
	return &PairCatalog{
		ctx:   ctx,
		pairs: make(map[string]*pairMeta),
	}
}

func (catalog *PairCatalog) Load() {
	rest := public.NewRest(catalog.ctx, public.EndpointTypeAssetPairs)

	var pairs map[string]*struct {
		Wsname    string      `json:"wsname"`
		Fees      [][]float64 `json:"fees"`
		FeesMaker [][]float64 `json:"fees_maker"`
		TickSize  string      `json:"tick_size"`
	}

	if err := rest.Get(catalog.ctx, fiber.Map{}, &pairs); err != nil {
		errnie.Error(err)

		return
	}

	catalog.mu.Lock()
	defer catalog.mu.Unlock()

	for _, pair := range pairs {
		if pair == nil || pair.Wsname == "" {
			continue
		}

		meta := &pairMeta{
			takerPct: defaultTakerFeePct,
			makerPct: DefaultMakerFeePct,
			tickSize: defaultTickSize,
			quote:    catalog.quoteAsset(pair.Wsname),
		}

		if len(pair.Fees) > 0 && len(pair.Fees[0]) >= 2 {
			meta.takerPct = pair.Fees[0][1]
		}

		if len(pair.FeesMaker) > 0 && len(pair.FeesMaker[0]) >= 2 {
			meta.makerPct = pair.FeesMaker[0][1]
		}

		if pair.TickSize != "" {
			if tickSize, err := strconv.ParseFloat(pair.TickSize, 64); err == nil && tickSize > 0 {
				meta.tickSize = tickSize
			}
		}

		catalog.pairs[pair.Wsname] = meta
	}
}

func (catalog *PairCatalog) Meta(symbol string) pairMeta {
	catalog.mu.RLock()
	meta := catalog.pairs[symbol]
	catalog.mu.RUnlock()

	if meta != nil {
		return *meta
	}

	return pairMeta{
		takerPct: defaultTakerFeePct,
		makerPct: DefaultMakerFeePct,
		tickSize: defaultTickSize,
		quote:    catalog.quoteAsset(symbol),
	}
}

func (catalog *PairCatalog) quoteAsset(symbol string) string {
	if index := strings.IndexByte(symbol, '/'); index >= 0 {
		return symbol[index+1:]
	}

	return "USD"
}

func (catalog *PairCatalog) baseAsset(symbol string) string {
	if index := strings.IndexByte(symbol, '/'); index >= 0 {
		return symbol[:index]
	}

	return symbol
}
