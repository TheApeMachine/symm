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

type symbolState struct {
	consecutiveCrossed atomic.Uint64
	consecutiveClean   atomic.Uint64
	hasSevereFault     atomic.Bool
	count              atomic.Uint64
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
	symbols   atomic.Pointer[map[string]*symbolState]
	mu        sync.Mutex
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

	initialMap := make(map[string]*symbolState)
	monitor.symbols.Store(&initialMap)

	monitor.running.Store(true)
	go monitor.drain()

	return monitor
}

func (monitor *AnomalyMonitor) lookup(symbol string) *symbolState {
	current := monitor.symbols.Load()
	if current != nil {
		if state, ok := (*current)[symbol]; ok {
			return state
		}
	}

	monitor.mu.Lock()
	defer monitor.mu.Unlock()

	current = monitor.symbols.Load()
	if current != nil {
		if state, ok := (*current)[symbol]; ok {
			return state
		}
	}

	newMap := make(map[string]*symbolState)
	if current != nil {
		for k, v := range *current {
			newMap[k] = v
		}
	}

	state := &symbolState{}
	newMap[symbol] = state
	monitor.symbols.Store(&newMap)
	return state
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

	monitor.total.Add(1)
	state := monitor.lookup(symbol)
	state.count.Add(1)

	monitor.ring.Put(MarketAnomaly{
		Symbol: symbol,
		Kind:   kind,
		At:     time.Now().UTC(),
	})

	if kind == AnomalyCrossedBook {
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

	state := monitor.lookup(symbol)
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

	current := monitor.symbols.Load()
	if current == nil {
		return false
	}

	state, ok := (*current)[symbol]
	if !ok {
		return false
	}

	return state.hasSevereFault.Load()
}

/* HasAnySevereFault reports whether any monitored symbol currently has an active severe structural fault. */
func (monitor *AnomalyMonitor) HasAnySevereFault() bool {
	if monitor == nil {
		return false
	}

	current := monitor.symbols.Load()
	if current == nil {
		return false
	}

	for _, state := range *current {
		if state.hasSevereFault.Load() {
			return true
		}
	}

	return false
}

/* Count returns the total number of anomalies recorded for a symbol. */
func (monitor *AnomalyMonitor) Count(symbol string) uint64 {
	if monitor == nil {
		return 0
	}

	current := monitor.symbols.Load()
	if current == nil {
		return 0
	}

	state, ok := (*current)[symbol]
	if !ok {
		return 0
	}

	return state.count.Load()
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
		_, ok := monitor.ring.Get()
		if !ok {
			select {
			case <-monitor.ctx.Done():
				return
			case <-time.After(5 * time.Millisecond):
				continue
			}
		}
	}
}
