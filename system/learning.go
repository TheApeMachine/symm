package system

import (
	"time"

	"github.com/spf13/viper"
)

/*
Learning holds the forward-learning agent's declared temporal operating
choices. There are deliberately few of them: the measurement window is derived
from each instrument's own movement and its own cost, so what remains here is
how much past a decision is conditioned on and the ceiling past which an
outcome is no longer attributable to it. Both are reported to the operator with
every reading rather than buried as literals.
*/
type Learning struct {
	/*
		PrecursorFrames bounds how many past impulse states enter a decision's
		context. Conditioning on an unbounded past does not deepen the model:
		a context carrying the whole run is unique to the moment it was built,
		so it trains a path nothing ever revisits, while the cost of binding
		and recalling it grows with uptime.
	*/
	PrecursorFrames int

	/*
		MaximumHorizon is the longest window an outcome may still be attributed
		to the decision that opened it.

		The window itself is derived, not declared: it is the time this
		instrument's own movement needs before it could plausibly cover the
		round trip the decision paid. This is the ceiling on that derivation,
		and it is an attribution choice rather than a physical one — beyond it,
		too much unrelated tape has passed for the outcome to be evidence about
		the decision.

		An instrument that needs longer than this to cover its own friction is
		reporting that it cannot be traded profitably at this size. That is a
		finding about the instrument, not a number to be tuned until it looks
		tradeable.
	*/
	MaximumHorizon time.Duration

	/*
		RetainedContexts bounds how many distinct observed contexts each market
		keeps, so an episode confirmed behind the tape can be trained against
		the state the agent actually held when it began.

		Grid regions are read from running estimators, so a past context is not
		recoverable from that coordinate's stored metrics — only the agent,
		standing there at the time, ever held it. This is how far back that
		memory reaches: an episode older than the trail is reported as no
		longer knowable rather than trained against a guess.
	*/
	RetainedContexts int

	/*
		EpisodeAuthority is the observation weight carried by evidence learned
		from a confirmed episode rather than from the agent's own resolved
		decision.

		It is below one deliberately. The episode is a real thing the market
		did, but the agent did not act on it, so what is being trained is the
		association between a context and an outcome that followed it — not a
		measured consequence of an action the agent took. Weighting it below a
		lived outcome keeps that difference in the evidence rather than only in
		the commentary.
	*/
	EpisodeAuthority float64

	/*
		WarmupRuns and WarmupContexts bound what the boot-time episode warmup
		reads from the retained record.

		Discovery walks the captured tape, and an unbounded walk back through
		every retained run at boot would compete with the capture writer on the
		database it is still writing to. These are the reach of that walk, and
		what it could not reach is reported rather than being left to look like
		an absence of opportunities.
	*/
	WarmupRuns     int
	WarmupContexts int
}

func NewLearning() *Learning {
	viper.SetDefault("learning.precursor_frames", 8)
	viper.SetDefault("learning.maximum_horizon", "10m")
	viper.SetDefault("learning.retained_contexts", 4096)
	viper.SetDefault("learning.episode_authority", 0.5)
	viper.SetDefault("learning.warmup_runs", 3)
	viper.SetDefault("learning.warmup_contexts", 250000)

	frames := viper.GetInt("learning.precursor_frames")

	if frames < 1 {
		frames = 1
	}

	ceiling := viper.GetDuration("learning.maximum_horizon")

	if ceiling <= 0 {
		ceiling = 10 * time.Minute
	}

	contexts := viper.GetInt("learning.retained_contexts")

	if contexts < 1 {
		contexts = 1
	}
	authority := viper.GetFloat64("learning.episode_authority")

	if authority <= 0 || authority > 1 {
		authority = 0.5
	}

	return &Learning{
		PrecursorFrames:  frames,
		MaximumHorizon:   ceiling,
		RetainedContexts: contexts,
		EpisodeAuthority: authority,
		WarmupRuns:       max(viper.GetInt("learning.warmup_runs"), 0),
		WarmupContexts:   max(viper.GetInt("learning.warmup_contexts"), 1),
	}
}
