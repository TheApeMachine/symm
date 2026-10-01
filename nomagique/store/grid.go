package store

import (
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/geometry"
)

/*
Grid is the Impulse Map — a 2D coordinate space where Signals and Logic
Solvers write their observations. Metrics cluster sympathetically following
three priority rules:

  - Priority 1: Values that move together attract (directional co-movement).
  - Priority 2: Values with closest relative magnitude during movement attract,
    but only if the sign relationship is consistent. Inconsistent pairs repel.
  - Priority 3: Maturity and SNR power attraction asymmetrically — weaker
    metrics move more toward stronger ones. When SNR/maturity is unavailable,
    "separation" (how much the strongest bucket stands out) is used.

Regions form naturally at the borders where the weakest cells meet. Once the
region partition stabilizes, the grid is frozen. Post-settlement, new
observations are only used to produce region tokens via LitRegions.
*/
type Grid struct {
	*core.PrimitiveError
	mu sync.RWMutex

	Metrics []*data.Metric[float64] `json:"metrics"`
	Settled bool                    `json:"settled"`

	Last      map[string]float64       `json:"last"`
	Present   map[string]bool          `json:"present"`
	Relations map[string]*GridRelation `json:"relations"`
	Regions   map[string]uint8         `json:"regions"`
	Authority map[string]float64       `json:"authority"`
	Bound     map[string]string        `json:"bound"`

	PosX map[string]float64 `json:"pos_x"`
	PosY map[string]float64 `json:"pos_y"`

	Partition string `json:"partition"`

	Observations        int64 `json:"observations"`
	PartitionRun        int   `json:"partition_run"`
	LongestPartitionRun int   `json:"longest_partition_run"`

	statsMu        sync.Mutex
	RegionLitCount [256]int       `json:"region_lit_count"`
	TotalTokens    int            `json:"total_tokens"`
	TokenCounts    map[string]int `json:"token_counts"`
}

/*
GridRelation tracks pairwise movement statistics for the sympathy calculation.

Priority 1 uses the four directional quadrants (both move same direction vs
opposite). Priority 2 uses magnitude similarity, gated by directional
consistency. The relation is keyed by the sorted pair of metric labels.
*/
type GridRelation struct {
	PositivePositive int64 `json:"positive_positive"`
	PositiveNegative int64 `json:"positive_negative"`
	NegativePositive int64 `json:"negative_positive"`
	NegativeNegative int64 `json:"negative_negative"`

	AZeroBPositive int64 `json:"a_zero_b_positive"`
	AZeroBNegative int64 `json:"a_zero_b_negative"`
	APositiveBZero int64 `json:"a_positive_b_zero"`
	ANegativeBZero int64 `json:"a_negative_b_zero"`

	MagnitudeSimilarity float64 `json:"magnitude_similarity"`
	MagnitudeSamples    int64   `json:"magnitude_samples"`
}

func NewGrid() *Grid {
	return &Grid{
		PrimitiveError: core.NewPrimitiveError(),
		Metrics:        make([]*data.Metric[float64], 0),
		Last:           make(map[string]float64),
		Present:        make(map[string]bool),
		Relations:      make(map[string]*GridRelation),
		Regions:        make(map[string]uint8),
		Authority:      make(map[string]float64),
		Bound:          make(map[string]string),
		PosX:           make(map[string]float64),
		PosY:           make(map[string]float64),
		TokenCounts:    make(map[string]int),
	}
}

// Settle locks the current partition.
func (grid *Grid) Settle() {
	grid.mu.Lock()
	defer grid.mu.Unlock()
	grid.Settled = true
}

// ResetSettlement unlocks the grid if further training is required.
func (grid *Grid) ResetSettlement() {
	grid.mu.Lock()
	defer grid.mu.Unlock()
	grid.Settled = false
	grid.PartitionRun = 0
	grid.LongestPartitionRun = 0
}

// IsSettled reports whether the partition is frozen (RLock — never read Settled racy).
func (grid *Grid) IsSettled() bool {
	if grid == nil {
		return false
	}

	grid.mu.RLock()
	defer grid.mu.RUnlock()

	return grid.Settled
}

func (grid *Grid) Add(metric *data.Metric[float64]) {
	grid.mu.Lock()
	defer grid.mu.Unlock()

	if metric == nil || grid.Settled {
		return
	}

	if existing := grid.find(metric.Label); existing != nil {
		*existing = *metric
		return
	}

	copyMetric := *metric
	grid.Metrics = append(grid.Metrics, &copyMetric)
	grid.place(metric.Label)
}

