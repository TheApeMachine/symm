package types

import (
	"slices"
	"time"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/* PerspectiveQuestion is the stable semantic question an Advisor answers. */
type PerspectiveQuestion string

/* PerspectiveState is one recognizable market state in a distribution. */
type PerspectiveState string

/* PerspectiveClass assigns probability mass to one named market state. */
type PerspectiveClass struct {
	State       PerspectiveState
	Probability float64
}

/* PerspectiveEvent names one causally observable, possibly compound, event. */
type PerspectiveEvent string

/* PerspectivePredictionEffect says how an observed event resolves a round. */
type PerspectivePredictionEffect uint8

const (
	PredictionSupports PerspectivePredictionEffect = iota + 1
	PredictionFalsifies
)

/* String returns the prediction effect's display name. */
func (effect PerspectivePredictionEffect) String() string {
	switch effect {
	case PredictionSupports:
		return "supports"
	case PredictionFalsifies:
		return "falsifies"
	default:
		return "unknown"
	}
}

/*
PerspectivePrediction names one terminal market event and whether observing it
supports or falsifies the issued Perspective. Predictions carry no arbitrary
weight: the first terminal event observed by Arena resolves the round.
*/
type PerspectivePrediction struct {
	Event  PerspectiveEvent
	Effect PerspectivePredictionEffect
}

/*
PerspectiveLease bounds one prediction round in a monotone market coordinate.
Clock names the coordinate (for example event-time nanoseconds, ticker ordinal,
or completed-volume-bar ordinal); From and Until are positions on that clock.
No wall-clock duration or universal fixed window is implied.
*/
type PerspectiveLease struct {
	Clock nmtypes.Symbol
	From  uint64
	Until uint64
}

/* PerspectiveLifecycle is the state of one falsifiable prediction round. */
type PerspectiveLifecycle uint8

const (
	PerspectiveIssued PerspectiveLifecycle = iota + 1
	PerspectiveSurvived
	PerspectiveFalsified
	PerspectiveExpired
	PerspectiveCensored
)

/* String returns the lifecycle state's display name. */
func (lifecycle PerspectiveLifecycle) String() string {
	switch lifecycle {
	case PerspectiveIssued:
		return "issued"
	case PerspectiveSurvived:
		return "survived"
	case PerspectiveFalsified:
		return "falsified"
	case PerspectiveExpired:
		return "expired"
	case PerspectiveCensored:
		return "censored"
	default:
		return "unknown"
	}
}

/*
Perspective is one Advisor's falsifiable distribution over named market states.
It describes a market claim and its observable survival conditions; it never
chooses a trading action or names a receiver.

Question makes the market question explicit. PositionID is empty for
symbol-level context and set when the question is about one open position. Peer
is empty unless the question concerns a symbol relationship. Round identifies
the Advisor's generation for this question. Sequence orders issued and terminal
updates within and across rounds. IssuedAt never changes during that round;
ResolvedAt and ResolvedCoordinate preserve the terminal evidence Arena saw.

Perspective issuance is sparse control-plane work, not one allocation per raw
market event. Arena owns its configured active-round bound and deep-copies the
slices on admission. ResolvedBy is empty while issued and names the observed
terminal event or Arena condition for a resolved round.
*/
type Perspective struct {
	Symbol     string
	Peer       string
	PositionID string

	Advisor            nmtypes.Symbol
	Question           PerspectiveQuestion
	IssuedAt           time.Time
	ResolvedAt         time.Time
	ResolvedCoordinate uint64
	Sequence           uint64
	Round              uint64
	Support            uint64

	Classes     []PerspectiveClass
	Predictions []PerspectivePrediction
	Lease       PerspectiveLease
	Lifecycle   PerspectiveLifecycle
	ResolvedBy  PerspectiveEvent

	// Err carries an Advisor or Arena transition failure for this update. A
	// Perspective with Err set is invalid and must halt its consumer.
	Err error
}

/*
PerspectiveKey is the structural identity of one Perspective stream. Round is
not part of the key: a new generation replaces the previous generation for the
same Advisor question rather than growing the latest-value store without bound.
*/
type PerspectiveKey struct {
	Symbol     string
	Peer       string
	PositionID string
	Advisor    nmtypes.Symbol
	Question   PerspectiveQuestion
}

/* Key returns the Perspective stream's complete structural identity. */
func (perspective *Perspective) Key() PerspectiveKey {
	return PerspectiveKey{
		Symbol:     perspective.Symbol,
		Peer:       perspective.Peer,
		PositionID: perspective.PositionID,
		Advisor:    perspective.Advisor,
		Question:   perspective.Question,
	}
}

/* Clone returns an independently owned Perspective for Arena admission. */
func (perspective Perspective) Clone() Perspective {
	perspective.Classes = slices.Clone(perspective.Classes)
	perspective.Predictions = slices.Clone(perspective.Predictions)

	return perspective
}

/*
Probability returns the mass assigned to a named state. The boolean separates a
real zero probability from a state absent from the distribution.
*/
func (perspective *Perspective) Probability(state PerspectiveState) (float64, bool) {
	for _, class := range perspective.Classes {

		if class.State == state {
			return class.Probability, true
		}
	}

	return 0, false
}

/*
Maturity reports the distribution's empirical support maturity using the same
effective-support mapping as Measurement. A first resolved round establishes
an observation but not yet a stable empirical distribution.
*/
func (perspective *Perspective) Maturity() float64 {
	if perspective == nil || perspective.Support <= 1 {
		return 0
	}

	return 1 - 1/float64(perspective.Support)
}

/* Prediction returns the declared terminal event with the given name. */
func (perspective *Perspective) Prediction(event PerspectiveEvent) (PerspectivePrediction, bool) {
	for _, prediction := range perspective.Predictions {

		if prediction.Event == event {
			return prediction, true
		}
	}

	return PerspectivePrediction{}, false
}
