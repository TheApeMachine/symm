package advisor

import (
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/symm/types"
)

/*
MarketMove is one of the seven qualitative reactions the market can answer an
action with. The market does not move in currency increments at decision time;
it transitions between recognizable regimes, and the search branches over those.
*/
type MarketMove int

const (
	MoveExplosivePump      MarketMove = 3
	MoveSteadyTrend        MarketMove = 2
	MoveWeakDrift          MarketMove = 1
	MoveStagnant           MarketMove = 0
	MoveWeakBleed          MarketMove = -1
	MoveStructuralPullback MarketMove = -2
	MoveFlashDump          MarketMove = -3
)

/* AllMarketMoves is the complete reaction space, strongest bull to strongest bear. */
var AllMarketMoves = []MarketMove{
	MoveExplosivePump,
	MoveSteadyTrend,
	MoveWeakDrift,
	MoveStagnant,
	MoveWeakBleed,
	MoveStructuralPullback,
	MoveFlashDump,
}

/* String returns the move's display name. */
func (move MarketMove) String() string {
	switch move {
	case MoveExplosivePump:
		return "explosive_pump"
	case MoveSteadyTrend:
		return "steady_trend"
	case MoveWeakDrift:
		return "weak_drift"
	case MoveStagnant:
		return "stagnant"
	case MoveWeakBleed:
		return "weak_bleed"
	case MoveStructuralPullback:
		return "structural_pullback"
	case MoveFlashDump:
		return "flash_dump"
	default:
		return "unknown"
	}
}

/*
InteractionType is the semantic relationship between two advisor conclusions.
*/
type InteractionType uint8

const (
	InteractionSynergy InteractionType = iota + 1
	InteractionVeto
)

/*
SemanticRule declares how two advisor states interact physically.

A rule is a statement about market mechanics, not a tuned weight: order-flow
absorption physically invalidates kinematic momentum, and a bid wall forming
immediately after a liquidity sweep is accumulation. Rules carry the reason so
a deliberation can be read back and argued with.
*/
type SemanticRule struct {
	AdvisorA string
	StateA   string
	AdvisorB string
	StateB   string
	Type     InteractionType
	Impact   MarketMove
	Reason   string
}

/*
DeliberationOutcome is the War Room's synthesized consensus: one coherent
distribution over the seven market moves, plus the vetoes and synergies that
shaped it.
*/
type DeliberationOutcome struct {
	Probabilities      map[MarketMove]float64 `json:"probabilities"`
	DominantMove       MarketMove             `json:"dominantMove"`
	Confidence         float64                `json:"confidence"`
	Vetoes             []string               `json:"vetoes,omitempty"`
	Synergies          []string               `json:"synergies,omitempty"`
	Participants       int                    `json:"participants"`
	IndependentSources int                    `json:"independentSources,omitempty"`
	Advisors           []types.AdvisorOpinion `json:"advisors,omitempty"`
	// Silent explains every advisor that contributed nothing to this round.
	Silent []types.AdvisorSilence `json:"silent,omitempty"`
	// UnmappedClasses names advisor classes ("advisor:class") whose weight no
	// projection rule accepted, so it never reached the consensus mass. Empty
	// in a correct build; non-empty means this outcome under-counts its own
	// evidence.
	UnmappedClasses []string  `json:"unmappedClasses,omitempty"`
	At              time.Time `json:"at"`
}

func advisorSourceGroup(advisor string) string {
	switch strings.ToLower(advisor) {
	case "auction", "depth", "book_depth", "wall_building", "liquidity_sweep":
		return "BookDepth"
	case "basis", "cross_venue", "futures", "derivatives":
		return "Derivatives"
	case "liquidity", "replenishment", "withdrawal":
		return "Liquidity"
	case "momentum", "hawkes", "excitation", "branching":
		return "Hawkes"
	case "participation", "flow", "cvd", "order_flow":
		return "OrderFlow"
	case "profit_run", "pullback", "trend":
		return "TrendDynamics"
	default:
		return advisor
	}
}