func (grid *Grid) find(label string) *data.Metric[float64] {
	for _, metric := range grid.Metrics {
		if metric != nil && metric.Label == label {
			return metric
		}
	}
	return nil
}

func (grid *Grid) place(label string) {
	if _, exists := grid.PosX[label]; exists {
		return
	}

	index := len(grid.PosX)
	width := int(math.Ceil(math.Sqrt(float64(index + 1))))
	if width < 1 {
		width = 1
	}

	grid.PosX[label] = float64(index % width)
	grid.PosY[label] = float64(index / width)
}

func (grid *Grid) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				continue
			}

			measurement := *(**data.Measurement[float64])(arriving)
			if measurement != nil {
				grid.Update(measurement)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (grid *Grid) Update(measurement *data.Measurement[float64]) {
	grid.update(measurement, true)
}

// ForceSettle forces the grid to finalize its regions immediately.
func (grid *Grid) ForceSettle() {
	grid.mu.Lock()
	defer grid.mu.Unlock()

	if !grid.Settled && len(grid.Metrics) > 0 {
		grid.Settled = true
		labels := make([]string, 0, len(grid.Metrics))
		for _, m := range grid.Metrics {
			if m != nil {
				labels = append(labels, m.Label)
			}
		}
		sort.Strings(labels)
		labels = slices.Compact(labels)
		grid.formRegions(labels)
		grid.writeCoordinates()
	}
}

func (grid *Grid) Observe(measurement *data.Measurement[float64]) {
	grid.update(measurement, false)
}

func (grid *Grid) Region(label string) uint8 {
	grid.mu.RLock()
	defer grid.mu.RUnlock()
	return grid.Regions[label]
}

/*
litRegionTokenSize is N in TRAINING.md: the region token is the N most-lit
regions (example [A, B, C]). Petal trained with the same N.
*/
const litRegionTokenSize = 3

/*
latticeSpacing is the initial step place() uses between adjacent cells.
Binding and watershed neighborhoods are measured against this lattice, not an
invented absolute.
*/
const latticeSpacing = 1.0

/*
LitRegions is the region token for this measurement alone: the N most-lit
regions (TRAINING.md), not a mean-threshold or argmax. Activity prefers
Standardized/Normalized over Raw so price-sized values cannot own every token.
After selecting the top N by activity, region IDs are emitted in ascending
order so identical sets share radix prefixes.

Scores the canonical observation: parent Metrics plus Source-keyed Peers,
matching Grid.Update inventory folds (parent key wins on collision).
*/
func (grid *Grid) LitRegions(measurement *data.Measurement[float64]) []byte {
	if measurement == nil {
		return nil
	}

	grid.mu.RLock()
	defer grid.mu.RUnlock()

	var activity [256]float64
	var count [256]int
	var present [256]bool

	type incomingMetric struct {
		source string
		key    string
		metric data.Metric[float64]
	}

	var allMetrics []incomingMetric

	source := measurement.GetSource()
	for key, metric := range measurement.MetricsSnapshot() {
		allMetrics = append(allMetrics, incomingMetric{source, key, metric})
	}

	for _, peer := range measurement.Peers {
		if peer == nil {
			continue
		}
		peerSource := peer.GetSource()
		for key, metric := range peer.MetricsSnapshot() {
			allMetrics = append(allMetrics, incomingMetric{peerSource, key, metric})
		}
	}

	for _, item := range allMetrics {
		name := item.metric.Label
		if name == "" {
			name = item.key
		}

		region := grid.Regions[cellKey(measurement.Label, item.source, name)]

		if region == 0 {
			continue
		}

		act := regionActivity(item.metric)
		if act > 0 {
			activity[region] += act
			count[region]++
			present[region] = true
		}
	}

	type score struct {
		region uint8
		value  float64
	}

	scores := make([]score, 0, 256)

	for region := 1; region < 256; region++ {
		if !present[region] || count[region] == 0 {
			continue
		}

		scores = append(scores, score{
			region: uint8(region),
			value:  activity[region] / float64(count[region]),
		})
	}

	if len(scores) == 0 {
		return nil
	}

	slices.SortFunc(scores, func(left, right score) int {
		if left.value > right.value {
			return -1
		}

		if left.value < right.value {
			return 1
		}

		return int(left.region) - int(right.region)
	})

	limit := min(litRegionTokenSize, len(scores))
	lit := scores[:limit]

	// Canonical ascending IDs after top-N selection so the same set is stable.
	slices.SortFunc(lit, func(left, right score) int {
		return int(left.region) - int(right.region)
	})

	token := make([]byte, limit)

	for index := range lit {
		token[index] = lit[index].region
	}

	grid.statsMu.Lock()
	grid.TotalTokens++
	for _, r := range token {
		grid.RegionLitCount[r]++
	}
	if grid.TokenCounts == nil {
		grid.TokenCounts = make(map[string]int)
	}
	grid.TokenCounts[fmt.Sprintf("%v", token)]++
	grid.statsMu.Unlock()

	return token
}

/*
regionActivity is the contribution of one metric to its region score.
Standardized/Normalized are unitless tape scales. Without them, presence
counts as 1 — abs(Raw) would let price-sized cells own every top-N slot.
*/
func regionActivity(metric data.Metric[float64]) float64 {
	if metric.Standardized != nil {
		return math.Abs(*metric.Standardized)
	}

	if metric.Normalized != nil {
		return math.Abs(*metric.Normalized)
	}

	return 0
}

/*
cellKey identifies one impulse-map cell by symbol and metric name.

Source is intentionally excluded: the disruptor shared-slot design mutates one
measurement across concurrent signal/logic stages, so Source is racy provenance
rather than cell identity. Replay rebuilds ingress + Source-keyed peers via
canonicalObservations so LitRegions matches live Training.Step; publish may
still rewrite Source to training:* on the UI clone only.
*/
func cellKey(symbol, source, name string) string {
	return symbol + "\x00" + source + "\x00" + name
}

func (grid *Grid) update(
	measurement *data.Measurement[float64],
	decorate bool,
) {
	if measurement == nil {
		return
	}

	type arrival struct {
		source string
		label  string
		metric data.Metric[float64]
	}

	var arrivals []arrival

	source := measurement.GetSource()
	for key, metric := range measurement.MetricsSnapshot() {
		name := metric.Label
		if name == "" {
			name = key
		}
		arrivals = append(arrivals, arrival{
			source: source,
			label:  cellKey(measurement.Label, source, name),
			metric: metric,
		})
	}

	for _, peer := range measurement.Peers {
		if peer == nil {
			continue
		}
		peerSource := peer.GetSource()
		for key, metric := range peer.MetricsSnapshot() {
			name := metric.Label
			if name == "" {
				name = key
			}
			arrivals = append(arrivals, arrival{
				source: peerSource,
				label:  cellKey(measurement.Label, peerSource, name),
				metric: metric,
			})
		}
	}

	if len(arrivals) == 0 {
		return
	}

	grid.mu.Lock()
	defer grid.mu.Unlock()

	if grid.Settled {
		if decorate {
			grid.decorate(measurement)
		}
		return
	}

	current := make(map[string]float64, len(arrivals))
	currentPresent := make(map[string]bool, len(arrivals))

	// Priority 3 Authority: Maturity × SNR when available.
	// When neither is defined, authority stays at 1.0 and separation
	// (computed per-metric below) takes over.
	baseAuthority := measurement.Maturity
	if measurement.SNRDefined && measurement.SNR > 0 {
		baseAuthority *= measurement.SNR
	}
	if baseAuthority <= 0 {
		baseAuthority = 1.0
	}

	// Register new cells in sorted label order so place() lattice indices are
	// deterministic.

	var newArrivals []arrival
	for i := range arrivals {
		a := &arrivals[i]
		label := a.label
		incoming := a.metric

		current[label] = incoming.Raw
		currentPresent[label] = true
		grid.Authority[label] = baseAuthority

		if grid.find(label) == nil {
			metric := incoming
			metric.Label = label
			a.metric = metric
			newArrivals = append(newArrivals, *a)
		}
	}

	sort.Slice(newArrivals, func(left, right int) bool {
		return newArrivals[left].label < newArrivals[right].label
	})

	for index := range newArrivals {
		metric := newArrivals[index].metric
		grid.Metrics = append(grid.Metrics, &metric)
		grid.place(newArrivals[index].label)
	}

	labels := make([]string, 0, len(grid.Metrics))
	for _, metric := range grid.Metrics {
		if metric != nil {
			labels = append(labels, metric.Label)
		}
	}

	sort.Strings(labels)
	labels = slices.Compact(labels)

	delta := make(map[string]float64, len(labels))
	state := make(map[string]int, len(labels))

	// Determine movement direction: -1 (down), 0 (flat/absent), +1 (up)
	for _, label := range labels {
		nowPresent := currentPresent[label]
		wasPresent := grid.Present[label]

		if !nowPresent || !wasPresent {
			state[label] = 0
			continue
		}

		change := current[label] - grid.Last[label]
		delta[label] = change

		switch {
		case change > 0:
			state[label] = 1
		case change < 0:
			state[label] = -1
		default:
			state[label] = 0
		}
	}

	// Priority 3 fallback: "separation" — when maturity/SNR is unavailable,
	// compute per-measurement bucket separation to determine authority.
	if !measurement.SNRDefined && measurement.Maturity <= 0 {
		grid.applySeparationAuthority(labels, state, delta, currentPresent, measurement)
	}

	/*
		Priority 1 & Priority 2 Learning Pass:
		Update the exact movement quadrants and relative magnitudes.
	*/
	for i := 0; i < len(labels); i++ {
		a := labels[i]

		for j := i + 1; j < len(labels); j++ {
			b := labels[j]

			// Only update pair stats if both metrics participated in this observation
			if !currentPresent[a] || !currentPresent[b] {
				continue
			}

			key := pair(a, b)
			relation := grid.Relations[key]
			if relation == nil {
				relation = &GridRelation{}
				grid.Relations[key] = relation
			}

			sa := state[a]
			sb := state[b]

			switch {
			case sa > 0 && sb > 0:
				relation.PositivePositive++
			case sa > 0 && sb < 0:
				relation.PositiveNegative++
			case sa < 0 && sb > 0:
				relation.NegativePositive++
			case sa < 0 && sb < 0:
				relation.NegativeNegative++
			case sa == 0 && sb > 0:
				relation.AZeroBPositive++
			case sa == 0 && sb < 0:
				relation.AZeroBNegative++
			case sa > 0 && sb == 0:
				relation.APositiveBZero++
			case sa < 0 && sb == 0:
				relation.ANegativeBZero++
			}

			// Priority 2: Closest relative magnitude during movement
			if sa != 0 && sb != 0 {
				aBase := math.Abs(grid.Last[a])
				bBase := math.Abs(grid.Last[b])

				var aRelative, bRelative float64
				if aBase > 0 {
					aRelative = math.Abs(delta[a]) / aBase
				} else {
					aRelative = math.Abs(delta[a])
				}

				if bBase > 0 {
					bRelative = math.Abs(delta[b]) / bBase
				} else {
					bRelative = math.Abs(delta[b])
				}

				larger := math.Max(aRelative, bRelative)
				if larger > 0 {
					relation.MagnitudeSimilarity += math.Min(aRelative, bRelative) / larger
					relation.MagnitudeSamples++
				}
			}
		}
	}

	/*
		Spatial Attraction Pass:
		Priority 1: Directional sympathy
		Priority 2: Magnitude similarity (consistency-gated, repulsion on inconsistency)
		Priority 3: Asymmetric movement (stronger moves less, weaker moves more)
	*/

	// Damping factor: step size decays as observations accumulate to aid convergence.
	// The denominator controls how quickly energy drops — 200 keeps enough
	// attraction/repulsion energy for regions to differentiate within typical
	// observation budgets while still converging for settlement.
	dampFactor := 1.0 / (1.0 + float64(grid.Observations)/200.0)

	for i := 0; i < len(labels); i++ {
		a := labels[i]

		for j := i + 1; j < len(labels); j++ {
			b := labels[j]

			// Only move pairs if both were observed in this cycle
			if !currentPresent[a] || !currentPresent[b] {
				continue
			}

			relation := grid.Relations[pair(a, b)]
			if relation == nil {
				continue
			}

			sympathy := relation.sympathy()
			if sympathy == 0 {
				continue
			}

			ax := grid.PosX[a]
			ay := grid.PosY[a]
			bx := grid.PosX[b]
			by := grid.PosY[b]

			dx := bx - ax
			dy := by - ay
			distance := math.Hypot(dx, dy)

			if distance < 1e-4 {
				dx = 0.1
				dy = 0.0
				distance = 0.1
			}

			ux := dx / distance
			uy := dy / distance

			// Priority 3: Asymmetric movement powered by Authority
			aAuth := grid.Authority[a]
			bAuth := grid.Authority[b]
			if aAuth <= 0 {
				aAuth = 1.0
			}
			if bAuth <= 0 {
				bAuth = 1.0
			}

			totalAuth := aAuth + bAuth
			aMove := bAuth / totalAuth // If B is stronger, A moves more toward B
			bMove := aAuth / totalAuth // If A is stronger, B moves more toward A

			// Step size: base attraction/repulsion damped for convergence
			step := 0.25 * sympathy * dampFactor

			grid.PosX[a] += ux * step * aMove
			grid.PosY[a] += uy * step * aMove

			grid.PosX[b] -= ux * step * bMove
			grid.PosY[b] -= uy * step * bMove
		}
	}

	// Update retained observations for delta calculations
	for _, label := range labels {
		if currentPresent[label] {
			grid.Last[label] = current[label]
			grid.Present[label] = true
		} else {
			grid.Present[label] = false
		}
	}

	// Re-form regions and test for stable convergence
	grid.formRegions(labels)

	grid.Observations++
	nextPartition := grid.partitionKey()

	if nextPartition == "" {
		grid.PartitionRun = 0
	}

	if nextPartition != "" && nextPartition == grid.Partition {
		grid.PartitionRun++
	}

	if nextPartition != "" && nextPartition != grid.Partition {
		if grid.Partition != "" && grid.PartitionRun > grid.LongestPartitionRun {
			grid.LongestPartitionRun = grid.PartitionRun
		}

		grid.Partition = nextPartition
		grid.PartitionRun = 1
	}

	if grid.LongestPartitionRun > 0 && grid.PartitionRun > grid.LongestPartitionRun {
		if grid.isSettlementCandidate(labels) {
			grid.Settled = true
		}
	}

	grid.writeCoordinates()

	if decorate {
		grid.decorate(measurement)
	}
}

/*
applySeparationAuthority computes authority via "separation" when SNR/maturity
is unavailable. Metrics are bucketed by movement direction, and authority is
determined by how much the strongest bucket stands out above the mean of the
other buckets.
*/
func (grid *Grid) applySeparationAuthority(
	labels []string,
	state map[string]int,
	delta map[string]float64,
	currentPresent map[string]bool,
	measurement *data.Measurement[float64],
) {
	// Bucket metrics by movement direction
	var positiveSum, negativeSum float64
	var positiveCount, negativeCount int

	for _, label := range labels {
		if !currentPresent[label] || state[label] == 0 {
			continue
		}

		scale := 1.0
		if metric, ok := measurement.LookupMetric(label); ok && metric.Scale > 0 {
			scale = metric.Scale
		} else if stored := grid.find(label); stored != nil && stored.Scale > 0 {
			scale = stored.Scale
		}

		absDelta := math.Abs(delta[label]) / scale

		if state[label] > 0 {
			positiveSum += absDelta
			positiveCount++
		} else {
			negativeSum += absDelta
			negativeCount++
		}
	}

	// Determine which bucket is stronger
	var positiveMean, negativeMean float64
	if positiveCount > 0 {
		positiveMean = positiveSum / float64(positiveCount)
	}
	if negativeCount > 0 {
		negativeMean = negativeSum / float64(negativeCount)
	}

	// Separation: how much the strongest bucket stands out
	strongerMean := math.Max(positiveMean, negativeMean)
	weakerMean := math.Min(positiveMean, negativeMean)
	separation := 1.0
	if weakerMean > 0 {
		separation = strongerMean / weakerMean
	}
	if separation < 1.0 {
		separation = 1.0
	}

	// Metrics in the stronger bucket get higher authority
	strongerSign := 1
	if negativeMean > positiveMean {
		strongerSign = -1
	}

	for _, label := range labels {
		if !currentPresent[label] || state[label] == 0 {
			continue
		}

		if state[label] == strongerSign {
			grid.Authority[label] = separation
		} else {
			grid.Authority[label] = 1.0
		}
	}
}

/*
sympathy computes the net sympathy score for a metric pair.

Priority 1 (direction): consistent co-movement (same or consistently inverse)
scores positively; inconsistent movement scores negatively (repulsion).

Priority 2 (magnitude): similarity of relative magnitude, only added when
the directional relationship is positive (consistent). If directional is
negative, only the repulsive directional score is returned.

The priorities are additive when they co-occur, and each holds standalone.
*/
func (relation *GridRelation) sympathy() float64 {
	if relation == nil {
		return 0
	}

	// Direct movement counts (both move same direction)
	directConsistent := float64(relation.PositivePositive + relation.NegativeNegative)

	// Inverse movement counts: requires BOTH quadrants to be consistent
	// A+ B- only attracts if A- B+ also holds (consistent inverse)
	inverseMin := math.Min(
		float64(relation.PositiveNegative),
		float64(relation.NegativePositive),
	)
	inverseConsistent := 2.0 * inverseMin
	inverseUnmatched := math.Abs(
		float64(relation.PositiveNegative) - float64(relation.NegativePositive),
	)

	// One-sided movements (one moves, other stays) are inconsistent signals
	oneSided := float64(
		relation.AZeroBPositive +
			relation.AZeroBNegative +
			relation.APositiveBZero +
			relation.ANegativeBZero,
	)

	var consistent, inconsistent float64
	if directConsistent >= inverseConsistent {
		consistent = directConsistent
		inconsistent = float64(relation.PositiveNegative+relation.NegativePositive) + oneSided
	} else {
		consistent = inverseConsistent
		inconsistent = inverseUnmatched + directConsistent + oneSided
	}

	total := consistent + inconsistent
	directionalScore := 0.0
	if total > 0 {
		// Normalized directional score in [-1.0, 1.0]
		directionalScore = (consistent - inconsistent) / total
	}

	// Priority 2: Relative magnitude similarity in [0.0, 1.0]
	magnitudeScore := 0.0
	if relation.MagnitudeSamples > 0 {
		magnitudeScore = relation.MagnitudeSimilarity / float64(relation.MagnitudeSamples)
	}

	// Multiply directional score by magnitude similarity so uncorrelated items
	// do not attract simply because they have similar magnitudes.
	return directionalScore * magnitudeScore
}

/*
formRegions assigns each metric label to a region using geometry.Watershed.
Points in 2D space climb along the minimum spanning forest of positive
sympathy edges to density peaks (attractors). Connected components become
multi-cell regions.
*/
func (grid *Grid) formRegions(labels []string) {
	if len(labels) == 0 {
		grid.Regions = make(map[string]uint8)
		return
	}

	grid.Bound = make(map[string]string)

	points := make([]*geometry.Point, len(labels))
	for i, label := range labels {
		auth := grid.Authority[label]
		if auth <= 0 {
			auth = 1.0
		}

		localSympathy := 0.0
		for _, b := range labels {
			if label == b {
				continue
			}
			rel := grid.Relations[pair(label, b)]
			if rel != nil {
				s := rel.sympathy()
				if s > 0 {
					localSympathy += s
				}
			}
		}

		points[i] = &geometry.Point{
			X:         grid.PosX[label],
			Y:         grid.PosY[label],
			Authority: auth * (1.0 + localSympathy),
		}
	}

	n := len(labels)
	edges := make([]geometry.Edge, n*(n-1)/2)
	for right := 1; right < n; right++ {
		for left := 0; left < right; left++ {
			a := labels[left]
			b := labels[right]
			rel := grid.Relations[pair(a, b)]
			sym := 0.0
			if rel != nil {
				sym = rel.sympathy()
			}
			edges[right*(right-1)/2+left] = geometry.Edge{
				Left:     left,
				Right:    right,
				Strength: sym,
			}
		}
	}

	geometry.Watershed{}.Step(points, edges)

	groups := make(map[int][]string)
	for i, label := range labels {
		basin := points[i].Basin
		groups[basin] = append(groups[basin], label)
		grid.Bound[label] = labels[basin]
	}

	basins := make([]int, 0, len(groups))
	for b := range groups {
		basins = append(basins, b)
	}

	if len(basins) > 254 {
		grid.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("hard experiment error: grid generated %d regions (> 254 limit)", len(basins)),
			nil,
		))
		return
	}

	sort.Slice(basins, func(i, j int) bool {
		a := append([]string(nil), groups[basins[i]]...)
		b := append([]string(nil), groups[basins[j]]...)
		sort.Strings(a)
		sort.Strings(b)
		return strings.Join(a, "\x00") < strings.Join(b, "\x00")
	})

	grid.Regions = make(map[string]uint8, len(labels))
	for index, b := range basins {
		region := uint8(index + 1)
		for _, label := range groups[b] {
			grid.Regions[label] = region
		}
	}
}

