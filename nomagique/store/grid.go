package store

import (
	"cmp"
	"encoding/json"
	"math"
	"slices"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/geometry"
)

const (
	LitRegionTokenSize   = 1
	ConvergenceTolerance = 1e-3 // Max spatial movement per cell to consider stationary
	ConvergenceStreak    = 5    // Consecutive active ticks of zero partition drift and sub-tolerance movement
)

/*
Cell represents a single metric's state and position on the impulse map.
*/
type Cell struct {
	ID          uint32  `json:"id"`
	Key         string  `json:"key"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Region      uint8   `json:"region"`
	Last        float64 `json:"last"`
	Initialized bool    `json:"initialized"`
}

/*
Relation records co-movement statistics between two metrics.
*/
type Relation struct {
	Same     int64 `json:"same"`
	Opposite int64 `json:"opposite"`
	Total    int64 `json:"total"`
}

func (relation *Relation) sympathy() float64 {
	if relation == nil || relation.Total == 0 {
		return 0
	}
	return float64(relation.Same-relation.Opposite) / float64(relation.Total)
}

func pairKey(leftID, rightID uint32) uint64 {
	if leftID < rightID {
		return (uint64(leftID) << 32) | uint64(rightID)
	}
	return (uint64(rightID) << 32) | uint64(leftID)
}

/*
Grid is the Impulse Map: a 2D coordinate space where metrics cluster sympathetically
based on streaming directional co-movement. Metrics cluster into cohesive
macroscopic regions via watershed basin clustering, guaranteeing zero singletons.
*/
type Grid struct {
	mu sync.RWMutex

	Settled       bool                 `json:"settled"`
	Observations  int64                `json:"observations"`
	Cells         map[string]*Cell     `json:"cells"`
	Relations     map[uint64]*Relation `json:"relations"`
	RegionMembers map[uint8]int        `json:"region_members"`

	stableStreak int
	lastRegions  map[string]uint8

	cellIDs  map[string]uint32
	idToCell []*Cell
}

func NewGrid() *Grid {
	return &Grid{
		Cells:         make(map[string]*Cell),
		Relations:     make(map[uint64]*Relation),
		RegionMembers: make(map[uint8]int),
		cellIDs:       make(map[string]uint32),
		idToCell:      make([]*Cell, 0),
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

func (grid *Grid) Region(cellKey string) uint8 {
	grid.mu.RLock()
	defer grid.mu.RUnlock()
	if cell := grid.Cells[cellKey]; cell != nil {
		return cell.Region
	}
	return 0
}

type gridArrival struct {
	key string
	raw float64
}

/*
Update streams incoming signal measurements into the grid, updates pairwise co-movement
relations, relaxes 2D positions, and checks for physical convergence.
*/
func (grid *Grid) Update(measurements ...*data.Measurement[float64]) {
	if len(measurements) == 0 {
		return
	}

	grid.mu.Lock()
	defer grid.mu.Unlock()

	var arrivals []gridArrival

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		symbol := measurement.Label
		if symbol == "" {
			symbol = measurement.GetSource()
		}
		source := measurement.GetSource()

		measurement.RangeMetrics(func(key string, metric data.Metric[float64]) bool {
			name := metric.Label
			if name == "" {
				name = key
			}

			arrivals = append(arrivals, gridArrival{
				key: CellKey(symbol, source, name),
				raw: metric.Raw,
			})
			return true
		})
	}

	if len(arrivals) == 0 {
		return
	}

	activeCellIDs := make([]uint32, 0, len(arrivals))
	directions := make(map[uint32]int, len(arrivals))

	for _, incoming := range arrivals {
		cell := grid.Cells[incoming.key]

		if cell == nil {
			newID := uint32(len(grid.idToCell))
			radial := math.Sqrt(float64(newID + 1))
			theta := float64(newID) * 2.399963229728653

			cell = &Cell{
				ID:          newID,
				Key:         incoming.key,
				X:           radial * math.Cos(theta),
				Y:           radial * math.Sin(theta),
				Last:        incoming.raw,
				Initialized: false,
			}

			grid.Cells[incoming.key] = cell
			grid.cellIDs[incoming.key] = newID
			grid.idToCell = append(grid.idToCell, cell)
		}

		if !cell.Initialized {
			cell.Last = incoming.raw
			cell.Initialized = true
			activeCellIDs = append(activeCellIDs, cell.ID)
			continue
		}

		change := incoming.raw - cell.Last
		cell.Last = incoming.raw

		if change > 0 {
			directions[cell.ID] = 1
		} else if change < 0 {
			directions[cell.ID] = -1
		}

		activeCellIDs = append(activeCellIDs, cell.ID)
	}

	slices.Sort(activeCellIDs)
	activeCellIDs = slices.Compact(activeCellIDs)

	if !grid.Settled {
		for firstIndex := 0; firstIndex < len(activeCellIDs); firstIndex++ {
			firstID := activeCellIDs[firstIndex]
			firstDir := directions[firstID]
			if firstDir == 0 {
				continue
			}

			for secondIndex := firstIndex + 1; secondIndex < len(activeCellIDs); secondIndex++ {
				secondID := activeCellIDs[secondIndex]
				secondDir := directions[secondID]
				if secondDir == 0 {
					continue
				}

				key := pairKey(firstID, secondID)
				relation := grid.Relations[key]
				if relation == nil {
					relation = &Relation{}
					grid.Relations[key] = relation
				}

				relation.Total++
				if firstDir == secondDir {
					relation.Same++
				} else {
					relation.Opposite++
				}
			}
		}

		prevPositions := make([]struct{ x, y float64 }, len(activeCellIDs))
		for i, id := range activeCellIDs {
			cell := grid.idToCell[id]
			prevPositions[i] = struct{ x, y float64 }{cell.X, cell.Y}
		}

		dampFactor := 1.0 / (1.0 + float64(grid.Observations)/100.0)
		forcesApplied := 0

		for firstIndex := 0; firstIndex < len(activeCellIDs); firstIndex++ {
			firstID := activeCellIDs[firstIndex]
			firstCell := grid.idToCell[firstID]

			for secondIndex := firstIndex + 1; secondIndex < len(activeCellIDs); secondIndex++ {
				secondID := activeCellIDs[secondIndex]
				relation := grid.Relations[pairKey(firstID, secondID)]
				if relation == nil {
					continue
				}

				sympathy := relation.sympathy()
				if sympathy == 0 {
					continue
				}

				secondCell := grid.idToCell[secondID]
				deltaX := secondCell.X - firstCell.X
				deltaY := secondCell.Y - firstCell.Y
				distance := math.Hypot(deltaX, deltaY)

				var unitX, unitY float64
				if distance < 1e-4 {
					angle := float64(firstID+secondID) * 1.61803398875
					unitX = math.Cos(angle)
					unitY = math.Sin(angle)
					distance = 1e-4
				} else {
					unitX = deltaX / distance
					unitY = deltaY / distance
				}

				targetDistance := 2.75 - 2.25*sympathy
				displacement := distance - targetDistance
				force := 0.1 * displacement * dampFactor
				force = math.Max(-1.0, math.Min(1.0, force))

				firstCell.X += unitX * force * 0.5
				firstCell.Y += unitY * force * 0.5
				secondCell.X -= unitX * force * 0.5
				secondCell.Y -= unitY * force * 0.5
				forcesApplied++
			}
		}

		var maxDisplacement float64
		for i, id := range activeCellIDs {
			cell := grid.idToCell[id]
			disp := math.Hypot(cell.X-prevPositions[i].x, cell.Y-prevPositions[i].y)
			if disp > maxDisplacement {
				maxDisplacement = disp
			}
		}

		grid.Observations++

		if forcesApplied > 0 {
			grid.checkConvergenceLocked(maxDisplacement)
		}
	}

	grid.decorate(measurements)
}

func (grid *Grid) checkConvergenceLocked(maxDisplacement float64) {
	if len(grid.Cells) < 2 || len(grid.Relations) < len(grid.Cells)-1 {
		grid.stableStreak = 0
		return
	}

	for _, cell := range grid.Cells {
		if !cell.Initialized {
			grid.stableStreak = 0
			return
		}
	}

	// 1. Physical motion must be sub-tolerance
	if maxDisplacement >= ConvergenceTolerance {
		grid.stableStreak = 0
		grid.lastRegions = nil
		return
	}

	// 2. Macroscopic basin clustering must have zero partition drift
	currentRegions := grid.computePartitionsLocked()
	if currentRegions == nil {
		grid.stableStreak = 0
		return
	}

	if grid.lastRegions != nil && partitionsEqual(grid.lastRegions, currentRegions) {
		grid.stableStreak++
		if grid.stableStreak >= ConvergenceStreak {
			grid.commitPartitionsLocked(currentRegions)
		}
	} else {
		grid.stableStreak = 1
		grid.lastRegions = currentRegions
	}
}

func partitionsEqual(left, right map[string]uint8) bool {
	if len(left) != len(right) {
		return false
	}
	for key, val := range left {
		if right[key] != val {
			return false
		}
	}
	return true
}

func (grid *Grid) Settle() {
	grid.mu.Lock()
	defer grid.mu.Unlock()

	if grid.Settled {
		return
	}

	partitions := grid.computePartitionsLocked()
	grid.commitPartitionsLocked(partitions)
}

func (grid *Grid) commitPartitionsLocked(partitions map[string]uint8) {
	grid.Settled = true
	if len(partitions) == 0 {
		return
	}

	grid.RegionMembers = make(map[uint8]int)
	for key, regionID := range partitions {
		if cell := grid.Cells[key]; cell != nil {
			cell.Region = regionID
			grid.RegionMembers[regionID]++
		}
	}
}

func (grid *Grid) computePartitionsLocked() map[string]uint8 {
	if len(grid.Cells) == 0 {
		return nil
	}

	keys := make([]string, 0, len(grid.Cells))
	for key := range grid.Cells {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	if len(keys) == 1 {
		return map[string]uint8{keys[0]: 1}
	}

	points := make([]*geometry.Point, len(keys))
	for index, key := range keys {
		cell := grid.Cells[key]
		points[index] = &geometry.Point{
			X:         cell.X,
			Y:         cell.Y,
			Authority: 1.0,
		}
	}

	totalKeys := len(keys)
	edges := make([]geometry.Edge, totalKeys*(totalKeys-1)/2)

	for rightIndex := 1; rightIndex < totalKeys; rightIndex++ {
		rightCell := grid.Cells[keys[rightIndex]]
		for leftIndex := 0; leftIndex < rightIndex; leftIndex++ {
			leftCell := grid.Cells[keys[leftIndex]]
			relation := grid.Relations[pairKey(leftCell.ID, rightCell.ID)]
			sympathy := 0.0
			if relation != nil {
				sympathy = relation.sympathy()
			}

			edges[rightIndex*(rightIndex-1)/2+leftIndex] = geometry.Edge{
				Left:     leftIndex,
				Right:    rightIndex,
				Strength: sympathy,
			}
		}
	}

	geometry.Watershed{}.Step(points, edges)

	groups := make(map[int][]string)
	for index, key := range keys {
		basin := points[index].Basin
		groups[basin] = append(groups[basin], key)
	}

	type basinCandidate struct {
		basinID int
		members []string
		size    int
	}

	var multiBasins []basinCandidate
	var minorBasins []basinCandidate

	basinIDs := make([]int, 0, len(groups))
	for basinID := range groups {
		basinIDs = append(basinIDs, basinID)
	}
	slices.Sort(basinIDs)

	for _, basinID := range basinIDs {
		memberKeys := groups[basinID]
		candidate := basinCandidate{
			basinID: basinID,
			members: memberKeys,
			size:    len(memberKeys),
		}

		if len(memberKeys) >= 2 {
			multiBasins = append(multiBasins, candidate)
			continue
		}
		minorBasins = append(minorBasins, candidate)
	}

	if len(multiBasins) == 0 {
		var allCandidates []basinCandidate
		for _, basinID := range basinIDs {
			memberKeys := groups[basinID]
			allCandidates = append(allCandidates, basinCandidate{
				basinID: basinID,
				members: memberKeys,
				size:    len(memberKeys),
			})
		}

		slices.SortFunc(allCandidates, func(left, right basinCandidate) int {
			if left.size != right.size {
				return right.size - left.size
			}
			return cmp.Compare(left.basinID, right.basinID)
		})

		multiBasins = append(multiBasins, allCandidates[0])
		minorBasins = allCandidates[1:]
	}

	slices.SortFunc(multiBasins, func(left, right basinCandidate) int {
		if left.size != right.size {
			return right.size - left.size
		}
		return cmp.Compare(left.basinID, right.basinID)
	})

	targetCount := min(20, len(multiBasins))
	if targetCount < 1 {
		targetCount = 1
	}

	finalClusters := make(map[int][]string, targetCount)
	for index := 0; index < targetCount; index++ {
		basinID := multiBasins[index].basinID
		finalClusters[basinID] = append([]string(nil), groups[basinID]...)
	}

	for index := targetCount; index < len(multiBasins); index++ {
		minorBasins = append(minorBasins, multiBasins[index])
	}

	primaryBasinID := multiBasins[0].basinID

	for _, minor := range minorBasins {
		bestTarget := primaryBasinID
		bestSympathy := -math.MaxFloat64

		for _, target := range multiBasins[:targetCount] {
			targetBasin := target.basinID
			targetMembers := groups[targetBasin]
			totalSympathy := 0.0
			pairCount := 0

			for _, minorKey := range minor.members {
				minorCell := grid.Cells[minorKey]
				for _, targetKey := range targetMembers {
					targetCell := grid.Cells[targetKey]
					relation := grid.Relations[pairKey(minorCell.ID, targetCell.ID)]
					if relation != nil {
						totalSympathy += relation.sympathy()
						pairCount++
					}
				}
			}

			if pairCount > 0 {
				meanSympathy := totalSympathy / float64(pairCount)
				if meanSympathy > bestSympathy || (meanSympathy == bestSympathy && targetBasin < bestTarget) {
					bestSympathy = meanSympathy
					bestTarget = targetBasin
				}
			}
		}

		finalClusters[bestTarget] = append(finalClusters[bestTarget], minor.members...)
	}

	type clusterOrder struct {
		basinID int
		members []string
	}

	ordered := make([]clusterOrder, 0, targetCount)
	for _, target := range multiBasins[:targetCount] {
		memberKeys := finalClusters[target.basinID]
		slices.Sort(memberKeys)
		ordered = append(ordered, clusterOrder{
			basinID: target.basinID,
			members: memberKeys,
		})
	}

	slices.SortFunc(ordered, func(left, right clusterOrder) int {
		if len(left.members) != len(right.members) {
			return len(right.members) - len(left.members)
		}
		return cmp.Compare(left.basinID, right.basinID)
	})

	assigned := make(map[string]uint8, len(grid.Cells))
	regionID := uint8(1)

	for _, cluster := range ordered {
		for _, key := range cluster.members {
			assigned[key] = regionID
		}
		regionID++
	}

	return assigned
}

/*
LitRegions computes active region scores across all incoming sensory measurements for a tick,
emitting a single 1-byte token where multi-metric cross-signal consensus defeats isolated spikes.
*/
func (grid *Grid) LitRegions(measurements ...*data.Measurement[float64]) [][]byte {
	if len(measurements) == 0 {
		return nil
	}

	grid.mu.RLock()
	defer grid.mu.RUnlock()

	activity := make(map[uint8]float64)
	activeCount := make(map[uint8]int)

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		symbol := measurement.Label
		if symbol == "" {
			symbol = measurement.GetSource()
		}
		source := measurement.GetSource()

		measurement.RangeMetrics(func(key string, metric data.Metric[float64]) bool {
			name := metric.Label
			if name == "" {
				name = key
			}

			cell := grid.Cells[CellKey(symbol, source, name)]
			if cell == nil || cell.Region == 0 {
				return true
			}

			act := regionActivity(metric)
			if act > 0 {
				activity[cell.Region] += act
				activeCount[cell.Region]++
			}
			return true
		})
	}

	if len(activity) == 0 {
		return nil
	}

	bestRegion := uint8(0)
	bestScore := -1.0

	regionIDs := make([]uint8, 0, len(activity))
	for region := range activity {
		regionIDs = append(regionIDs, region)
	}
	slices.Sort(regionIDs)

	for _, region := range regionIDs {
		totalAct := activity[region]
		count := activeCount[region]
		memberCount := grid.RegionMembers[region]
		if memberCount <= 0 {
			memberCount = 1
		}

		participation := math.Min(1.0, float64(count)/float64(memberCount))
		score := totalAct * math.Sqrt(participation)

		if score > bestScore {
			bestScore = score
			bestRegion = region
		}
	}

	if bestRegion > 0 {
		return [][]byte{{bestRegion}}
	}

	return nil
}

func regionActivity(metric data.Metric[float64]) float64 {
	if metric.Deformation != nil {
		return math.Abs(*metric.Deformation)
	}
	if metric.Standardized != nil {
		return math.Abs(*metric.Standardized)
	}
	if metric.Normalized != nil {
		return math.Abs(*metric.Normalized)
	}
	return 0
}

func (grid *Grid) decorate(measurements []*data.Measurement[float64]) {
	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		symbol := measurement.Label
		if symbol == "" {
			symbol = measurement.GetSource()
		}
		source := measurement.GetSource()

		measurement.RangeMetrics(func(key string, incoming data.Metric[float64]) bool {
			name := incoming.Label
			if name == "" {
				name = key
			}

			cell := grid.Cells[CellKey(symbol, source, name)]
			if cell != nil {
				incoming.X = int64(math.Round(cell.X))
				incoming.Y = int64(math.Round(cell.Y))
				incoming.Region = cell.Region
				measurement.SetMetric(key, incoming)
			}
			return true
		})
	}
}

type GridSnapshot struct {
	Settled       bool                 `json:"settled"`
	Observations  int64                `json:"observations"`
	Cells         map[string]*Cell     `json:"cells"`
	Relations     map[uint64]*Relation `json:"relations"`
	RegionMembers map[uint8]int        `json:"region_members"`
}

func (grid *Grid) Snapshot() ([]byte, error) {
	grid.mu.RLock()

	snapshot := GridSnapshot{
		Settled:       grid.Settled,
		Observations:  grid.Observations,
		Cells:         make(map[string]*Cell, len(grid.Cells)),
		Relations:     make(map[uint64]*Relation, len(grid.Relations)),
		RegionMembers: make(map[uint8]int, len(grid.RegionMembers)),
	}

	for key, cell := range grid.Cells {
		copiedCell := *cell
		snapshot.Cells[key] = &copiedCell
	}

	for key, relation := range grid.Relations {
		copiedRel := *relation
		snapshot.Relations[key] = &copiedRel
	}

	for region, count := range grid.RegionMembers {
		snapshot.RegionMembers[region] = count
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
	var snapshot GridSnapshot
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"grid: decode snapshot",
			err,
		))
	}

	grid.mu.Lock()
	defer grid.mu.Unlock()

	grid.Settled = snapshot.Settled
	grid.Observations = snapshot.Observations
	grid.Cells = snapshot.Cells
	if grid.Cells == nil {
		grid.Cells = make(map[string]*Cell)
	}

	grid.Relations = snapshot.Relations
	if grid.Relations == nil {
		grid.Relations = make(map[uint64]*Relation)
	}

	grid.RegionMembers = snapshot.RegionMembers
	if grid.RegionMembers == nil {
		grid.RegionMembers = make(map[uint8]int)
	}

	grid.cellIDs = make(map[string]uint32, len(grid.Cells))
	grid.idToCell = make([]*Cell, len(grid.Cells))

	for key, cell := range grid.Cells {
		grid.cellIDs[key] = cell.ID
		if int(cell.ID) >= len(grid.idToCell) {
			extended := make([]*Cell, cell.ID+1)
			copy(extended, grid.idToCell)
			grid.idToCell = extended
		}
		grid.idToCell[cell.ID] = cell
	}

	return nil
}
