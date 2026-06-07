package broker

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
SymbolStress holds the latest stress-regime readings used by execution gates.
*/
type SymbolStress struct {
	ToxicityCategory  types.CategoryType
	ToxicitySNR       float64
	FluidCategory     types.CategoryType
	FluidSNR          float64
	SentimentCategory types.CategoryType
	SentimentSNR      float64
	HawkesCategory    types.CategoryType
	HawkesSNR         float64
}

/*
StressCache ingests perspective measurements and exposes per-symbol stress snapshots.
*/
type StressCache struct {
	ctx     context.Context
	cancel  context.CancelFunc
	slots   sync.Map
	started atomic.Bool
}

/*
ResetStressCacheForTest is a no-op; caches are constructed per runtime.Runtime.
*/
func ResetStressCacheForTest() {}

/*
EnsureStressCache constructs a fresh stress cache. Prefer runtime.Runtime at the
process root so live components share one instance via dependency injection.
*/
func EnsureStressCache(ctx context.Context, pool *qpool.Q[any]) *StressCache {
	cache := NewStressCache(ctx, pool)
	cache.Start(pool)

	return cache
}

func NewStressCache(ctx context.Context, _ *qpool.Q[any]) *StressCache {
	ctx, cancel := context.WithCancel(ctx)

	return &StressCache{
		ctx:    ctx,
		cancel: cancel,
	}
}

/*
Start begins ingesting measurement frames into the cache.
*/
func (cache *StressCache) Start(pool *qpool.Q[any]) {
	if cache == nil || !cache.started.CompareAndSwap(false, true) {
		return
	}

	go cache.run(pool)
}

func (cache *StressCache) run(pool *qpool.Q[any]) {
	if pool == nil {
		return
	}

	consumer := pool.Subscribe("measurements")

	if consumer == nil {
		return
	}

	for {
		message, err := consumer.Wait(cache.ctx)

		if err != nil {
			return
		}

		if message == nil || message.Value == nil {
			continue
		}

		measurement, typeOK := message.Value.(types.Measurement)

		if !typeOK {
			continue
		}

		cache.ingestMeasurement(measurement)
	}
}

func (cache *StressCache) ingestMeasurement(measurement types.Measurement) {
	if measurement.Symbol == "" {
		return
	}

	slot := cache.slotFor(measurement.Symbol)
	slot.mu.Lock()
	stress, _ := slot.value()

	switch measurement.Source {
	case types.SourceToxicity:
		stress.ToxicityCategory = measurement.Category
		stress.ToxicitySNR = measurement.SNR
	case types.SourceFluid:
		stress.FluidCategory = measurement.Category
		stress.FluidSNR = measurement.SNR
	case types.SourceSentiment:
		stress.SentimentCategory = measurement.Category
		stress.SentimentSNR = measurement.SNR
	case types.SourceHawkes:
		stress.HawkesCategory = measurement.Category
		stress.HawkesSNR = measurement.SNR
	default:
		slot.mu.Unlock()
		return
	}

	slot.store(stress)
	slot.mu.Unlock()
}

/*
Snapshot returns the latest stress readings for one symbol.
*/
func (cache *StressCache) Snapshot(symbol string) SymbolStress {
	if cache == nil {
		return SymbolStress{}
	}

	slot, ok := cache.slots.Load(symbol)

	if !ok {
		return SymbolStress{}
	}

	stress, _ := slot.(*stressSlot).value()

	return stress
}

/*
InstallStressForTest seeds one symbol stress snapshot for unit tests.
*/
func (cache *StressCache) InstallStressForTest(symbol string, stress SymbolStress) {
	if cache == nil || symbol == "" {
		return
	}

	slot := cache.slotFor(symbol)
	slot.mu.Lock()
	slot.store(stress)
	slot.mu.Unlock()
}
