package broker

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/nomagique/mcts"
)

// SCM Table column mapping
const (
	policyActionColumn = 0 // Treatment (Cut or Trailing Pullback parameter)
	policyPeakColumn   = 1 // Running peak at exit
	policyAgeColumn    = 2 // Relative age in episode
	policySwingColumn  = 3 // Largest mark-to-mark volatility step
	policyBankedColumn = 4 // Target (Realized return)
)

const (
	policyEpisodeCap = 256
	policyIterations = 128
)

type episode struct {
	trace          []float64
	banked         float64
	duration       time.Duration
	timeToPeak     time.Duration
	postExitMaxPct float64
	regretPct      float64
	shadowBars     int
}

type learnedExitPolicy struct {
	invalidationCut float64       // Optimal cut loss threshold
	trailingStop    float64       // Optimal trailing giveback threshold
	maxHoldDuration time.Duration // Empirical lifetime of winning excursions (wall-clock)
	maxHoldBars     int           // Empirical winning horizon in reprice bars (tape timebase)
	worst           float64
	samples         int
}

type policyRecord struct {
	mu       sync.Mutex
	episodes []episode
	rows     [][]float64
	policy   learnedExitPolicy
}

func (desk *Desk) observePolicy(position *Position) {
	if desk == nil || desk.policies == nil || position == nil || position.Symbol == "" || position.PnLPct == nil {
		return
	}

	trace := make([]float64, 0, len(position.PnLTrace))
	for _, val := range position.PnLTrace {
		if !math.IsNaN(val) && !math.IsInf(val, 0) {
			trace = append(trace, val)
		}
	}

	realized := position.PnLPct.Float64()
	if len(trace) == 0 {
		trace = []float64{realized}
	}

	duration := time.Duration(position.HoldNs)
	if duration <= 0 && position.ClosedAt != nil && !position.OpenedAt.IsZero() {
		duration = position.ClosedAt.Sub(position.OpenedAt)
	}

	// Calculate empirical time to high-water mark
	timeToPeak := duration
	peakVal := trace[0]
	peakIdx := 0
	for i, val := range trace {
		if val > peakVal {
			peakVal = val
			peakIdx = i
		}
	}
	if len(trace) > 1 {
		timeToPeak = time.Duration(float64(duration) * (float64(peakIdx) / float64(len(trace)-1)))
	}

	known := desk.ensurePolicyRecord(position.Symbol)
	if known == nil {
		return
	}

	known.mu.Lock()

	known.episodes = append(known.episodes, episode{
		trace:      trace,
		banked:     realized,
		duration:   duration,
		timeToPeak: timeToPeak,
	})

	if len(known.episodes) > policyEpisodeCap {
		known.episodes = append([]episode(nil), known.episodes[len(known.episodes)-policyEpisodeCap:]...)
	}

	if known.policy.samples == 0 || realized < known.policy.worst {
		known.policy.worst = realized
	}

	known.policy.samples++
	known.rows = policyRows(known.episodes)
	symbol := position.Symbol
	known.mu.Unlock()

	desk.persistExitPolicy(symbol)
}

func (desk *Desk) recordPolicy(symbol string) *policyRecord {
	if desk == nil || desk.policies == nil {
		return nil
	}
	val, ok := desk.policies.Load(symbol)
	if !ok {
		return nil
	}
	known, _ := val.(*policyRecord)
	return known
}

func (desk *Desk) ensurePolicyRecord(symbol string) *policyRecord {
	if desk == nil || desk.policies == nil || symbol == "" {
		return nil
	}

	if known := desk.recordPolicy(symbol); known != nil {
		return known
	}

	known := &policyRecord{}
	if actual, loaded := desk.policies.LoadOrStore(symbol, known); loaded {
		return actual.(*policyRecord)
	}

	return known
}

