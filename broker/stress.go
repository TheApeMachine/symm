package broker

import (
	"context"
	"sync"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
SymbolStress holds the latest stress-regime readings used by execution gates.
*/
type SymbolStress struct {
	ToxicityCategory   perspectives.CategoryType
	ToxicitySNR        float64
	FluidCategory      perspectives.CategoryType
	FluidSNR           float64
	SentimentCategory  perspectives.CategoryType
	SentimentSNR       float64
}

/*
DeskRegime reports how aggressively the desk may open new positions.
*/
type DeskRegime int

const (
	DeskRegimeNormal DeskRegime = iota
	DeskRegimeRestricted
)

/*
StressCache ingests perspective measurements and exposes per-symbol stress snapshots.
*/
type StressCache struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
	bySymbol map[string]SymbolStress
}

var (
	stressCacheMu sync.Mutex
	sharedStress  *StressCache
)

/*
EnsureStressCache returns the process-wide stress cache bound to a live context.
*/
func EnsureStressCache(ctx context.Context, pool *qpool.Q) *StressCache {
	stressCacheMu.Lock()
	defer stressCacheMu.Unlock()

	if sharedStress != nil && sharedStress.ctx.Err() == nil {
		return sharedStress
	}

	if sharedStress != nil {
		sharedStress.cancel()
	}

	sharedStress = NewStressCache(ctx, pool)
	go sharedStress.run(pool)

	return sharedStress
}

/*
ResetStressCacheForTest tears down the shared cache between isolated harness runs.
*/
func ResetStressCacheForTest() {
	stressCacheMu.Lock()
	defer stressCacheMu.Unlock()

	if sharedStress == nil {
		return
	}

	sharedStress.cancel()
	sharedStress = nil
}

func NewStressCache(ctx context.Context, _ *qpool.Q) *StressCache {
	ctx, cancel := context.WithCancel(ctx)

	return &StressCache{
		ctx:      ctx,
		cancel:   cancel,
		bySymbol: make(map[string]SymbolStress),
	}
}

func (cache *StressCache) run(pool *qpool.Q) {
	if pool == nil {
		return
	}

	group := pool.CreateBroadcastGroup("measurements", 0)
	subscriber := group.Subscribe("broker:stress", 4096)

	if subscriber == nil {
		return
	}

	for {
		select {
		case <-cache.ctx.Done():
			return
		case message, ok := <-subscriber.Incoming:
			if !ok {
				return
			}

			if message == nil || message.Value == nil {
				continue
			}

			measurement, typeOK := message.Value.(perspectives.Measurement)

			if !typeOK {
				continue
			}

			cache.ingestMeasurement(measurement)
		}
	}
}

func (cache *StressCache) ingestMeasurement(measurement perspectives.Measurement) {
	if measurement.Symbol == "" {
		return
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()

	stress := cache.bySymbol[measurement.Symbol]

	switch measurement.Source {
	case perspectives.SourceToxicity:
		stress.ToxicityCategory = measurement.Category
		stress.ToxicitySNR = measurement.SNR
	case perspectives.SourceFluid:
		stress.FluidCategory = measurement.Category
		stress.FluidSNR = measurement.SNR
	case perspectives.SourceSentiment:
		stress.SentimentCategory = measurement.Category
		stress.SentimentSNR = measurement.SNR
	default:
		return
	}

	cache.bySymbol[measurement.Symbol] = stress
}

/*
Snapshot returns the latest stress readings for one symbol.
*/
func (cache *StressCache) Snapshot(symbol string) SymbolStress {
	if cache == nil {
		return SymbolStress{}
	}

	cache.mu.RLock()
	defer cache.mu.RUnlock()

	return cache.bySymbol[symbol]
}

/*
InstallStressForTest seeds one symbol stress snapshot for unit tests.
*/
func (cache *StressCache) InstallStressForTest(symbol string, stress SymbolStress) {
	if cache == nil || symbol == "" {
		return
	}

	cache.mu.Lock()
	cache.bySymbol[symbol] = stress
	cache.mu.Unlock()
}
