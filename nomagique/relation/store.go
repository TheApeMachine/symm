package relation

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

/*
RetentionPolicy names the store's eviction contract. It is infrastructure
provenance: capacity is not a statistical horizon, and no value, SNR, or
Influence threshold ever decides eviction.
*/
type RetentionPolicy struct {
	// Capacity is the maximum number of observations retained per
	// coordinate. It is an infrastructure bound, not a statistical claim.
	Capacity int
}

/*
StoreSnapshot is an instantaneous observation count used by the conformance
suite to prove that simulation never becomes observation.
*/
type StoreSnapshot struct {
	// Coordinates is the number of distinct coordinates with retained data.
	Coordinates int
	// Observations is the total number of retained observations.
	Observations int
	// Appended is the cumulative number of observations ever appended.
	Appended uint64
}

/*
ObservationStore is a typed observational coordinate store with coordinate-local
locking. Each coordinate keeps its own bounded chronological ring; eviction
follows the retention policy only. Observed zero is stored and remains distinct
from missing.
*/
type ObservationStore struct {
	policy   RetentionPolicy
	rings    sync.Map
	appended atomic.Uint64
}

type observationRing struct {
	mu         sync.RWMutex
	coordinate Coordinate
	entries    []Observation
	head       int
	size       int
}

/*
NewObservationStore builds a store with the given per-coordinate capacity.
Capacity is infrastructure provenance.
*/
func NewObservationStore(capacity int) *ObservationStore {
	if capacity < 1 {
		capacity = 1
	}

	return &ObservationStore{
		policy: RetentionPolicy{Capacity: capacity},
	}
}

/*
Retention returns the explicit retention policy.
*/
func (store *ObservationStore) Retention() RetentionPolicy {
	if store == nil {
		return RetentionPolicy{}
	}

	return store.policy
}

func (store *ObservationStore) getOrCreateRing(coordinate Coordinate) *observationRing {
	if stored, found := store.rings.Load(coordinate); found {
		return stored.(*observationRing)
	}

	candidate := &observationRing{
		coordinate: coordinate,
		entries:    make([]Observation, store.policy.Capacity),
	}
	actual, _ := store.rings.LoadOrStore(coordinate, candidate)

	return actual.(*observationRing)
}

func (store *ObservationStore) getRing(coordinate Coordinate) *observationRing {
	stored, found := store.rings.Load(coordinate)

	if !found {
		return nil
	}

	return stored.(*observationRing)
}

/*
Append stores one observation for its coordinate, evicting the oldest
retained observation when the ring is full. It never blocks and never drops
because of a value threshold.
*/
func (store *ObservationStore) Append(observation Observation) {
	if store == nil {
		return
	}

	ring := store.getOrCreateRing(observation.Coordinate)
	ring.mu.Lock()
	ring.push(observation)
	ring.mu.Unlock()

	store.appended.Add(1)
}

/*
AppendObservations stores a batch of observations.
*/
func (store *ObservationStore) AppendObservations(observations []Observation) {
	if store == nil {
		return
	}

	for _, observation := range observations {
		store.Append(observation)
	}
}

/*
History returns the retained observations for one coordinate in
chronological order. A coordinate that has never been observed returns an
empty slice; that is missing, not zero.
*/
func (store *ObservationStore) History(coordinate Coordinate) []Observation {
	if store == nil {
		return nil
	}

	ring := store.getRing(coordinate)

	if ring == nil {
		return nil
	}

	ring.mu.RLock()

	if ring.size == 0 {
		ring.mu.RUnlock()
		return nil
	}

	history := make([]Observation, ring.size)

	for index := 0; index < ring.size; index++ {
		history[index] = ring.at(index)
	}

	ring.mu.RUnlock()

	sort.Slice(history, func(left int, right int) bool {
		return history[left].At.Before(history[right].At)
	})

	return history
}

/*
Latest returns the most recent retained observation for one coordinate,
reading the newest ring entry directly under the coordinate's read lock.
*/
func (store *ObservationStore) Latest(coordinate Coordinate) (Observation, bool) {
	if store == nil {
		return Observation{}, false
	}

	ring := store.getRing(coordinate)

	if ring == nil {
		return Observation{}, false
	}

	ring.mu.RLock()
	defer ring.mu.RUnlock()

	if ring.size == 0 {
		return Observation{}, false
	}

	return ring.at(ring.size - 1), true
}

/*
Count returns the number of retained observations for one coordinate.
*/
func (store *ObservationStore) Count(coordinate Coordinate) int {
	if store == nil {
		return 0
	}

	ring := store.getRing(coordinate)

	if ring == nil {
		return 0
	}

	ring.mu.RLock()
	defer ring.mu.RUnlock()

	return ring.size
}

/*
Coordinates returns every coordinate with retained data.
*/
func (store *ObservationStore) Coordinates() []Coordinate {
	if store == nil {
		return nil
	}

	var coordinates []Coordinate

	store.rings.Range(func(key, value any) bool {
		coord, validCoord := key.(Coordinate)
		ring, validRing := value.(*observationRing)

		if !validCoord || !validRing {
			return true
		}

		ring.mu.RLock()
		hasData := ring.size > 0
		ring.mu.RUnlock()

		if hasData {
			coordinates = append(coordinates, coord)
		}

		return true
	})

	sort.Slice(coordinates, func(left int, right int) bool {
		return coordinates[left].ID() < coordinates[right].ID()
	})

	return coordinates
}

/*
Snapshot returns the current observation counts. The conformance suite uses
it to verify that MCTS rollouts never become observational evidence.
*/
func (store *ObservationStore) Snapshot() StoreSnapshot {
	if store == nil {
		return StoreSnapshot{}
	}

	snapshot := StoreSnapshot{
		Appended: store.appended.Load(),
	}

	store.rings.Range(func(key, value any) bool {
		ring, valid := value.(*observationRing)

		if valid && ring != nil {
			ring.mu.RLock()
			if ring.size > 0 {
				snapshot.Coordinates++
				snapshot.Observations += ring.size
			}
			ring.mu.RUnlock()
		}

		return true
	})

	return snapshot
}

/*
TimeRange returns the earliest and latest observation time across all
retained data, when any exists.
*/
func (store *ObservationStore) TimeRange() (time.Time, time.Time, bool) {
	if store == nil {
		return time.Time{}, time.Time{}, false
	}

	var earliest, latest time.Time
	found := false

	store.rings.Range(func(key, value any) bool {
		ring, valid := value.(*observationRing)

		if !valid || ring == nil {
			return true
		}

		ring.mu.RLock()

		for index := 0; index < ring.size; index++ {
			observation := ring.at(index)

			if !found || observation.At.Before(earliest) {
				earliest = observation.At
			}

			if !found || observation.At.After(latest) {
				latest = observation.At
			}

			found = true
		}

		ring.mu.RUnlock()
		return true
	})

	return earliest, latest, found
}

func (ring *observationRing) push(observation Observation) {
	ring.entries[ring.head] = observation
	ring.head = (ring.head + 1) % len(ring.entries)

	if ring.size < len(ring.entries) {
		ring.size++
	}
}

// at returns the observation at logical index 0..size-1 in insertion order.
func (ring *observationRing) at(index int) Observation {
	logicalHead := (ring.head - ring.size + len(ring.entries)) % len(ring.entries)
	return ring.entries[(logicalHead+index)%len(ring.entries)]
}
