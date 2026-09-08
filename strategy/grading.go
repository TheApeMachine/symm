package strategy

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/types"
)

/*
	Verdict keeps retrospective price development, current wallet performance and

elapsed time separate. Tape is a signed log coordinate change per unit exposure;
it is not executable profit. The wallet supplies that different measurement.
*/
type Verdict struct {
	Decision  TraderDecision `json:"decision"`
	Grade     float64        `json:"grade"`
	Tape      float64        `json:"tape"`
	Wallet    float64        `json:"wallet"`
	Clock     float64        `json:"clock"`
	Matched   bool           `json:"matched"`
	Episode   string         `json:"episode,omitempty"`
	Alignment float64        `json:"alignment"`
	Excursion float64        `json:"excursion"`
	Reason    string         `json:"reason"`
}

/*
	Grade evaluates only a fully confirmed leg containing the decision. The next

priced reference measures what actually followed it. Unmatched or still-open
context remains unresolved; absence of a confirmed move is never a reward.
A flat wait has no exposure. Holding inventory follows the tape, while reducing
is evaluated by the movement it avoided. Size and fees remain wallet facts.
*/
func Grade(decision TraderDecision, episodes []hindsight.Episode, wealth float64) (Verdict, bool, error) {
	verdict := Verdict{Decision: decision, Wallet: wealth}
	var end hindsight.ReferencePoint
	var selected hindsight.Episode

	for _, episode := range episodes {
		if !episode.Confirmed || episode.Symbol != decision.Symbol || decision.Sequence < episode.FromSequence || decision.Sequence > episode.ToSequence {
			continue
		}

		for _, reference := range episode.References {
			if !reference.HasValue || reference.Capture.Sequence <= decision.Sequence || reference.Capture.Sequence > episode.ToSequence {
				continue
			}

			if end.HasValue && reference.Capture.Sequence >= end.Capture.Sequence {
				continue
			}
			end, selected = reference, episode
		}
	}

	if !end.HasValue || end.ReceivedAt.IsZero() || !end.ReceivedAt.After(decision.At) {
		verdict.Reason = "awaiting a completed forward price leg with capture timing"
		return verdict, false, nil
	}

	if decision.Price <= 0 || end.Value <= 0 {
		return verdict, false, errnie.Error(errnie.Err(errnie.Validation, "grading: positive observed prices required", nil))
	}
	growth := math.Log(end.Value / decision.Price)
	verdict.Matched, verdict.Episode = true, selected.ID
	verdict.Excursion = growth
	verdict.Clock = end.ReceivedAt.Sub(decision.At).Seconds()
	verdict.Alignment = float64(decision.Sequence-selected.FromSequence) / float64(end.Capture.Sequence-selected.FromSequence)
	verdict.Tape = growth

	if decision.Action.Reduce {
		verdict.Tape = -growth
	}

	if decision.Action.Kind == types.ActionHold && decision.State == "flat" {
		verdict.Tape = 0
	}
	verdict.Grade = verdict.Tape
	verdict.Reason = "signed observed price development; wallet return and elapsed time reported separately"
	return verdict, true, nil
}
