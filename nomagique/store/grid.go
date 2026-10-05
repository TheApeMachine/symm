package store

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"gonum.org/v1/gonum/mat"
)

const (
	// LitRegionTokenSize is kept as a one-byte region identifier for the
	// existing cognition/token pipeline.
	LitRegionTokenSize = 1

	// The learned partition is laid out as a 5x4 display grid. The layout is
	// deliberately separate from partition learning: graph structure decides
	// membership, these constants only decide where the 20 learned regions are
	// drawn.
	GridBinsX      = 5
	GridBinsY      = 4
	TotalGridCells = GridBinsX * GridBinsY
	TotalGridEdges = TotalGridCells * (TotalGridCells - 1) / 2

	TargetRegionCount = TotalGridCells
	MinRegionSize     = 2

	// Pairwise correlation is shrunk toward zero when only a few co-observations
	// exist. With 8 pseudo-observations, a pair seen 8 times contributes at
	// half strength and a pair seen 40 times contributes at 5/6 strength.
	CorrelationShrinkage = 8.0

	// Kept for source compatibility with the previous Grid API. Settlement is
	// now explicit graph partitioning rather than physical-position convergence.
	ConvergenceTolerance = 1e-3
	ConvergenceStreak    = 5
)

/*
Cell is one learned metric channel.

Key is identity-independent of map iteration order and is built from
(symbol, source, metric-name). Region is assigned only by Settle. X/Y are the
center of the region's display cell in [-1, 1] x [-1, 1].
*/
type Cell struct {
	ID          uint32  `json:"id"`
	Key         string  `json:"key"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Region      uint8   `json:"region"`
	Last        float64 `json:"last"`
	Visits      int64   `json:"visits"`
	Initialized bool    `json:"initialized"`
}

/*
Relation stores sufficient statistics for a weighted Pearson correlation
between two metric deformations. Same/Opposite/Total are retained for
compatibility and diagnostics; partitioning uses the correlation when it is
well-defined and falls back to directional agreement otherwise.
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

func (relation *Relation) update(x, y, weight float64) {
	if relation == nil || weight <= 0 || !finite(x) || !finite(y) || !finite(weight) {
		return
	}

	relation.Total++
	if x*y > 0 {
		relation.Same++
	} else if x*y < 0 {
		relation.Opposite++
	}

	relation.Weight += weight
	relation.SumX += weight * x
	relation.SumY += weight * y
	relation.SumXX += weight * x * x
	relation.SumYY += weight * y * y
	relation.SumXY += weight * x * y
}

func (relation *Relation) correlation() (float64, bool) {
	if relation == nil || relation.Weight <= 0 {
		return 0, false
	}

	w := relation.Weight
	cov := w*relation.SumXY - relation.SumX*relation.SumY
	varX := w*relation.SumXX - relation.SumX*relation.SumX
	varY := w*relation.SumYY - relation.SumY*relation.SumY

	// Roundoff can produce tiny negative variances around zero.
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

func (relation *Relation) directionalAgreement() float64 {
	if relation == nil || relation.Total == 0 {
		return 0
	}
	return float64(relation.Same-relation.Opposite) / float64(relation.Total)
}

func (relation *Relation) affinity() float64 {
	if relation == nil {
		return 0
	}

	strength := 0.0
	evidence := relation.Weight

	if corr, ok := relation.correlation(); ok {
		strength = corr
	} else {
		strength = relation.directionalAgreement()
		if evidence <= 0 {
			evidence = float64(relation.Total)
		}
	}

	// Anti-correlated metrics are deliberately not joined by a positive edge.
	// The graph partitioner therefore keeps "moves together" as the region
	// semantics rather than mixing a mode with its inverse.
	if strength <= 0 || evidence <= 0 {
		return 0
	}

	reliability := evidence / (evidence + CorrelationShrinkage)
	return strength * reliability
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
RegionScore is the ranked activation of one learned region for one evaluation
pass. Score is the mean damped activity of the unique metric channels from
that pass that landed in the region, so five supplied metrics do not beat two
supplied metrics merely because there are more of them.
*/
type RegionScore struct {
	Region       uint8   `json:"region"`
	Score        float64 `json:"score"`
	Contributors int     `json:"contributors"`
	Members      int     `json:"members"`
	Coverage     float64 `json:"coverage"`
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
Grid learns a weighted metric-affinity graph and partitions it into roughly 20
balanced regions.

Learning:
  - each metric channel is a vertex;
  - each co-observed pair gets a weighted Pearson correlation edge over metric
    deformation;
  - Settle performs recursively balanced spectral bisection, then exact-size
    swap refinement that increases within-region positive affinity.

Evaluation:
  - all supplied Measurements form one pass;
  - one Maturity/SNR attenuation is computed per Measurement and applied to
    every metric owned by that Measurement;
  - region activity is a mean, never a sum, so region comparison is not biased
    by how many supplied metrics happen to land there.
*/
type Grid struct {
	mu sync.RWMutex

	Settled       bool                 `json:"settled"`
	Observations  int64                `json:"observations"`
	Cells         map[string]*Cell     `json:"cells"`
	Relations     map[uint64]*Relation `json:"relations"`
	RegionMembers map[uint8]int        `json:"region_members"`

	cellIDs  map[string]uint32
	idToCell []*Cell
	prevRaw  map[string]float64
	dirty    bool

	// When historical measurements are streamed one-at-a-time with the same
	// non-zero Tick, hold their metric samples until the next Tick so they are
	// learned as one cross-measurement pass. Explicit multi-measurement Update
	// calls are already a complete pass and are committed immediately.
	pendingTick int64
	pending     map[uint32]pendingSample
}

func NewGrid() *Grid {
	return &Grid{
		Cells:         make(map[string]*Cell),
		Relations:     make(map[uint64]*Relation),
		RegionMembers: make(map[uint8]int),
		cellIDs:       make(map[string]uint32),
		idToCell:      make([]*Cell, 0, 512),
		prevRaw:       make(map[string]float64),
		pending:       make(map[uint32]pendingSample),
	}
}

func CellKey(symbol, source, name string) string {
	return symbol + "\x00" + source + "\x00" + name
}

func (grid *Grid) IsSettled() bool {
	if grid == nil {
		return false
	}
	grid.mu.RLock()
	defer grid.mu.RUnlock()
	return grid.Settled
}

func (grid *Grid) Region(key string) uint8 {
	if grid == nil {
		return 0
	}
	grid.mu.RLock()
	defer grid.mu.RUnlock()
	if cell := grid.Cells[key]; cell != nil {
		return cell.Region
	}
	return 0
}

// RegionAt returns the learned region whose display-cell center is nearest to
// (x, y). It is retained for source compatibility with the previous grid.
func (grid *Grid) RegionAt(x, y float64) uint8 {
	if grid == nil {
		return 0
	}
	grid.mu.RLock()
	defer grid.mu.RUnlock()

	bestRegion := uint8(0)
	bestDistance := math.Inf(1)
	seen := make(map[uint8]struct{}, len(grid.RegionMembers))

	for _, cell := range grid.idToCell {
		if cell == nil || cell.Region == 0 {
			continue
		}
		if _, ok := seen[cell.Region]; ok {
			continue
		}
		seen[cell.Region] = struct{}{}

		d := math.Hypot(cell.X-x, cell.Y-y)
		if d < bestDistance || (d == bestDistance && cell.Region < bestRegion) {
			bestDistance = d
			bestRegion = cell.Region
		}
	}

	return bestRegion
}

// CellAt returns a copy of the metric cell nearest to the supplied display
// coordinates. Returning a copy avoids exposing mutable grid state outside
// the Grid mutex.
func (grid *Grid) CellAt(x, y float64) *Cell {
	if grid == nil {
		return nil
	}
	grid.mu.RLock()
	defer grid.mu.RUnlock()

	var best *Cell
	bestDistance := math.Inf(1)

	for _, cell := range grid.idToCell {
		if cell == nil {
			continue
		}
		d := math.Hypot(cell.X-x, cell.Y-y)
		if d < bestDistance || (d == bestDistance && (best == nil || cell.ID < best.ID)) {
			bestDistance = d
			best = cell
		}
	}

	if best == nil {
		return nil
	}
	copyCell := *best
	return &copyCell
}

type metricArrival struct {
	id      uint32
	value   float64
	quality float64
}

/*
Update learns metric-to-metric affinity. If several Measurements are supplied,
they are one pass immediately. If historical storage streams one Measurement
at a time and Tick is non-zero, measurements sharing a Tick are buffered and
committed together when the next Tick arrives (or on Settle).
*/
func (grid *Grid) Update(measurements ...*data.Measurement[float64]) {
	if grid == nil || len(measurements) == 0 {
		return
	}

	grid.mu.Lock()
	defer grid.mu.Unlock()

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
	} else if roots[0].Tick > 0 {
		tick := roots[0].Tick
		if grid.pendingTick != 0 && grid.pendingTick != tick {
			grid.flushPendingLocked()
		}
		grid.pendingTick = tick
		grid.mergePendingLocked(arrivals)
	} else {
		grid.flushPendingLocked()
		grid.commitPassLocked(arrivals)
	}

	grid.decorateLocked(roots)
}

func nonNilMeasurements(in []*data.Measurement[float64]) []*data.Measurement[float64] {
	out := make([]*data.Measurement[float64], 0, len(in))
	for _, measurement := range in {
		if measurement != nil {
			out = append(out, measurement)
		}
	}
	return out
}

func walkMeasurements(
	roots []*data.Measurement[float64],
	fn func(*data.Measurement[float64]),
) {
	seen := make(map[*data.Measurement[float64]]struct{}, len(roots)*2)
	var visit func(*data.Measurement[float64])

	visit = func(measurement *data.Measurement[float64]) {
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

func measurementChannel(measurement *data.Measurement[float64], key string, metric data.Metric[float64]) string {
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
	if cell := grid.Cells[key]; cell != nil {
		return cell
	}

	id := uint32(len(grid.idToCell))
	cell := &Cell{
		ID:   id,
		Key:  key,
		Last: raw,
	}
	grid.Cells[key] = cell
	grid.cellIDs[key] = id
	grid.idToCell = append(grid.idToCell, cell)
	grid.dirty = true
	return cell
}

func (grid *Grid) extractTrainingArrivalsLocked(
	roots []*data.Measurement[float64],
) []metricArrival {
	arrivals := make([]metricArrival, 0, 128)

	walkMeasurements(roots, func(measurement *data.Measurement[float64]) {
		quality := measurementDamp(measurement)

		measurement.RangeMetrics(func(key string, metric data.Metric[float64]) bool {
			channel := measurementChannel(measurement, key, metric)
			cell := grid.ensureCellLocked(channel, metric.Raw)
			cell.Visits++

			raw := metric.Raw
			deformation := 0.0
			validDeformation := false

			if metric.Deformation != nil && finite(*metric.Deformation) {
				deformation = *metric.Deformation
				validDeformation = true
			} else if previous, ok := grid.prevRaw[channel]; ok && finite(previous) && finite(raw) {
				deformation = data.Deformation(previous, raw)
				validDeformation = finite(deformation)
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
	grid.commitPassLocked(arrivals)
}

func (grid *Grid) commitPassLocked(arrivals []metricArrival) {
	if len(arrivals) == 0 {
		return
	}

	// Collapse duplicate channels inside the same pass before updating pair
	// statistics. This keeps one channel from acquiring extra influence merely
	// because it appeared twice in the supplied measurement graph.
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
		leftID := uint32(ids[first])
		for second := first + 1; second < len(ids); second++ {
			rightID := uint32(ids[second])
			weight := math.Sqrt(qualities[leftID] * qualities[rightID])
			if weight <= 0 || !finite(weight) {
				continue
			}

			key := pairKey(leftID, rightID)
			relation := grid.Relations[key]
			if relation == nil {
				relation = &Relation{}
				grid.Relations[key] = relation
			}
			relation.update(values[leftID], values[rightID], weight)
			grid.dirty = true
		}
	}

	grid.Observations++
}

/*
Settle freezes the current graph into balanced regions. Calling Settle again
is cheap when nothing has changed and re-partitions when new learning has made
the graph dirty.
*/
func (grid *Grid) Settle() {
	if grid == nil {
		return
	}

	grid.mu.Lock()
	defer grid.mu.Unlock()

	grid.flushPendingLocked()
	if grid.Settled && !grid.dirty {
		return
	}

	partitions := grid.computePartitionsLocked()
	grid.commitPartitionsLocked(partitions)
	grid.Settled = true
	grid.dirty = false
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
	if k <= 1 {
		assigned := make(map[string]uint8, n)
		for _, key := range keys {
			assigned[key] = 1
		}
		return assigned
	}

	affinity := grid.affinityMatrixLocked(keys)
	capacities := balancedCapacities(n, k)

	vertices := make([]int, n)
	for i := range vertices {
		vertices[i] = i
	}

	groups := make([][]int, 0, k)
	var split func([]int, []int)
	split = func(group []int, caps []int) {
		if len(caps) <= 1 {
			copyGroup := append([]int(nil), group...)
			groups = append(groups, copyGroup)
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

	groups = refineBalancedGroups(groups, affinity, keys)
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

func (grid *Grid) affinityMatrixLocked(keys []string) [][]float64 {
	n := len(keys)
	matrix := make([][]float64, n)
	for i := range matrix {
		matrix[i] = make([]float64, n)
	}

	for right := 1; right < n; right++ {
		rightCell := grid.Cells[keys[right]]
		for left := 0; left < right; left++ {
			leftCell := grid.Cells[keys[left]]
			relation := grid.Relations[pairKey(leftCell.ID, rightCell.ID)]
			weight := relation.affinity()
			if weight <= 0 || !finite(weight) {
				continue
			}
			matrix[left][right] = weight
			matrix[right][left] = weight
		}
	}
	return matrix
}

/*
spectralBalancedSplit performs one exact-cardinality spectral bisection using
the Fiedler ordering of the symmetric normalized graph Laplacian. If the
subgraph has no positive affinity (or eigendecomposition fails), it falls back
to a deterministic key split.
*/
func spectralBalancedSplit(
	vertices []int,
	leftSize int,
	affinity [][]float64,
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
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			w := affinity[vertices[i]][vertices[j]]
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
	for i := 0; i < n; i++ {
		if degree[i] > 0 {
			laplacian.SetSym(i, i, 1)
		}
	}
	for i := 0; i < n; i++ {
		if degree[i] <= 0 {
			continue
		}
		for j := i + 1; j < n; j++ {
			w := affinity[vertices[i]][vertices[j]]
			if w <= 0 || degree[j] <= 0 {
				continue
			}
			laplacian.SetSym(i, j, -w/math.Sqrt(degree[i]*degree[j]))
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
		} else {
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

/*
refineBalancedGroups performs pair swaps only, so every region keeps exactly
the capacity produced by the balanced spectral recursion. Each accepted swap
strictly increases total within-region affinity.
*/
func refineBalancedGroups(groups [][]int, affinity [][]float64, keys []string) [][]int {
	if len(groups) <= 1 {
		return groups
	}

	n := len(keys)
	parts := make([]int, n)
	for part, group := range groups {
		for _, vertex := range group {
			parts[vertex] = part
		}
	}

	maxSwaps := min(n, 128)
	for iteration := 0; iteration < maxSwaps; iteration++ {
		connections := make([][]float64, n)
		for i := range connections {
			connections[i] = make([]float64, len(groups))
		}
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				w := affinity[i][j]
				if w <= 0 {
					continue
				}
				connections[i][parts[j]] += w
				connections[j][parts[i]] += w
			}
		}

		bestGain := 1e-12
		bestLeft, bestRight := -1, -1

		for left := 0; left < n; left++ {
			leftPart := parts[left]
			for right := left + 1; right < n; right++ {
				rightPart := parts[right]
				if leftPart == rightPart {
					continue
				}

				pairWeight := affinity[left][right]
				gain := connections[left][rightPart] - pairWeight - connections[left][leftPart]
				gain += connections[right][leftPart] - pairWeight - connections[right][rightPart]

				if gain > bestGain+1e-12 || (math.Abs(gain-bestGain) <= 1e-12 && pairLexLess(left, right, bestLeft, bestRight, keys)) {
					bestGain = gain
					bestLeft = left
					bestRight = right
				}
			}
		}

		if bestLeft < 0 {
			break
		}
		parts[bestLeft], parts[bestRight] = parts[bestRight], parts[bestLeft]
	}

	refined := make([][]int, len(groups))
	for vertex, part := range parts {
		refined[part] = append(refined[part], vertex)
	}
	return refined
}

func pairLexLess(left, right, bestLeft, bestRight int, keys []string) bool {
	if bestLeft < 0 {
		return true
	}
	leftA, leftB := keys[left], keys[right]
	bestA, bestB := keys[bestLeft], keys[bestRight]
	if leftA != bestA {
		return leftA < bestA
	}
	return leftB < bestB
}

func canonicalRegionAssignment(groups [][]int, keys []string) map[string]uint8 {
	for _, group := range groups {
		sort.Slice(group, func(i, j int) bool { return keys[group[i]] < keys[group[j]] })
	}

	sort.SliceStable(groups, func(i, j int) bool {
		if len(groups[i]) == 0 {
			return false
		}
		if len(groups[j]) == 0 {
			return true
		}
		return keys[groups[i][0]] < keys[groups[j][0]]
	})

	assigned := make(map[string]uint8, len(keys))
	for index, group := range groups {
		region := uint8(index + 1)
		for _, vertex := range group {
			assigned[keys[vertex]] = region
		}
	}
	return assigned
}

func (grid *Grid) commitPartitionsLocked(partitions map[string]uint8) {
	grid.RegionMembers = make(map[uint8]int)
	if len(partitions) == 0 {
		return
	}

	for key, cell := range grid.Cells {
		region := partitions[key]
		cell.Region = region
		if region > 0 {
			grid.RegionMembers[region]++
		}
	}

	regionCount := len(grid.RegionMembers)
	for _, cell := range grid.idToCell {
		if cell == nil || cell.Region == 0 {
			continue
		}
		cell.X, cell.Y = regionCenter(cell.Region, regionCount)
	}
}

func regionCenter(region uint8, regionCount int) (float64, float64) {
	if region == 0 || regionCount <= 0 {
		return 0, 0
	}

	columns := min(GridBinsX, regionCount)
	rows := int(math.Ceil(float64(regionCount) / float64(columns)))
	index := int(region) - 1
	column := index % columns
	row := index / columns

	x := 0.0
	if columns > 1 {
		x = -1 + 2*float64(column)/float64(columns-1)
	}
	y := 0.0
	if rows > 1 {
		y = 1 - 2*float64(row)/float64(rows-1)
	}
	return x, y
}

/*
RegionScores evaluates all supplied measurements as one pass and returns every
represented region ranked strongest-first.

For Measurement m, every owned metric receives the same attenuation:

	q_m = maturity_m * snr_m/(1+snr_m)

when SNR is defined, and q_m = maturity_m when it is not. A zero maturity on a
non-estimated measurement is treated as the legacy "unspecified" value and
therefore as 1.0; the data Finalizer uses the same convention.

Within a region the score is the mean damped activity over unique supplied
metric channels, not a sum. Coverage is reported separately instead of being
mixed into Score.
*/
func (grid *Grid) RegionScores(measurements ...*data.Measurement[float64]) []RegionScore {
	if grid == nil || len(measurements) == 0 {
		return nil
	}
	grid.mu.RLock()
	defer grid.mu.RUnlock()
	return grid.regionScoresLocked(nonNilMeasurements(measurements))
}

func (grid *Grid) regionScoresLocked(roots []*data.Measurement[float64]) []RegionScore {
	type cellAggregate struct {
		sum   float64
		count int
	}
	perCell := make(map[uint32]cellAggregate)

	walkMeasurements(roots, func(measurement *data.Measurement[float64]) {
		damp := measurementDamp(measurement)

		measurement.RangeMetrics(func(key string, metric data.Metric[float64]) bool {
			cell := grid.lookupCellLocked(measurement, key, metric)
			if cell == nil || cell.Region == 0 {
				return true
			}

			activity, ok := grid.metricActivityLocked(cell, metric)
			if !ok {
				return true
			}

			agg := perCell[cell.ID]
			agg.sum += math.Abs(activity) * damp
			agg.count++
			perCell[cell.ID] = agg
			return true
		})
	})

	type regionAggregate struct {
		sum   float64
		count int
	}
	regions := make(map[uint8]regionAggregate)

	for id, aggregate := range perCell {
		if aggregate.count <= 0 || int(id) >= len(grid.idToCell) {
			continue
		}
		cell := grid.idToCell[id]
		if cell == nil || cell.Region == 0 {
			continue
		}

		contribution := aggregate.sum / float64(aggregate.count)
		reg := regions[cell.Region]
		reg.sum += contribution
		reg.count++
		regions[cell.Region] = reg
	}

	scores := make([]RegionScore, 0, len(regions))
	for region, aggregate := range regions {
		if aggregate.count <= 0 {
			continue
		}
		members := grid.RegionMembers[region]
		coverage := 0.0
		if members > 0 {
			coverage = float64(aggregate.count) / float64(members)
		}
		scores = append(scores, RegionScore{
			Region:       region,
			Score:        aggregate.sum / float64(aggregate.count),
			Contributors: aggregate.count,
			Members:      members,
			Coverage:     coverage,
		})
	}

	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].Score == scores[j].Score {
			return scores[i].Region < scores[j].Region
		}
		return scores[i].Score > scores[j].Score
	})
	return scores
}

/*
LitRegions keeps the existing token API deliberately conservative: it returns
the strongest region from the same fair RegionScores pass. Call RegionScores
when the UI or diagnostics need the complete ranked set and actual intensities.
*/
func (grid *Grid) LitRegions(measurements ...*data.Measurement[float64]) [][]byte {
	if grid == nil || len(measurements) == 0 {
		return nil
	}
	grid.mu.RLock()
	defer grid.mu.RUnlock()

	scores := grid.regionScoresLocked(nonNilMeasurements(measurements))
	if len(scores) == 0 || scores[0].Score <= 0 || scores[0].Region == 0 {
		return nil
	}
	return [][]byte{{scores[0].Region}}
}

func (grid *Grid) lookupCellLocked(
	measurement *data.Measurement[float64],
	key string,
	metric data.Metric[float64],
) *Cell {
	channel := measurementChannel(measurement, key, metric)
	if cell := grid.Cells[channel]; cell != nil {
		return cell
	}

	// Compatibility path for callers/tests that carry the full CellKey in the
	// metric label rather than the local metric name.
	if metric.Label != "" {
		if cell := grid.Cells[metric.Label]; cell != nil {
			return cell
		}
	}
	if cell := grid.Cells[key]; cell != nil {
		return cell
	}
	return nil
}

func (grid *Grid) metricActivityLocked(cell *Cell, metric data.Metric[float64]) (float64, bool) {
	if metric.Deformation != nil && finite(*metric.Deformation) {
		return *metric.Deformation, true
	}
	if metric.Standardized != nil && finite(*metric.Standardized) {
		return *metric.Standardized, true
	}
	if metric.Normalized != nil && finite(*metric.Normalized) {
		return *metric.Normalized, true
	}

	if cell != nil && finite(metric.Raw) {
		if previous, ok := grid.prevRaw[cell.Key]; ok && finite(previous) {
			value := data.Deformation(previous, metric.Raw)
			if finite(value) {
				return value, true
			}
		}
	}
	return 0, false
}

func measurementDamp(measurement *data.Measurement[float64]) float64 {
	if measurement == nil {
		return 0
	}

	maturity := measurement.Maturity
	if !finite(maturity) {
		return 0
	}
	maturity = clamp(maturity, 0, 1)
	if maturity == 0 && !measurement.Estimated {
		// Measurement historically used 0 as "not supplied" for ordinary
		// observations. Finalizer upgrades that case to 1; retain the same
		// compatibility for directly-constructed measurements.
		maturity = 1
	}

	snrFactor := 1.0
	if measurement.SNRDefined {
		if !finite(measurement.SNR) || measurement.SNR <= 0 {
			return 0
		}
		snrFactor = measurement.SNR / (1 + measurement.SNR)
	}

	return clamp(maturity*snrFactor, 0, 1)
}

func (grid *Grid) decorateLocked(roots []*data.Measurement[float64]) {
	walkMeasurements(roots, func(measurement *data.Measurement[float64]) {
		measurement.RangeMetrics(func(key string, metric data.Metric[float64]) bool {
			cell := grid.lookupCellLocked(measurement, key, metric)
			if cell == nil {
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

const gridSnapshotVersion = 2

type GridSnapshot struct {
	Version       int                      `json:"version"`
	Settled       bool                     `json:"settled"`
	Observations  int64                    `json:"observations"`
	Cells         map[string]*Cell         `json:"cells"`
	Relations     map[uint64]*Relation     `json:"relations"`
	RegionMembers map[uint8]int            `json:"region_members"`
	PrevRaw       map[string]float64       `json:"prev_raw,omitempty"`
	Dirty         bool                     `json:"dirty,omitempty"`
	PendingTick   int64                    `json:"pending_tick,omitempty"`
	Pending       map[uint32]pendingSample `json:"pending,omitempty"`
}

func (grid *Grid) Snapshot() ([]byte, error) {
	if grid == nil {
		return nil, fmt.Errorf("grid: snapshot nil grid")
	}

	grid.mu.RLock()
	snapshot := GridSnapshot{
		Version:       gridSnapshotVersion,
		Settled:       grid.Settled,
		Observations:  grid.Observations,
		Cells:         make(map[string]*Cell, len(grid.Cells)),
		Relations:     make(map[uint64]*Relation, len(grid.Relations)),
		RegionMembers: make(map[uint8]int, len(grid.RegionMembers)),
		PrevRaw:       make(map[string]float64, len(grid.prevRaw)),
		Dirty:         grid.dirty,
		PendingTick:   grid.pendingTick,
		Pending:       make(map[uint32]pendingSample, len(grid.pending)),
	}

	for key, cell := range grid.Cells {
		if cell == nil {
			continue
		}
		copyCell := *cell
		snapshot.Cells[key] = &copyCell
	}
	for key, relation := range grid.Relations {
		if relation == nil {
			continue
		}
		copyRelation := *relation
		snapshot.Relations[key] = &copyRelation
	}
	for region, count := range grid.RegionMembers {
		snapshot.RegionMembers[region] = count
	}
	for key, value := range grid.prevRaw {
		snapshot.PrevRaw[key] = value
	}
	for id, sample := range grid.pending {
		snapshot.Pending[id] = sample
	}
	grid.mu.RUnlock()

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
		if cell.Key != key {
			return fmt.Errorf("grid: snapshot cell key mismatch %q != %q", key, cell.Key)
		}
		if int(cell.ID) >= cellCount {
			return fmt.Errorf("grid: snapshot cell %q has out-of-range id %d", key, cell.ID)
		}
		if idToCell[cell.ID] != nil {
			return fmt.Errorf("grid: duplicate snapshot cell id %d", cell.ID)
		}
		idToCell[cell.ID] = cell
		cellIDs[key] = cell.ID
	}
	for id, cell := range idToCell {
		if cell == nil {
			return fmt.Errorf("grid: snapshot is missing cell id %d", id)
		}
	}

	for key, relation := range snapshot.Relations {
		if relation == nil {
			return fmt.Errorf("grid: snapshot relation %d is null", key)
		}
		left, right := pairIDs(key)
		if left == right || int(left) >= cellCount || int(right) >= cellCount {
			return fmt.Errorf("grid: snapshot relation %d references invalid cells", key)
		}
	}
	for id := range snapshot.Pending {
		if int(id) >= cellCount {
			return fmt.Errorf("grid: snapshot pending sample references invalid cell %d", id)
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

	grid.mu.Lock()
	defer grid.mu.Unlock()

	grid.Settled = snapshot.Settled
	grid.Observations = snapshot.Observations
	grid.Cells = snapshot.Cells
	grid.Relations = snapshot.Relations
	grid.RegionMembers = regionMembers
	grid.cellIDs = cellIDs
	grid.idToCell = idToCell
	grid.prevRaw = snapshot.PrevRaw
	grid.dirty = snapshot.Dirty || (snapshot.Settled && unassigned)
	grid.pendingTick = snapshot.PendingTick
	grid.pending = snapshot.Pending
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func clamp(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
