package logic

import (
	"sort"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

type epochIdentity struct {
	symbol string
	at     int64
}

/*
Composer groups typed numerical measurements into exact event-time epochs.
Exact alignment is deliberately narrow: it preserves evidence now without
inventing a shared cadence or carrying observations forward before the elastic
multi-track clock has a correct implementation.
*/
type Composer struct{}

/*
NewComposer creates the stateless measurement composer used synchronously by
Analyzer. It owns no queue or callback because the trading loop already owns
the ordered measurement batch.
*/
func NewComposer() *Composer {
	return &Composer{}
}

/*
Compose validates and groups one typed batch by symbol and exact event time.
Measurements retain input order inside an epoch, while epochs are returned in
event-time and symbol order for deterministic replay.
*/
func (composer *Composer) Compose(
	measurements []*types.Measurement,
) ([]types.LogicEpoch, error) {
	epochs := make(map[epochIdentity]*types.LogicEpoch)

	for _, measurement := range measurements {
		if measurement == nil {
			return nil, errnie.Err(
				errnie.Validation,
				"logic composer: nil measurement",
				nil,
			)
		}

		if err := measurement.Validate(); err != nil {
			return nil, errnie.Err(
				errnie.Validation,
				"logic composer: invalid typed measurement",
				err,
			)
		}

		identity := epochIdentity{
			symbol: measurement.Symbol,
			at:     measurement.At.UnixNano(),
		}
		epoch := epochs[identity]

		if epoch == nil {
			epoch = &types.LogicEpoch{
				Symbol:       measurement.Symbol,
				At:           measurement.At,
				Measurements: make([]types.Measurement, 0, 1),
			}
			epochs[identity] = epoch
		}

		epoch.Measurements = append(epoch.Measurements, *measurement)
	}

	return composer.ordered(epochs), nil
}

func (composer *Composer) ordered(
	epochs map[epochIdentity]*types.LogicEpoch,
) []types.LogicEpoch {
	ordered := make([]types.LogicEpoch, 0, len(epochs))

	for _, epoch := range epochs {
		ordered = append(ordered, *epoch)
	}

	sort.Slice(ordered, func(left int, right int) bool {
		comparison := ordered[left].At.Compare(ordered[right].At)

		if comparison == 0 {
			return ordered[left].Symbol < ordered[right].Symbol
		}

		return comparison < 0
	})

	return ordered
}
