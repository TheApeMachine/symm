package relation

import (
	"fmt"
	"iter"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
StoreSnapshot is an instantaneous observation count used by the conformance
suite to prove that simulation never becomes observation. It is an immutable
fact: Capacity is the infrastructure retention bound, Coordinates is the
number of distinct coordinates with retained data, Observations is the total
number of retained observations, and Appended is the cumulative number of
observations ever appended.
*/
type StoreSnapshot struct {
	Capacity     int
	Coordinates  int
	Observations int
	Appended     uint64
}

/*
MeasurementAppend asks the store to split one data.Measurement into
per-coordinate observations and append each of them under the given model
epoch.
*/
type MeasurementAppend struct {
	Measurement *data.Measurement[float64]
	Epoch       uint64
}

/*
CoordinateScope asks for the resident ordered coordinate index, optionally
restricted to one symbol. Symbol is the primary field in the canonical order,
so one symbol's coordinates occupy one contiguous run of the resident index.
*/
type CoordinateScope struct {
	Symbol string
}

/*
RingRequest asks for a read-locked view of one coordinate's resident ring.
*/
type RingRequest struct {
	Coordinate Coordinate
}

/*
HistoryRequest asks for the retained history of one coordinate, copied in
chronological order.
*/
type HistoryRequest struct {
	Coordinate Coordinate
}

/*
LatestRequest asks for the most recent retained observation of one coordinate.
*/
type LatestRequest struct {
	Coordinate Coordinate
}

/*
CountRequest asks for the number of retained observations of one coordinate.
*/
type CountRequest struct {
	Coordinate Coordinate
}

/*
SnapshotRequest asks for the current observation counts.
*/
type SnapshotRequest struct{}

/*
TimeRangeRequest asks for the earliest and latest observation time across all
retained data.
*/
type TimeRangeRequest struct{}

/*
VersionRequest asks for the monotonic committed-transition version: the
cumulative number of observations ever appended.
*/
type VersionRequest struct{}

/*
StoreCommand discriminates one observation-store operation. Exactly one field
is set; anything else is a shape failure.
*/
type StoreCommand struct {
	Register    *Coordinate
	Append      *Observation
	AppendAll   *[]Observation
	Measurement *MeasurementAppend
	Coordinates *CoordinateScope
	Ring        *RingRequest
	History     *HistoryRequest
	Latest      *LatestRequest
	Count       *CountRequest
	Snapshot    *SnapshotRequest
	TimeRange   *TimeRangeRequest
	Version     *VersionRequest
}

/*
StoreResult is one command response: the post-command version, the ordered
coordinate index (whole or one symbol), a read-locked ring view with its
existence, one coordinate's copied history, latest observation, count,
snapshot counts, or the retained time range.
*/
type StoreResult struct {
	Version      uint64
	Coordinates  []Coordinate
	Ring         RingView
	Found        bool
	Observations []Observation
	Observation  Observation
	Count        int
	Snapshot     StoreSnapshot
	From         time.Time
	To           time.Time
	TimeFound    bool
}

/*
RingView is a read-locked window over one resident observation ring. It is
the estimation path's zero-copy access to resident history: Len and At read
the ring in place in chronological order, and Close releases the read lock.
A view must be closed exactly once. The zero view (an unregistered
coordinate) has length zero and closes without effect.
*/
type RingView struct {
	ring   *observationRing
	unlock func()
}

/*
Len returns the number of retained observations in the ring.
*/
func (ringView RingView) Len() int {
	if ringView.ring == nil {
		return 0
	}

	return ringView.ring.size
}

/*
At returns the observation at logical index 0..Len-1 in insertion order.
*/
func (ringView RingView) At(index int) Observation {
	return ringView.ring.at(index)
}

/*
TimeAt returns only the timestamp at logical index 0..Len-1 in insertion
order, without copying the surrounding Observation. Alignment scans read the
timestamp to find the newest entry at or before a cutoff; they never need the
184-byte Observation struct for that comparison, so a per-step full copy is
pure waste on the estimation hot path.
*/
func (ringView RingView) TimeAt(index int) time.Time {
	return ringView.ring.at(index).At
}

/*
Close releases the ring read lock. It must be called exactly once per view
that was found.
*/
func (ringView RingView) Close() {
	if ringView.unlock != nil {
		ringView.unlock()
	}
}

/*
ObservationStore is resident streaming state: one bounded chronological ring
per Coordinate plus a resident ordered coordinate index, owned by one
Primitive. Coordinates are structural — registered once and traversed in
place; measurements never discover, clone, snapshot, sort, or recreate the
coordinate universe. Rings are traversed in place too: a ring is
chronological by construction, so retained history is never copied or
re-sorted on the hot path. The store owns its mutexes: rings are individually
read-write locked, the coordinate index is guarded by its own lock, and the
appended version is atomic, so reads from other goroutines are safe.
*/
type ObservationStore struct {
	*core.PrimitiveError

	capacity int

	rings sync.Map

	// indexMu guards the resident ordered coordinate index. Structural
	// registration is the only writer; readers iterate under RLock.
	// Measurement traffic never touches it.
	indexMu sync.RWMutex
	order   []Coordinate

	appended atomic.Uint64

	out StoreResult
}

type observationRing struct {
	mu         sync.RWMutex
	coordinate Coordinate
	entries    []Observation
	head       int
	size       int
}

/*
NewObservationStore builds a store Primitive with the given per-coordinate
capacity. Capacity is infrastructure provenance, not a statistical claim. A
non-positive capacity is recorded as a domain failure and every stream over
the store yields nothing.
*/
func NewObservationStore(capacity int) *ObservationStore {
	if capacity < 1 {
		return &ObservationStore{PrimitiveError: core.NewPrimitiveError(fmt.Errorf(
			"%w: relation: store capacity %d must be positive",
			core.ErrDomain, capacity,
		),
		)}
	}

	return &ObservationStore{PrimitiveError: core.NewPrimitiveError(), capacity: capacity}
}

/*
Next receives *StoreCommand payloads and yields a *StoreResult for each:
registration and append acknowledgements, coordinate traversals, read-locked
ring views, history copies, counts, snapshots, and time ranges. Any invalid
command ends the stream with the error recorded.
*/
func (observationStore *ObservationStore) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if observationStore.
		Error() !=
		nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			command := (*StoreCommand)(arriving)
			result, err := observationStore.execute(command)

			if err != nil {
				observationStore.Error(err)
				return
			}

			observationStore.out = result

			if !yield(unsafe.Pointer(&observationStore.out)) {
				return
			}
		}
	}
}