/*
WarRoom cross-examines advisor perspectives and maintains their credibility.

Advisors do not cast isolated votes that get averaged. Each one's conclusion
either reinforces, qualifies, or invalidates another's, and the credibility
ledger scales how loudly each speaks based on whether its past claims survived
contact with the market.
*/
type WarRoom struct {
	mu          sync.RWMutex
	rules       []SemanticRule
	credibility map[string]float64
	// resident holds the latest perspective each advisor issued, keyed by
	// symbol then advisor name.
	//
	// This cache is what makes deliberation possible at all. Advisors are
	// clocked to pumpdump/completed_volume_bar_ordinal, so they speak only on
	// trade envelopes when a volume bar closes. The planner decides on ticker
	// envelopes, which carry an empty Perspectives slice. Without retaining
	// what the council last said, every deliberation would find zero
	// participants and the system would never trade.
	resident map[string]map[string]*types.Perspective
	clocks   map[string]map[string]uint64

	// silences holds why each advisor last failed to publish, by symbol then
	// advisor. It is retained for the same reason perspectives are: advisors
	// speak on trade envelopes and decisions are taken on ticker envelopes, so
	// a round that did not retain this would have nothing to report.
	silences map[string]map[string]types.AdvisorSilence
}

/*
KnownAdvisors is the full council. A round reports against this roster rather
than against whoever happened to speak, so an advisor that has never once
published is visible as an empty seat instead of simply being absent from the
output.
*/
var KnownAdvisors = []string{
	MomentumName,
	AuctionName,
	ParticipationName,
	PullbackName,
	ProfitRunName,
	LiquidityName,
	BasisName,
}

/*
clockNow is the coordinate a symbol's market clock currently stands at, which
is what dates a lease. Zero when the clock has not been observed.

The caller already holds the read lock.
*/
func (room *WarRoom) clockNow(symbol, clock string) uint64 {
	if clock == "" {
		return 0
	}

	return room.clocks[symbol][clock]
}

/*
silentAdvisors explains every advisor that is not in the active set.

An advisor whose reading has expired is reported against the clock that expired
it; one that never published is reported with the declared evidence it is still
missing. An advisor this room has never heard from at all is reported as
incomplete with nothing named, which is itself the finding.

The caller already holds the read lock.
*/
func (room *WarRoom) silentAdvisors(
	symbol string,
	active map[string]*types.Perspective,
) []types.AdvisorSilence {
	silent := make([]types.AdvisorSilence, 0)

	for _, name := range KnownAdvisors {
		if _, speaking := active[name]; speaking {
			continue
		}

		if perspective, resident := room.resident[symbol][name]; resident {
			now := room.clockNow(symbol, perspective.Lease.Clock)

			if perspective.Lease.Until > 0 && now > perspective.Lease.Until {
				silent = append(silent, types.AdvisorSilence{
					Advisor:    name,
					Reason:     "expired",
					LeaseUntil: perspective.Lease.Until,
					ClockNow:   now,
				})

				continue
			}
		}

		recorded, found := room.silences[symbol][name]

		if !found {
			silent = append(silent, types.AdvisorSilence{
				Advisor: name,
				Reason:  "incomplete",
			})

			continue
		}

		silent = append(silent, recorded)
	}

	return silent
}

/*
Note records why advisors could not publish on one envelope, so a later round
can say what a silent seat was waiting for.
*/
func (room *WarRoom) Note(symbol string, silences []types.AdvisorSilence) {
	if room == nil || symbol == "" || len(silences) == 0 {
		return
	}

	room.mu.Lock()
	defer room.mu.Unlock()

	if room.silences == nil {
		room.silences = make(map[string]map[string]types.AdvisorSilence)
	}

	bySymbol, found := room.silences[symbol]

	if !found {
		bySymbol = make(map[string]types.AdvisorSilence)
		room.silences[symbol] = bySymbol
	}

	for _, silence := range silences {
		if silence.Advisor == "" {
			continue
		}

		bySymbol[silence.Advisor] = silence
	}
}

/*
credibilityFloor keeps a discredited advisor audible. An advisor that has been
wrong repeatedly should lose influence, but silencing it entirely would prevent
it from ever demonstrating recovery.
*/
const credibilityFloor = 0.10

/*
baselineMaturity is the influence an advisor carries before its predictions
have survived enough rounds to establish an empirical record. It is a discount,
not a silencing: an unproven reading should count for less than a proven one,
but counting for nothing makes a cold start indistinguishable from silence.
*/
const baselineMaturity = 0.4

