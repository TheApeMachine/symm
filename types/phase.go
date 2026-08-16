package types

import (
	"math"
	"time"
)

/*
PhaseOutcome is what a stored universe state is tagged with: the direction the
observable market took over the following manifold cuts, measured against the
mass-weighted book scale of every symbol that contributed to that cut.

Why:

	The label has to be ground truth. Anything a model concluded is that model's
	opinion, and tagging retained history with an opinion makes a scan report how
	self-consistent that model is rather than whether the field has structure.
	The manifold is one gas, so the label is one composite, not a per-symbol vote.
*/
type PhaseOutcome struct {
	Direction string  `json:"direction"`
	Return    float64 `json:"return"`
	Horizon   int     `json:"horizon"`
}

/*
PhaseResponse is one sampled angle of the dial: the signed response of the
resident fingerprint rotated by that angle against retained history, and the
outcome that owns it.
*/
type PhaseResponse struct {
	Angle      float64      `json:"angle"`
	Similarity float64      `json:"similarity"`
	ObservedAt string       `json:"observedAt"`
	Outcome    PhaseOutcome `json:"outcome"`
}

/*
PhaseReading is the universe sweep for one cut, stamped on the Thesis so stages
after the manifold can read the dial instead of re-deriving it.
*/
type PhaseReading struct {
	At        time.Time       `json:"at"`
	Ready     bool            `json:"ready"`
	Reason    string          `json:"reason,omitempty"`
	Responses []PhaseResponse `json:"responses,omitempty"`
}

/*
PhaseInference is the directional statement supported by the entire geodesic
scan. Similarity is signed by the phase rotation, so an antipodal match reverses
its retained outcome rather than copying that outcome onto the present market.
Return magnitudes remain corpus observations and never become a price forecast.
*/
type PhaseInference struct {
	Direction     float64 `json:"direction"`
	Confidence    float64 `json:"confidence"`
	Support       float64 `json:"support"`
	Contradiction float64 `json:"contradiction"`
	Balance       float64 `json:"balance"`
	Responses     int     `json:"responses"`
}

/*
Inference reduces the ranked angular corpus to a phase-projected direction vote.
Every response contributes its signed similarity and observed outcome class.
The result is normalized by total directional response mass, so no fixed angle,
return target, or hand-selected confidence boundary is introduced here.
*/
func (reading PhaseReading) Inference() (PhaseInference, bool) {
	if !reading.Ready || len(reading.Responses) == 0 {
		return PhaseInference{}, false
	}

	inference := PhaseInference{}

	for _, response := range reading.Responses {
		if math.IsNaN(response.Similarity) || math.IsInf(response.Similarity, 0) {
			continue
		}

		direction := phaseDirection(response.Outcome.Direction)

		if direction == 0 {
			continue
		}

		vote := direction * response.Similarity

		if vote > 0 {
			inference.Support += vote
		} else if vote < 0 {
			inference.Contradiction -= vote
		}

		inference.Responses++
	}

	total := inference.Support + inference.Contradiction

	if !(total > 0) || inference.Responses == 0 {
		return PhaseInference{}, false
	}

	inference.Balance = (inference.Support - inference.Contradiction) / total
	inference.Confidence = math.Abs(inference.Balance)

	if inference.Balance > 0 {
		inference.Direction = 1
	} else if inference.Balance < 0 {
		inference.Direction = -1
	}

	return inference, true
}

func phaseDirection(direction string) float64 {
	switch direction {
	case "up":
		return 1
	case "down":
		return -1
	default:
		return 0
	}
}

/*
Alignment reports the angle whose retained response is most constructive, which
is where the dial's ray points. It is retained for visualization; decision code
uses Inference so the rest of the geodesic is not discarded.
*/
func (reading PhaseReading) Alignment() (PhaseResponse, bool) {
	if !reading.Ready || len(reading.Responses) == 0 {
		return PhaseResponse{}, false
	}

	best := reading.Responses[0]

	for _, response := range reading.Responses[1:] {
		if response.Similarity > best.Similarity {
			best = response
		}
	}

	return best, true
}
