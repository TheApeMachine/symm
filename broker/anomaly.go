package broker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"golang.design/x/lockfree/wf"
)

type AnomalyKind uint8

const (
	AnomalyCrossedBook AnomalyKind = iota
	AnomalyInsufficientDepth
	AnomalyIncompleteBook
)

func (kind AnomalyKind) String() string {
	switch kind {
	case AnomalyCrossedBook:
		return "crossed_book"
	case AnomalyInsufficientDepth:
		return "insufficient_depth"
	case AnomalyIncompleteBook:
		return "incomplete_book"
	default:
		return "unknown"
	}
}

type MarketAnomaly struct {
	Symbol string
	Kind   AnomalyKind
	At     time.Time
}

type symbolStats struct {
	count atomic.Uint64
}

type symbolFaultState struct {
	consecutiveCrossed atomic.Uint64
	consecutiveClean   atomic.Uint64
	hasSevereFault     atomic.Bool
}

/*
AnomalyMonitor is a lock-free off-ramp for unprocessable book shapes. It decouples
recording on the critical pricing path from metric aggregation, preserving
single-digit nanosecond execution while exposing quantitative venue health.
*/
type AnomalyMonitor struct {
	ctx       context.Context
	cancel    context.CancelFunc
	ring      *wf.RingBuffer[MarketAnomaly]
	stats     sync.Map
	faults    sync.Map
	onFault   atomic.Pointer[[]func(symbol string)]
	onRecover atomic.Pointer[[]func(symbol string)]
	total     atomic.Uint64
	running   atomic.Bool
}

func NewAnomalyMonitor(ctx context.Context, capacity int) *AnomalyMonitor {
	if capacity <= 0 {
		configured := viper.GetInt("market.anomaly.capacity")
		if configured > 0 {
			capacity = configured
		}

		if capacity <= 0 {
			capacity = 4096
		}
	}

	monitorCtx, cancel := context.WithCancel(ctx)
	monitor := &AnomalyMonitor{
		ctx:    monitorCtx,
		cancel: cancel,
		ring:   wf.NewRingBuffer[MarketAnomaly](capacity),
	}

	monitor.running.Store(true)
	go monitor.drain()

	return monitor
}

func (monitor *AnomalyMonitor) faultState(symbol string) *symbolFaultState {
	actual, _ := monitor.faults.LoadOrStore(symbol, &symbolFaultState{})
	return actual.(*symbolFaultState)
}

/* SetOnFault registers a callback invoked when a symbol experiences a severe structural fault. */
func (monitor *AnomalyMonitor) SetOnFault(hook func(symbol string)) {
	if monitor == nil || hook == nil {
		return
	}

	for {
		current := monitor.onFault.Load()
		capacity := 1

		if current != nil {
			capacity = len(*current) + 1
		}

		next := make([]func(symbol string), capacity)

		if current != nil {
			copy(next, *current)
		}

		next[capacity-1] = hook

		if monitor.onFault.CompareAndSwap(current, &next) {
			return
		}
	}
}

/* SetOnRecover registers a callback invoked when a symbol uncrosses and stabilizes. */
func (monitor *AnomalyMonitor) SetOnRecover(hook func(symbol string)) {
	if monitor == nil || hook == nil {
		return
	}

	for {
		current := monitor.onRecover.Load()
		capacity := 1

		if current != nil {
			capacity = len(*current) + 1
		}

		next := make([]func(symbol string), capacity)

		if current != nil {
			copy(next, *current)
		}

		next[capacity-1] = hook

		if monitor.onRecover.CompareAndSwap(current, &next) {
			return
		}
	}
}

/* Record pushes a market shape anomaly onto the wait-free ring buffer without blocking. */
func (monitor *AnomalyMonitor) Record(symbol string, kind AnomalyKind) {
	if monitor == nil || !monitor.running.Load() {
		return
	}

	monitor.ring.Put(MarketAnomaly{
		Symbol: symbol,
		Kind:   kind,
		At:     time.Now().UTC(),
	})

	if kind == AnomalyCrossedBook {
		state := monitor.faultState(symbol)
		state.consecutiveClean.Store(0)
		crossed := state.consecutiveCrossed.Add(1)

		if crossed > 3 && !state.hasSevereFault.Swap(true) {
			hooks := monitor.onFault.Load()

			if hooks != nil {
				for _, hook := range *hooks {
					hook(symbol)
				}
			}
		}
	}
}

/* RecordClean registers a clean, uncrossed book tick to advance the stabilization counter. */
func (monitor *AnomalyMonitor) RecordClean(symbol string) {
	if monitor == nil || !monitor.running.Load() {
		return
	}

	state := monitor.faultState(symbol)
	state.consecutiveCrossed.Store(0)
	clean := state.consecutiveClean.Add(1)

	if clean >= 3 && state.hasSevereFault.Load() && state.hasSevereFault.Swap(false) {
		hooks := monitor.onRecover.Load()

		if hooks != nil {
			for _, hook := range *hooks {
				hook(symbol)
			}
		}
	}
}

/* HasSevereFault reports whether an impossible market state has persisted beyond transient noise. */
func (monitor *AnomalyMonitor) HasSevereFault(symbol string) bool {
	if monitor == nil {
		return false
	}

	actual, found := monitor.faults.Load(symbol)

	if !found {
		return false
	}

	return actual.(*symbolFaultState).hasSevereFault.Load()
}

/* HasAnySevereFault reports whether any monitored symbol currently has an active severe structural fault. */
func (monitor *AnomalyMonitor) HasAnySevereFault() bool {
	if monitor == nil {
		return false
	}

	hasFault := false

	monitor.faults.Range(func(key, value any) bool {
		state, ok := value.(*symbolFaultState)

		if ok && state.hasSevereFault.Load() {
			hasFault = true
			return false
		}

		return true
	})

	return hasFault
}

/* Count returns the total number of anomalies recorded for a symbol. */
func (monitor *AnomalyMonitor) Count(symbol string) uint64 {
	if monitor == nil {
		return 0
	}

	value, found := monitor.stats.Load(symbol)
	if !found {
		return 0
	}

	return value.(*symbolStats).count.Load()
}

/* Total returns the aggregate number of anomalies recorded across all symbols. */
func (monitor *AnomalyMonitor) Total() uint64 {
	if monitor == nil {
		return 0
	}

	return monitor.total.Load()
}

/*
Health derives an operational quality score in [0.0, 1.0].
Severe structural faults immediately bypass gradual decay to 0.0.
*/
func (monitor *AnomalyMonitor) Health(symbol string) float64 {
	if monitor == nil {
		return 1.0
	}

	if monitor.HasSevereFault(symbol) {
		return 0.0
	}

	count := monitor.Count(symbol)
	if count == 0 {
		return 1.0
	}

	return 1.0 / (1.0 + float64(count))
}

/* Close gracefully shuts down the background drain worker. */
func (monitor *AnomalyMonitor) Close() error {
	if monitor == nil {
		return nil
	}

	if !monitor.running.Swap(false) {
		return nil
	}

	monitor.cancel()
	return nil
}

func (monitor *AnomalyMonitor) drain() {
	for monitor.running.Load() {
		anomaly, ok := monitor.ring.Get()
		if !ok {
			select {
			case <-monitor.ctx.Done():
				return
			case <-time.After(5 * time.Millisecond):
				continue
			}
		}

		monitor.total.Add(1)

		actual, _ := monitor.stats.LoadOrStore(anomaly.Symbol, &symbolStats{})
		stats := actual.(*symbolStats)
		stats.count.Add(1)
	}
}