func (grid *Grid) isSettlementCandidate(labels []string) bool {
	totalCells := len(labels)
	if totalCells <= 1 {
		return false
	}

	totalRegions := grid.totalRegionsLocked()
	// Hard requirement: N metrics becoming N-1 regions MUST fail
	if totalRegions >= totalCells-1 {
		return false
	}

	// Real dimensional compression required (at least 25% dimensional reduction)
	if float64(totalRegions)/float64(totalCells) > 0.75 {
		return false
	}

	// A stable near-singleton partition is NOT settled
	if grid.singletonFractionLocked() >= 0.5 {
		return false
	}

	// Stronger within-region coherence than between-region coherence
	within, between := grid.withinVsBetweenCoherenceLocked()
	if within <= between || within <= 0 {
		return false
	}

	return true
}

func (grid *Grid) totalRegionsLocked() int {
	regions := make(map[uint8]bool)
	for _, r := range grid.Regions {
		if r > 0 {
			regions[r] = true
		}
	}
	return len(regions)
}

func (grid *Grid) singletonFractionLocked() float64 {
	counts := make(map[uint8]int)
	for _, r := range grid.Regions {
		if r > 0 {
			counts[r]++
		}
	}
	if len(counts) == 0 {
		return 0
	}
	singletons := 0
	for _, c := range counts {
		if c == 1 {
			singletons++
		}
	}
	return float64(singletons) / float64(len(counts))
}

