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

/*
AnomalyMonitor is a lock-free off-ramp for unprocessable book shapes. It decouples
recording on the critical pricing path from metric aggregation, preserving
single-digit nanosecond execution while exposing quantitative venue health.
*/
type AnomalyMonitor struct {
	ctx     context.Context
	cancel  context.CancelFunc
	ring    *wf.RingBuffer[MarketAnomaly]
	stats   sync.Map
	total   atomic.Uint64
	running atomic.Bool
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
Health derives a continuous operational quality score in [0.0, 1.0].
Prisinte books produce 1.0; recurring anomalies degrade the score continuously.
*/
func (monitor *AnomalyMonitor) Health(symbol string) float64 {
	if monitor == nil {
		return 1.0
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