/*
refreshPolicy derives candidate stopping boundaries from empirical quantiles
and executes the Causal MCTS search.
*/
func (desk *Desk) refreshPolicy(symbol string) {
	known := desk.recordPolicy(symbol)
	if known == nil {
		return
	}

	known.mu.Lock()
	desk.refreshPolicyLocked(known)
	known.mu.Unlock()

	if symbol != "" {
		desk.persistExitPolicy(symbol)
	}
}

func (desk *Desk) refreshPolicyLocked(known *policyRecord) {
	if known == nil {
		return
	}

	// Minimum sample check derived from nomagique statistical scale: MinimumPrior = 9
	if float64(len(known.episodes)) < 9 {
		return
	}

	// 1. Candidate Invalidation Cuts: sample quantiles of observed drawdowns
	candidateCuts := extractDrawdownQuantiles(known.episodes)
	if len(candidateCuts) == 0 {
		return
	}

	// 2. Candidate Trailing Pullbacks: sample quantiles of observed givebacks
	candidatePullbacks := extractPullbackQuantiles(known.episodes)
	if len(candidatePullbacks) == 0 {
		return
	}

	// 3. Empirical Max Duration: distribution of time-to-peak for winning excursions
	maxDuration := extractWinningDurationHorizon(known.episodes)
	maxBars := extractWinningBarHorizon(known.episodes)

	search := mcts.NewCausalMCTS(
		mcts.DefaultCausalEngine{},
		1.0,
		0.5,
		int(9),
		policyActionColumn,
		policyBankedColumn,
		[]int{policyPeakColumn, policyAgeColumn, policySwingColumn},
		[]int{policyPeakColumn, policyAgeColumn, policySwingColumn, policyActionColumn},
		true,
	)

	root := &stoppingTreeRoot{
		episodes:           known.episodes,
		rows:               known.rows,
		candidateCuts:      candidateCuts,
		candidatePullbacks: candidatePullbacks,
	}

	bestNode, _, err := search.Search(root, policyIterations, known.rows)

	if err != nil || bestNode == nil {
		return
	}

	optCut, optTrail := extractOptimalPolicy(bestNode)

	if cap := worstCaseAdverseDrop(known.episodes); cap > 0 && optCut > cap {
		optCut = cap
	}

	if optCut > 0 && optTrail > 0 {
		optTrail = widenTrailForRegret(optTrail, known.episodes)
		known.policy.invalidationCut = optCut
		known.policy.trailingStop = optTrail
		known.policy.maxHoldDuration = maxDuration
		known.policy.maxHoldBars = maxBars
	}
}

func widenTrailForRegret(trail float64, episodes []episode) float64 {
	if trail <= 0 {
		return trail
	}

	var regrets []float64
	for _, ep := range episodes {
		if ep.regretPct > 0 {
			regrets = append(regrets, ep.regretPct)
		}
	}

	if len(regrets) == 0 {
		return trail
	}

	sort.Float64s(regrets)
	median := regrets[len(regrets)/2]
	return trail + 0.5*median
}

/*
worstCaseAdverseDrop is the largest entry-to-trough drawdown seen on losing
episodes; it caps how wide the learned invalidation cut can be so stops stay
tight after the tape has already shown deeper damage on this symbol.
*/
func worstCaseAdverseDrop(episodes []episode) float64 {
	worst := 0.0
	for _, ep := range episodes {
		if ep.banked > 0 || len(ep.trace) == 0 {
			continue
		}
		entryVal := ep.trace[0]
		minVal := entryVal
		for _, v := range ep.trace {
			if v < minVal {
				minVal = v
			}
		}
		if drop := entryVal - minVal; drop > worst {
			worst = drop
		}
	}
	return worst
}

func (desk *Desk) refreshPolicies() {
	if desk == nil || desk.policies == nil {
		return
	}

	desk.policies.Range(func(key, _ any) bool {
		if symbol, ok := key.(string); ok && symbol != "" {
			desk.refreshPolicy(symbol)
		}
		return true
	})
}

