package strategy

import (
	"math"
	"time"
)

/*
Mode is what the agent is currently doing, and there are only two answers: it
is calibrating on its own virtual lanes, or it is trading the account. Learning
never stops — a trading agent keeps issuing, resolving and re-estimating on
those lanes, which is what lets a degrading edge pull it back.

Where it trades is not a rung on this ladder. Paper and real are the same
behaviour against different accounts, chosen by configuration, and the agent
does not earn its way from one to the other.
*/
type Mode uint8

const (
	ModeLearning Mode = iota
	ModeTrading
)

/* String names the mode for journals and operator display. */
func (mode Mode) String() string {
	if mode == ModeTrading {
		return "trading"
	}

	return "learning"
}

/*
Account is which account trading reaches, taken from configuration. It says
nothing about competence: an agent with a measured edge trades whichever
account it was pointed at, and one without an edge trades neither.

AccountNone means no account is attached, so the agent cannot leave learning
however good its measurement looks.
*/
type Account uint8

const (
	AccountNone Account = iota
	AccountPaper
	AccountReal
)

/* String names the configured account for journals and operator display. */
func (account Account) String() string {
	switch account {
	case AccountPaper:
		return "paper"
	case AccountReal:
		return "real"
	default:
		return "none"
	}
}

/* ParseAccount reads the configured trading model; unknown text attaches none. */
func ParseAccount(text string) Account {
	switch text {
	case "paper":
		return AccountPaper
	case "real":
		return AccountReal
	default:
		return AccountNone
	}
}

/*
skillSigma is the stated one-sided confidence multiple used for promotion.
Two standard errors is a declared operating choice, not a derived constant,
and it is reported with every reading so the bar is never implicit.
*/
const skillSigma = 2.0

/*
skillMemory is the exponential retention rate of the meter, in resolved policy
decisions. Measured competence must forget: an agent that earned an edge in one
regime and lost it in the next has to be demotable, which a cumulative
since-boot mean cannot do. This is a declared retention window, reported with
every reading, not a hidden constant.

Exponentially decayed weights at rate 1/skillMemory reach a Kish effective size
of about twice this figure, so Support settles near 2*skillMemory rather than
at it. Support reports the weights actually in use; it is not re-scaled to make
the retention window look like a sample count.
*/
const skillMemory = 512.0

/*
SkillReading is the measured competence of the policy lane and the execution
authority that measurement currently justifies.

Mean is the average realized return-to-go of a policy decision as a fraction
of the lane's starting capital. LowerBound is Mean minus skillSigma standard
errors: promotion requires it to exceed zero, so an edge has to be larger than
its own measurement error. Confidence is the empirical normal probability that
the mean is positive; it is a sampling statement about these observations, not
a calibrated posterior or a probability that the next trade wins.

Samples counts admitted policy decisions; Support is their Kish effective size
under issue-time authority weighting. Only decisions covering disjoint forward
windows are admitted, so these are not overlapping observations counted as
independent ones.

Qualified reports whether the evidence is thick enough for the bound to mean
anything: below skillSigma squared effective observations, Confidence and
LowerBound are arithmetic on too little evidence and must not be presented as
a measurement.
*/
type SkillReading struct {
	Mode            string    `json:"mode"`
	Account         string    `json:"account"`
	Since           time.Time `json:"since"`
	Reason          string    `json:"reason"`
	Samples         uint64    `json:"samples"`
	Support         float64   `json:"support"`
	Defined         bool      `json:"defined"`
	VarianceDefined bool      `json:"varianceDefined"`
	Qualified       bool      `json:"qualified"`
	Mean            float64   `json:"mean"`
	Variance        float64   `json:"variance"`
	StandardError   float64   `json:"standardError"`
	LowerBound      float64   `json:"lowerBound"`
	Confidence      float64   `json:"confidence"`
	Sigma           float64   `json:"sigma"`
	Memory          float64   `json:"memory"`
	Promotions      uint64    `json:"promotions"`
	Demotions       uint64    `json:"demotions"`
	Wins            uint64    `json:"wins"`
	Losses          uint64    `json:"losses"`
}

/*
SkillMeter estimates the policy lane's forward competence and owns the mode
transition it justifies. It observes resolved return-to-go targets with the
authority fixed when each decision issued, under exponential forgetting, so a
lost edge decays out of the estimate instead of being averaged away.

Going live needs a positive lower bound: measured edge larger than its own
standard error. Falling back needs only a non-positive mean. The asymmetry is
deliberate hysteresis — starting to trade is held to a higher bar than
stopping, because the cost of trading without an edge is not the cost of
waiting. It falls back to learning only; it never stops learning.
*/
type SkillMeter struct {
	account                  Account
	mode                     Mode
	since                    time.Time
	reason                   string
	weight, squaredWeight    float64
	mean, deviation          float64
	samples                  uint64
	wins, losses             uint64
	promotions, demotions    uint64
}

/* NewSkillMeter starts calibrating against a configured account. */
func NewSkillMeter(account Account, at time.Time) *SkillMeter {
	return &SkillMeter{
		account: account,
		mode:    ModeLearning,
		since:   at,
		reason:  "no resolved policy outcomes yet",
	}
}

