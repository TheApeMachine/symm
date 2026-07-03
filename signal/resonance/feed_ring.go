package resonance

import (
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/statistic"
)

const defaultFeedRingCapacity = 64

type SymbolWindow struct {
	Prices        []float64
	Spreads       []float64
	LatestElement []byte
	Elapsed       float64
}

type symbolRing struct {
	capacity       int
	elements       [][]byte
	writeIndex     int
	count          int
	prices         []float64
	spreads        []float64
	stamps         []float64
	firstObserved  time.Time
	latestObserved time.Time
}

type feedStore struct {
	capacity int
	rings    sync.Map
}

func feedRingCapacity() int {
	capacity := viper.GetInt("signals.feed_ring_capacity")

	if capacity <= 0 {
		return defaultFeedRingCapacity
	}

	return capacity
}

func newFeedStore() *feedStore {
	return &feedStore{capacity: feedRingCapacity()}
}

func (store *feedStore) ring(symbol string) *symbolRing {
	if symbol == "" {
		return nil
	}

	raw, _ := store.rings.LoadOrStore(symbol, &symbolRing{capacity: store.capacity})

	ring, ok := raw.(*symbolRing)

	if !ok {
		return nil
	}

	return ring
}

func (ring *symbolRing) push(
	element []byte,
	price float64,
	spread float64,
	observed time.Time,
) {
	if ring == nil || len(element) == 0 {
		return
	}

	if observed.IsZero() {
		observed = time.Now()
	}

	if ring.firstObserved.IsZero() {
		ring.firstObserved = observed
	}

	if !observed.IsZero() {
		ring.latestObserved = observed
	}

	oldCapacity := ring.capacity
	ring.refreshCapacity()

	if ring.capacity <= 0 {
		return
	}

	if oldCapacity != ring.capacity {
		ring.resizeElements(oldCapacity)
	}

	if len(ring.elements) < ring.capacity {
		ring.elements = append(ring.elements, nil)
	}

	slotIndex := ring.writeIndex % ring.capacity
	ring.elements[slotIndex] = append([]byte(nil), element...)
	ring.writeIndex++
	ring.count = min(ring.count+1, ring.capacity)
	ring.prices = append(ring.prices, price)
	ring.spreads = append(ring.spreads, spread)
	ring.stamps = append(ring.stamps, float64(observed.UnixNano()))

	ring.trimToCapacity()
}

func (ring *symbolRing) refreshCapacity() {
	if ring.count < 2 {
		ring.capacity = feedRingCapacity()

		return
	}

	_, capacity, err := statistic.ResolveWindows(ring.stamps, 0, 0)

	if err != nil || capacity <= 0 {
		ring.capacity = feedRingCapacity()
		return
	}

	ring.capacity = min(max(capacity, 2), feedRingCapacity())
}

func (ring *symbolRing) trimToCapacity() {
	if ring.capacity <= 0 {
		return
	}

	if len(ring.prices) > ring.capacity {
		ring.prices = ring.prices[len(ring.prices)-ring.capacity:]
	}

	if len(ring.spreads) > ring.capacity {
		ring.spreads = ring.spreads[len(ring.spreads)-ring.capacity:]
	}

	if len(ring.stamps) > ring.capacity {
		ring.stamps = ring.stamps[len(ring.stamps)-ring.capacity:]
	}
}

func (ring *symbolRing) resizeElements(oldCapacity int) {
	if ring == nil || ring.capacity <= 0 {
		return
	}

	if oldCapacity <= 0 {
		oldCapacity = len(ring.elements)
	}
	if oldCapacity <= 0 {
		ring.elements = nil
		ring.writeIndex = 0
		ring.count = 0
		return
	}

	keep := min(ring.count, ring.capacity)
	if keep <= 0 {
		ring.elements = nil
		ring.writeIndex = 0
		ring.count = 0
		return
	}

	start := ring.writeIndex - ring.count
	if start < 0 {
		start = 0
	}
	if drop := ring.count - keep; drop > 0 {
		start += drop
	}

	elements := make([][]byte, ring.capacity)
	outIndex := 0
	for index := start; index < ring.writeIndex && outIndex < keep; index++ {
		slotIndex := index % oldCapacity
		if slotIndex < 0 || slotIndex >= len(ring.elements) {
			continue
		}
		elements[outIndex] = ring.elements[slotIndex]
		outIndex++
	}

	ring.elements = elements
	ring.count = outIndex
	ring.writeIndex = outIndex
}

func (ring *symbolRing) orderedElements() [][]byte {
	if ring == nil || ring.count == 0 {
		return nil
	}

	ordered := make([][]byte, 0, ring.count)
	start := ring.writeIndex - ring.count

	if start < 0 {
		start = 0
	}

	for index := start; index < ring.writeIndex; index++ {
		slotIndex := index % ring.capacity
		element := ring.elements[slotIndex]

		if len(element) == 0 {
			continue
		}

		ordered = append(ordered, element)
	}

	return ordered
}

func (ring *symbolRing) latestElement() []byte {
	if ring == nil || ring.count == 0 {
		return nil
	}

	slotIndex := (ring.writeIndex - 1) % ring.capacity

	return ring.elements[slotIndex]
}

func (ring *symbolRing) window() SymbolWindow {
	if ring == nil || ring.count == 0 {
		return SymbolWindow{}
	}

	elapsed := 0.0

	if !ring.firstObserved.IsZero() && !ring.latestObserved.IsZero() {
		elapsed = ring.latestObserved.Sub(ring.firstObserved).Seconds()
	}

	latestElement := ring.latestElement()

	return SymbolWindow{
		Prices:        append([]float64(nil), ring.prices...),
		Spreads:       append([]float64(nil), ring.spreads...),
		LatestElement: append([]byte(nil), latestElement...),
		Elapsed:       elapsed,
	}
}
