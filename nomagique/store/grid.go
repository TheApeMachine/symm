package store

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"golang.design/x/lockfree/lf"
	"gonum.org/v1/gonum/mat"
)

const (
	// LitRegionTokenSize is kept as a one-byte region identifier for the
	// existing cognition/token pipeline.
	LitRegionTokenSize = 1

	// The learned partition is laid out as a 5x4 display grid. The layout is
	// deliberately separate from partition learning: graph structure decides
	// which metric channel belongs to which region, and display layout decides
	// where regions are drawn on screen.
	DisplayColumns = 5
	DisplayRows    = 4

	// TargetRegionCount defines the nominal number of balanced clusters.
	TargetRegionCount = DisplayColumns * DisplayRows

	// MinRegionSize prevents singletons or tiny residual regions. If the metric
	// universe is smaller than TargetRegionCount*MinRegionSize, partition count
	// shrinks adaptively so every region has genuine support.
	MinRegionSize = 4

	// CorrelationShrinkage dampens low-observation edge confidence toward zero.
	CorrelationShrinkage = 32.0

	// ConvergenceStreak is the length of each of the two windows of
	// comparable passes whose mean co-membership drift Converged compares
	// (see observeDriftLocked).
	ConvergenceStreak = 5
)

/*
PairStats stores the cross-metric statistical moments in a packed symmetric format.
Sized to 64 bytes to align with CPU cache lines without pointer indirection.
*/
type PairStats struct {
	Same     int32
	Opposite int32
	Total    int32
	_        int32
	Weight   float64
	SumX     float64
	SumY     float64
	SumXX    float64
	SumYY    float64
	SumXY    float64
}

func (pair *PairStats) update(leftVal, rightVal, weight float64) {
	if weight <= 0 || !finite(weight) || !finite(leftVal) || !finite(rightVal) {
		return
	}

	pair.Total++

	product := leftVal * rightVal

	if product > 0 {
		pair.Same++
	}

	if product < 0 {
		pair.Opposite++
	}

	pair.Weight += weight
	pair.SumX += weight * leftVal
	pair.SumY += weight * rightVal
	pair.SumXX += weight * leftVal * leftVal
	pair.SumYY += weight * rightVal * rightVal
	pair.SumXY += weight * leftVal * rightVal
}

func (pair *PairStats) correlation() (float64, bool) {
	if pair == nil || pair.Weight <= 0 {
		return 0, false
	}

	w := pair.Weight
	cov := w*pair.SumXY - pair.SumX*pair.SumY
	varX := w*pair.SumXX - pair.SumX*pair.SumX
	varY := w*pair.SumYY - pair.SumY*pair.SumY

	if varX < 0 && varX > -1e-12 {
		varX = 0
	}

	if varY < 0 && varY > -1e-12 {
		varY = 0
	}

	if varX <= 0 || varY <= 0 {
		return 0, false
	}

	corr := cov / math.Sqrt(varX*varY)

	if !finite(corr) {
		return 0, false
	}

	return clamp(corr, -1, 1), true
}

func (pair *PairStats) directionalAgreement() float64 {
	if pair == nil || pair.Total == 0 {
		return 0
	}

	return float64(pair.Same-pair.Opposite) / float64(pair.Total)
}

func (pair *PairStats) affinity() float64 {
	if pair == nil {
		return 0
	}

	strength := 0.0
	evidence := pair.Weight

	if corr, ok := pair.correlation(); ok {
		strength = corr
	}

	if _, ok := pair.correlation(); !ok {
		strength = pair.directionalAgreement()

		if evidence <= 0 {
			evidence = float64(pair.Total)
		}
	}

	if strength <= 0 || evidence <= 0 {
		return 0
	}

	reliability := evidence / (evidence + CorrelationShrinkage)
	return strength * reliability
}

func symIdx(left, right int) int {
	if left > right {
		left, right = right, left
	}

	return right*(right+1)/2 + left
}

/*
Relation is maintained for snapshot serialization compatibility.
*/
type Relation struct {
	Same     int64 `json:"same"`
	Opposite int64 `json:"opposite"`
	Total    int64 `json:"total"`

	Weight float64 `json:"weight,omitempty"`
	SumX   float64 `json:"sum_x,omitempty"`
	SumY   float64 `json:"sum_y,omitempty"`
	SumXX  float64 `json:"sum_xx,omitempty"`
	SumYY  float64 `json:"sum_yy,omitempty"`
	SumXY  float64 `json:"sum_xy,omitempty"`
}

func (relation *Relation) update(leftVal, rightVal, weight float64) {
	if relation == nil || weight <= 0 || !finite(leftVal) || !finite(rightVal) || !finite(weight) {
		return
	}

	relation.Total++

	product := leftVal * rightVal

	if product > 0 {
		relation.Same++
	}

	if product < 0 {
		relation.Opposite++
	}

	relation.Weight += weight
	relation.SumX += weight * leftVal
	relation.SumY += weight * rightVal
	relation.SumXX += weight * leftVal * leftVal
	relation.SumYY += weight * rightVal * rightVal
	relation.SumXY += weight * leftVal * rightVal
}

func pairKey(leftID, rightID uint32) uint64 {
	if leftID < rightID {
		return (uint64(leftID) << 32) | uint64(rightID)
	}

	return (uint64(rightID) << 32) | uint64(leftID)
}

func pairIDs(key uint64) (uint32, uint32) {
	return uint32(key >> 32), uint32(key)
}

/*
partitionDrift is the co-membership drift of one comparable partition pass
and the evidence (Observations) it was computed from.
*/
type partitionDrift struct {
	drift        float64
	observations int64
}

