package advisor

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

/*
classTieTolerance is the mass difference below which two classes are held to
lean the same way. Class probabilities arrive from a softmax normalization, so
exact equality is a property of the float encoding rather than of the market.
*/
const classTieTolerance = 1e-9

/*
Arena retains issued Perspectives until their declared support, contradiction,
or market-clock expiry resolves them. Only supported Perspectives reach Node.
*/
type Arena struct {
	node    runtime.Node[*types.Envelope]
	advisor string
	name    string
	classes map[types.PerspectiveState]*Class
	active  map[string]map[types.PerspectiveKey]*arenaRound
	support map[types.PerspectiveKey]uint64
	// court is the War Room whose credibility ledger this Arena reports to.

	// This is the Court of Causal Accountability of MCTS.md §6. The Arena is
	// the only place that observes whether a falsifiable prediction was borne
	// out, so it is the only place that can hold an advisor to it. Without
	// this link the ledger was written once at construction and never again:
	// every advisor kept credibility 1.0 forever, and being repeatedly wrong
	// cost nothing.

	// It is optional. An Arena built without a court still runs its rounds;
	// it simply reports to no one.
	court *WarRoom

	// rejected counts Perspectives this Arena declined to admit. They are a
	// normal operating condition, not a lifecycle failure.
	rejected map[string]uint64

	semanticHits      map[string]uint64
	semanticMisses    map[string]uint64
	directionalHits   map[string]uint64
	directionalMisses map[string]uint64
	expiredRounds     map[string]uint64
	censoredRounds    map[string]uint64

	// booted reports whether the system has finished coming up. Until it
	// returns true the Arena issues no Perspectives at all: during boot the
	// symbol universe is only partially subscribed, so every classifier is
	// cold and its distribution is uniform noise. Deciding on that is not a
	// weaker decision, it is a meaningless one.
	//
	// A nil booted means no gate was attached, and the Arena runs unguarded.
	booted func() bool

	activeSize int
	capacity   int
	err        error
}

/*
Booted attaches the readiness predicate that must report true before this
Arena will admit or issue any Perspective.
*/
func (arena *Arena) Booted(ready func() bool) *Arena {
	arena.booted = ready

	return arena
}

/* ready reports whether the boot gate, if any, is open. */
func (arena *Arena) ready() bool {
	return arena.booted == nil || arena.booted()
}

/* Court attaches the credibility ledger this Arena reports verdicts to. */
func (arena *Arena) Court(room *WarRoom) *Arena {
	arena.court = room

	return arena
}

/* NewArena wraps one Node with one Advisor's bounded prediction rounds. */
func NewArena(
	name string,
	features []*Feature,
	node runtime.Node[*types.Envelope],
	capacity int,
) (*Arena, error) {
	if name == "" || node == nil || capacity < 1 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[advisor] Arena requires a name, Node, and positive capacity",
			nil,
		))
	}

	if _, _, err := featureGroups(features); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[advisor] Arena feature contract failed",
			err,
		))
	}

	classes := make(map[types.PerspectiveState]*Class, len(features))

	for _, feature := range features {
		classes[types.PerspectiveState(feature.Class.Label)] = feature.Class
	}

	return &Arena{
		node:              node,
		advisor:           name,
		name:              name,
		classes:           classes,
		active:            make(map[string]map[types.PerspectiveKey]*arenaRound),
		support:           make(map[types.PerspectiveKey]uint64),
		capacity:          capacity,
		rejected:          make(map[string]uint64),
		semanticHits:      make(map[string]uint64),
		semanticMisses:    make(map[string]uint64),
		directionalHits:   make(map[string]uint64),
		directionalMisses: make(map[string]uint64),
		expiredRounds:     make(map[string]uint64),
		censoredRounds:    make(map[string]uint64),
	}, nil
}

