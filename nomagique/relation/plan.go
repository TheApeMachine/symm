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
coordinates of an ObservationStore. Eligibility is structural only: model
epoch, symbol scope, peer scope, explicit pairs, the Sources × Targets cross
product (self-pairs excluded), and exact controls. It never depends on
current evidence values: a low-gain or zero-gain Relation remains eligible.

Selectors are [3]string{source, metric, side}; an empty field is a wildcard.
Each control selector has the lag at its index in controlLags (a missing or
non-positive lag aligns the control at the source lag).

Each arrival is **data.Adapter carrying text "symbol" and number "epoch". A
foreign epoch or a symbol outside the plan scope compiles nothing. Otherwise
it yields one fresh **data.Adapter per candidate, ready for Influence:

	text    "source", "target", "control.<i>"
	number  "controls", "control.<i>.lag", "min_lag", "max_lag"
	number  "controls_complete"   1, or 0 when an exact control is not resident

A missing exact control makes the Relation unavailable rather than silently
changing the model: the unresolved selector is carried as the control key,
which no resident coordinate matches, so Influence reports
FitControlUnavailable.
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
	scope       data.Map[string]
	stamp       data.Map[string]
	keys        []string
}

/*
NewPlanner builds one plan. An empty symbol or peer means no restriction. A
zero minLag or maxLag lets Influence derive that bound.
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
		scope:          data.NewLiteral("symbol"),
		stamp:          data.NewMap("epoch", "epoch"),
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
			if arriving == nil || *(**data.Adapter)(arriving) == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)
			var symbol string
			var epoch float64

			for pointer := range adapter.Next(data.NewValue(op.scope)) {
				symbol = (*(*data.Map[string])(pointer)).Values["symbol"]
			}

			for pointer := range adapter.Next(data.NewValue(op.stamp)) {
				epoch = (*(*data.Map[float64])(pointer)).Values["epoch"]
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if uint64(epoch) != op.epoch || (op.symbol != "" && op.symbol != symbol) {
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

			// fields: symbol|source|metric|side|peer|unit|timescale|epoch.
			resident := make([][]string, 0, len(op.keys))

			for _, key := range op.keys {
				fields := strings.Split(key, "|")

				if len(fields) != 8 || fields[0] != symbol || fields[7] != stamp ||
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

						roles := data.NewTextMap()
						roles.Values["source"] = strings.Join(source, "|")
						roles.Values["target"] = strings.Join(target, "|")

						domain := data.NewOutputMap()
						domain.Values["controls"] = float64(len(controlKeys))
						domain.Values["min_lag"] = op.lag[0]
						domain.Values["max_lag"] = op.lag[1]
						domain.Values["controls_complete"] = 0

						if complete {
							domain.Values["controls_complete"] = 1
						}

						for index, key := range controlKeys {
							roles.Values["control."+strconv.Itoa(index)] = key
							domain.Values["control."+strconv.Itoa(index)+".lag"] = controlLags[index]
						}

						candidate := data.NewAdapter(nil, data.NewState(data.NewMap()))

						for range candidate.Next(data.NewValue(roles)) {
						}

						for range candidate.Next(data.NewValue(domain)) {
						}

						if err := candidate.Error(); err != nil {
							op.Error(err)
							return
						}

						if !yield(unsafe.Pointer(&candidate)) {
							return
						}
					}
				}
			}
		}
	}
}
