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
	sharedStress.Start(pool)

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
		ctx:    ctx,
		cancel: cancel,
	}
}

/*
Start begins ingesting measurement frames into the cache.
*/
func (cache *StressCache) Start(pool *qpool.Q) {
	if cache == nil || !cache.started.CompareAndSwap(false, true) {
		return
	}

	go cache.run(pool)
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

			measurement, typeOK := message.Value.(types.Measurement)

			if !typeOK {
				continue
			}

			cache.ingestMeasurement(measurement)
		}
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
