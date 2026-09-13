package crosssection

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
drive pushes one payload pointer through one primitive and returns the answer
the primitive yielded.
*/
func drive[From, To any](op core.Primitive, payload *From) To {
	var answer To

	for out := range op.Next(transport.NewOne(unsafe.Pointer(payload)).Next(nil)) {
		answer = *(*To)(out)
	}

	return answer
}

/*
UpdateMember retains the focal member's price, derives its causal change
against the value the store replaced (calculus.RelativeChange), and retains
the change facts in the member store. The stage owns only the store wiring;
the mathematics lives in the primitives it drives.
*/
type UpdateMember struct {
	err    error
	label  string
	prices *store.Latest[string, float64]
	membrs *store.Latest[string, data.CrossMember]
}

func NewUpdateMember(
	label string, prices *store.Latest[string, float64], changes *store.Latest[string, data.CrossMember],
) core.Primitive {
	return &UpdateMember{label: label, prices: prices, membrs: changes}
}

func (op *UpdateMember) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			reading := drive[store.LatestCommand[string, float64], store.LatestReading[string, float64]](
				op.prices,
				&store.LatestCommand[string, float64]{Key: m.Label, Value: m.Metrics[op.label].Raw},
			)

			if !reading.HasPrior {
				if !yield(arriving) {
					return
				}

				continue
			}

			input := calculus.RelativeChangeInput{Previous: reading.Prior, Current: reading.Current}
			change := drive[calculus.RelativeChangeInput, float64](calculus.NewRelativeChange(), &input)

			drive[store.LatestCommand[string, data.CrossMember], store.LatestReading[string, data.CrossMember]](
				op.membrs,
				&store.LatestCommand[string, data.CrossMember]{
					Key: m.Label, Value: data.CrossMember{Label: m.Label, Change: change, At: m.At, From: m.At},
				},
			)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *UpdateMember) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}

/*
StampPeers reads the retained member changes and stamps them onto the
measurement's Peers, so every reduction branch reads the cross-section from
the measurement itself.
*/
type StampPeers struct {
	err    error
	membrs *store.Latest[string, data.CrossMember]
}

func NewStampPeers(changes *store.Latest[string, data.CrossMember]) core.Primitive {
	return &StampPeers{membrs: changes}
}

func (op *StampPeers) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			snapshot := drive[store.LatestCommand[string, data.CrossMember], map[string]data.CrossMember](
				op.membrs, &store.LatestCommand[string, data.CrossMember]{Read: true},
			)

			peers := make([]*data.Measurement[float64], 0, len(snapshot))

			for _, member := range snapshot {
				peer := data.NewMeasurement[float64]("cross-section", map[string]data.Metric[float64]{
					"change": {Label: "change", Raw: member.Change},
				})
				peer.Label, peer.At, peer.From = member.Label, member.At, member.From
				peers = append(peers, peer)
			}

			m.Peers = peers

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *StampPeers) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
