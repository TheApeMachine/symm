package advisor

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

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
	court      *WarRoom
	activeSize int
	capacity   int
	err        error
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
		node:     node,
		advisor:  name,
		name:     name,
		classes:  classes,
		active:   make(map[string]map[types.PerspectiveKey]*arenaRound),
		support:  make(map[types.PerspectiveKey]uint64),
		capacity: capacity,
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

	issued := envelope.Perspectives
	envelope.Perspectives = nil

	if err := arena.resolve(envelope); err != nil {
		arena.fail(err)

		return nil
	}

	for _, perspective := range issued {
		if perspective == nil || perspective.Advisor != arena.advisor {
			envelope.Perspectives = append(envelope.Perspectives, perspective)
			continue
		}

		if err := arena.admit(envelope, perspective); err != nil {
			arena.fail(err)

			return nil
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
			arena.evict(symbol, key)
		}
	}

	return nil
}

/*
report submits one resolved prediction to the Court of Causal Accountability.

An advisor is judged on the move it actually argued for. A prediction that was
borne out is credited; one the market falsified is debited — and a reading that
imposed a veto is held to the higher standard, because blocking a move that
then happened is the expensive error (MCTS.md §6.1).

The verdict is expressed as the realized move: a supported prediction realizes
what the advisor claimed, a falsified one realizes its opposite. That is the
honest reading of a falsifiable contract — the advisor named a specific
observable, and the market either produced it or did not.
*/
func (arena *Arena) report(round *arenaRound, supported bool) {
	if arena.court == nil || round == nil {
		return
	}

	claimed := MoveForState(string(round.perspective.TopClass()))
	realized := claimed

	if !supported {
		realized = -claimed
	}

	// A reading that argues against the market moving up is the one capable of
	// vetoing an entry, and is scored under the veto standard.
	wasVeto := claimed <= MoveStagnant

	arena.court.UpdateCredibility(arena.name, wasVeto, realized, claimed)
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

	winner := perspective.Classes[0]
	tied := false

	for _, class := range perspective.Classes[1:] {
		if class.Probability > winner.Probability {
			winner = class
			tied = false
			continue
		}

		if class.Probability == winner.Probability {
			tied = true
		}
	}

	if tied {
		return nil, errnie.Err(
			errnie.NotAcceptable,
			"[advisor] Arena cannot admit a tied Perspective",
			nil,
		)
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
