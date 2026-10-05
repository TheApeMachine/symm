package store

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
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

	// ConvergenceStreak defines the number of consecutive passes with identical
	// metric discovery and region assignments required to consider the grid converged.
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
Cell is one learned vertex in the metric-affinity graph.
*/
type Cell struct {
	ID          uint32  `json:"id"`
	Key         string  `json:"key"`
	Region      uint8   `json:"region"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Visits      uint64  `json:"visits"`
	Last        float64 `json:"last"`
	Initialized bool    `json:"initialized"`
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
	updateMu     sync.Mutex
	settled      atomic.Bool
	observations atomic.Int64
	cellsLF      *lf.OrderedMap[string, *Cell]
	idToCellLF   atomic.Pointer[[]*Cell]

	Settled       bool                 `json:"settled"`
	Observations  int64                `json:"observations"`
	Cells         map[string]*Cell     `json:"cells"`
	Relations     map[uint64]*Relation `json:"relations,omitempty"`
	RegionMembers map[uint8]int        `json:"region_members"`

	cellIDs  map[string]uint32
	idToCell []*Cell
	prevRaw  map[string]float64
	dirty    bool

	pairs    []PairStats
	capacity int

	pendingTick    int64
	pending        map[uint32]pendingSample
	stablePasses   int
	prevCellCount  int
	prevPartitions map[string]uint8
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
		Relations:      make(map[uint64]*Relation),
		RegionMembers:  make(map[uint8]int),
		cellIDs:        make(map[string]uint32),
		idToCell:       idToCell,
		prevRaw:        make(map[string]float64),
		pending:        make(map[uint32]pendingSample),
		prevPartitions: make(map[string]uint8),
	}

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

func CellKey(symbol, source, name string) string {
	return symbol + "\x00" + source + "\x00" + name
}

func (grid *Grid) IsSettled() bool {
	if grid == nil {
		return false
	}

	return grid.settled.Load() || grid.Settled
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

func (grid *Grid) CellAt(symbol, source, name string) *Cell {
	if grid == nil {
		return nil
	}

	key := CellKey(symbol, source, name)

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

func (grid *Grid) Update(measurements ...*data.Measurement) {
	if grid == nil || len(measurements) == 0 {
		return
	}

	grid.updateMu.Lock()
	defer grid.updateMu.Unlock()

	roots := nonNilMeasurements(measurements)

	if len(roots) == 0 {
		return
	}

	arrivals := grid.extractTrainingArrivalsLocked(roots)

	if len(arrivals) == 0 {
		grid.decorateLocked(roots)
		return
	}

	if len(roots) > 1 {
		grid.flushPendingLocked()
		grid.commitPassLocked(arrivals)
		grid.decorateLocked(roots)
		return
	}

	if roots[0].Tick > 0 {
		tick := roots[0].Tick

		if grid.pendingTick != 0 && grid.pendingTick != tick {
			grid.flushPendingLocked()
		}

		grid.pendingTick = tick
		grid.mergePendingLocked(arrivals)
		grid.decorateLocked(roots)
		return
	}

	grid.flushPendingLocked()
	grid.commitPassLocked(arrivals)
	grid.decorateLocked(roots)
}

func nonNilMeasurements(measurements []*data.Measurement) []*data.Measurement {
	filtered := make([]*data.Measurement, 0, len(measurements))

	for _, meas := range measurements {
		if meas != nil {
			filtered = append(filtered, meas)
		}
	}

	return filtered
}

func walkMeasurements(
	roots []*data.Measurement,
	fn func(*data.Measurement),
) {
	seen := make(map[*data.Measurement]struct{})
	var visit func(*data.Measurement)

	visit = func(measurement *data.Measurement) {
		if measurement == nil {
			return
		}

		if _, ok := seen[measurement]; ok {
			return
		}

		seen[measurement] = struct{}{}
		fn(measurement)

		for _, peer := range measurement.Peers {
			visit(peer)
		}
	}

	for _, root := range roots {
		visit(root)
	}
}

func measurementChannel(measurement *data.Measurement, key string, metric data.Metric) string {
	symbol := measurement.Label

	if symbol == "" {
		symbol = measurement.GetSource()
	}

	source := measurement.GetSource()
	name := metric.Label

	if name == "" {
		name = key
	}

	return CellKey(symbol, source, name)
}

func (grid *Grid) ensureCellLocked(key string, raw float64) *Cell {
	if cell, ok := grid.cellsLF.Get(key); ok && cell != nil {
		return cell
	}

	id := uint32(len(grid.idToCell))

	if int(id) >= grid.capacity {
		grid.growPairs(int(id) + 1)
	}

	cell := &Cell{
		ID:   id,
		Key:  key,
		Last: raw,
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

func (grid *Grid) extractTrainingArrivalsLocked(
	roots []*data.Measurement,
) []metricArrival {
	totalMetrics := 0
	walkMeasurements(roots, func(measurement *data.Measurement) {
		if measurement != nil {
			totalMetrics += len(measurement.Metrics)
		}
	})

	arrivals := make([]metricArrival, 0, totalMetrics)

	walkMeasurements(roots, func(measurement *data.Measurement) {
		quality := measurementDamp(measurement)

		measurement.RangeMetrics(func(key string, metric data.Metric) bool {
			channel := measurementChannel(measurement, key, metric)
			cell := grid.ensureCellLocked(channel, metric.Raw)
			cell.Visits++

			raw := metric.Raw
			deformation := 0.0
			validDeformation := false

			if metric.Deformation != nil && finite(*metric.Deformation) {
				deformation = *metric.Deformation
				validDeformation = true
			}

			if !validDeformation {
				if previous, ok := grid.prevRaw[channel]; ok && finite(previous) && finite(raw) {
					deformation = data.Deformation(previous, raw)
					validDeformation = finite(deformation)
				}
			}

			if finite(raw) {
				grid.prevRaw[channel] = raw
				cell.Last = raw
				cell.Initialized = true
			}

			if validDeformation && quality > 0 {
				arrivals = append(arrivals, metricArrival{
					id:      cell.ID,
					value:   deformation,
					quality: quality,
				})
			}

			return true
		})
	})

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

	ids := make([]int, 0, len(collapsed))
	values := make(map[uint32]float64, len(collapsed))
	qualities := make(map[uint32]float64, len(collapsed))

	for id, sample := range collapsed {
		value, quality, ok := sample.value()

		if !ok {
			continue
		}

		ids = append(ids, int(id))
		values[id] = value
		qualities[id] = quality
	}

	slices.Sort(ids)

	for first := 0; first < len(ids); first++ {
		leftID := ids[first]

		for second := first + 1; second < len(ids); second++ {
			rightID := ids[second]
			weight := math.Sqrt(qualities[uint32(leftID)] * qualities[uint32(rightID)])

			if weight <= 0 || !finite(weight) {
				continue
			}

			idx := symIdx(leftID, rightID)

			if idx >= len(grid.pairs) {
				grid.growPairs(max(leftID, rightID) + 1)
			}

			grid.pairs[idx].update(values[uint32(leftID)], values[uint32(rightID)], weight)

			pairHash := pairKey(uint32(leftID), uint32(rightID))
			relation := grid.Relations[pairHash]

			if relation == nil {
				relation = &Relation{}
				grid.Relations[pairHash] = relation
			}

			relation.update(values[uint32(leftID)], values[uint32(rightID)], weight)

			if !grid.IsSettled() {
				grid.dirty = true
			}
		}
	}

	grid.Observations++
	grid.observations.Add(1)
}

/*
Partition discovers and updates regions for all currently observed metrics without freezing the grid.
*/
func (grid *Grid) Partition() {
	if grid == nil {
		return
	}

	grid.updateMu.Lock()
	defer grid.updateMu.Unlock()

	grid.flushPendingLocked()

	cellCount := len(grid.Cells)
	errnie.Info(fmt.Sprintf("[grid] Partition() started: %d cells observed", cellCount))

	partitions := grid.computePartitionsLocked()

	if len(partitions) == 0 {
		errnie.Info("[grid] Partition() computed 0 partitions")
		return
	}

	grid.commitPartitionsLocked(partitions)
	errnie.Info(fmt.Sprintf("[grid] Partition() committed: %d regions across %d cells (stable passes: %d/%d)", len(grid.RegionMembers), cellCount, grid.stablePasses, ConvergenceStreak))
}

/*
Settle freezes the current graph into balanced regions using vectorized Gonum eigensolvers.
*/
func (grid *Grid) Settle() {
	if grid == nil {
		return
	}

	errnie.Info(fmt.Sprintf("[grid] Settle() requested: %d cells", len(grid.Cells)))
	grid.Partition()

	grid.updateMu.Lock()
	defer grid.updateMu.Unlock()

	grid.settled.Store(true)
	grid.Settled = true
	grid.dirty = false
	errnie.Info(fmt.Sprintf("[grid] Settle() complete: grid settled with %d regions across %d cells", len(grid.RegionMembers), len(grid.Cells)))
}

func (grid *Grid) computePartitionsLocked() map[string]uint8 {
	n := len(grid.Cells)

	if n == 0 {
		return nil
	}

	keys := make([]string, 0, n)

	for key := range grid.Cells {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	k := targetPartitionCount(n)
	errnie.Info(fmt.Sprintf("[grid] computePartitionsLocked: n=%d metrics, target clusters k=%d", n, k))

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

	affinity := grid.affinityMatrixLocked(keys)
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

	maxWithoutSingletons := metricCount / MinRegionSize

	if maxWithoutSingletons < 1 {
		maxWithoutSingletons = 1
	}

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

func (grid *Grid) commitPartitionsLocked(partitions map[string]uint8) {
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

	for _, cell := range grid.Cells {
		if cell == nil || cell.Region == 0 {
			continue
		}

		cell.X, cell.Y = regionCenter(cell.Region, regionCount)
	}

	currentCount := len(grid.Cells)

	if currentCount > 0 && currentCount == grid.prevCellCount && len(grid.prevPartitions) == currentCount {
		changed := false

		for key, region := range partitions {
			if grid.prevPartitions[key] != region {
				changed = true
				break
			}
		}

		if !changed {
			grid.stablePasses++
		}

		if changed {
			grid.stablePasses = 0
		}
	}

	if currentCount != grid.prevCellCount || len(grid.prevPartitions) != currentCount {
		grid.stablePasses = 0
	}

	grid.prevCellCount = currentCount
	grid.prevPartitions = partitions
}

/*
Converged reports whether the metric partition has stabilized across consecutive passes.
*/
func (grid *Grid) Converged() bool {
	if grid == nil {
		return false
	}

	metricCount := len(grid.Cells)

	if metricCount < 4 {
		return false
	}

	if len(grid.RegionMembers) < 2 {
		return false
	}

	converged := grid.stablePasses >= ConvergenceStreak

	if converged {
		errnie.Info(fmt.Sprintf("[grid] Converged! %d metrics stabilized across %d regions (streak: %d)", metricCount, len(grid.RegionMembers), grid.stablePasses))
	}

	return converged
}

/*
Decorate stamps measurements with their assigned Region and display coordinates (X, Y).
Wait-free execution without locking.
*/
func (grid *Grid) Decorate(measurements ...*data.Measurement) {
	if grid == nil || len(measurements) == 0 {
		return
	}

	grid.decorateLocked(nonNilMeasurements(measurements))
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
RegionScore aggregates normalized activity within one partition.
*/
type RegionScore struct {
	Region       uint8   `json:"region"`
	Score        float64 `json:"score"`
	Contributors int     `json:"contributors"`
	Members      int     `json:"members"`
	Coverage     float64 `json:"coverage"`
}

type regionAggregate struct {
	sumDeformation float64
	observedCount  int
	totalMembers   int
}

/*
RegionScores evaluates mean activity within each region.
Wait-free execution without locking.
*/
func (grid *Grid) RegionScores(measurements ...*data.Measurement) []RegionScore {
	if grid == nil || len(measurements) == 0 {
		return nil
	}

	return grid.regionScoresLocked(nonNilMeasurements(measurements))
}

func (grid *Grid) regionScoresLocked(roots []*data.Measurement) []RegionScore {
	type cellAggregate struct {
		sumDeformation float64
		count          int
	}

	perCell := make(map[uint32]cellAggregate)

	walkMeasurements(roots, func(measurement *data.Measurement) {
		quality := measurementDamp(measurement)

		if quality <= 0 {
			return
		}

		measurement.RangeMetrics(func(key string, metric data.Metric) bool {
			cell := grid.lookupCellLocked(measurement, key, metric)

			if cell == nil || cell.Region == 0 {
				return true
			}

			deformation := 0.0

			if metric.Deformation != nil && finite(*metric.Deformation) {
				deformation = *metric.Deformation
			}

			if metric.Deformation == nil || !finite(*metric.Deformation) {
				channel := measurementChannel(measurement, key, metric)
				previous, ok := grid.prevRaw[channel]

				if ok && finite(previous) && finite(metric.Raw) {
					deformation = data.Deformation(previous, metric.Raw)
				}
			}

			if !finite(deformation) {
				return true
			}

			agg := perCell[cell.ID]
			agg.sumDeformation += math.Abs(deformation) * quality
			agg.count++
			perCell[cell.ID] = agg
			return true
		})
	})

	if len(perCell) == 0 {
		return nil
	}

	regions := make(map[uint8]regionAggregate)
	idToCellPtr := grid.idToCellLF.Load()

	if idToCellPtr == nil {
		return nil
	}

	idToCell := *idToCellPtr

	for id, aggregate := range perCell {
		if aggregate.count <= 0 || int(id) >= len(idToCell) {
			continue
		}

		cell := idToCell[id]

		if cell == nil || cell.Region == 0 {
			continue
		}

		entry := regions[cell.Region]
		entry.sumDeformation += aggregate.sumDeformation / float64(aggregate.count)
		entry.observedCount++
		regions[cell.Region] = entry
	}

	if len(regions) == 0 {
		return nil
	}

	scores := make([]RegionScore, 0, len(regions))

	for region, aggregate := range regions {
		total := grid.RegionMembers[region]

		if total < aggregate.observedCount {
			total = aggregate.observedCount
		}

		if total <= 0 {
			continue
		}

		score := aggregate.sumDeformation / float64(aggregate.observedCount)
		coverage := float64(aggregate.observedCount) / float64(total)

		if score <= 0 || !finite(score) {
			continue
		}

		scores = append(scores, RegionScore{
			Region:       region,
			Score:        score,
			Contributors: aggregate.observedCount,
			Members:      total,
			Coverage:     coverage,
		})
	}

	sort.SliceStable(scores, func(firstIndex, secondIndex int) bool {
		if scores[firstIndex].Score == scores[secondIndex].Score {
			return scores[firstIndex].Region < scores[secondIndex].Region
		}

		return scores[firstIndex].Score > scores[secondIndex].Score
	})

	return scores
}

/*
LitRegions answers the token sequence of active regions above noise.
LitRegions keeps the existing token API deliberately conservative: it returns
the strongest region from the same fair RegionScores pass. Call RegionScores
when the UI or diagnostics need the complete ranked set and actual intensities.
*/
func (grid *Grid) LitRegions(measurements ...*data.Measurement) [][]byte {
	if grid == nil || len(measurements) == 0 {
		return nil
	}

	scores := grid.regionScoresLocked(nonNilMeasurements(measurements))

	if len(scores) == 0 || scores[0].Score <= 0 || scores[0].Region == 0 {
		return nil
	}

	return [][]byte{{scores[0].Region}}
}

func (grid *Grid) lookupCellLocked(
	measurement *data.Measurement,
	key string,
	metric data.Metric,
) *Cell {
	channel := measurementChannel(measurement, key, metric)

	if cell, ok := grid.cellsLF.Get(channel); ok && cell != nil {
		return cell
	}

	if cell := grid.Cells[channel]; cell != nil {
		grid.cellsLF.Set(channel, cell)
		return cell
	}

	if metric.Label != "" {
		if cell, ok := grid.cellsLF.Get(metric.Label); ok && cell != nil {
			return cell
		}

		if cell := grid.Cells[metric.Label]; cell != nil {
			grid.cellsLF.Set(metric.Label, cell)
			return cell
		}
	}

	if cell, ok := grid.cellsLF.Get(key); ok && cell != nil {
		return cell
	}

	if cell := grid.Cells[key]; cell != nil {
		grid.cellsLF.Set(key, cell)
		return cell
	}

	return nil
}

func (grid *Grid) decorateLocked(roots []*data.Measurement) {
	walkMeasurements(roots, func(measurement *data.Measurement) {
		measurement.RangeMetrics(func(key string, metric data.Metric) bool {
			cell := grid.lookupCellLocked(measurement, key, metric)

			if cell == nil || cell.Region == 0 {
				return true
			}

			metric.X = int64(math.Round(cell.X * 100))
			metric.Y = int64(math.Round(cell.Y * 100))
			metric.Region = cell.Region
			measurement.SetMetric(key, metric)
			return true
		})
	})
}

func measurementDamp(measurement *data.Measurement) float64 {
	if measurement == nil {
		return 0
	}

	maturity := measurement.Maturity

	if !finite(maturity) {
		return 0
	}

	maturity = clamp(maturity, 0, 1)

	if maturity == 0 && !measurement.Estimated {
		maturity = 1
	}

	snrFactor := 1.0

	if measurement.SNRDefined {
		if !finite(measurement.SNR) || measurement.SNR <= 0 {
			return 0
		}

		snrFactor = measurement.SNR / (1.0 + measurement.SNR)
	}

	return clamp(maturity*snrFactor, 0, 1)
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
	PrevRaw       map[string]float64       `json:"prev_raw"`
	Dirty         bool                     `json:"dirty"`
	PendingTick   int64                    `json:"pending_tick"`
	Pending       map[uint32]pendingSample `json:"pending"`
}

const gridSnapshotVersion = 2

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
		PrevRaw:       make(map[string]float64, len(grid.prevRaw)),
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

	for region, count := range grid.RegionMembers {
		snapshot.RegionMembers[region] = count
	}

	for key, val := range grid.prevRaw {
		snapshot.PrevRaw[key] = val
	}

	for id, sample := range grid.pending {
		snapshot.Pending[id] = sample
	}

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
		return fmt.Errorf("grid: restore into nil grid")
	}

	var snapshot GridSnapshot

	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"grid: decode snapshot",
			err,
		))
	}

	if snapshot.Version > gridSnapshotVersion {
		return fmt.Errorf("grid: unsupported snapshot version %d", snapshot.Version)
	}

	if snapshot.Cells == nil {
		snapshot.Cells = make(map[string]*Cell)
	}

	if snapshot.Relations == nil {
		snapshot.Relations = make(map[uint64]*Relation)
	}

	if snapshot.PrevRaw == nil {
		snapshot.PrevRaw = make(map[string]float64)
	}

	if snapshot.Pending == nil {
		snapshot.Pending = make(map[uint32]pendingSample)
	}

	cellCount := len(snapshot.Cells)
	idToCell := make([]*Cell, cellCount)
	cellIDs := make(map[string]uint32, cellCount)

	for key, cell := range snapshot.Cells {
		if cell == nil {
			return fmt.Errorf("grid: snapshot cell %q is null", key)
		}

		if cell.Key == "" {
			cell.Key = key
		}

		if int(cell.ID) >= cellCount {
			return fmt.Errorf("grid: snapshot cell %q ID %d out of range (cell count %d)", key, cell.ID, cellCount)
		}

		if idToCell[cell.ID] != nil {
			return fmt.Errorf("grid: duplicate cell ID %d in snapshot", cell.ID)
		}

		idToCell[cell.ID] = cell
		cellIDs[key] = cell.ID
	}

	for id, cell := range idToCell {
		if cell == nil {
			return fmt.Errorf("grid: gap in snapshot cell IDs at %d", id)
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
	grid.cellIDs = cellIDs
	grid.idToCell = idToCell
	copied := make([]*Cell, len(idToCell))
	copy(copied, idToCell)
	grid.idToCellLF.Store(&copied)
	grid.prevRaw = snapshot.PrevRaw
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
