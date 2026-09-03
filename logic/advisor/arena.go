package advisor

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
Arena retains issued Perspectives until their declared support, contradiction,
or market-clock expiry resolves them. Only supported Perspectives reach Node.
*/
type Arena struct {
	node       runtime.Node[*types.Envelope]
	advisor    nmtypes.Symbol
	classes    map[types.PerspectiveState]*Class
	active     map[string]map[types.PerspectiveKey]*arenaRound
	support    map[types.PerspectiveKey]uint64
	activeSize int
	capacity   int
	err        error
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
		advisor:  nmtypes.MustIntern(name),
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
			arena.evict(symbol, key)
			continue
		}

		prediction, found, err = round.observed(envelope, types.PredictionSupports)

		if err != nil {
			return err
		}

		if found {
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

func (arena *Arena) admit(envelope *types.Envelope, perspective *types.Perspective) error {
	if perspective.Lifecycle != types.PerspectiveIssued || perspective.Symbol == "" ||
		perspective.Lease.Clock == 0 || perspective.Lease.Until <= perspective.Lease.From {
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
func (arena *Arena) Error() error {
	return arena.err
}

func (arena *Arena) fail(err error) {
	arena.err = errnie.Error(err)
}

/* Active reports the number of retained prediction rounds. */
func (arena *Arena) Active() int {
	return arena.activeSize
}
