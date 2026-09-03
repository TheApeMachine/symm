package advisor

import (
	"math"
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
	Probabilities map[MarketMove]float64 `json:"probabilities"`
	DominantMove  MarketMove             `json:"dominantMove"`
	Confidence    float64                `json:"confidence"`
	Vetoes        []string               `json:"vetoes,omitempty"`
	Synergies     []string               `json:"synergies,omitempty"`
	Participants  int                    `json:"participants"`
	At            time.Time              `json:"at"`
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
	active := room.admit(perspectives, symbol)

	room.mu.RLock()
	defer room.mu.RUnlock()

	mass := priorMass()

	for name, perspective := range active {
		room.project(mass, name, perspective)
	}

	outcome := &DeliberationOutcome{
		Participants: len(active),
		At:           at,
	}

	room.crossExamine(mass, active, outcome)
	normalize(mass, outcome)

	return outcome
}

/*
admit folds any freshly issued perspectives into the resident council and
returns the council as it now stands for one symbol.

Perspectives arrive on trade envelopes and decisions are made on ticker
envelopes, so the council has to persist between the two. Each advisor holds
one seat per symbol: a new perspective replaces that advisor's previous one
rather than accumulating, so the council reflects the latest reading and cannot
grow without bound.
*/
func (room *WarRoom) admit(
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

		if seats == nil {
			seats = make(map[string]*types.Perspective)
			room.resident[perspective.Symbol] = seats
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
	active := make(map[string]*types.Perspective, len(seats))

	for name, perspective := range seats {
		active[name] = perspective
	}

	return active
}

/*
project applies one advisor's own lean to the move mass, weighted by how much
that advisor has earned the right to be heard.
*/
func (room *WarRoom) project(
	mass map[MarketMove]float64,
	name string,
	perspective *types.Perspective,
) {
	topClass := string(perspective.TopClass())
	probability, found := perspective.Probability(types.PerspectiveState(topClass))

	if !found {
		return
	}

	credibility := room.credibilityOf(name)

	// A perspective that has survived at most one round reports zero maturity.
	// Multiplying by it would mute a freshly speaking advisor entirely, so an
	// unproven reading is discounted rather than silenced: on a cold start
	// every advisor would otherwise weigh nothing and the council would fall
	// back to its own stagnant prior, vetoing every entry.
	maturity := perspective.Maturity()

	if maturity < baselineMaturity {
		maturity = baselineMaturity
	}

	weight := probability * credibility * maturity

	if weight <= 0 {
		return
	}

	switch topClass {
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
	}
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

		joint := probabilityA * probabilityB *
			room.credibilityOf(rule.AdvisorA) * room.credibilityOf(rule.AdvisorB)

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