/* Step resolves prior rounds, admits new rounds, then invokes the wrapped Node. */
func (arena *Arena) Step(envelope *types.Envelope) *types.Envelope {
	if arena.err != nil {
		return nil
	}

	if envelope == nil {
		arena.fail(errnie.Err(
			errnie.BadRequest,
			"[advisor] Arena received nil envelope",
			nil,
		))

		return nil
	}

	// The system must be fully booted before any decision is made. While the
	// instrument universe is still subscribing, only the first batch of
	// symbols is streaming and every classifier is cold, so the distributions
	// carry no information. Drop the issued Perspectives on the floor and let
	// the envelope pass through untouched: no round is opened, no advisor is
	// judged on a reading it could not have made, and the pipeline below still
	// sees the market data.
	if !arena.ready() {
		envelope.Perspectives = nil

		return arena.node.Step(envelope)
	}

	issued := envelope.Perspectives
	envelope.Perspectives = nil

	if err := arena.resolve(envelope); err != nil {
		arena.fail(err)

		return nil
	}

	for _, perspective := range issued {
		if perspective == nil {
			continue
		}

		if perspective.Advisor != arena.advisor {
			envelope.Perspectives = append(envelope.Perspectives, perspective)
			continue
		}

		// A Perspective that cannot be admitted is a fact about that one
		// Perspective, not about the Arena. A stale round, an exhausted
		// capacity, or a distribution with no declared winner used to call
		// fail(), which latches arena.err permanently — every later envelope
		// then returned nil, so a single bad frame silenced this advisor and
		// everything downstream of it for the rest of the process. Skip the
		// Perspective, keep serving the stream.
		if err := arena.admit(envelope, perspective); err != nil {
			arena.rejected[arena.advisor]++
			errnie.Error(err)

			continue
		}

		// 	The advisor speaks now, and is judged later.

		// 	This used to swallow the freshly issued Perspective: it entered the
		// 	pending-round map and was re-emitted only once one of its
		// 	falsifiable predictions had survived a full volume bar. The War Room
		// 	therefore never heard a current reading — only a past one that had
		// 	already been proven — so on a live symbol the council was empty and
		// 	the planner reported "no advisor prediction has survived a round for
		// 	this symbol yet" forever.

		// 	That inverts the architecture (MCTS.md §3 and §6). Deliberation is
		// 	about what the specialists observe *right now*; the Thunderdome is a
		// 	Court of Causal Accountability that adjusts credibility *afterwards*.
		// 	An advisor whose past calls were poor should be quieter at the table
		// 	— which credibility weighting already does — not absent from it.

		// 	So the round is still admitted above (accountability is preserved,
		// 	and a survived Perspective is still re-emitted by resolve with its
		// 	Support incremented), and the reading is also published immediately.
		envelope.Perspectives = append(envelope.Perspectives, perspective)
	}

	return arena.node.Step(envelope)
}

func (arena *Arena) resolve(envelope *types.Envelope) error {
	symbol := envelopeSymbol(envelope)
	rounds := arena.active[symbol]

	for key, round := range rounds {
		coordinate, advanced, err := round.advance(envelope)

		if err != nil {
			return err
		}

		if advanced && coordinate > round.perspective.Lease.Until {
			advisor := round.perspective.Advisor

			if advisor == "" {
				advisor = arena.name
			}

			arena.expiredRounds[advisor]++

			if arena.court != nil {
				arena.court.Evict(symbol, round.perspective.Advisor)
			}

			arena.evict(symbol, key)

			continue
		}

		prediction, found, err := round.observed(envelope, types.PredictionFalsifies)

		if err != nil {
			return err
		}

		if found {
			// The prediction was falsified: the advisor said this would happen
			// and it did not. The court records the miss.
			arena.report(round, false)

			if arena.court != nil {
				arena.court.Evict(symbol, round.perspective.Advisor)
			}

			arena.evict(symbol, key)

			continue
		}

		prediction, found, err = round.observed(envelope, types.PredictionSupports)

		if err != nil {
			return err
		}

		if found {
			// The prediction was borne out. The court records the hit.
			arena.report(round, true)

			resolved := round.perspective.Clone()
			arena.support[key]++
			resolved.Support = arena.support[key]
			resolved.Lifecycle = types.PerspectiveSurvived
			resolved.ResolvedAt = envelopeAt(envelope)
			resolved.ResolvedCoordinate = round.coordinate
			resolved.ResolvedBy = types.PerspectiveEvent(prediction.Label)
			envelope.Perspectives = append(envelope.Perspectives, &resolved)
			arena.evict(symbol, key)
			continue
		}

		if advanced && coordinate == round.perspective.Lease.Until {
			advisor := round.perspective.Advisor

			if advisor == "" {
				advisor = arena.name
			}

			arena.expiredRounds[advisor]++

			if arena.court != nil {
				arena.court.Evict(symbol, round.perspective.Advisor)
			}

			arena.evict(symbol, key)
		}
	}

	return nil
}

/*
report submits one resolved prediction to the Court of Causal Accountability.

An advisor is judged on the move it actually argued for against the realized market
move. Semantic accuracy and directional move accuracy are tracked separately.
*/
func (arena *Arena) report(round *arenaRound, supported bool) {
	if round == nil {
		return
	}

	advisor := round.perspective.Advisor

	if advisor == "" {
		advisor = arena.name
	}

	if supported {
		arena.semanticHits[advisor]++
	} else {
		arena.semanticMisses[advisor]++
	}

	claimed := MoveForState(string(round.perspective.TopClass()))
	realized := claimed

	if round.baselinePrice > 0 && round.latestPrice > 0 {
		returnFrac := (round.latestPrice - round.baselinePrice) / round.baselinePrice
		realized = MoveForReturn(returnFrac)

		if math.Abs(float64(realized-claimed)) <= 1 {
			arena.directionalHits[advisor]++
		} else {
			arena.directionalMisses[advisor]++
		}

		if !supported && realized == claimed {
			realized = -claimed
		}
	} else if !supported {
		arena.directionalMisses[advisor]++
		realized = -claimed
	} else {
		arena.directionalHits[advisor]++
	}

	if arena.court == nil {
		return
	}

	wasVeto := claimed <= MoveStagnant
	arena.court.UpdateCredibility(advisor, wasVeto, realized, claimed)
}

