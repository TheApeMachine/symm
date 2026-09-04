package data

import (
	"math"
	"strings"
	"time"
)

/*
Readout is the common mechanism that protects observations from escaping into
the rest of the system as naked numbers that merely look meaningful.

Reading a metric means resolving it through the information that determines how
much authority it deserves: its maturity, its SNR, its validity, and any
supporting or contradicting evidence. Supports and contradictions are themselves
Readouts, so their influence is subject to their own quality. A weak corroborating
observation provides weak corroboration; a mature, high-quality contradiction
matters substantially more.
*/
type Readout struct {
	Source string    `json:"source"`
	Label  string    `json:"label"`
	At     time.Time `json:"at"`

	Raw          float64   `json:"raw"`
	Normalized   *float64  `json:"normalized,omitempty"`
	Standardized *float64  `json:"standardized,omitempty"`
	Unit         Unit      `json:"unit,omitempty"`
	Timescale    Timescale `json:"timescale,omitempty"`

	Maturity   float64 `json:"maturity"`
	SNR        float64 `json:"snr"`
	SNRDefined bool    `json:"snrDefined"`
	Estimated  bool    `json:"estimated"`
	Defined    bool    `json:"defined"`

	Supports       []*Readout `json:"supports,omitempty"`
	Contradictions []*Readout `json:"contradictions,omitempty"`

	Credibility float64 `json:"credibility"`
}

/*
NewReadout constructs an initial Readout for a metric with default credibility 1.0.
*/
func NewReadout(
	source string,
	label string,
	raw float64,
	maturity float64,
	snr float64,
	snrDefined bool,
	estimated bool,
	at time.Time,
) *Readout {
	return &Readout{
		Source:      source,
		Label:       label,
		At:          at,
		Raw:         raw,
		Maturity:    clamp(maturity, 0.0, 1.0),
		SNR:         snr,
		SNRDefined:  snrDefined,
		Estimated:   estimated,
		Defined:     true,
		Credibility: 1.0,
	}
}

/*
WithSupport attaches a corroborating Readout.
*/
func (readout *Readout) WithSupport(support *Readout) *Readout {
	if readout == nil || support == nil {
		return readout
	}

	readout.Supports = append(readout.Supports, support)

	return readout
}

/*
WithContradiction attaches an opposing Readout.
*/
func (readout *Readout) WithContradiction(contradiction *Readout) *Readout {
	if readout == nil || contradiction == nil {
		return readout
	}

	readout.Contradictions = append(readout.Contradictions, contradiction)

	return readout
}

/*
CorroborateWith attaches multiple supporting and contradicting Readouts.
*/
func (readout *Readout) CorroborateWith(supports []*Readout, contradictions []*Readout) *Readout {
	if readout == nil {
		return nil
	}

	for _, support := range supports {
		readout.WithSupport(support)
	}

	for _, contradiction := range contradictions {
		readout.WithContradiction(contradiction)
	}

	return readout
}

/*
WithCredibility adjusts the context-level credibility factor in [0, 1].
*/
func (readout *Readout) WithCredibility(credibility float64) *Readout {
	if readout == nil {
		return readout
	}

	readout.Credibility = clamp(credibility, 0.0, 1.0)

	return readout
}

/*
Authority computes the continuous statistical authority in [0, 1] this observation
commands.

An immature metric carries little authority; poor SNR reduces authority; and
contradictions from high-quality observations strongly penalize authority.
*/
func (readout *Readout) Authority() float64 {
	if readout == nil || !readout.Defined {
		return 0.0
	}

	credibility := readout.Credibility

	if credibility <= 0.0 {
		return 0.0
	}

	maturity := clamp(readout.Maturity, 0.0, 1.0)

	if maturity <= 0.0 {
		return 0.0
	}

	snrFactor := 1.0

	if readout.Estimated {
		snrFactor = 0.5

		if readout.SNRDefined {
			if readout.SNR <= 0.0 {
				snrFactor = 0.1
			} else {
				snrFactor = readout.SNR / (1.0 + readout.SNR)
			}
		}
	}

	baseQuality := maturity * snrFactor * credibility

	supportWeight := 0.0

	for _, support := range readout.Supports {
		if support != nil {
			supportWeight += support.Authority()
		}
	}

	contradictionWeight := 0.0

	for _, contradiction := range readout.Contradictions {
		if contradiction != nil {
			contradictionWeight += contradiction.Authority()
		}
	}

	numerator := 1.0 + supportWeight
	denominator := 1.0 + 2.0*contradictionWeight

	combined := baseQuality * (numerator / denominator)

	return clamp(combined, 0.0, 1.0)
}

/*
Value returns the usable float64 value resolved through its authority.
If authority is near zero, the effective value attenuates toward zero,
preventing unearned influence. Coordinate counters and ordinals retain their
exact discrete coordinate so market clocks advance accurately.
*/
func (readout *Readout) Value() float64 {
	if readout == nil {
		return 0.0
	}

	if readout.Unit == UnitCount || strings.Contains(readout.Label, "ordinal") {
		return readout.Raw
	}

	authority := readout.Authority()

	return readout.Raw * authority
}

func clamp(v, min, max float64) float64 {
	if math.IsNaN(v) {
		return min
	}

	if v < min {
		return min
	}

	if v > max {
		return max
	}

	return v
}
