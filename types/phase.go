package types

import "time"

/*
PhaseOutcome is what a stored market state is tagged with: the direction price
actually took over the following manifold cuts, measured against that symbol's
own book scale.

Why:

	The label has to be ground truth. Anything a model concluded is that model's
	opinion, and tagging retained history with an opinion makes a scan report how
	self-consistent that model is rather than whether the field has structure.
*/
type PhaseOutcome struct {
	Symbol    string  `json:"symbol"`
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
PhaseReading is one symbol's complete sweep for a cut, stamped on the Thesis so
stages after the manifold can read the dial instead of re-deriving it.
*/
type PhaseReading struct {
	Symbol           string          `json:"symbol"`
	At               time.Time       `json:"at"`
	EvidenceRevision uint64          `json:"evidenceRevision"`
	Ready            bool            `json:"ready"`
	Reason           string          `json:"reason,omitempty"`
	Responses        []PhaseResponse `json:"responses,omitempty"`
}

/*
Alignment reports the angle whose retained response is most constructive, which
is where the dial's ray points. It is the reading's single summary: the phase
this symbol's field currently rhymes with history at, and what that history did.
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