/* SemanticAccuracy returns the observed semantic hit and miss counts for an advisor. */
func (arena *Arena) SemanticAccuracy(advisor string) (uint64, uint64) {
	if arena == nil {
		return 0, 0
	}

	return arena.semanticHits[advisor], arena.semanticMisses[advisor]
}

/* DirectionalAccuracy returns the observed directional move hit and miss counts for an advisor. */
func (arena *Arena) DirectionalAccuracy(advisor string) (uint64, uint64) {
	if arena == nil {
		return 0, 0
	}

	return arena.directionalHits[advisor], arena.directionalMisses[advisor]
}

/* ExpiredRounds returns the count of rounds that reached lease expiry for an advisor. */
func (arena *Arena) ExpiredRounds(advisor string) uint64 {
	if arena == nil {
		return 0
	}

	return arena.expiredRounds[advisor]
}

/* CensoredRounds returns the count of censored rounds for an advisor. */
func (arena *Arena) CensoredRounds(advisor string) uint64 {
	if arena == nil {
		return 0
	}

	return arena.censoredRounds[advisor]
}

func (arena *Arena) admit(envelope *types.Envelope, perspective *types.Perspective) error {
	if perspective.Lifecycle != types.PerspectiveIssued || perspective.Symbol == "" ||
		perspective.Lease.Clock == "" || perspective.Lease.Until <= perspective.Lease.From {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[advisor] Arena received an invalid issued Perspective",
			nil,
		)
	}

	class, err := arena.winningClass(perspective)

	if err != nil {
		return err
	}

	// The distribution had no unique winner: the advisor has nothing to say
	// this frame. Open no round and keep the Arena alive.
	if class == nil {
		return nil
	}

	if len(class.Predictions) == 0 {
		return errnie.Err(
			errnie.PreconditionFailed,
			"[advisor] Arena received a Perspective without declared Predictions",
			nil,
		)
	}

	key := perspective.Key()
	rounds := arena.active[perspective.Symbol]
	existing, replacing := rounds[key]

	if replacing && perspective.Round <= existing.perspective.Round {
		return errnie.Err(
			errnie.Conflict,
			"[advisor] Arena received a stale Perspective round",
			nil,
		)
	}

	if !replacing && arena.activeSize >= arena.capacity {
		return errnie.Err(
			errnie.TooManyRequests,
			"[advisor] Arena active Perspective capacity exhausted",
			nil,
		)
	}

	round, err := newArenaRound(envelope, perspective, class)

	if err != nil {
		return err
	}

	if rounds == nil {
		rounds = make(map[types.PerspectiveKey]*arenaRound)
		arena.active[perspective.Symbol] = rounds
	}

	rounds[key] = round

	if !replacing {
		arena.activeSize++
	}

	return nil
}

func (arena *Arena) winningClass(perspective *types.Perspective) (*Class, error) {
	if len(perspective.Classes) < 2 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[advisor] Arena requires a competing class distribution",
			nil,
		)
	}

	// Find the winner first, then judge the tie against it. Deciding both in
	// one incremental pass was wrong: `tied` was only cleared by a strictly
	// greater class, so a distribution whose leaders tie ahead of a later
	// smaller class stayed flagged, and one whose equals appear after the
	// winner was never flagged at all.
	winner := perspective.Classes[0]

	for _, class := range perspective.Classes[1:] {
		if class.Probability > winner.Probability {
			winner = class
		}
	}

	// Compare on a tolerance, never on ==. These probabilities are a softmax
	// normalization, so two classes that hold the same mass routinely differ
	// in the last bits, and two that differ only by float noise are not
	// meaningfully distinct either way.
	leaders := 0

	for _, class := range perspective.Classes {
		if math.Abs(class.Probability-winner.Probability) <= classTieTolerance {
			leaders++
		}
	}

	if leaders > 1 {
		// No unique lean. This is a normal, expected state — an advisor whose
		// classifier currently holds no opinion — not a lifecycle failure.
		// Reporting it as terminal made one uninformative frame latch
		// arena.err and silence the advisor for the rest of the process.
		return nil, nil
	}

	class := arena.classes[winner.State]

	if class == nil {
		return nil, errnie.Err(
			errnie.NotFound,
			"[advisor] Arena Perspective winner is not declared: "+string(winner.State),
			nil,
		)
	}

	return class, nil
}

func (arena *Arena) evict(symbol string, key types.PerspectiveKey) {
	rounds := arena.active[symbol]
	delete(rounds, key)
	arena.activeSize--

	if len(rounds) == 0 {
		delete(arena.active, symbol)
	}
}

/* Error returns Arena's first terminal lifecycle failure. */
func (arena *Arena) Error() error { return arena.err }

func (arena *Arena) fail(err error) { arena.err = errnie.Error(err) }

/* Active reports the number of retained prediction rounds. */
func (arena *Arena) Active() int { return arena.activeSize }