/* NewWarRoom builds the deliberation table with the default physical rules. */
func NewWarRoom() *WarRoom {
	room := &WarRoom{
		rules:       defaultSemanticRules(),
		credibility: make(map[string]float64),
		resident:    make(map[string]map[string]*types.Perspective),
		clocks:      make(map[string]map[string]uint64),
	}

	for _, name := range []string{
		"momentum", "auction", "pullback", "liquidity",
		"basis", "participation", "profit_run",
	} {
		room.credibility[name] = 1.0
	}

	return room
}

/*
defaultSemanticRules encodes physical compatibility across advisor domains.
Every state named here is a declared class label of the advisor it belongs to.
*/
func defaultSemanticRules() []SemanticRule {
	return []SemanticRule{
		{
			AdvisorA: "momentum", StateA: "Building",
			AdvisorB: "auction", StateB: "SellersAbsorbing",
			Type: InteractionVeto, Impact: MoveStagnant,
			Reason: "sellers absorb every market buy at the ceiling; building momentum is a bull trap",
		},
		{
			AdvisorA: "momentum", StateA: "Building",
			AdvisorB: "liquidity", StateB: "VacuumForming",
			Type: InteractionVeto, Impact: MoveFlashDump,
			Reason: "bids are hollowing out beneath the move; upward momentum has no structural floor",
		},
		{
			AdvisorA: "profit_run", StateA: "Exhausting",
			AdvisorB: "liquidity", StateB: "Depleting",
			Type: InteractionVeto, Impact: MoveFlashDump,
			Reason: "retail is buying into an exhausted book; the pumper's exit liquidity is imminent",
		},
		{
			AdvisorA: "pullback", StateA: "StructuralBreakdown",
			AdvisorB: "auction", StateB: "SellersBreakingThrough",
			Type: InteractionVeto, Impact: MoveStructuralPullback,
			Reason: "structural support failed while sellers break through; the level is gone",
		},
		{
			AdvisorA: "pullback", StateA: "LiquiditySweep",
			AdvisorB: "liquidity", StateB: "WallBuilding",
			Type: InteractionSynergy, Impact: MoveExplosivePump,
			Reason: "a bid wall formed immediately after the sweep; accumulation is confirmed",
		},
		{
			AdvisorA: "auction", StateA: "BuyersBreakingThrough",
			AdvisorB: "basis", StateB: "LeverageSqueeze",
			Type: InteractionSynergy, Impact: MoveExplosivePump,
			Reason: "order-book breakout coupled with a futures short-squeeze cascade",
		},
		{
			AdvisorA: "momentum", StateA: "Sustaining",
			AdvisorB: "participation", StateB: "BroadLift",
			Type: InteractionSynergy, Impact: MoveSteadyTrend,
			Reason: "market-wide breadth confirms a genuine systemic trend, not an isolated move",
		},
		{
			AdvisorA: "pullback", StateA: "OrderlyPullback",
			AdvisorB: "basis", StateB: "DiscountExpanding",
			Type: InteractionSynergy, Impact: MoveExplosivePump,
			Reason: "shorts pile into an orderly pullback; the coil is loading",
		},
	}
}

/*
priorMass is the deliberation's starting belief before any advisor speaks:
centered on stagnation, symmetric, with the violent tails rare. It encodes no
directional opinion of its own.
*/
func priorMass() map[MarketMove]float64 {
	return map[MarketMove]float64{
		MoveExplosivePump:      0.05,
		MoveSteadyTrend:        0.15,
		MoveWeakDrift:          0.15,
		MoveStagnant:           0.30,
		MoveWeakBleed:          0.15,
		MoveStructuralPullback: 0.15,
		MoveFlashDump:          0.05,
	}
}