func (desk *Desk) teachPolicy(position *Position) {
	if position == nil {
		return
	}

	known := desk.recordPolicy(position.Symbol)

	if known == nil {
		return
	}

	known.mu.Lock()
	pol := known.policy
	known.mu.Unlock()

	if pol.samples == 0 {
		return
	}

	if pol.trailingStop > 0 {
		position.ExitPullbackPct = decimal.NewFromFloat64(pol.trailingStop)
	}
	if pol.invalidationCut > 0 {
		position.NurseryCutPct = decimal.NewFromFloat64(pol.invalidationCut)
	}
	if pol.maxHoldDuration > 0 {
		position.MaxLifetimeNs = pol.maxHoldDuration.Nanoseconds()
	}
	if pol.maxHoldBars > 0 {
		position.MaxLifetimeBars = pol.maxHoldBars
	}
	position.WorstPct = decimal.NewFromFloat64(pol.worst)
}

func (desk *Desk) Policy(symbol string) (exitPullbackPct float64, worstPct float64, ok bool) {
	known := desk.recordPolicy(symbol)
	if known == nil {
		return 0, 0, false
	}

	known.mu.Lock()
	defer known.mu.Unlock()

	if known.policy.samples == 0 {
		return 0, 0, false
	}
	return known.policy.trailingStop, known.policy.worst, true
}

// ----------------------------------------------------------------------------
// Empirical Quantile Derivations (Zero Magic Numbers)
// ----------------------------------------------------------------------------

func extractDrawdownQuantiles(episodes []episode) []float64 {
	var winningDips []float64

	for _, ep := range episodes {
		// Only learn cuts from trades that went on to be profitable
		if ep.banked <= 0 || len(ep.trace) == 0 {
			continue
		}

		entryVal := ep.trace[0]
		minVal := entryVal

		for _, v := range ep.trace {
			if v < minVal {
				minVal = v
			}
		}

		// Calculate adverse drop RELATIVE to the entry mark (absorbing initial fees/spread)
		adverseDrop := entryVal - minVal

		if adverseDrop > 0 {
			winningDips = append(winningDips, adverseDrop)
		}
	}

	if len(winningDips) == 0 {
		return nil
	}

	return computeQuantileGrid(winningDips)
}

func extractPullbackQuantiles(episodes []episode) []float64 {
	var pullbacks []float64
	for _, ep := range episodes {
		if len(ep.trace) == 0 {
			continue
		}
		peak := ep.trace[0]
		for _, v := range ep.trace {
			if v > peak {
				peak = v
			}
			diff := peak - v
			if diff > 0 {
				pullbacks = append(pullbacks, diff)
			}
		}

		if ep.regretPct > 0 {
			pullbacks = append(pullbacks, ep.regretPct)
		}
	}

	return computeQuantileGrid(pullbacks)
}

func computeQuantileGrid(data []float64) []float64 {
	if len(data) == 0 {
		return nil
	}

	sort.Float64s(data)
	numSteps := int(9) // 9 quantiles derived from Tolerance = 0.25
	if len(data) < numSteps {
		numSteps = len(data)
	}

	candidates := make([]float64, 0, numSteps)
	seen := make(map[float64]bool)

	for step := 1; step <= numSteps; step++ {
		rank := float64(len(data)-1) * float64(step) / float64(numSteps+1)
		val := data[int(math.Round(rank))]
		if !seen[val] && val > 0 {
			seen[val] = true
			candidates = append(candidates, val)
		}
	}

	sort.Float64s(candidates)
	return candidates
}

func extractWinningDurationHorizon(episodes []episode) time.Duration {
	var winningDurations []time.Duration
	for _, ep := range episodes {
		if ep.banked > 0 && ep.timeToPeak > 0 {
			winningDurations = append(winningDurations, ep.timeToPeak)
		}
	}

	if len(winningDurations) == 0 {
		return 0
	}

	sort.Slice(winningDurations, func(i, j int) bool {
		return winningDurations[i] < winningDurations[j]
	})

	// 1 - Tolerance (75th percentile) of duration for successful runs
	rank := int(math.Round(float64(len(winningDurations)-1) * (1.0 - 0.25)))
	return winningDurations[rank]
}

