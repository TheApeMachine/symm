package resonance

import (
	"sync"
	"time"

	"github.com/spf13/viper"
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

func (store *feedStore) reset() {
	store.rings = sync.Map{}
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
	if ring == nil || ring.capacity <= 0 || len(element) == 0 {
		return
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

	if ring.firstObserved.IsZero() {
		ring.firstObserved = observed
	}

	if !observed.IsZero() {
		ring.latestObserved = observed
	}
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
