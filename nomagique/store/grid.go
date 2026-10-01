package store

import (
	"encoding/json"
	"iter"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
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
	}
}

// Settle locks the current partition.
func (grid *Grid) Settle() {
	grid.Settled = true
}

// ResetSettlement unlocks the grid if further training is required.
func (grid *Grid) ResetSettlement() {
	grid.Settled = false
	grid.PartitionRun = 0
	grid.LongestPartitionRun = 0
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
		grid.formRegions(labels)
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
LitRegions is the region token for this measurement alone. A region is lit
when its activity is above the mean activity of the regions present here.
Peers are not scored: they are not part of the stored measurement, so a token
that included them could not be replayed.
*/
func (grid *Grid) LitRegions(measurement *data.Measurement[float64]) []byte {
	if measurement == nil {
		return nil
	}

	grid.mu.RLock()
	defer grid.mu.RUnlock()

	var activity [256]float64
	var present [256]bool

	source := measurement.GetSource()
	for key, metric := range measurement.MetricsSnapshot() {
		name := metric.Label

		if name == "" {
			name = key
		}

		region := grid.Regions[cellKey(measurement.Label, source, name)]

		if region == 0 {
			continue
		}

		activity[region] += math.Abs(metric.Raw)
		present[region] = true
	}

	type score struct {
		region uint8
		value  float64
	}

	scores := make([]score, 0, 256)
	total := 0.0

	for region := 1; region < 256; region++ {
		if !present[region] {
			continue
		}

		scores = append(scores, score{
			region: uint8(region),
			value:  activity[region],
		})
		total += activity[region]
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

	mean := total / float64(len(scores))
	lit := make([]score, 0, len(scores))

	for _, item := range scores {
		if item.value > mean {
			lit = append(lit, item)
		}
	}

	if len(lit) == 0 {
		lit = scores
	}

	token := make([]byte, len(lit))

	for index := range lit {
		token[index] = lit[index].region
	}

	return token
}

/*
cellKey identifies one impulse-map cell by symbol and metric name.

Source is intentionally excluded: the disruptor shared-slot design mutates one
measurement across concurrent signal/logic stages, so Source is racy provenance
rather than cell identity. Including it made LitRegions miss during historical
replay (preferRows kept a different Source than Training.Update registered) and
blanked precursor_tokens after publish rewrote Source to training:*.
*/
func cellKey(symbol, _source, name string) string {
	return symbol + "\x00" + name
}

// modernizeCellKey rewrites legacy symbol\0source\0name keys from checkpoints.
func modernizeCellKey(label string) string {
	parts := strings.Split(label, "\x00")
	if len(parts) != 3 {
		return label
	}
	return parts[0] + "\x00" + parts[2]
}

func (grid *Grid) update(
	measurement *data.Measurement[float64],
	decorate bool,
) {
	if measurement == nil {
		return
	}

	metrics := measurement.MetricsSnapshot()
	if len(metrics) == 0 {
		return
	}

	source := measurement.GetSource()

	grid.mu.Lock()
	defer grid.mu.Unlock()

	if grid.Settled {
		if decorate {
			grid.decorate(measurement)
		}
		return
	}

	current := make(map[string]float64, len(metrics))
	currentPresent := make(map[string]bool, len(metrics))

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

	for key, incoming := range metrics {
		name := incoming.Label

		if name == "" {
			name = key
		}

		label := cellKey(measurement.Label, source, name)
		current[label] = incoming.Raw
		currentPresent[label] = true
		grid.Authority[label] = baseAuthority

		if grid.find(label) == nil {
			metric := incoming
			metric.Label = label
			grid.Metrics = append(grid.Metrics, &metric)
			grid.place(label)
		}
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

			// Only update pair stats if at least one metric participated
			if !currentPresent[a] && !currentPresent[b] {
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

			// Only move pairs if at least one was observed in this cycle
			if !currentPresent[a] && !currentPresent[b] {
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
		grid.Settled = true
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

	// Priorities are additive when directional is positive (consistent)
	if directionalScore >= 0 {
		return directionalScore + magnitudeScore
	}

	// Contradictory / inconsistent relationships repel
	return directionalScore
}

/*
formRegions assigns each metric label to a region using hill-climbing.
Each label attaches to the nearest label with higher strength, forming a
forest of attractors. Connected components become regions.

Strength is Authority (Priority 3) scaled by total positive sympathy with
neighbors, ensuring that well-connected, authoritative metrics become region
centers.
*/
func (grid *Grid) formRegions(labels []string) {
	if len(labels) == 0 {
		grid.Regions = make(map[string]uint8)
		return
	}

	regionPower := make(map[string]float64)
	for _, root := range grid.Bound {
		regionPower[root] += 1.0
	}

	strength := make(map[string]float64, len(labels))

	for _, a := range labels {
		auth := grid.Authority[a]
		if auth <= 0 {
			auth = 1.0
		}

		localSympathy := 0.0
		for _, b := range labels {
			if a == b {
				continue
			}
			rel := grid.Relations[pair(a, b)]
			if rel != nil {
				s := rel.sympathy()
				if s > 0 {
					localSympathy += s
				}
			}
		}

		// Combined strength: Authority (Priority 3) scaled by sympathetic density
		baseStrength := auth * (1.0 + localSympathy)

		powerMultiplier := 1.0
		root := a
		if boundRoot, ok := grid.Bound[a]; ok {
			root = boundRoot
		}
		if extraPower, ok := regionPower[root]; ok {
			powerMultiplier += extraPower * 0.5
		}

		strength[a] = baseStrength * powerMultiplier
	}

	parent := make(map[string]string, len(labels))

	for _, label := range labels {
		if boundTo, ok := grid.Bound[label]; ok {
			parent[label] = boundTo
			continue
		}

		parent[label] = label
		best := label
		bestDistance := math.Inf(1)

		for _, candidate := range labels {
			if candidate == label {
				continue
			}
			if strength[candidate] <= strength[label] {
				continue
			}

			// Only attach to a stronger neighbor if they share positive sympathy.
			// Without this gate, unrelated metrics form one giant component.
			rel := grid.Relations[pair(label, candidate)]
			if rel == nil || rel.sympathy() <= 0 {
				continue
			}

			distance := math.Hypot(
				grid.PosX[candidate]-grid.PosX[label],
				grid.PosY[candidate]-grid.PosY[label],
			)

			if distance < bestDistance {
				best = candidate
				bestDistance = distance
			}
		}

		if best != label && bestDistance < 0.5 {
			bindRoot := best
			if candidateRoot, ok := grid.Bound[best]; ok {
				bindRoot = candidateRoot
			}
			grid.Bound[label] = bindRoot
		}

		parent[label] = best
	}

	var root func(string, map[string]bool) string
	root = func(label string, visited map[string]bool) string {
		if visited[label] {
			return label // break cycle
		}
		visited[label] = true
		next := parent[label]
		if next == label {
			return label
		}
		parent[label] = root(next, visited)
		return parent[label]
	}

	groups := make(map[string][]string)
	for _, label := range labels {
		r := root(label, make(map[string]bool))
		groups[r] = append(groups[r], label)
	}

	roots := make([]string, 0, len(groups))
	for r := range groups {
		roots = append(roots, r)
	}

	sort.Slice(roots, func(i, j int) bool {
		a := append([]string(nil), groups[roots[i]]...)
		b := append([]string(nil), groups[roots[j]]...)
		sort.Strings(a)
		sort.Strings(b)
		return strings.Join(a, "\x00") < strings.Join(b, "\x00")
	})

	grid.Regions = make(map[string]uint8, len(labels))
	for index, r := range roots {
		if index >= 255 {
			break
		}
		region := uint8(index + 1)
		for _, label := range groups[r] {
			grid.Regions[label] = region
		}
	}
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

func (grid *Grid) modernizeLegacyKeys() {
	rewrite := func(store map[string]float64) {
		if store == nil {
			return
		}
		next := make(map[string]float64, len(store))
		for key, value := range store {
			next[modernizeCellKey(key)] = value
		}
		for key := range store {
			delete(store, key)
		}
		for key, value := range next {
			store[key] = value
		}
	}
	rewriteBool := func(store map[string]bool) {
		if store == nil {
			return
		}
		next := make(map[string]bool, len(store))
		for key, value := range store {
			next[modernizeCellKey(key)] = value
		}
		for key := range store {
			delete(store, key)
		}
		for key, value := range next {
			store[key] = value
		}
	}
	rewriteU8 := func(store map[string]uint8) {
		if store == nil {
			return
		}
		next := make(map[string]uint8, len(store))
		for key, value := range store {
			next[modernizeCellKey(key)] = value
		}
		for key := range store {
			delete(store, key)
		}
		for key, value := range next {
			store[key] = value
		}
	}
	rewriteStr := func(store map[string]string) {
		if store == nil {
			return
		}
		next := make(map[string]string, len(store))
		for key, value := range store {
			next[modernizeCellKey(key)] = modernizeCellKey(value)
		}
		for key := range store {
			delete(store, key)
		}
		for key, value := range next {
			store[key] = value
		}
	}

	rewrite(grid.Last)
	rewriteBool(grid.Present)
	rewrite(grid.Authority)
	rewrite(grid.PosX)
	rewrite(grid.PosY)
	rewriteU8(grid.Regions)
	rewriteStr(grid.Bound)

	oldToNew := make(map[string]string, len(grid.Metrics))
	for _, metric := range grid.Metrics {
		if metric == nil || metric.Label == "" {
			continue
		}
		oldToNew[metric.Label] = modernizeCellKey(metric.Label)
	}

	if grid.Relations != nil {
		next := make(map[string]*GridRelation, len(grid.Relations))
		labels := make([]string, 0, len(oldToNew))
		for old := range oldToNew {
			labels = append(labels, old)
		}
		sort.Strings(labels)
		for i := 0; i < len(labels); i++ {
			for j := i + 1; j < len(labels); j++ {
				oldKey := pair(labels[i], labels[j])
				rel := grid.Relations[oldKey]
				if rel == nil {
					continue
				}
				next[pair(oldToNew[labels[i]], oldToNew[labels[j]])] = rel
			}
		}
		grid.Relations = next
	}

	for _, metric := range grid.Metrics {
		if metric != nil {
			metric.Label = modernizeCellKey(metric.Label)
		}
	}

	grid.Partition = ""
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
	grid.modernizeLegacyKeys()
	return nil
}