/*
extractWinningBarHorizon returns the 75th-percentile bar index of the peak of
winning excursions. A bar is one trace mark (one reprice event), so the horizon
lives in the tape timebase the rest of the stop uses, not in wall-clock seconds.
*/
func extractWinningBarHorizon(episodes []episode) int {
	var winningBars []int
	for _, ep := range episodes {
		if ep.banked <= 0 || len(ep.trace) == 0 {
			continue
		}

		peakIdx := 0
		peakVal := ep.trace[0]
		for i, v := range ep.trace {
			if v > peakVal {
				peakVal = v
				peakIdx = i
			}
		}
		winningBars = append(winningBars, peakIdx)
	}

	if len(winningBars) == 0 {
		return 0
	}

	sort.Ints(winningBars)

	rank := int(math.Round(float64(len(winningBars)-1) * (1.0 - 0.25)))
	return winningBars[rank]
}

// ----------------------------------------------------------------------------
// Optimal Stopping MCTS Tree
// ----------------------------------------------------------------------------

type stoppingTreeRoot struct {
	episodes           []episode
	rows               [][]float64
	candidateCuts      []float64
	candidatePullbacks []float64
}

func (r *stoppingTreeRoot) IsTerminal() bool   { return false }
func (r *stoppingTreeRoot) GetReward() float64 { return 0 }
func (r *stoppingTreeRoot) GetPossibleActions() []float64 {
	return append([]float64(nil), r.candidateCuts...)
}
func (r *stoppingTreeRoot) GetInterventionLevel(a float64) float64 { return a }

func (r *stoppingTreeRoot) ApplyAction(cut float64) mcts.State {
	return &invalidationCutNode{
		cut:                cut,
		episodes:           r.episodes,
		rows:               r.rows,
		candidatePullbacks: r.candidatePullbacks,
	}
}

func (r *stoppingTreeRoot) ToVector() []float64 {
	if len(r.rows) == 0 {
		return []float64{0, 0, 0, 0, 0}
	}
	return append([]float64(nil), r.rows[len(r.rows)-1]...)
}

type invalidationCutNode struct {
	cut                float64
	episodes           []episode
	rows               [][]float64
	candidatePullbacks []float64
}

func (n *invalidationCutNode) IsTerminal() bool   { return false }
func (n *invalidationCutNode) GetReward() float64 { return 0 }
func (n *invalidationCutNode) GetPossibleActions() []float64 {
	return append([]float64(nil), n.candidatePullbacks...)
}
func (n *invalidationCutNode) GetInterventionLevel(_ float64) float64 { return n.cut }

func (n *invalidationCutNode) ApplyAction(pullback float64) mcts.State {
	return evaluateStoppingPolicy(n.cut, pullback, n.episodes)
}

func (n *invalidationCutNode) ToVector() []float64 {
	if len(n.episodes) == 0 {
		return []float64{n.cut, 0, 0, 0, 0}
	}
	sim := simulateStoppingPolicy(n.episodes[len(n.episodes)-1], n.cut, n.cut)
	return []float64{n.cut, sim.peak, sim.ageRatio, sim.swing, sim.banked}
}

type terminalStoppingNode struct {
	cut      float64
	pullback float64
	reward   float64
	row      []float64
}

func (t *terminalStoppingNode) IsTerminal() bool                       { return true }
func (t *terminalStoppingNode) GetReward() float64                     { return t.reward }
func (t *terminalStoppingNode) GetPossibleActions() []float64          { return nil }
func (t *terminalStoppingNode) ApplyAction(_ float64) mcts.State       { return t }
func (t *terminalStoppingNode) ToVector() []float64                    { return append([]float64(nil), t.row...) }
func (t *terminalStoppingNode) GetInterventionLevel(_ float64) float64 { return t.pullback }