/*
execute dispatches one command to its intent and returns its result.
*/
func (observationStore *ObservationStore) execute(command *StoreCommand) (StoreResult, error) {
	intents := 0

	if command.Register != nil {
		intents++
	}

	if command.Append != nil {
		intents++
	}

	if command.AppendAll != nil {
		intents++
	}

	if command.Measurement != nil {
		intents++
	}

	if command.Coordinates != nil {
		intents++
	}

	if command.Ring != nil {
		intents++
	}

	if command.History != nil {
		intents++
	}

	if command.Latest != nil {
		intents++
	}

	if command.Count != nil {
		intents++
	}

	if command.Snapshot != nil {
		intents++
	}

	if command.TimeRange != nil {
		intents++
	}

	if command.Version != nil {
		intents++
	}

	if intents != 1 {
		return StoreResult{}, fmt.Errorf(
			"%w: relation: store command must set exactly one intent",
			core.ErrShape,
		)
	}

	if command.Register != nil {
		observationStore.registerRing(*command.Register)

		return StoreResult{Version: observationStore.appended.Load()}, nil
	}

	if command.Append != nil {
		return observationStore.append(*command.Append), nil
	}

	if command.AppendAll != nil {
		result := StoreResult{}

		for _, observation := range *command.AppendAll {
			result = observationStore.append(observation)
		}

		return result, nil
	}

	if command.Measurement != nil {
		return observationStore.appendMeasurement(command.Measurement)
	}

	if command.Coordinates != nil {
		return observationStore.coordinates(command.Coordinates.Symbol), nil
	}

	if command.Ring != nil {
		return observationStore.viewRing(command.Ring.Coordinate), nil
	}

	if command.History != nil {
		return observationStore.history(command.History.Coordinate), nil
	}

	if command.Latest != nil {
		return observationStore.latest(command.Latest.Coordinate), nil
	}

	if command.Count != nil {
		return observationStore.count(command.Count.Coordinate), nil
	}

	if command.Snapshot != nil {
		return observationStore.snapshot(), nil
	}

	if command.TimeRange != nil {
		return observationStore.timeRange(), nil
	}

	return StoreResult{Version: observationStore.appended.Load()}, nil
}

