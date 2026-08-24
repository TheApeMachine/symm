package relation

import (
	"sort"
	"sync"
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
ObservationStore is a typed observational coordinate store. Each coordinate
keeps its own bounded chronological ring; eviction follows the retention
policy only. Observed zero is stored and remains distinct from missing.
*/
type ObservationStore struct {
	mu       sync.RWMutex
	policy   RetentionPolicy
	rings    map[Coordinate]*observationRing
	appended uint64
}

type observationRing struct {
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
		rings:  make(map[Coordinate]*observationRing),
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

/*
Append stores one observation for its coordinate, evicting the oldest
retained observation when the ring is full. It never blocks and never drops
because of a value threshold.
*/
func (store *ObservationStore) Append(observation Observation) {
	if store == nil {
		return
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	ring := store.rings[observation.Coordinate]

	if ring == nil {
		ring = &observationRing{
			coordinate: observation.Coordinate,
			entries:    make([]Observation, store.policy.Capacity),
		}
		store.rings[observation.Coordinate] = ring
	}

	ring.push(observation)
	store.appended++
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

	store.mu.RLock()
	defer store.mu.RUnlock()

	ring := store.rings[coordinate]

	if ring == nil || ring.size == 0 {
		return nil
	}

	history := make([]Observation, ring.size)

	for index := 0; index < ring.size; index++ {
		history[index] = ring.at(index)
	}

	sort.Slice(history, func(left int, right int) bool {
		return history[left].At.Before(history[right].At)
	})

	return history
}

/*
Latest returns the most recent retained observation for one coordinate,
reading the newest ring entry directly under the store's read lock.
*/
func (store *ObservationStore) Latest(coordinate Coordinate) (Observation, bool) {
	if store == nil {
		return Observation{}, false
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	ring := store.rings[coordinate]

	if ring == nil || ring.size == 0 {
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

	store.mu.RLock()
	defer store.mu.RUnlock()

	ring := store.rings[coordinate]

	if ring == nil {
		return 0
	}

	return ring.size
}

/*
Coordinates returns every coordinate with retained data.
*/
func (store *ObservationStore) Coordinates() []Coordinate {
	if store == nil {
		return nil
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	coordinates := make([]Coordinate, 0, len(store.rings))

	for coordinate, ring := range store.rings {
		if ring.size > 0 {
			coordinates = append(coordinates, coordinate)
		}
	}

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

	store.mu.RLock()
	defer store.mu.RUnlock()

	snapshot := StoreSnapshot{
		Coordinates: len(store.rings),
		Appended:    store.appended,
	}

	for _, ring := range store.rings {
		snapshot.Observations += ring.size
	}

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

	store.mu.RLock()
	defer store.mu.RUnlock()

	var earliest, latest time.Time
	found := false

	for _, ring := range store.rings {
		if ring.size == 0 {
			continue
		}

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
	}

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
