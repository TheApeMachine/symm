package equation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
drive pushes one payload pointer through one primitive and returns the answer
the primitive yielded.
*/
func drive[From, To any](op core.Primitive, payload *From) To {
	var answer To

	for out := range op.Next(transportOne(payload)) {
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
			m := *(*data.Measurement[float64])(arriving)

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

			change := drive[float64, float64](calculus.NewRelativeChange(), &calculus.RelativeChangeInput{
				Previous: reading.Prior,
				Current:  reading.Current,
			})

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
			m := *(*data.Measurement[float64])(arriving))

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

/*
ChangeCounts takes the sign census of the member changes and writes the sign
counts, the valid member count, and the signed fraction equation
(positive - negative) / valid.
*/
type ChangeCounts struct {
	err error
}

func NewChangeCounts() core.Primitive {
	return &ChangeCounts{}
}

func (op *ChangeCounts) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(*data.Measurement[float64])(arriving))

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			positive, negative, zero := 0.0, 0.0, 0.0

			for _, peer := range m.Peers {
				change := peer.Metrics["change"].Raw

				switch {
				case change > 0:
					positive++
				case change < 0:
					negative++
				default:
					zero++
				}
			}

			valid := positive + negative + zero

			m.Metrics["valid_member_count"] = m.Metrics["valid_member_count"].Write(valid)
			m.Metrics["positive_count"] = m.Metrics["positive_count"].Write(positive)
			m.Metrics["negative_count"] = m.Metrics["negative_count"].Write(negative)
			m.Metrics["zero_count"] = m.Metrics["zero_count"].Write(zero)

			if valid > 0 {
				m.Metrics["signed_fraction"] = m.Metrics["signed_fraction"].Write((positive - negative) / valid)
			}

			m.Metadata[data.MetadataSupport] = valid

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *ChangeCounts) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}

/*
ChangeMedian reduces the member changes to their median.
*/
type ChangeMedian struct {
	err    error
	median core.Primitive
}

func NewChangeMedian() core.Primitive {
	return &ChangeMedian{median: statistic.NewMedian()}
}

func (op *ChangeMedian) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(*data.Measurement[float64])(arriving))

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			if len(m.Peers) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			median := drive[float64, float64](op.median, peerValues(m))

			m.Metrics["signed_median"] = m.Metrics["signed_median"].Write(median)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *ChangeMedian) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}

/*
peerValues extracts the peer changes of one cross-section.
*/
func peerValues(m *data.Measurement[float64]) []float64 {
	changes := make([]float64, 0, len(m.Peers))

	for _, peer := range m.Peers {
		changes = append(changes, peer.Metrics["change"].Raw)
	}

	return changes
}