/*
Deliberate cross-examines the active perspectives for one symbol and returns
the synthesized distribution over the seven market moves.

Each advisor first projects its own lean onto the move space, weighted by its
probability, its credibility, and its empirical maturity. The semantic rules
then apply: vetoes suppress the moves they invalidate, synergies amplify the
move their combination predicts.
*/
func (room *WarRoom) Deliberate(
	perspectives []*types.Perspective,
	symbol string,
	at time.Time,
) *DeliberationOutcome {
	active := room.Admit(perspectives, symbol)

	if len(active) == 0 {
		room.mu.RLock()
		silent := room.silentAdvisors(symbol, active)
		room.mu.RUnlock()

		return &DeliberationOutcome{
			Silent:        silent,
			Participants:  0,
			At:            at,
			Probabilities: priorMass(),
			DominantMove:  MoveStagnant,
			Confidence:    0,
		}
	}

	room.mu.RLock()
	defer room.mu.RUnlock()

	mass := priorMass()

	// Classes no projection rule recognises. Their weight never reaches the
	// mass, so the round must say so rather than quietly under-counting.
	var unmapped []string

	uniqueSources := make(map[string]int)

	for name := range active {
		group := advisorSourceGroup(name)
		uniqueSources[group]++
	}

	advisors := make([]types.AdvisorOpinion, 0, len(active))

	for name, perspective := range active {
		group := advisorSourceGroup(name)
		groupCount := uniqueSources[group]
		discount := 1.0

		if groupCount > 1 {
			discount = 1.0 / math.Sqrt(float64(groupCount))
		}

		// The advisor's own mass is projected twice: once into the shared
		// consensus, and once into a scratch map that is this advisor's
		// contribution alone. Reading a single advisor's effect back out of the
		// shared total afterwards is not possible once several have added to it.
		own := make(map[MarketMove]float64, len(AllMarketMoves))
		var ownUnmapped []string

		room.project(mass, name, perspective, discount, &unmapped)
		room.project(own, name, perspective, discount, &ownUnmapped)

		topState := string(perspective.TopClass())
		probability, _ := perspective.Probability(types.PerspectiveState(topState))
		credibility := room.credibilityOf(name)
		maturity := perspective.Maturity()

		if maturity < baselineMaturity {
			maturity = baselineMaturity
		}

		factor := credibility * maturity * discount

		classes := make([]types.AdvisorClass, 0, len(perspective.Classes))

		for _, class := range perspective.Classes {
			classes = append(classes, types.AdvisorClass{
				State:       string(class.State),
				Probability: class.Probability,
			})
		}

		contribution := make([]types.AdvisorMoveMass, 0, len(own))

		for _, move := range AllMarketMoves {
			if own[move] <= 0 {
				continue
			}

			contribution = append(contribution, types.AdvisorMoveMass{
				Move: move.String(),
				Mass: own[move],
			})
		}

		advisors = append(advisors, types.AdvisorOpinion{
			Advisor:      name,
			State:        topState,
			Probability:  probability,
			Credibility:  credibility,
			Weight:       factor,
			Classes:      classes,
			Maturity:     maturity,
			Contribution: contribution,
			Unmapped:     ownUnmapped,
			Unscored:     append([]string(nil), perspective.Unscored...),
			Clock:        perspective.Lease.Clock,
			LeaseFrom:    perspective.Lease.From,
			LeaseUntil:   perspective.Lease.Until,
			ClockNow:     room.clockNow(symbol, perspective.Lease.Clock),
		})
	}

	slices.SortFunc(advisors, func(left, right types.AdvisorOpinion) int {
		if left.Advisor < right.Advisor {
			return -1
		}

		return 1
	})

	outcome := &DeliberationOutcome{
		Participants:       len(active),
		IndependentSources: len(uniqueSources),
		Advisors:           advisors,
		Silent:             room.silentAdvisors(symbol, active),
		UnmappedClasses:    unmapped,
		At:                 at,
	}

	room.crossExamine(mass, active, outcome)
	normalize(mass, outcome)

	return outcome
}

/*
Evict removes an advisor's resident perspective for a symbol.
*/
func (room *WarRoom) Evict(symbol, advisor string) {
	room.mu.Lock()
	defer room.mu.Unlock()

	seats, found := room.resident[symbol]

	if !found {
		return
	}

	delete(seats, advisor)

	if len(seats) == 0 {
		delete(room.resident, symbol)
	}
}

