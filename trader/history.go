package trader

import (
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

type historySlot[T any] struct {
	Wall    time.Time
	Payload T
}

type historyRing[T any] struct {
	mu    sync.Mutex
	slots []historySlot[T]
	next  int
	full  bool
}

type History[T any] struct {
	cache *sync.Map
	depth int
}

func NewHistory[T any]() *History[T] {
	return &History[T]{
		cache: &sync.Map{},
		depth: viper.GetViper().GetInt("signals.feed_ring_capacity"),
	}
}

func (history *History[T]) Measure(symbol string, wall time.Time, payload T) error {
	if history.depth <= 0 {
		return errnie.Err(
			errnie.Validation,
			"trader: signals.feed_ring_capacity must be positive",
			nil,
		)
	}

	if strings.TrimSpace(symbol) == "" {
		return errnie.Err(errnie.Validation, "trader: symbol required", nil)
	}

	if wall.IsZero() {
		return errnie.Err(errnie.Validation, "trader: timestamp required", nil)
	}

	ring, _ := history.cache.LoadOrStore(
		symbol,
		newHistoryRing[T](history.depth),
	)

	clock := ring.(*historyRing[T])
	clock.Push(historySlot[T]{
		Wall:    wall,
		Payload: payload,
	})

	return nil
}

func newHistoryRing[T any](depth int) *historyRing[T] {
	return &historyRing[T]{
		slots: make([]historySlot[T], depth),
	}
}

func (ring *historyRing[T]) Push(slot historySlot[T]) {
	ring.mu.Lock()
	defer ring.mu.Unlock()

	ring.slots[ring.next] = slot
	ring.next = (ring.next + 1) % len(ring.slots)
	ring.full = ring.full || ring.next == 0
}

func (ring *historyRing[T]) Len() int {
	ring.mu.Lock()
	defer ring.mu.Unlock()

	if ring.full {
		return len(ring.slots)
	}

	return ring.next
}