func (grid *Grid) withinVsBetweenCoherenceLocked() (within, between float64) {
	var withinSum, betweenSum float64
	var withinCount, betweenCount int

	labels := make([]string, 0, len(grid.Regions))
	for l, r := range grid.Regions {
		if r > 0 {
			labels = append(labels, l)
		}
	}

	for i := 0; i < len(labels); i++ {
		a := labels[i]
		ra := grid.Regions[a]
		for j := i + 1; j < len(labels); j++ {
			b := labels[j]
			rb := grid.Regions[b]
			rel := grid.Relations[pair(a, b)]
			if rel == nil {
				continue
			}
			sym := rel.sympathy()
			if ra == rb {
				withinSum += sym
				withinCount++
			} else {
				betweenSum += sym
				betweenCount++
			}
		}
	}

	if withinCount > 0 {
		within = withinSum / float64(withinCount)
	}
	if betweenCount > 0 {
		between = betweenSum / float64(betweenCount)
	}
	return within, between
}

// TotalCells returns the total number of cells in the grid.
func (grid *Grid) TotalCells() int {
	grid.mu.RLock()
	defer grid.mu.RUnlock()
	return len(grid.Regions)
}

// TotalRegions returns the count of distinct non-zero regions.
func (grid *Grid) TotalRegions() int {
	grid.mu.RLock()
	defer grid.mu.RUnlock()
	return grid.totalRegionsLocked()
}