/* Account reports which account trading would reach, from configuration. */
func (meter *SkillMeter) Account() Account { return meter.account }

/* Mode reports the execution authority the measurement currently justifies. */
func (meter *SkillMeter) Mode() Mode { return meter.mode }

/*
Observe incorporates one resolved policy decision and re-evaluates authority.
Authority is the observation quality fixed at issue time, in [0, 1]; a zero
records the completion without inventing trusted evidence.
*/
func (meter *SkillMeter) Observe(target, authority float64, at time.Time) {
	meter.samples++

	if target > 0 {
		meter.wins++
	}

	if target < 0 {
		meter.losses++
	}

	if authority <= 0 || authority > 1 || math.IsNaN(target) || math.IsInf(target, 0) {
		meter.evaluate(at)
		return
	}

	// Exponential forgetting over the declared retention window. Discounting
	// both weight sums keeps the Kish support bounded by that window, so a
	// long-lived agent cannot accumulate unfalsifiable confidence.
	decay := 1 - 1/skillMemory
	meter.weight *= decay
	meter.squaredWeight *= decay * decay

	if meter.weight == 0 {
		meter.mean, meter.weight, meter.squaredWeight = target, authority, authority*authority
		meter.evaluate(at)
		return
	}

	meter.deviation *= decay
	meter.weight += authority
	meter.squaredWeight += authority * authority
	difference := target - meter.mean
	meter.mean += authority / meter.weight * difference
	meter.deviation += authority * difference * (target - meter.mean)
	meter.evaluate(at)
}

/* Reading exposes the current estimate and authority without changing either. */
func (meter *SkillMeter) Reading() SkillReading {
	reading := SkillReading{
		Mode: meter.mode.String(), Account: meter.account.String(), Since: meter.since,
		Reason: meter.reason, Samples: meter.samples, Sigma: skillSigma, Memory: skillMemory,
		Promotions: meter.promotions, Demotions: meter.demotions,
		Wins: meter.wins, Losses: meter.losses,
	}

	if meter.weight == 0 {
		return reading
	}

	reading.Defined, reading.Mean = true, meter.mean
	reading.Support = meter.weight * meter.weight / meter.squaredWeight

	if reading.Support <= 1 {
		return reading
	}

	degrees := meter.weight - meter.squaredWeight/meter.weight

	if degrees <= 0 {
		return reading
	}

	reading.VarianceDefined = true
	reading.Variance = meter.deviation / degrees

	if reading.Variance < 0 {
		reading.Variance = 0
	}

	reading.StandardError = math.Sqrt(reading.Variance / reading.Support)
	reading.LowerBound = reading.Mean - skillSigma*reading.StandardError
	reading.Qualified = reading.Support > skillSigma*skillSigma

	if reading.StandardError > 0 {
		// One-sided empirical normal probability that the mean exceeds zero.
		reading.Confidence = 0.5 * math.Erfc(-reading.Mean/(reading.StandardError*math.Sqrt2))
	}

	return reading
}

/*
evaluate promotes on a measured edge and demotes on a lost one. A promotion
climbs one step so paper always precedes real, and no transition can exceed
the configured ceiling regardless of how good the measurement looks.
*/
func (meter *SkillMeter) evaluate(at time.Time) {
	reading := meter.Reading()

	if !reading.VarianceDefined {
		meter.reason = "dispersion is not estimable yet"
		return
	}

	/*
		A bound stated at skillSigma standard errors assumes at least that many
		errors' worth of evidence behind it. Below skillSigma squared effective
		observations the bound is carried by fewer samples than the multiple
		assumes, and a run of similar outcomes would report near-zero dispersion
		and read as certainty. The floor is derived from the declared confidence
		multiple, not chosen separately from it.
	*/
	if !reading.Qualified {
		meter.reason = "effective evidence is thinner than the confidence bound assumes"

		if meter.mode > ModeLearning && reading.Mean <= 0 {
			meter.mode--
			meter.since, meter.demotions = at, meter.demotions+1
			meter.reason = "measured edge is no longer positive"
		}

		return
	}

	if reading.LowerBound > 0 && meter.mode == ModeLearning && meter.account != AccountNone {
		meter.mode = ModeTrading
		meter.since, meter.promotions = at, meter.promotions+1
		meter.reason = "measured edge exceeds its own standard error"
		return
	}

	if reading.Mean <= 0 && meter.mode == ModeTrading {
		meter.mode = ModeLearning
		meter.since, meter.demotions = at, meter.demotions+1
		meter.reason = "measured edge is no longer positive"
		return
	}

	if reading.LowerBound > 0 && meter.account == AccountNone {
		meter.reason = "measured edge is positive but no account is attached"
		return
	}

	if reading.LowerBound > 0 {
		meter.reason = "trading the configured account on a measured edge"
		return
	}

	if reading.Mean > 0 {
		meter.reason = "edge is positive but inside its measurement error"
		return
	}

	meter.reason = "no measured edge"
}
