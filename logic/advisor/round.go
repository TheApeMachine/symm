package advisor

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/* arenaRound owns one admitted claim's baselines and market-clock progress. */
type arenaRound struct {
	perspective types.Perspective
	class       *Class
	baselines   map[*Falsifiable]float64
	clock       Falsifiable
	coordinate  uint64
}

func newArenaRound(
	envelope *types.Envelope,
	perspective *types.Perspective,
	class *Class,
) (*arenaRound, error) {
	clock := perspective.Lease.Clock

	if clock == "" {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[advisor] Arena received an opaque market clock",
			nil,
		)
	}

	stored := perspective.Clone()
	round := &arenaRound{
		perspective: stored,
		class:       class,
		baselines:   make(map[*Falsifiable]float64, 2*len(class.Predictions)),
		clock:       Falsifiable{Label: clock, Type: METRIC},
		coordinate:  perspective.Lease.From,
	}

	for _, prediction := range class.Predictions {
		if err := round.seed(envelope, prediction.Support); err != nil {
			return nil, err
		}

		if err := round.seed(envelope, prediction.Contradict); err != nil {
			return nil, err
		}
	}

	return round, nil
}

func (round *arenaRound) advance(
	envelope *types.Envelope,
) (uint64, bool, error) {
	value, found, err := round.clock.observe(envelope)

	if err != nil {
		return 0, false, err
	}

	if !found {
		return round.coordinate, false, nil
	}

	if value < 0 || float64(uint64(value)) != value {
		return 0, false, errnie.Err(
			errnie.UnprocessableContent,
			"[advisor] Arena market clock must be a non-negative ordinal",
			nil,
		)
	}

	coordinate := uint64(value)

	if coordinate < round.coordinate {
		return 0, false, errnie.Err(
			errnie.Conflict,
			"[advisor] Arena market clock moved backwards",
			nil,
		)
	}

	advanced := coordinate > round.coordinate
	round.coordinate = coordinate

	return coordinate, advanced, nil
}

func (round *arenaRound) seed(envelope *types.Envelope, event *Falsifiable) error {
	value, found, err := event.observe(envelope)

	if err != nil {
		return err
	}

	if found {
		round.baselines[event] = value
	}

	return nil
}

func (round *arenaRound) observed(
	envelope *types.Envelope,
	effect types.PerspectivePredictionEffect,
) (*Falsifiable, bool, error) {
	for _, prediction := range round.class.Predictions {
		event := prediction.Support

		if effect == types.PredictionFalsifies {
			event = prediction.Contradict
		}

		value, found, err := event.observe(envelope)

		if err != nil {
			return nil, false, err
		}

		if !found {
			continue
		}

		baseline, seeded := round.baselines[event]

		if !seeded {
			round.baselines[event] = value
			continue
		}

		matched, err := event.matches(baseline, value)

		if err != nil {
			return nil, false, err
		}

		if matched {
			return event, true, nil
		}
	}

	return nil, false, nil
}