// MembersPerRegion returns a map of region ID to count of member cells.
func (grid *Grid) MembersPerRegion() map[uint8]int {
	grid.mu.RLock()
	defer grid.mu.RUnlock()
	counts := make(map[uint8]int)
	for _, r := range grid.Regions {
		if r > 0 {
			counts[r]++
		}
	}
	return counts
}

// SingletonFraction returns the fraction of active regions containing exactly one cell.
func (grid *Grid) SingletonFraction() float64 {
	grid.mu.RLock()
	defer grid.mu.RUnlock()
	return grid.singletonFractionLocked()
}

// WithinVsBetweenCoherence returns average sympathy within regions versus across regions.
func (grid *Grid) WithinVsBetweenCoherence() (within, between float64) {
	grid.mu.RLock()
	defer grid.mu.RUnlock()
	return grid.withinVsBetweenCoherenceLocked()
}

// TopRegionFrequency returns the frequency with which each region appears in emitted tokens.
func (grid *Grid) TopRegionFrequency() map[uint8]float64 {
	grid.statsMu.Lock()
	defer grid.statsMu.Unlock()
	if grid.TotalTokens == 0 {
		return nil
	}
	freq := make(map[uint8]float64)
	for r := 1; r < 256; r++ {
		if count := grid.RegionLitCount[r]; count > 0 {
			freq[uint8(r)] = float64(count) / float64(grid.TotalTokens)
		}
	}
	return freq
}