/*
registerRing structurally registers one coordinate, creating its resident
ring and inserting it into the resident ordered coordinate index. It is
idempotent and is the only path that grows the coordinate universe;
measurements for an unregistered coordinate take this same structural path
exactly once. Registration is a slow structural mutation, never a hot-path
operation, and no speculative ring is ever allocated by a concurrent loser.
*/
func (observationStore *ObservationStore) insertOrdered(coordinate Coordinate) {
	position := sort.Search(len(observationStore.order), func(index int) bool {
		return compareCoordinate(observationStore.order[index], coordinate) >= 0
	})

	observationStore.order = append(observationStore.order, Coordinate{})
	copy(observationStore.order[position+1:], observationStore.order[position:])
	observationStore.order[position] = coordinate
}

/*
append stores one observation for its coordinate, evicting the oldest
retained observation when the ring is full. It never blocks and never drops
because of a value threshold. A coordinate observed for the first time is
structurally registered exactly once; concurrent first observations never
allocate competing candidate rings.
*/
func (observationStore *ObservationStore) append(observation Observation) StoreResult {
	stored, found := observationStore.rings.Load(observation.Coordinate)

	if !found {
		stored = observationStore.registerRing(observation.Coordinate)
	}

	ring := stored.(*observationRing)
	ring.mu.Lock()
	ring.push(observation)
	ring.mu.Unlock()

	observationStore.appended.Add(1)

	return StoreResult{Version: observationStore.appended.Load()}
}

/*
appendMeasurement splits one data.Measurement into per-coordinate
observations and appends each of them. A measurement carrying an error is
rejected as a whole: the error is recorded and nothing is appended.
*/
func (observationStore *ObservationStore) appendMeasurement(
	request *MeasurementAppend,
) (StoreResult, error) {
	observations, err := splitMeasurement(request.Measurement, request.Epoch)

	if err != nil {
		return StoreResult{}, fmt.Errorf(
			"%w: relation: measurement carries an error",
			core.ErrDomain,
		)
	}

	result := StoreResult{}

	for _, observation := range observations {
		result = observationStore.append(observation)
	}

	return result, nil
}

/*
registerRing is the structural registration under the index lock, returning
the coordinate's resident ring whether it already existed or was just
created.
*/
func (observationStore *ObservationStore) registerRing(coordinate Coordinate) *observationRing {
	observationStore.indexMu.Lock()
	defer observationStore.indexMu.Unlock()

	if stored, found := observationStore.rings.Load(coordinate); found {
		return stored.(*observationRing)
	}

	ring := &observationRing{
		coordinate: coordinate,
		entries:    make([]Observation, observationStore.capacity),
	}

	observationStore.rings.Store(coordinate, ring)
	observationStore.insertOrdered(coordinate)

	return ring
}

/*
coordinates returns every registered coordinate in canonical
compareCoordinate order, in place, with one allocation for the result copy.
An empty Symbol returns the whole resident index.
*/
func (observationStore *ObservationStore) coordinates(symbol string) StoreResult {
	observationStore.indexMu.RLock()
	defer observationStore.indexMu.RUnlock()

	start := 0

	if symbol != "" {
		start = sort.Search(len(observationStore.order), func(index int) bool {
			return observationStore.order[index].Symbol >= symbol
		})
	}

	end := len(observationStore.order)

	if symbol != "" {
		end = start

		for index := start; index < len(observationStore.order) && observationStore.order[index].Symbol == symbol; index++ {
			end++
		}
	}

	coordinates := make([]Coordinate, end-start)
	copy(coordinates, observationStore.order[start:end])

	return StoreResult{
		Version:     observationStore.appended.Load(),
		Coordinates: coordinates,
	}
}