/*
Cell is one learned vertex in the metric-affinity graph.

Mean and M2 are the Welford moments of the cell's deformation magnitude over
its Visits while the grid develops. They are the cell's own noise floor: a
frozen grid standardizes every lit deformation against them, so cells (and
regions) of different natural activity are compared in one unit.
*/
type Cell struct {
	ID     uint32  `json:"id"`
	Key    string  `json:"key"`
	Region uint8   `json:"region"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Visits uint64  `json:"visits"`
	Mean   float64 `json:"mean"`
	M2     float64 `json:"m2"`
}

/*
observe folds one deformation magnitude into the cell's noise floor.
*/
func (cell *Cell) observe(magnitude float64) {
	cell.Visits++
	delta := magnitude - cell.Mean
	cell.Mean += delta / float64(cell.Visits)
	cell.M2 += delta * (magnitude - cell.Mean)
}

/*
standardize answers how far magnitude stands above the cell's noise floor, in
units of its dispersion. A cell that has not shown any dispersion has no
noise floor yet, and its excitation is undefined rather than zero.
*/
func (cell *Cell) standardize(magnitude float64) (float64, bool) {
	if cell.Visits < 2 || cell.M2 <= 0 {
		return 0, false
	}

	return (magnitude - cell.Mean) / math.Sqrt(cell.M2/float64(cell.Visits-1)), true
}

type metricArrival struct {
	id      uint32
	value   float64
	quality float64
}

type pendingSample struct {
	WeightedValue float64 `json:"weighted_value"`
	QualityWeight float64 `json:"quality_weight"`
	QualitySum    float64 `json:"quality_sum"`
	Count         int     `json:"count"`
}

func (sample *pendingSample) add(value, quality float64) {
	if sample == nil || quality <= 0 || !finite(value) || !finite(quality) {
		return
	}

	sample.WeightedValue += value * quality
	sample.QualityWeight += quality
	sample.QualitySum += quality
	sample.Count++
}

func (sample pendingSample) value() (float64, float64, bool) {
	if sample.QualityWeight <= 0 || sample.Count <= 0 {
		return 0, 0, false
	}

	value := sample.WeightedValue / sample.QualityWeight
	quality := clamp(sample.QualitySum/float64(sample.Count), 0, 1)
	return value, quality, finite(value)
}

/*
Grid learns a vectorized metric-affinity graph using contiguous packed symmetric matrices
and partitions it into balanced regions with zero map locks for streaming evaluation.
*/
type Grid struct {
	*runtime.System
	updateMu     sync.Mutex
	settled      atomic.Bool
	observations atomic.Int64
	cellsLF      *lf.OrderedMap[string, *Cell]
	idToCellLF   atomic.Pointer[[]*Cell]

	Settled       bool             `json:"settled"`
	Observations  int64            `json:"observations"`
	Cells         map[string]*Cell `json:"cells"`
	RegionMembers map[uint8]int    `json:"region_members"`

	cellIDs  map[string]uint32
	idToCell []*Cell
	dirty    bool

	pairs    []PairStats
	capacity int

	pendingTick    int64
	pending        map[uint32]pendingSample
	prevPartitions map[string]uint8

	// drifts holds the co-membership drift of the latest comparable
	// partitions (at most two ConvergenceStreak windows) since the cell set
	// last changed.
	drifts []partitionDrift

	// partitioning single-flights PartitionAsync: at most one spectral
	// partition computes at a time, and the caller never waits for it.
	partitioning atomic.Bool
	partitionWG  sync.WaitGroup

	// regionsFormed mirrors len(RegionMembers) for lock-free telemetry.
	regionsFormed atomic.Int32
}

func stringLess(a, b string) bool {
	return a < b
}

func NewGrid() *Grid {
	cellsLF := lf.NewOrderedMap[string, *Cell](stringLess)
	idToCell := make([]*Cell, 0)

	grid := &Grid{
		cellsLF:        cellsLF,
		Cells:          make(map[string]*Cell),
		RegionMembers:  make(map[uint8]int),
		cellIDs:        make(map[string]uint32),
		idToCell:       idToCell,
		pending:        make(map[uint32]pendingSample),
		prevPartitions: make(map[string]uint8),
	}

	grid.System = runtime.NewSystem(context.Background(), "store:grid")

	grid.idToCellLF.Store(&idToCell)
	return grid
}

func (grid *Grid) growPairs(neededCapacity int) {
	if neededCapacity <= grid.capacity {
		return
	}

	newCap := neededCapacity
	newPairs := make([]PairStats, newCap*(newCap+1)/2)
	copy(newPairs, grid.pairs)
	grid.pairs = newPairs
	grid.capacity = newCap
}

/*
CellKey identifies a grid cell by metric label alone. The grid learns the
geometry of metrics, not of symbols or producers: the same metric observed
for BTC/USD and ETH/USD, or published by two producers, is one cell.

Pair facts published as "<fact>@<peer symbol>" (correlation, leadlag) carry
a symbol in the label; the peer qualifier is dropped so every peer of one
fact lands in the single <fact> cell. Without this the grid grows one cell
per fact per peer symbol and the O(n^2) pair update explodes.
*/
func CellKey(name string) string {
	if fact, _, qualified := strings.Cut(name, "@"); qualified {
		return fact
	}

	return name
}

func (grid *Grid) IsSettled() bool {
	if grid == nil {
		return false
	}

	return grid.settled.Load() || grid.Settled
}

/*
RegionsFormed answers how many regions the last committed partition holds
(0 until a partition has committed). It never takes the update lock.
*/
func (grid *Grid) RegionsFormed() int {
	if grid == nil {
		return 0
	}

	return int(grid.regionsFormed.Load())
}

func (grid *Grid) RegionCount() int {
	return TargetRegionCount
}

func (grid *Grid) Region(cellKey string) uint8 {
	if grid == nil {
		return 0
	}

	if cell, ok := grid.cellsLF.Get(cellKey); ok && cell != nil {
		return cell.Region
	}

	return 0
}

func (grid *Grid) RegionAt(x, y float64) (uint8, bool) {
	if grid == nil {
		return 0, false
	}

	idToCellPtr := grid.idToCellLF.Load()

	if idToCellPtr == nil {
		return 0, false
	}

	bestRegion := uint8(0)
	bestDist := math.MaxFloat64

	for _, cell := range *idToCellPtr {
		if cell == nil || cell.Region == 0 {
			continue
		}

		dx := cell.X - x
		dy := cell.Y - y
		dist := dx*dx + dy*dy

		if dist < bestDist {
			bestDist = dist
			bestRegion = cell.Region
		}
	}

	if bestRegion == 0 || bestDist > 0.05 {
		return 0, false
	}

	return bestRegion, true
}

func (grid *Grid) CellAt(name string) *Cell {
	if grid == nil {
		return nil
	}

	key := CellKey(name)

	if cell, ok := grid.cellsLF.Get(key); ok && cell != nil {
		return cell
	}

	idToCellPtr := grid.idToCellLF.Load()

	if idToCellPtr == nil {
		return nil
	}

	for _, cell := range *idToCellPtr {
		if cell != nil && cell.Key == key {
			return cell
		}
	}

	return nil
}

func (grid *Grid) CellCount() int {
	if grid == nil {
		return 0
	}

	idToCellPtr := grid.idToCellLF.Load()

	if idToCellPtr != nil {
		return len(*idToCellPtr)
	}

	return len(grid.idToCell)
}

/*
Update folds one observation pass into the grid. Deformations are keyed by
CellKey(metric) and carry each channel's movement within its own stream (see
Stream.Deform), so a cell shared by several symbols never deforms across
them. A positive tick buffers the pass until the tick changes, so every
channel observed in one market tick contributes to a single pass; a
non-positive tick commits immediately.
*/
func (grid *Grid) Update(tick int64, deformations map[string]float64) {
	if grid == nil || len(deformations) == 0 {
		return
	}

	grid.updateMu.Lock()
	defer grid.updateMu.Unlock()

	arrivals := grid.extractTrainingArrivalsLocked(deformations)

	if len(arrivals) == 0 {
		return
	}

	if tick > 0 {
		if grid.pendingTick != 0 && grid.pendingTick != tick {
			grid.flushPendingLocked()
		}

		grid.pendingTick = tick
		grid.mergePendingLocked(arrivals)
		return
	}

	grid.flushPendingLocked()
	grid.commitPassLocked(arrivals)
}

func (grid *Grid) ensureCellLocked(key string) *Cell {
	if cell, ok := grid.cellsLF.Get(key); ok && cell != nil {
		return cell
	}

	id := uint32(len(grid.idToCell))

	if int(id) >= grid.capacity {
		grid.growPairs(int(id) + 1)
	}

	cell := &Cell{
		ID:  id,
		Key: key,
	}
	grid.Cells[key] = cell
	grid.cellsLF.Set(key, cell)
	grid.cellIDs[key] = id
	grid.idToCell = append(grid.idToCell, cell)
	copied := make([]*Cell, len(grid.idToCell))
	copy(copied, grid.idToCell)
	grid.idToCellLF.Store(&copied)

	grid.dirty = true
	return cell
}

/*
extractTrainingArrivalsLocked turns one pass of channel deformations into
arrivals. Channels are visited in key order so cell IDs are assigned
deterministically.
*/
func (grid *Grid) extractTrainingArrivalsLocked(
	deformations map[string]float64,
) []metricArrival {
	keys := slices.Sorted(maps.Keys(deformations))
	arrivals := make([]metricArrival, 0, len(keys))

	for _, channel := range keys {
		cell := grid.ensureCellLocked(channel)
		cell.observe(math.Abs(deformations[channel]))

		arrivals = append(arrivals, metricArrival{
			id:      cell.ID,
			value:   deformations[channel],
			quality: 1,
		})
	}

	return arrivals
}

func (grid *Grid) mergePendingLocked(arrivals []metricArrival) {
	if grid.pending == nil {
		grid.pending = make(map[uint32]pendingSample)
	}

	for _, arrival := range arrivals {
		sample := grid.pending[arrival.id]
		sample.add(arrival.value, arrival.quality)
		grid.pending[arrival.id] = sample
	}
}

func (grid *Grid) flushPendingLocked() {
	if len(grid.pending) == 0 {
		grid.pendingTick = 0
		return
	}

	arrivals := make([]metricArrival, 0, len(grid.pending))
	ids := make([]int, 0, len(grid.pending))

	for id := range grid.pending {
		ids = append(ids, int(id))
	}

	slices.Sort(ids)

	for _, rawID := range ids {
		id := uint32(rawID)
		value, quality, ok := grid.pending[id].value()

		if !ok {
			continue
		}

		arrivals = append(arrivals, metricArrival{id: id, value: value, quality: quality})
	}

	grid.pending = make(map[uint32]pendingSample)
	grid.pendingTick = 0

	if len(arrivals) >= 2 {
		grid.commitPassLocked(arrivals)
	}
}

func (grid *Grid) commitPassLocked(arrivals []metricArrival) {
	if len(arrivals) < 2 {
		return
	}

	collapsed := make(map[uint32]pendingSample, len(arrivals))

	for _, arrival := range arrivals {
		sample := collapsed[arrival.id]
		sample.add(arrival.value, arrival.quality)
		collapsed[arrival.id] = sample
	}

	type passValue struct {
		id      int
		value   float64
		quality float64
	}

	pass := make([]passValue, 0, len(collapsed))

	for id, sample := range collapsed {
		value, quality, ok := sample.value()

		if !ok {
			continue
		}

		pass = append(pass, passValue{id: int(id), value: value, quality: quality})
	}

	slices.SortFunc(pass, func(left, right passValue) int { return left.id - right.id })

	if len(pass) > 0 {
		grid.growPairs(pass[len(pass)-1].id + 1)
	}

	touched := false

	for first := range pass {
		left := pass[first]

		for second := first + 1; second < len(pass); second++ {
			right := pass[second]
			weight := math.Sqrt(left.quality * right.quality)

			if weight <= 0 || !finite(weight) {
				continue
			}

			grid.pairs[symIdx(left.id, right.id)].update(left.value, right.value, weight)
			touched = true
		}
	}

	if touched && !grid.IsSettled() {
		grid.dirty = true
	}

	grid.Observations++
	grid.observations.Add(1)
}

/*
Relations answers the non-empty pair statistics of the learned graph, derived
from the packed pair matrix (the only pair store).
*/
func (grid *Grid) Relations() []Relation {
	if grid == nil {
		return nil
	}

	grid.updateMu.Lock()
	defer grid.updateMu.Unlock()

	n := len(grid.idToCell)
	relations := make([]Relation, 0)

	for right := 1; right < n; right++ {
		for left := 0; left < right; left++ {
			idx := symIdx(left, right)

			if idx >= len(grid.pairs) {
				continue
			}

			pair := &grid.pairs[idx]

			if pair.Total == 0 && pair.Weight == 0 {
				continue
			}

			relations = append(relations, Relation{
				Same:     int64(pair.Same),
				Opposite: int64(pair.Opposite),
				Total:    int64(pair.Total),
				Weight:   pair.Weight,
				SumX:     pair.SumX,
				SumY:     pair.SumY,
				SumXX:    pair.SumXX,
				SumYY:    pair.SumYY,
				SumXY:    pair.SumXY,
			})
		}
	}

	return relations
}

/*
Partition discovers and updates regions for all currently observed metrics
without freezing the grid. It blocks the caller for the full spectral
computation; hot paths use PartitionAsync. The lock is held only to copy the
affinity graph and to commit the result, never across the eigensolves, so
Update keeps flowing while regions are computed.
*/
func (grid *Grid) Partition() {
	if grid == nil {
		return
	}

	grid.partition(false)
}

/*
PartitionAsync starts a background Partition unless one is already running,
and returns immediately either way. It reports whether a pass was started.
*/
func (grid *Grid) PartitionAsync() bool {
	if grid == nil || grid.IsSettled() || !grid.partitioning.CompareAndSwap(false, true) {
		return false
	}

	grid.partitionWG.Add(1)

	go func() {
		defer grid.partitionWG.Done()
		defer grid.partitioning.Store(false)
		grid.partition(true)
	}()

	return true
}

/*
WaitPartition blocks until a running PartitionAsync pass has committed.
*/
func (grid *Grid) WaitPartition() {
	if grid == nil {
		return
	}

	grid.partitionWG.Wait()
}

/*
partition computes and commits one region assignment. A background pass
(background=true) that finishes after the grid settled is discarded: only
Settle's own partition may assign regions to a frozen grid.
*/
func (grid *Grid) partition(background bool) {
	grid.updateMu.Lock()
	grid.flushPendingLocked()
	keys, affinity := grid.affinitySnapshotLocked()
	grid.updateMu.Unlock()

	cellCount := len(keys)
	errnie.Info(fmt.Sprintf("[grid] Partition() started: %d cells observed", cellCount))

	partitions := computePartitions(keys, affinity)

	if len(partitions) == 0 {
		errnie.Info("[grid] Partition() computed 0 partitions")
		return
	}

	grid.updateMu.Lock()
	defer grid.updateMu.Unlock()

	if background && grid.IsSettled() {
		return
	}

	grid.commitPartitionsLocked(partitions, cellCount)
	errnie.Info(fmt.Sprintf("[grid] Partition() committed: %d regions across %d cells (comparable passes: %d/%d)", len(grid.RegionMembers), cellCount, len(grid.drifts), ConvergenceStreak*2))
}

/*
affinitySnapshotLocked copies the sorted cell keys and their affinity matrix
so the spectral partition can run without the update lock.
*/
func (grid *Grid) affinitySnapshotLocked() ([]string, *mat.SymDense) {
	n := len(grid.Cells)

	if n == 0 {
		return nil, nil
	}

	keys := make([]string, 0, n)

	for key := range grid.Cells {
		keys = append(keys, key)
	}

	slices.Sort(keys)
	return keys, grid.affinityMatrixLocked(keys)
}

/*
Settle freezes the current graph into balanced regions using vectorized Gonum eigensolvers.
*/
func (grid *Grid) Settle() {
	if grid == nil {
		return
	}

	// A background pass committing after settlement would reassign frozen
	// regions, so it finishes before the settling partition runs.
	grid.WaitPartition()

	errnie.Info(fmt.Sprintf("[grid] Settle() requested: %d cells", grid.CellCount()))
	grid.Partition()

	grid.updateMu.Lock()
	defer grid.updateMu.Unlock()

	grid.settled.Store(true)
	grid.Settled = true
	grid.dirty = false
	errnie.Info(fmt.Sprintf("[grid] Settle() complete: grid settled with %d regions across %d cells", len(grid.RegionMembers), len(grid.Cells)))
}

func computePartitions(keys []string, affinity *mat.SymDense) map[string]uint8 {
	n := len(keys)

	if n == 0 {
		return nil
	}

	k := targetPartitionCount(n)
	errnie.Info(fmt.Sprintf("[grid] computePartitions: n=%d metrics, target clusters k=%d", n, k))

	if k <= 1 {
		assigned := make(map[string]uint8, n)

		for _, key := range keys {
			assigned[key] = 1
		}

		return assigned
	}

	capacities := balancedCapacities(n, k)
	vertices := make([]int, n)

	for i := range vertices {
		vertices[i] = i
	}

	groups := make([][]int, 0, k)

	var split func([]int, []int)

	split = func(group []int, caps []int) {
		if len(caps) <= 1 {
			groups = append(groups, group)
			return
		}

		leftParts := len(caps) / 2
		leftSize := 0

		for _, size := range caps[:leftParts] {
			leftSize += size
		}

		left, right := spectralBalancedSplit(group, leftSize, affinity, keys)
		split(left, caps[:leftParts])
		split(right, caps[leftParts:])
	}

	split(vertices, capacities)
	return canonicalRegionAssignment(groups, keys)
}

func targetPartitionCount(metricCount int) int {
	if metricCount <= 0 {
		return 0
	}

	if metricCount < MinRegionSize {
		return 1
	}

	maxWithoutSingletons := max(metricCount/MinRegionSize, 1)

	return min(TargetRegionCount, maxWithoutSingletons)
}

func balancedCapacities(n, k int) []int {
	caps := make([]int, k)
	base := n / k
	extra := n % k

	for i := range caps {
		caps[i] = base

		if i < extra {
			caps[i]++
		}
	}

	return caps
}

func (grid *Grid) affinityMatrixLocked(keys []string) *mat.SymDense {
	n := len(keys)
	matrix := mat.NewSymDense(n, nil)

	for right := 1; right < n; right++ {
		rightCell := grid.Cells[keys[right]]
		rightID := int(rightCell.ID)

		for left := 0; left < right; left++ {
			leftCell := grid.Cells[keys[left]]
			leftID := int(leftCell.ID)

			idx := symIdx(leftID, rightID)

			if idx >= len(grid.pairs) {
				continue
			}

			weight := grid.pairs[idx].affinity()

			if weight <= 0 || !finite(weight) {
				continue
			}

			matrix.SetSym(left, right, weight)
		}
	}

	return matrix
}

/*
spectralBalancedSplit performs one exact-cardinality spectral bisection using
the Fiedler ordering of the symmetric normalized graph Laplacian.
*/
func spectralBalancedSplit(
	vertices []int,
	leftSize int,
	affinity *mat.SymDense,
	keys []string,
) ([]int, []int) {
	n := len(vertices)

	if leftSize <= 0 {
		return nil, append([]int(nil), vertices...)
	}

	if leftSize >= n {
		return append([]int(nil), vertices...), nil
	}

	if n <= 2 {
		ordered := append([]int(nil), vertices...)
		sort.Slice(ordered, func(i, j int) bool { return keys[ordered[i]] < keys[ordered[j]] })
		return append([]int(nil), ordered[:leftSize]...), append([]int(nil), ordered[leftSize:]...)
	}

	degree := make([]float64, n)
	totalWeight := 0.0

	for i := range n {
		vi := vertices[i]

		for j := i + 1; j < n; j++ {
			vj := vertices[j]
			w := affinity.At(vi, vj)

			if w <= 0 {
				continue
			}

			degree[i] += w
			degree[j] += w
			totalWeight += w
		}
	}

	if totalWeight <= 0 {
		return deterministicSplit(vertices, leftSize, keys)
	}

	laplacian := mat.NewSymDense(n, nil)

	for i := range n {
		if degree[i] > 0 {
			laplacian.SetSym(i, i, 1)
		}
	}

	for i := range n {
		if degree[i] <= 0 {
			continue
		}

		invSqrtI := 1.0 / math.Sqrt(degree[i])
		vi := vertices[i]

		for j := i + 1; j < n; j++ {
			if degree[j] <= 0 {
				continue
			}

			vj := vertices[j]
			w := affinity.At(vi, vj)

			if w <= 0 {
				continue
			}

			laplacian.SetSym(i, j, -w*(invSqrtI/math.Sqrt(degree[j])))
		}
	}

	var eigen mat.EigenSym

	if ok := eigen.Factorize(laplacian, true); !ok {
		return deterministicSplit(vertices, leftSize, keys)
	}

	values := eigen.Values(nil)

	if len(values) < 2 {
		return deterministicSplit(vertices, leftSize, keys)
	}

	indices := make([]int, len(values))

	for i := range indices {
		indices[i] = i
	}

	sort.SliceStable(indices, func(i, j int) bool {
		left, right := values[indices[i]], values[indices[j]]

		if left == right {
			return indices[i] < indices[j]
		}

		return left < right
	})

	var vectors mat.Dense
	eigen.VectorsTo(&vectors)
	fiedlerColumn := indices[1]

	type scoredVertex struct {
		vertex int
		score  float64
	}

	ordered := make([]scoredVertex, n)

	for local, vertex := range vertices {
		score := vectors.At(local, fiedlerColumn)

		if !finite(score) {
			return deterministicSplit(vertices, leftSize, keys)
		}

		ordered[local] = scoredVertex{vertex: vertex, score: score}
	}

	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].score == ordered[j].score {
			return keys[ordered[i].vertex] < keys[ordered[j].vertex]
		}

		return ordered[i].score < ordered[j].score
	})

	left := make([]int, leftSize)
	right := make([]int, n-leftSize)

	for i := range ordered {
		if i < leftSize {
			left[i] = ordered[i].vertex
		}

		if i >= leftSize {
			right[i-leftSize] = ordered[i].vertex
		}
	}

	return left, right
}

func deterministicSplit(vertices []int, leftSize int, keys []string) ([]int, []int) {
	ordered := append([]int(nil), vertices...)
	sort.Slice(ordered, func(i, j int) bool { return keys[ordered[i]] < keys[ordered[j]] })
	return append([]int(nil), ordered[:leftSize]...), append([]int(nil), ordered[leftSize:]...)
}

func canonicalRegionAssignment(groups [][]int, keys []string) map[string]uint8 {
	type canonicalGroup struct {
		representative string
		members        []string
	}

	prepared := make([]canonicalGroup, 0, len(groups))

	for _, group := range groups {
		if len(group) == 0 {
			continue
		}

		memberKeys := make([]string, len(group))

		for i, vertex := range group {
			memberKeys[i] = keys[vertex]
		}

		slices.Sort(memberKeys)
		prepared = append(prepared, canonicalGroup{
			representative: memberKeys[0],
			members:        memberKeys,
		})
	}

	sort.Slice(prepared, func(i, j int) bool {
		return prepared[i].representative < prepared[j].representative
	})

	assigned := make(map[string]uint8, len(keys))

	for index, group := range prepared {
		region := uint8(index + 1)

		for _, key := range group.members {
			assigned[key] = region
		}
	}

	return assigned
}

func (grid *Grid) commitPartitionsLocked(partitions map[string]uint8, partitionedCount int) {
	grid.RegionMembers = make(map[uint8]int, len(partitions))

	for key, region := range partitions {
		cell := grid.Cells[key]

		if cell == nil {
			continue
		}

		cell.Region = region
		grid.RegionMembers[region]++
	}

	regionCount := len(grid.RegionMembers)
	grid.regionsFormed.Store(int32(regionCount))

	for _, cell := range grid.Cells {
		if cell == nil || cell.Region == 0 {
			continue
		}

		cell.X, cell.Y = regionCenter(cell.Region, regionCount)
	}

	grid.observeDriftLocked(partitions, partitionedCount)
	grid.prevPartitions = partitions
}

/*
observeDriftLocked records the co-membership drift between this partition
and the previous one. Balanced regions must cut through latent groups larger
than a region, and the members of such a group are exchangeable: which of
them lands on which side of the cut is decided by sampling noise on every
pass, so the drift has an irreducible floor and exact repetition need never
happen. More evidence only lowers the drift while it still resolves
structure.

Converged compares the mean drift of the latest ConvergenceStreak passes with
the mean of the ConvergenceStreak passes before them. The grid has converged
once the latest window no longer drifts less than the earlier one: what still
moves after that is exchangeable noise, which freezing cannot make worse.

A pass is only comparable with its predecessor over the same cell set and
only once new evidence arrived between them; a repeat over unchanged pair
statistics would record a drift of zero that measures nothing. Cells that
arrived while an async partition computed, or a new metric, restart the
record.
*/
func (grid *Grid) observeDriftLocked(partitions map[string]uint8, partitionedCount int) {
	if len(grid.Cells) != partitionedCount || len(grid.prevPartitions) != partitionedCount {
		grid.drifts = grid.drifts[:0]
		return
	}

	if count := len(grid.drifts); count > 0 && grid.drifts[count-1].observations == grid.Observations {
		return
	}

	drift, ok := coMembershipDrift(grid.prevPartitions, partitions)

	if !ok {
		grid.drifts = grid.drifts[:0]
		return
	}

	grid.drifts = append(grid.drifts, partitionDrift{
		drift:        drift,
		observations: grid.Observations,
	})

	if excess := len(grid.drifts) - ConvergenceStreak*2; excess > 0 {
		grid.drifts = append(grid.drifts[:0], grid.drifts[excess:]...)
	}
}

/*
driftSettledLocked applies the window comparison described on
observeDriftLocked.
*/
func (grid *Grid) driftSettledLocked() bool {
	if len(grid.drifts) < ConvergenceStreak*2 {
		return false
	}

	earlier := grid.drifts[:ConvergenceStreak]
	latest := grid.drifts[ConvergenceStreak:]
	earlierDrift, latestDrift := 0.0, 0.0

	for index := range ConvergenceStreak {
		earlierDrift += earlier[index].drift
		latestDrift += latest[index].drift
	}

	return latestDrift >= earlierDrift
}

/*
coMembershipDrift is the fraction of cell pairs whose same-region relation
differs between two partitions of one cell set (1 minus the Rand index). It
is invariant to region relabeling. It is undefined (false) when the cell sets
differ or hold fewer than two cells.
*/
func coMembershipDrift(previous, current map[string]uint8) (float64, bool) {
	if len(previous) != len(current) || len(current) < 2 {
		return 0, false
	}

	keys := slices.Sorted(maps.Keys(current))

	for _, key := range keys {
		if _, ok := previous[key]; !ok {
			return 0, false
		}
	}

	disagreements := 0
	pairs := 0

	for first := range keys {
		for second := first + 1; second < len(keys); second++ {
			was := previous[keys[first]] == previous[keys[second]]
			is := current[keys[first]] == current[keys[second]]

			if was != is {
				disagreements++
			}

			pairs++
		}
	}

	return float64(disagreements) / float64(pairs), true
}

/*
Converged reports whether the metric partition has stabilized across consecutive passes.
*/
func (grid *Grid) Converged() bool {
	if grid == nil {
		return false
	}

	grid.updateMu.Lock()
	defer grid.updateMu.Unlock()

	metricCount := len(grid.Cells)

	if metricCount < 4 {
		return false
	}

	if len(grid.RegionMembers) < 2 {
		return false
	}

	converged := grid.driftSettledLocked()

	if converged {
		errnie.Info(fmt.Sprintf("[grid] Converged! %d metrics stabilized across %d regions (drift %.4f)", metricCount, len(grid.RegionMembers), grid.drifts[len(grid.drifts)-1].drift))
	}

	return converged
}

func regionCenter(region uint8, regionCount int) (float64, float64) {
	if region == 0 {
		return 0, 0
	}

	columns := DisplayColumns
	rows := DisplayRows

	if regionCount > 0 && regionCount < TargetRegionCount {
		columns = int(math.Ceil(math.Sqrt(float64(regionCount))))
		rows = int(math.Ceil(float64(regionCount) / float64(columns)))
	}

	index := int(region - 1)
	col := index % columns
	row := index / columns

	cellWidth := 1.0 / float64(columns)
	cellHeight := 1.0 / float64(rows)

	centerX := (float64(col) + 0.5) * cellWidth
	centerY := (float64(row) + 0.5) * cellHeight
	return centerX, centerY
}

/*
Excitation is one channel of one lighting pass: its deformation (see
Stream.Deform) and the Confidence of the Measurement that observed it (see
data.Measurement.Confidence).
*/
type Excitation struct {
	Deformation float64
	Confidence  float64
}

/*
RegionScore is the brightness of one region in one lighting pass.
*/
type RegionScore struct {
	Region       uint8   `json:"region"`
	Score        float64 `json:"score"`
	Contributors int     `json:"contributors"`
	Members      int     `json:"members"`
	Coverage     float64 `json:"coverage"`
}

type regionAggregate struct {
	evidence      float64
	observedCount int
}

/*
RegionScores ranks the regions lit by one pass, brightest first. It never
takes the update lock.

Every contributing cell is standardized against its own noise floor (see
Cell.standardize) and the result is dampened by the confidence of the
observation it came from, so a deteriorating Measurement dims all of its
Metrics together. A region's brightness is the Stouffer combination of its
contributors' dampened evidence, sum / sqrt(contributors): under the noise
floor every region then has the same unit spread whatever its size. A plain
mean would not: averaging n contributors shrinks its spread by sqrt(n), so a
one- or two-cell region would outshine a large region on noise alone, while a
plain sum would do the opposite.

Channels without a region (never observed, or arrived after the partition)
and cells without a noise floor do not contribute.
*/
func (grid *Grid) RegionScores(pass map[string]Excitation) []RegionScore {
	if grid == nil || len(pass) == 0 {
		return nil
	}

	regions := make(map[uint8]regionAggregate)

	for channel, excitation := range pass {
		cell, ok := grid.cellsLF.Get(channel)

		if !ok || cell == nil || cell.Region == 0 {
			continue
		}

		evidence, defined := cell.standardize(math.Abs(excitation.Deformation))

		if !defined {
			continue
		}

		entry := regions[cell.Region]
		entry.evidence += excitation.Confidence * evidence
		entry.observedCount++
		regions[cell.Region] = entry
	}

	scores := make([]RegionScore, 0, len(regions))

	for region, aggregate := range regions {
		members := max(grid.RegionMembers[region], aggregate.observedCount)

		scores = append(scores, RegionScore{
			Region:       region,
			Score:        aggregate.evidence / math.Sqrt(float64(aggregate.observedCount)),
			Contributors: aggregate.observedCount,
			Members:      members,
			Coverage:     float64(aggregate.observedCount) / float64(members),
		})
	}

	slices.SortStableFunc(scores, func(left, right RegionScore) int {
		if order := cmp.Compare(right.Score, left.Score); order != 0 {
			return order
		}

		return cmp.Compare(left.Region, right.Region)
	})

	return scores
}

/*
LitRegion answers the one-hot region token of a pass: the brightest region,
provided it stands above its noise floor. A pass whose regions all read at or
below their noise floor lights nothing. One region per pass keeps the token
alphabet at the region count; tokens built from the N brightest regions would
grow it combinatorially instead of reducing the grid's dimensionality.
*/
func (grid *Grid) LitRegion(pass map[string]Excitation) []byte {
	scores := grid.RegionScores(pass)

	if len(scores) == 0 || scores[0].Score <= 0 {
		return nil
	}

	return []byte{scores[0].Region}
}

/*
deform is the signed, scale-free magnitude movement between two raw values:
|(|current| - |previous|)| / (|previous| + |current|), signed by direction.
Unchanged values and equal-magnitude reversals produce 0.
*/
func deform(previous, current float64) float64 {
	extent := math.Abs(previous) + math.Abs(current)

	if current == previous || extent == 0 {
		return 0
	}

	movement := math.Abs(math.Abs(current)-math.Abs(previous)) / extent

	if current < previous {
		return -movement
	}

	return movement
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func clamp(v, minVal, maxVal float64) float64 {
	if v < minVal {
		return minVal
	}

	if v > maxVal {
		return maxVal
	}

	return v
}

/*
GridSnapshot captures complete grid state for persistence.
*/
type GridSnapshot struct {
	Version       int                      `json:"version"`
	Settled       bool                     `json:"settled"`
	Observations  int64                    `json:"observations"`
	Cells         map[string]*Cell         `json:"cells"`
	Relations     map[uint64]*Relation     `json:"relations,omitempty"`
	RegionMembers map[uint8]int            `json:"region_members"`
	Dirty         bool                     `json:"dirty"`
	PendingTick   int64                    `json:"pending_tick"`
	Pending       map[uint32]pendingSample `json:"pending"`
}

/*
gridSnapshotVersion 5 keys cells by metric label alone, holds no previous raw
values (deformation is measured per stream by its owner, Stream), and carries
every cell's noise floor (Cell.Mean, Cell.M2). Versions 1 and 2 keyed cells by
symbol\x00source\x00metric, version 3 carried one previous raw value per
label shared across symbols, and version 4 had no noise floors to light
against; none maps onto this grid, so they are rejected rather than restored.
*/
const gridSnapshotVersion = 5

func (grid *Grid) Snapshot() ([]byte, error) {
	if grid == nil {
		return nil, fmt.Errorf("grid: snapshot nil grid")
	}

	snapshot := GridSnapshot{
		Version:       gridSnapshotVersion,
		Settled:       grid.settled.Load() || grid.Settled,
		Observations:  grid.observations.Load(),
		Cells:         make(map[string]*Cell),
		Relations:     make(map[uint64]*Relation),
		RegionMembers: make(map[uint8]int, len(grid.RegionMembers)),
		Dirty:         grid.dirty,
		PendingTick:   grid.pendingTick,
		Pending:       make(map[uint32]pendingSample, len(grid.pending)),
	}

	if grid.Observations > snapshot.Observations {
		snapshot.Observations = grid.Observations
	}

	grid.cellsLF.Range("", "\xff", func(key string, cell *Cell) {
		if cell != nil {
			copyCell := *cell
			snapshot.Cells[key] = &copyCell
		}
	})

	for key, cell := range grid.Cells {
		if _, ok := snapshot.Cells[key]; !ok && cell != nil {
			copyCell := *cell
			snapshot.Cells[key] = &copyCell
		}
	}

	n := len(grid.idToCell)

	for right := 1; right < n; right++ {
		for left := 0; left < right; left++ {
			idx := symIdx(left, right)

			if idx >= len(grid.pairs) {
				continue
			}

			p := &grid.pairs[idx]

			if p.Total == 0 && p.Weight == 0 {
				continue
			}

			key := pairKey(uint32(left), uint32(right))
			snapshot.Relations[key] = &Relation{
				Weight:   p.Weight,
				SumX:     p.SumX,
				SumY:     p.SumY,
				SumXX:    p.SumXX,
				SumYY:    p.SumYY,
				SumXY:    p.SumXY,
				Same:     int64(p.Same),
				Opposite: int64(p.Opposite),
				Total:    int64(p.Total),
			}
		}
	}

	maps.Copy(snapshot.RegionMembers, grid.RegionMembers)
	maps.Copy(snapshot.Pending, grid.pending)

	encoded, err := json.Marshal(snapshot)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"grid: encode snapshot",
			err,
		))
	}

	return encoded, nil
}

func (grid *Grid) RestoreSnapshot(encoded []byte) error {
	if grid == nil {
		return errnie.Error(fmt.Errorf("grid: restore into nil grid"))
	}

	var snapshot GridSnapshot

	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"grid: decode snapshot",
			err,
		))
	}

	if snapshot.Version != gridSnapshotVersion {
		return grid.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"grid: unsupported snapshot version %d (want %d; older snapshots key cells by symbol/source or share raw values across symbols and must be deleted)",
				snapshot.Version, gridSnapshotVersion,
			),
			nil,
		))
	}

	if snapshot.Cells == nil {
		snapshot.Cells = make(map[string]*Cell)
	}

	if snapshot.Relations == nil {
		snapshot.Relations = make(map[uint64]*Relation)
	}

	if snapshot.Pending == nil {
		snapshot.Pending = make(map[uint32]pendingSample)
	}

	cellCount := len(snapshot.Cells)
	idToCell := make([]*Cell, cellCount)
	cellIDs := make(map[string]uint32, cellCount)

	for key, cell := range snapshot.Cells {
		if cell == nil {
			return grid.Error(fmt.Errorf("grid: snapshot cell %q is null", key))
		}

		if cell.Key == "" {
			cell.Key = key
		}

		if cell.Key != key || strings.Contains(key, "\x00") {
			return grid.Error(fmt.Errorf("grid: snapshot cell %q is not a metric-label key", key))
		}

		if int(cell.ID) >= cellCount {
			return grid.Error(fmt.Errorf("grid: snapshot cell %q ID %d out of range (cell count %d)", key, cell.ID, cellCount))
		}

		if idToCell[cell.ID] != nil {
			return grid.Error(fmt.Errorf("grid: duplicate cell ID %d in snapshot", cell.ID))
		}

		idToCell[cell.ID] = cell
		cellIDs[key] = cell.ID
	}

	for id, cell := range idToCell {
		if cell == nil {
			return grid.Error(fmt.Errorf("grid: gap in snapshot cell IDs at %d", id))
		}
	}

	regionMembers := make(map[uint8]int)
	unassigned := false

	for _, cell := range idToCell {
		if cell.Region == 0 {
			unassigned = true
			continue
		}

		regionMembers[cell.Region]++
	}

	grid.updateMu.Lock()
	defer grid.updateMu.Unlock()

	grid.settled.Store(snapshot.Settled)
	grid.Settled = snapshot.Settled
	grid.observations.Store(snapshot.Observations)
	grid.Observations = snapshot.Observations
	grid.Cells = snapshot.Cells
	grid.RegionMembers = regionMembers
	grid.regionsFormed.Store(int32(len(regionMembers)))
	grid.cellIDs = cellIDs
	grid.idToCell = idToCell
	copied := make([]*Cell, len(idToCell))
	copy(copied, idToCell)
	grid.idToCellLF.Store(&copied)
	grid.dirty = snapshot.Dirty || (snapshot.Settled && unassigned)
	grid.pendingTick = snapshot.PendingTick
	grid.pending = snapshot.Pending

	for key, cell := range snapshot.Cells {
		grid.cellsLF.Set(key, cell)
	}

	grid.capacity = cellCount
	grid.pairs = make([]PairStats, grid.capacity*(grid.capacity+1)/2)

	for key, rel := range snapshot.Relations {
		if rel == nil {
			continue
		}

		left, right := pairIDs(key)
		maxID := int(max(left, right))

		if maxID >= grid.capacity {
			grid.growPairs(maxID + 1)
		}

		idx := symIdx(int(left), int(right))
		grid.pairs[idx] = PairStats{
			Weight:   rel.Weight,
			SumX:     rel.SumX,
			SumY:     rel.SumY,
			SumXX:    rel.SumXX,
			SumYY:    rel.SumYY,
			SumXY:    rel.SumXY,
			Same:     int32(rel.Same),
			Opposite: int32(rel.Opposite),
			Total:    int32(rel.Total),
		}
	}

	return nil
}