// ContributionBreakdown decomposes a measurement's active lit regions into member cell contributions.
func (grid *Grid) ContributionBreakdown(
	measurement *data.Measurement[float64],
) map[uint8]map[string]float64 {
	if measurement == nil {
		return nil
	}

	grid.mu.RLock()
	defer grid.mu.RUnlock()

	type incomingMetric struct {
		source string
		key    string
		metric data.Metric[float64]
	}

	var allMetrics []incomingMetric

	source := measurement.GetSource()
	for key, metric := range measurement.MetricsSnapshot() {
		allMetrics = append(allMetrics, incomingMetric{source, key, metric})
	}

	for _, peer := range measurement.Peers {
		if peer == nil {
			continue
		}
		peerSource := peer.GetSource()
		for key, metric := range peer.MetricsSnapshot() {
			allMetrics = append(allMetrics, incomingMetric{peerSource, key, metric})
		}
	}

	breakdown := make(map[uint8]map[string]float64)

	for _, item := range allMetrics {
		name := item.metric.Label
		if name == "" {
			name = item.key
		}

		cell := cellKey(measurement.Label, item.source, name)
		region := grid.Regions[cell]
		if region == 0 {
			continue
		}

		act := regionActivity(item.metric)
		if act > 0 {
			if breakdown[region] == nil {
				breakdown[region] = make(map[string]float64)
			}
			breakdown[region][cell] += act
		}
	}

	return breakdown
}