/*
Admit folds any freshly issued perspectives into the resident council and
returns the council as it now stands for one symbol.

Perspectives arrive on trade envelopes and decisions are made on ticker
envelopes, so the council has to persist between the two. Each advisor holds
one seat per symbol: a new perspective replaces that advisor's previous one
rather than accumulating, so the council reflects the latest reading and cannot
grow without bound.
*/
func (room *WarRoom) Admit(
	perspectives []*types.Perspective,
	symbol string,
) map[string]*types.Perspective {
	room.mu.Lock()
	defer room.mu.Unlock()

	for _, perspective := range perspectives {
		if perspective == nil || perspective.Err != nil {
			continue
		}

		if perspective.Symbol == "" {
			continue
		}

		name := perspective.Advisor

		if name == "" {
			continue
		}

		seats := room.resident[perspective.Symbol]

		if perspective.Lifecycle == types.PerspectiveExpired ||
			perspective.Lifecycle == types.PerspectiveFalsified {
			if seats != nil {
				delete(seats, name)

				if len(seats) == 0 {
					delete(room.resident, perspective.Symbol)
				}
			}

			continue
		}

		if seats == nil {
			seats = make(map[string]*types.Perspective)
			room.resident[perspective.Symbol] = seats
		}

		if existing, exists := seats[name]; exists && existing != nil {
			if perspective.Sequence > 0 && existing.Sequence > 0 && perspective.Sequence <= existing.Sequence {
				continue
			}

			if !perspective.IssuedAt.IsZero() && !existing.IssuedAt.IsZero() && perspective.IssuedAt.Before(existing.IssuedAt) {
				continue
			}

			if perspective.Lease.Clock != "" && existing.Lease.Clock == perspective.Lease.Clock && perspective.Lease.From < existing.Lease.From {
				continue
			}
		}

		if perspective.Lease.Clock != "" {
			symbolClocks := room.clocks[perspective.Symbol]

			if symbolClocks == nil {
				symbolClocks = make(map[string]uint64)
				room.clocks[perspective.Symbol] = symbolClocks
			}

			if perspective.Lease.From > symbolClocks[perspective.Lease.Clock] {
				symbolClocks[perspective.Lease.Clock] = perspective.Lease.From
			}
		}

		// Clone on admission: the envelope's slice is recycled downstream,
		// and a retained pointer into it would mutate under the council.
		retained := perspective.Clone()
		seats[name] = &retained
	}

	if symbol == "" {
		return nil
	}

	seats := room.resident[symbol]

	if seats == nil {
		return nil
	}

	symbolClocks := room.clocks[symbol]
	active := make(map[string]*types.Perspective, len(seats))

	for name, perspective := range seats {
		if perspective.Lease.Clock != "" && perspective.Lease.Until > 0 && symbolClocks != nil {
			currentCoord := symbolClocks[perspective.Lease.Clock]

			if currentCoord > perspective.Lease.Until {
				delete(seats, name)
				continue
			}
		}

		active[name] = perspective
	}

	if len(seats) == 0 {
		delete(room.resident, symbol)
	}

	return active
}

/*
project applies one advisor's full perspective distribution to the move mass,
weighted by probability, credibility, maturity, and source group discount.
*/
func (room *WarRoom) project(
	mass map[MarketMove]float64,
	name string,
	perspective *types.Perspective,
	discount float64,
	unmapped *[]string,
) {
	if perspective == nil {
		return
	}

	credibility := room.credibilityOf(name)

	maturity := perspective.Maturity()

	if maturity < baselineMaturity {
		maturity = baselineMaturity
	}

	if discount <= 0 {
		discount = 1.0
	}

	factor := credibility * maturity * discount

	if factor <= 0 {
		return
	}

	if len(perspective.Classes) == 0 {
		topClass := string(perspective.TopClass())
		probability, found := perspective.Probability(types.PerspectiveState(topClass))

		if !found {
			return
		}

		if !projectClass(mass, topClass, probability*factor) {
			*unmapped = append(*unmapped, name+":"+topClass)
		}

		return
	}

	for _, pc := range perspective.Classes {
		if pc.Probability <= 0 {
			continue
		}

		if !projectClass(mass, string(pc.State), pc.Probability*factor) {
			*unmapped = append(*unmapped, name+":"+string(pc.State))
		}
	}
}

