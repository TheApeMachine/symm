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
ObservationStore is resident streaming state: one bounded chronological ring
per Coordinate plus a resident ordered coordinate index. Coordinates are
structural — registered once (see RegisterCoordinate) and traversed in place
(see RangeCoordinates); measurements never discover, clone, snapshot, sort,
or recreate the coordinate universe. Rings are traversed in place too (see
RangeHistory): a ring is chronological by construction, so retained history
is never copied or re-sorted on the hot path.
*/
type ObservationStore struct {
	policy RetentionPolicy

	rings sync.Map

	// indexMu guards the resident ordered coordinate index. Structural
	// registration is the only writer; readers iterate under RLock.
	// Measurement traffic never touches it.
	indexMu sync.RWMutex
	order   []Coordinate

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

/*
RegisterCoordinate structurally registers one coordinate, creating its
resident ring and inserting it into the resident ordered coordinate index. It
is idempotent and is the only path that grows the coordinate universe;
measurements for an unregistered coordinate take this same structural path
exactly once. Registration is a slow structural mutation, never a hot-path
operation, and no speculative ring is ever allocated by a concurrent loser.
*/
func (store *ObservationStore) RegisterCoordinate(coordinate Coordinate) {
	if store == nil {
		return
	}

	store.register(coordinate)
}

func (store *ObservationStore) register(coordinate Coordinate) *observationRing {
	store.indexMu.Lock()
	defer store.indexMu.Unlock()

	if stored, found := store.rings.Load(coordinate); found {
		return stored.(*observationRing)
	}

	ring := &observationRing{
		coordinate: coordinate,
		entries:    make([]Observation, store.policy.Capacity),
	}

	store.rings.Store(coordinate, ring)
	store.insertOrdered(coordinate)

	return ring
}

// insertOrdered inserts one coordinate into the resident canonical order.
func (store *ObservationStore) insertOrdered(coordinate Coordinate) {
	position := sort.Search(len(store.order), func(index int) bool {
		return CompareCoordinate(store.order[index], coordinate) >= 0
	})

	store.order = append(store.order, Coordinate{})
	copy(store.order[position+1:], store.order[position:])
	store.order[position] = coordinate
}

/*
Append stores one observation for its coordinate, evicting the oldest
retained observation when the ring is full. It never blocks and never drops
because of a value threshold. A coordinate observed for the first time is
structurally registered exactly once; concurrent first observations never
allocate competing candidate rings.
*/
func (store *ObservationStore) Append(observation Observation) {
	if store == nil {
		return
	}

	stored, found := store.rings.Load(observation.Coordinate)

	if !found {
		stored = store.register(observation.Coordinate)
	}

	ring := stored.(*observationRing)
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
RangeCoordinates visits every registered coordinate in canonical
CompareCoordinate order, in place, with zero allocation. The resident index
is maintained at registration time, so no universe is ever enumerated,
cloned, or sorted on the read path.
*/
func (store *ObservationStore) RangeCoordinates(visit func(Coordinate) bool) {
	if store == nil {
		return
	}

	store.indexMu.RLock()
	defer store.indexMu.RUnlock()

	for _, coordinate := range store.order {
		if !visit(coordinate) {
			return
		}
	}
}

/*
RangeCoordinatesForSymbol visits every registered coordinate for one symbol, in
canonical CompareCoordinate order. Symbol is the primary field in that order, so
one symbol's coordinates occupy one contiguous run of the resident index;
binary search finds its bounds instead of walking every coordinate in the
store. Candidate compilation calls this once per symbol per plan-pair, so an
O(total coordinates) scan here becomes the dominant cost once the coordinate
universe grows large — this keeps it O(this symbol's coordinates).
*/
func (store *ObservationStore) RangeCoordinatesForSymbol(symbol string, visit func(Coordinate) bool) {
	if store == nil {
		return
	}

	store.indexMu.RLock()
	defer store.indexMu.RUnlock()

	start := sort.Search(len(store.order), func(index int) bool {
		return store.order[index].Symbol >= symbol
	})

	for index := start; index < len(store.order) && store.order[index].Symbol == symbol; index++ {
		if !visit(store.order[index]) {
			return
		}
	}
}

/*
CoordinateCount returns the number of resident coordinates.
*/
func (store *ObservationStore) CoordinateCount() int {
	if store == nil {
		return 0
	}

	store.indexMu.RLock()
	defer store.indexMu.RUnlock()

	return len(store.order)
}

/*
RingView is a read-locked window over one resident observation ring. It is
the estimation path's zero-copy access to resident history: Len and At read
the ring in place in chronological order, and Close releases the read lock.
A view must be closed exactly once and must not be used when ViewRing
reports not found.
*/
type RingView struct {
	ring   *observationRing
	unlock func()
}

/*
Len returns the number of retained observations in the ring.
*/
func (view RingView) Len() int {
	return view.ring.size
}

/*
At returns the observation at logical index 0..Len-1 in insertion order.
*/
func (view RingView) At(index int) Observation {
	return view.ring.at(index)
}

/*
TimeAt returns only the timestamp at logical index 0..Len-1 in insertion
order, without copying the surrounding Observation. Alignment scans read the
timestamp to find the newest entry at or before a cutoff; they never need the
184-byte Observation struct for that comparison, so a per-step full copy is
pure waste on the estimation hot path.
*/
func (view RingView) TimeAt(index int) time.Time {
	return view.ring.at(index).At
}

/*
Close releases the ring read lock. It must be called exactly once.
*/
func (view RingView) Close() {
	view.unlock()
}

/*
ViewRing returns a read-locked view of one coordinate's resident ring. The
boolean reports whether the coordinate is registered. A missing coordinate
is missing, not zero.
*/
func (store *ObservationStore) ViewRing(coordinate Coordinate) (RingView, bool) {
	if store == nil {
		return RingView{}, false
	}

	stored, found := store.rings.Load(coordinate)

	if !found {
		return RingView{}, false
	}

	ring := stored.(*observationRing)
	ring.mu.RLock()

	return RingView{ring: ring, unlock: ring.mu.RUnlock}, true
}

/*
RangeHistory visits every retained observation of one coordinate in
chronological order, in place, with zero allocation. The ring is
chronological by construction, so nothing is copied or re-sorted. A
coordinate that has never been observed visits nothing; that is missing,
not zero.
*/
func (store *ObservationStore) RangeHistory(coordinate Coordinate, visit func(Observation) bool) {
	if store == nil {
		return
	}

	stored, found := store.rings.Load(coordinate)

	if !found {
		return
	}

	ring := stored.(*observationRing)
	ring.mu.RLock()
	defer ring.mu.RUnlock()

	for index := 0; index < ring.size; index++ {
		if !visit(ring.at(index)) {
			return
		}
	}
}

/*
Latest returns the most recent retained observation for one coordinate,
reading the newest ring entry directly under the coordinate's read lock.
*/
func (store *ObservationStore) Latest(coordinate Coordinate) (Observation, bool) {
	if store == nil {
		return Observation{}, false
	}

	stored, found := store.rings.Load(coordinate)

	if !found {
		return Observation{}, false
	}

	ring := stored.(*observationRing)
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

	stored, found := store.rings.Load(coordinate)

	if !found {
		return 0
	}

	ring := stored.(*observationRing)
	ring.mu.RLock()
	defer ring.mu.RUnlock()

	return ring.size
}

/*
Version returns the monotonic committed-transition version of this store: the
cumulative number of observations ever appended. It is the per-component state
version Hindsight records so replay can reconstruct the exact transition order of
shared resident state, independent of external CaptureSequence.
*/
func (store *ObservationStore) Version() uint64 {
	if store == nil {
		return 0
	}

	return store.appended.Load()
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