func evaluateStoppingPolicy(
	cut, pullback float64,
	episodes []episode,
) *terminalStoppingNode {
	if len(episodes) == 0 {
		return &terminalStoppingNode{
			cut: cut, pullback: pullback, reward: 0,
			row: []float64{pullback, 0, 1, 0, 0},
		}
	}

	totalBanked, totalPeak, totalAge, totalSwing := 0.0, 0.0, 0.0, 0.0

	for _, ep := range episodes {
		sim := simulateStoppingPolicy(ep, cut, pullback)
		totalBanked += sim.banked
		totalPeak += sim.peak
		totalAge += sim.ageRatio
		totalSwing += sim.swing
	}

	n := float64(len(episodes))
	return &terminalStoppingNode{
		cut:      cut,
		pullback: pullback,
		reward:   totalBanked / n,
		row:      []float64{pullback, totalPeak / n, totalAge / n, totalSwing / n, totalBanked / n},
	}
}

type policySim struct {
	banked   float64
	peak     float64
	ageRatio float64
	swing    float64
}

/*
simulateStoppingPolicy replays an unconstrained episode under candidate thresholds:
- Exits on invalidation if PnL <= -cut.
- Exits on giveback if PnL <= Peak - pullback (once in profit).
- Otherwise banks the natural trade resolution.
*/
func simulateStoppingPolicy(ep episode, cut, pullback float64) policySim {
	if len(ep.trace) == 0 {
		return policySim{banked: ep.banked}
	}

	entryVal := ep.trace[0]
	peak := entryVal
	running := 0.0
	previous := entryVal
	totalMarks := float64(len(ep.trace))

	for index, value := range ep.trace {
		if value > peak {
			peak = value
		}

		span := math.Abs(value - previous)

		if span > running {
			running = span
		}

		previous = value
		ageRatio := float64(index) / totalMarks

		// 1. Invalidation Cut: adverse move relative to entry mark
		adverseMove := entryVal - value

		if cut > 0 && adverseMove >= cut {
			return policySim{
				banked:   value,
				peak:     peak,
				ageRatio: ageRatio,
				swing:    running,
			}
		}

		// 2. Trailing Profit Stop: giveback from peak once in profit
		if pullback > 0 && peak > 0 && (peak-value) >= pullback {
			return policySim{
				banked:   value,
				peak:     peak,
				ageRatio: ageRatio,
				swing:    running,
			}
		}
	}

	return policySim{banked: ep.banked, peak: peak, ageRatio: 1.0, swing: running}
}

func extractOptimalPolicy(root *mcts.Node) (cut, trail float64) {
	bestCutNode := bestVisitedChild(root)

	if bestCutNode == nil {
		return 0, 0
	}

	cut = bestCutNode.Action
	bestTrailNode := bestVisitedChild(bestCutNode)

	if bestTrailNode == nil {
		return cut, 0
	}

	trail = bestTrailNode.Action
	return cut, trail
}

func bestVisitedChild(node *mcts.Node) *mcts.Node {
	if node == nil || len(node.Children) == 0 {
		return nil
	}

	var best *mcts.Node
	maxVisits := -1

	for _, child := range node.Children {
		if child.Visits > maxVisits {
			maxVisits = child.Visits
			best = child
		}
	}

	return best
}

func policyRows(episodes []episode) [][]float64 {
	rows := make([][]float64, 0, len(episodes)*8)

	for _, current := range episodes {
		if len(current.trace) == 0 {
			continue
		}

		peak := current.trace[0]
		running := 0.0
		previous := current.trace[0]

		for index, value := range current.trace {
			if value > peak {
				peak = value
			}

			span := math.Abs(value - previous)

			if span > running {
				running = span
			}

			previous = value

			rows = append(rows, []float64{
				peak - value,
				peak,
				float64(index) / float64(len(current.trace)),
				running,
				value,
			})
		}
	}

	return rows
}