// TokenFrequency returns the count of each emitted token sequence.
func (grid *Grid) TokenFrequency() map[string]int {
	grid.statsMu.Lock()
	defer grid.statsMu.Unlock()
	out := make(map[string]int, len(grid.TokenCounts))
	maps.Copy(out, grid.TokenCounts)
	return out
}

// TokenEntropy returns the Shannon entropy of the emitted token distribution in bits.
func (grid *Grid) TokenEntropy() float64 {
	grid.statsMu.Lock()
	defer grid.statsMu.Unlock()
	if grid.TotalTokens == 0 {
		return 0
	}
	var entropy float64
	for _, count := range grid.TokenCounts {
		if count > 0 {
			p := float64(count) / float64(grid.TotalTokens)
			entropy -= p * math.Log2(p)
		}
	}
	return entropy
}

func (grid *Grid) writeCoordinates() {
	for _, metric := range grid.Metrics {
		if metric == nil {
			continue
		}
		metric.X = int64(math.Round(grid.PosX[metric.Label]))
		metric.Y = int64(math.Round(grid.PosY[metric.Label]))
		metric.Region = grid.Regions[metric.Label]
	}
}


func (grid *Grid) decorate(
	measurement *data.Measurement[float64],
) {
	source := measurement.GetSource()
	for key, incoming := range measurement.MetricsSnapshot() {
		name := incoming.Label

		if name == "" {
			name = key
		}

		stored := grid.find(cellKey(measurement.Label, source, name))
		if stored == nil {
			continue
		}

		incoming.X = stored.X
		incoming.Y = stored.Y
		incoming.Region = stored.Region

		measurement.SetMetric(key, incoming)
	}
}