/*
viewRing returns a read-locked view of one coordinate's resident ring. The
Found boolean reports whether the coordinate is registered. A missing
coordinate is missing, not zero.
*/
func (observationStore *ObservationStore) viewRing(coordinate Coordinate) StoreResult {
	stored, found := observationStore.rings.Load(coordinate)

	if !found {
		return StoreResult{Version: observationStore.appended.Load()}
	}

	ring := stored.(*observationRing)
	ring.mu.RLock()

	return StoreResult{
		Version: observationStore.appended.Load(),
		Ring:    RingView{ring: ring, unlock: ring.mu.RUnlock},
		Found:   true,
	}
}

/*
history returns one coordinate's retained observations copied in
chronological order. The ring is chronological by construction, so nothing is
re-sorted. A coordinate that has never been observed yields Found false and
no observations; that is missing, not zero.
*/
func (observationStore *ObservationStore) history(coordinate Coordinate) StoreResult {
	stored, found := observationStore.rings.Load(coordinate)

	if !found {
		return StoreResult{Version: observationStore.appended.Load()}
	}

	ring := stored.(*observationRing)
	ring.mu.RLock()
	defer ring.mu.RUnlock()

	if ring.size == 0 {
		return StoreResult{Version: observationStore.appended.Load(), Found: true}
	}

	observations := make([]Observation, ring.size)

	for index := 0; index < ring.size; index++ {
		observations[index] = ring.at(index)
	}

	return StoreResult{
		Version:      observationStore.appended.Load(),
		Found:        true,
		Observations: observations,
	}
}

/*
latest returns the most recent retained observation for one coordinate,
reading the newest ring entry directly under the coordinate's read lock.
*/
func (observationStore *ObservationStore) latest(coordinate Coordinate) StoreResult {
	stored, found := observationStore.rings.Load(coordinate)

	if !found {
		return StoreResult{Version: observationStore.appended.Load()}
	}

	ring := stored.(*observationRing)
	ring.mu.RLock()
	defer ring.mu.RUnlock()

	if ring.size == 0 {
		return StoreResult{Version: observationStore.appended.Load(), Found: true}
	}

	return StoreResult{
		Version:     observationStore.appended.Load(),
		Found:       true,
		Observation: ring.at(ring.size - 1),
	}
}

/*
count returns the number of retained observations for one coordinate.
*/
func (observationStore *ObservationStore) count(coordinate Coordinate) StoreResult {
	stored, found := observationStore.rings.Load(coordinate)

	if !found {
		return StoreResult{Version: observationStore.appended.Load()}
	}

	ring := stored.(*observationRing)
	ring.mu.RLock()
	defer ring.mu.RUnlock()

	return StoreResult{
		Version: observationStore.appended.Load(),
		Found:   true,
		Count:   ring.size,
	}
}

/*
snapshot returns the current observation counts. The conformance suite uses
it to verify that MCTS rollouts never become observational evidence.
*/
func (observationStore *ObservationStore) snapshot() StoreResult {
	snapshot := StoreSnapshot{
		Capacity: observationStore.capacity,
		Appended: observationStore.appended.Load(),
	}

	observationStore.rings.Range(func(key, value any) bool {
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

	return StoreResult{
		Version:  observationStore.appended.Load(),
		Snapshot: snapshot,
	}
}

/*
timeRange returns the earliest and latest observation time across all
retained data, when any exists.
*/
func (observationStore *ObservationStore) timeRange() StoreResult {
	var earliest, latest time.Time
	found := false

	observationStore.rings.Range(func(key, value any) bool {
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

	return StoreResult{
		Version:   observationStore.appended.Load(),
		From:      earliest,
		To:        latest,
		TimeFound: found,
	}
}

func (observationRing *observationRing) push(observation Observation) {
	observationRing.entries[observationRing.head] = observation
	observationRing.head = (observationRing.head + 1) % len(observationRing.entries)

	if observationRing.size < len(observationRing.entries) {
		observationRing.size++
	}
}

// at returns the observation at logical index 0..size-1 in insertion order.
func (observationRing *observationRing) at(index int) Observation {
	logicalHead := (observationRing.head - observationRing.size + len(observationRing.entries)) % len(observationRing.entries)
	return observationRing.entries[(logicalHead+index)%len(observationRing.entries)]
}