/*
projectClass adds one advisor class's weight to the moves that class asserts.

It reports whether the class was recognised. An unmapped class is a contract
failure, not a neutral event: its weight would otherwise vanish from the
consensus while the advisor still appears to have spoken, so the council would
silently deliberate on a fraction of the evidence it was given. Every label any
Advisor can emit must appear here, and TestProjectClassCoversEveryAdvisorClass
holds that closed.
*/
func projectClass(mass map[MarketMove]float64, className string, weight float64) bool {
	switch className {
	case "Building", "BuyersBreakingThrough":
		mass[MoveExplosivePump] += weight * 1.5
		mass[MoveSteadyTrend] += weight
	case "Sustaining", "BroadLift", "Extending", "Replenishing":
		mass[MoveSteadyTrend] += weight * 1.2
	case "LiquiditySweep", "WallBuilding", "LeverageSqueeze", "DiscountExpanding":
		mass[MoveExplosivePump] += weight
		mass[MoveWeakDrift] += weight * 0.5
	case "Stalling", "Balanced", "Consolidating", "NeutralBasis", "Unresolved":
		mass[MoveStagnant] += weight * 1.5
	case "OrderlyPullback", "FollowerMove":
		mass[MoveStructuralPullback] += weight
		mass[MoveStagnant] += weight * 0.5
	case "SellersAbsorbing", "BuyersAbsorbing":
		mass[MoveStagnant] += weight
		mass[MoveWeakBleed] += weight * 0.5
	case "Reversing", "Exhausting", "GivingBack", "Depleting", "PremiumExpanding":
		mass[MoveStructuralPullback] += weight
		mass[MoveFlashDump] += weight * 0.8
	case "SellersBreakingThrough", "StructuralBreakdown", "VacuumForming",
		"LiquidationsCascading":
		mass[MoveFlashDump] += weight * 1.5
		mass[MoveStructuralPullback] += weight

	// This instrument is moving ahead of its peer group: a real directional
	// impulse, but a narrow one that the group has not yet joined.
	case "LocalLeader":
		mass[MoveSteadyTrend] += weight
		mass[MoveExplosivePump] += weight * 0.5

	// This instrument is moving with no peer participation at all. The move is
	// real but unconfirmed, which is the shape that reverts rather than trends.
	case "IsolatedMove":
		mass[MoveWeakDrift] += weight
		mass[MoveStagnant] += weight * 0.5

	default:
		return false
	}

	return true
}

/*
MoveForState is the market move one advisor class asserts.

It is the same reading of the class vocabulary that project uses to build the
consensus mass, stated once so accountability and deliberation cannot drift
apart: an advisor must be judged against the move it actually argued for.
*/
func MoveForState(state string) MarketMove {
	switch state {
	case "Building", "BuyersBreakingThrough",
		"LiquiditySweep", "WallBuilding", "LeverageSqueeze", "DiscountExpanding":
		return MoveExplosivePump
	case "Sustaining", "BroadLift", "Extending", "Replenishing":
		return MoveSteadyTrend
	case "OrderlyPullback", "FollowerMove":
		return MoveStructuralPullback
	case "SellersAbsorbing", "BuyersAbsorbing":
		return MoveWeakBleed
	case "Reversing", "Exhausting", "GivingBack", "Depleting", "PremiumExpanding":
		return MoveStructuralPullback
	case "SellersBreakingThrough", "StructuralBreakdown", "VacuumForming",
		"LiquidationsCascading":
		return MoveFlashDump
	}

	return MoveStagnant
}

/*
MoveForReturn maps an observed fractional price return to a qualitative MarketMove.
*/
func MoveForReturn(returnFrac float64) MarketMove {
	if returnFrac >= 0.02 {
		return MoveExplosivePump
	}

	if returnFrac >= 0.005 {
		return MoveSteadyTrend
	}

	if returnFrac >= 0.001 {
		return MoveWeakDrift
	}

	if returnFrac <= -0.02 {
		return MoveFlashDump
	}

	if returnFrac <= -0.005 {
		return MoveStructuralPullback
	}

	if returnFrac <= -0.001 {
		return MoveWeakBleed
	}

	return MoveStagnant
}