func (grid *Grid) partitionKey() string {
	if len(grid.Regions) == 0 {
		return ""
	}

	groups := make(map[uint8][]string)
	for label, region := range grid.Regions {
		if region == 0 {
			continue
		}
		groups[region] = append(groups[region], label)
	}

	if len(groups) == 0 {
		return ""
	}

	parts := make([]string, 0, len(groups))
	for _, labels := range groups {
		sort.Strings(labels)
		parts = append(parts, strings.Join(labels, ","))
	}

	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func pair(a, b string) string {
	if a < b {
		return a + "\x00" + b
	}
	return b + "\x00" + a
}

/*
GridSnapshot is the frozen geometry and the per-series memory required to
continue it. Cell keys are symbol and metric name (source is not part of identity).
*/
type GridSnapshot struct {
	Metrics             []*data.Metric[float64]  `json:"metrics"`
	Settled             bool                     `json:"settled"`
	Last                map[string]float64       `json:"last"`
	Present             map[string]bool          `json:"present"`
	Relations           map[string]*GridRelation `json:"relations"`
	Regions             map[string]uint8         `json:"regions"`
	Authority           map[string]float64       `json:"authority"`
	Bound               map[string]string        `json:"bound"`
	PosX                map[string]float64       `json:"pos_x"`
	PosY                map[string]float64       `json:"pos_y"`
	Partition           string                   `json:"partition"`
	Observations        int64                    `json:"observations"`
	PartitionRun        int                      `json:"partition_run"`
	LongestPartitionRun int                      `json:"longest_partition_run"`
}

func (grid *Grid) Snapshot() ([]byte, error) {
	grid.mu.RLock()
	defer grid.mu.RUnlock()

	encoded, err := json.Marshal(GridSnapshot{
		Metrics:             grid.Metrics,
		Settled:             grid.Settled,
		Last:                grid.Last,
		Present:             grid.Present,
		Relations:           grid.Relations,
		Regions:             grid.Regions,
		Authority:           grid.Authority,
		Bound:               grid.Bound,
		PosX:                grid.PosX,
		PosY:                grid.PosY,
		Partition:           grid.Partition,
		Observations:        grid.Observations,
		PartitionRun:        grid.PartitionRun,
		LongestPartitionRun: grid.LongestPartitionRun,
	})

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

	if snapshot.Last == nil || snapshot.Present == nil || snapshot.Relations == nil || snapshot.Regions == nil || snapshot.Authority == nil || snapshot.Bound == nil || snapshot.PosX == nil || snapshot.PosY == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"grid: snapshot is missing a series map",
			nil,
		))
	}

	grid.mu.Lock()
	defer grid.mu.Unlock()

	grid.Metrics = snapshot.Metrics
	grid.Settled = snapshot.Settled
	grid.Last = snapshot.Last
	grid.Present = snapshot.Present
	grid.Relations = snapshot.Relations
	grid.Regions = snapshot.Regions
	grid.Authority = snapshot.Authority
	grid.Bound = snapshot.Bound
	grid.PosX = snapshot.PosX
	grid.PosY = snapshot.PosY
	grid.Partition = snapshot.Partition
	grid.Observations = snapshot.Observations
	grid.PartitionRun = snapshot.PartitionRun
	grid.LongestPartitionRun = snapshot.LongestPartitionRun
	return nil
}
