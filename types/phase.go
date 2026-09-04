package types

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
