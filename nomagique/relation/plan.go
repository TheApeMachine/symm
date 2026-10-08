package relation

import (
	"fmt"
	"iter"
	"slices"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Planner is one explicit relation plan compiled against the resident
coordinates of an ObservationStore.
*/
type Planner struct {
	*core.PrimitiveError
	store       core.Primitive
	epoch       uint64
	symbol      string
	peer        string
	lag         [2]float64
	pairs       [][2][3]string
	controls    [][3]string
	controlLags []time.Duration
	keys        []string
}

/*
NewPlanner builds one plan.
*/
func NewPlanner(
	store core.Primitive,
	epoch uint64,
	symbol string,
	peer string,
	minLag time.Duration,
	maxLag time.Duration,
	pairs [][2][3]string,
	sources [][3]string,
	targets [][3]string,
	controls [][3]string,
	controlLags ...time.Duration,
) *Planner {
	op := &Planner{
		PrimitiveError: core.NewPrimitiveError(),
		store:          store,
		epoch:          epoch,
		symbol:         symbol,
		peer:           peer,
		lag:            [2]float64{float64(minLag), float64(maxLag)},
		pairs:          slices.Clone(pairs),
		controls:       controls,
		controlLags:    controlLags,
	}

	for _, source := range sources {
		for _, target := range targets {
			if source != target {
				op.pairs = append(op.pairs, [2][3]string{source, target})
			}
		}
	}

	if store == nil {
		op.Error(fmt.Errorf("%w: relation: planner requires a store", core.ErrDomain))
	}

	return op
}

func (op *Planner) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil {
			return
		}

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			scope := (*PlanScope)(arriving)

			if scope == nil {
				op.Error(core.ErrShape)
				return
			}

			if scope.Epoch != op.epoch || (op.symbol != "" && op.symbol != scope.Symbol) {
				continue
			}

			stamp := strconv.FormatUint(op.epoch, 10)
			op.keys = op.keys[:0]

			for pointer := range op.store.Next(nil) {
				for key := range *(*map[string][]float64)(pointer) {
					op.keys = append(op.keys, key)
				}
			}

			if err := op.store.Error(); err != nil {
				op.Error(err)
				return
			}

			slices.Sort(op.keys)

			resident := make([][]string, 0, len(op.keys))

			for _, key := range op.keys {
				fields := strings.Split(key, "|")

				if len(fields) != 8 || fields[0] != scope.Symbol || fields[7] != stamp ||
					(op.peer != "" && fields[4] != op.peer) {
					continue
				}

				resident = append(resident, fields)
			}

			controlKeys := make([]string, 0, len(op.controls))
			controlLags := make([]float64, 0, len(op.controls))
			complete := true

			for index, selector := range op.controls {
				lag := 0.0

				if index < len(op.controlLags) {
					lag = float64(op.controlLags[index])
				}

				matched := false

				for _, fields := range resident {
					if (selector[0] == "" || selector[0] == fields[1]) &&
						(selector[1] == "" || selector[1] == fields[2]) &&
						(selector[2] == "" || selector[2] == fields[3]) {
						controlKeys = append(controlKeys, strings.Join(fields, "|"))
						controlLags = append(controlLags, lag)
						matched = true
					}
				}

				if !matched && selector != [3]string{} {
					controlKeys = append(controlKeys[:0], strings.Join(selector[:], "|"))
					controlLags = append(controlLags[:0], lag)
					complete = false
					break
				}
			}

			for _, pair := range op.pairs {
				for _, source := range resident {
					if (pair[0][0] != "" && pair[0][0] != source[1]) ||
						(pair[0][1] != "" && pair[0][1] != source[2]) ||
						(pair[0][2] != "" && pair[0][2] != source[3]) {
						continue
					}

					for _, target := range resident {
						if (pair[1][0] != "" && pair[1][0] != target[1]) ||
							(pair[1][1] != "" && pair[1][1] != target[2]) ||
							(pair[1][2] != "" && pair[1][2] != target[3]) ||
							slices.Equal(source, target) {
							continue
						}

						candidate := &Candidate{
							Source:           strings.Join(source, "|"),
							Target:           strings.Join(target, "|"),
							Controls:         controlKeys,
							ControlLags:      controlLags,
							MinLag:           op.lag[0],
							MaxLag:           op.lag[1],
							ControlsComplete: complete,
						}

						for value := range data.NewValue(unsafe.Pointer(candidate)).Next(nil) {
							if !yield(value) {
								return
							}
						}
					}
				}
			}
		}
	}
}