/*
crossExamine applies the semantic rules to the accumulated mass. A veto is a
suppression of the moves it invalidates, not a subtraction from a score; a
synergy is an amplification of the move the combination physically predicts.
*/
func (room *WarRoom) crossExamine(
	mass map[MarketMove]float64,
	active map[string]*types.Perspective,
	outcome *DeliberationOutcome,
) {
	for _, rule := range room.rules {
		first, foundFirst := active[rule.AdvisorA]
		second, foundSecond := active[rule.AdvisorB]

		if !foundFirst || !foundSecond {
			continue
		}

		if string(first.TopClass()) != rule.StateA ||
			string(second.TopClass()) != rule.StateB {
			continue
		}

		probabilityA, _ := first.Probability(types.PerspectiveState(rule.StateA))
		probabilityB, _ := second.Probability(types.PerspectiveState(rule.StateB))

		maturityA := first.Maturity()

		if maturityA < baselineMaturity {
			maturityA = baselineMaturity
		}

		maturityB := second.Maturity()

		if maturityB < baselineMaturity {
			maturityB = baselineMaturity
		}

		joint := probabilityA * probabilityB *
			room.credibilityOf(rule.AdvisorA) * room.credibilityOf(rule.AdvisorB) *
			maturityA * maturityB

		if joint <= 0 {
			continue
		}

		switch rule.Type {
		case InteractionVeto:
			outcome.Vetoes = append(outcome.Vetoes, rule.Reason)
			// Suppression is proportional to how strongly the veto is held,
			// so a marginal veto does not erase a strong bullish reading.
			suppression := 1 - joint

			if suppression < 0.05 {
				suppression = 0.05
			}

			mass[MoveExplosivePump] *= suppression
			mass[MoveSteadyTrend] *= suppression
			mass[rule.Impact] += joint * 3

		case InteractionSynergy:
			outcome.Synergies = append(outcome.Synergies, rule.Reason)
			mass[rule.Impact] += joint * 4
		}
	}
}

/*
normalize converts accumulated mass into a probability distribution and records
the dominant move. A small floor keeps every move reachable: the market is
never strictly incapable of a reaction, and a zeroed branch would make the
search blind to it.
*/
func normalize(mass map[MarketMove]float64, outcome *DeliberationOutcome) {
	total := 0.0

	for _, move := range AllMarketMoves {
		if mass[move] < 0.01 {
			mass[move] = 0.01
		}

		total += mass[move]
	}

	outcome.Probabilities = make(map[MarketMove]float64, len(AllMarketMoves))
	outcome.DominantMove = MoveStagnant
	outcome.Confidence = 0

	for _, move := range AllMarketMoves {
		probability := mass[move] / total
		outcome.Probabilities[move] = probability

		if probability > outcome.Confidence {
			outcome.Confidence = probability
			outcome.DominantMove = move
		}
	}
}

/* credibilityOf returns one advisor's current credibility, floored. */
func (room *WarRoom) credibilityOf(name string) float64 {
	credibility, found := room.credibility[name]

	if !found || credibility <= 0 {
		return credibilityFloor
	}

	return credibility
}

/* Credibility reports one advisor's current standing at the table. */
func (room *WarRoom) Credibility(name string) float64 {
	room.mu.RLock()
	defer room.mu.RUnlock()

	return room.credibilityOf(name)
}

/*
UpdateCredibility adjusts an advisor's standing after the market resolves.

An advisor that imposed a veto is held to a higher standard than one that
merely offered an opinion: blocking a move that then happened is the expensive
error, and it is penalized harder than an ordinary miss.
*/
func (room *WarRoom) UpdateCredibility(
	name string,
	wasVeto bool,
	realized MarketMove,
	predicted MarketMove,
) {
	room.mu.Lock()
	defer room.mu.Unlock()

	current, found := room.credibility[name]

	if !found {
		current = 1.0
	}

	if wasVeto {
		if realized <= MoveStagnant {
			// The veto was right: an ambush was avoided.
			current += (1 - current) * 0.15
		} else if realized >= MoveSteadyTrend {
			// The veto blocked a real move. This is the costly error.
			current *= 0.80
		}
	} else {
		if math.Abs(float64(realized-predicted)) <= 1 {
			current += (1 - current) * 0.05
		} else {
			current *= 0.95
		}
	}

	if current < credibilityFloor {
		current = credibilityFloor
	}

	if current > 1 {
		current = 1
	}

	room.credibility[name] = current
}
