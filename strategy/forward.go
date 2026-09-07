package strategy

import (
	"context"
	"math"
	"slices"
	"time"

	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
exposureSpans bounds the retained exposure history per symbol. Reviews arrive
behind the tape, so only recent spans can still be judged; an older episode is
reviewed against evidence that has already rolled off and is reported as
unknown rather than as a miss.
*/
const exposureSpans = 64

/* exposureSpan is one stretch during which the policy lane held inventory. */
type exposureSpan struct {
	fromSeq, toSeq hindsight.CaptureSequence
	from, to       time.Time
	open           bool
}

/*
MissedOpportunity is one confirmed price excursion on the tape, judged against
what the policy lane was actually holding while it happened.

Exposed reports that the lane held inventory at some point inside the episode.
It is not a claim that the lane captured the excursion, or that entering would
have been correct: an excursion is only visible once it has completed, and the
decision had to be made before that. Unreviewable marks an episode older than
the retained exposure history, where the honest answer is that we no longer
know.
*/
type MissedOpportunity struct {
	Symbol       string                    `json:"symbol"`
	Kind         string                    `json:"kind"`
	FromSequence hindsight.CaptureSequence `json:"fromSequence,omitempty"`
	ToSequence   hindsight.CaptureSequence `json:"toSequence,omitempty"`
	FromAt       time.Time                 `json:"fromAt"`
	ToAt         time.Time                 `json:"toAt"`
	Excursion    float64                   `json:"excursion"`
	Observations int                       `json:"observations"`
	Exposed      bool                      `json:"exposed"`
	Unreviewable bool                      `json:"unreviewable"`

	/*
		Trained reports that this episode became evidence rather than only a
		tally. Untrainable names why it could not, so an episode the agent
		learned nothing from is never silently indistinguishable from one it
		learned from.
	*/
	Trained     bool   `json:"trained"`
	Untrainable string `json:"untrainable,omitempty"`
}

/*
ForwardReview is the standing account of what the tape offered against what the
policy lane held. It is the forward-testing counterpart to a backtest: rather
than replaying history against the current model, it lets the market run
slightly ahead of the reviewer and asks what actually happened.

Exposed and Unexposed count confirmed excursions the lane was and was not holding
through. Neither is a score — an excursion nobody could have known about in
advance is not a mistake — but a policy that is never exposed to any of them
has no path to an edge, and that is visible here and nowhere else. Captured and
Missed are retained as alias fields for wire compatibility.
*/
type ForwardReview struct {
	Reviewed     uint64 `json:"reviewed"`
	Exposed      uint64 `json:"exposed"`
	Unexposed    uint64 `json:"unexposed"`
	Captured     uint64 `json:"captured"`
	Missed       uint64 `json:"missed"`
	Unreviewable uint64 `json:"unreviewable"`

	/*
		Trained counts the confirmed episodes that became model evidence. This
		is the number that says whether the tape is teaching the agent anything:
		an exposure tally can look healthy while nothing at all is being learned
		from it.
	*/
	Trained         uint64    `json:"trained"`
	Untrained       uint64    `json:"untrained"`
	LastUntrainable string    `json:"lastUntrainable,omitempty"`
	At              time.Time `json:"at"`

	Recent []MissedOpportunity `json:"recent"`
}

/* recentReviewed bounds the episode list retained for operator inspection. */
const recentReviewed = 40

/*
markExposure records the policy lane's inventory transitions for this symbol.
Only transitions are stored: a lane holding through a thousand book updates is
one span, not a thousand. Both capture sequence (causal order) and wall time
are recorded.
*/
func (market *learningMarket) markExposure(
	exposed bool, seq hindsight.CaptureSequence, at time.Time,
) {
	last := len(market.exposure) - 1

	if exposed {
		if last >= 0 && market.exposure[last].open {
			market.exposure[last].to = at
			market.exposure[last].toSeq = seq
			return
		}

		market.exposure = append(market.exposure, exposureSpan{
			fromSeq: seq, toSeq: seq,
			from: at, to: at,
			open: true,
		})

		if len(market.exposure) > exposureSpans {
			market.exposure = append(market.exposure[:0], market.exposure[1:]...)
		}

		return
	}

	if last >= 0 && market.exposure[last].open {
		market.exposure[last].to = at
		market.exposure[last].toSeq = seq
		market.exposure[last].open = false
	}
}

/*
heldDuring reports whether the policy lane held inventory inside an episode window.
Causal sequence identity (FromSequence/ToSequence) is preferred over wall time
whenever positive sequence values are present.
*/
func (market *learningMarket) heldDuring(
	fromSeq, toSeq hindsight.CaptureSequence, from, to time.Time,
) (held, known bool) {
	if len(market.exposure) == 0 {
		return false, false
	}

	// Use causal sequence order if both boundaries carry positive sequences.
	if fromSeq > 0 && toSeq > 0 && market.exposure[0].fromSeq > 0 {
		if toSeq < market.exposure[0].fromSeq {
			return false, false
		}

		for _, span := range market.exposure {
			endSeq := span.toSeq

			if span.open {
				endSeq = toSeq
			}

			if span.fromSeq <= toSeq && endSeq >= fromSeq {
				return true, true
			}
		}

		return false, true
	}

	// An episode that ended before the retained history began cannot be judged
	// from it, and saying "not exposed" there would invent a miss.
	if to.Before(market.exposure[0].from) {
		return false, false
	}

	for _, span := range market.exposure {
		end := span.to

		if span.open {
			end = to
		}

		if !span.from.After(to) && !end.Before(from) {
			return true, true
		}
	}

	return false, true
}

/* PolicyReview owns retrospective exposure comparisons and no model dependency. */
type PolicyReview struct {
	ctx      context.Context
	reviews  chan []hindsight.Episode
	reviewed map[string]struct{}
	forward  ForwardReview
	local    *LocalLearning
}

/*
Review folds one batch of confirmed episodes into the standing account. The
caller supplies episodes the market has already resolved; this compares them
against what the policy lane was holding and never re-judges an episode twice.
*/
func (reviewer *PolicyReview) Review(ctx context.Context, episodes []hindsight.Episode) error {
	if len(episodes) == 0 {
		return nil
	}

	select {
	case reviewer.reviews <- episodes:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-reviewer.ctx.Done():
		return reviewer.ctx.Err()
	}
}

/* review runs exclusively on the workspace owner, off the ordinary hot path. */
func (reviewer *PolicyReview) review(episodes []hindsight.Episode) {
	reviewer.forward.At = reviewer.local.now()

	for _, episode := range episodes {
		if !episode.Confirmed || episode.ID == "" || reviewer.local.markets[episode.Symbol] == nil {
			continue
		}

		if _, seen := reviewer.reviewed[episode.ID]; seen {
			continue
		}

		if reviewer.reviewed == nil {
			reviewer.reviewed = make(map[string]struct{})
		}

		reviewer.reviewed[episode.ID] = struct{}{}
		opportunity := MissedOpportunity{
			Symbol: episode.Symbol, Kind: string(episode.Kind),
			FromSequence: episode.FromSequence, ToSequence: episode.ToSequence,
			FromAt: episode.FromAt, ToAt: episode.ToAt,
			Excursion: episode.ObservedExcursion, Observations: episode.Observations,
		}

		market := reviewer.local.markets[episode.Symbol]

		if market == nil {
			opportunity.Unreviewable = true
		} else {
			held, known := market.heldDuring(
				episode.FromSequence, episode.ToSequence, episode.FromAt, episode.ToAt,
			)
			opportunity.Exposed, opportunity.Unreviewable = held, !known

			// The tape has answered what followed this context. That answer is
			// the training signal; counting the episode is only the report.
			trained, reason := reviewer.train(market, episode)
			opportunity.Trained, opportunity.Untrainable = trained, reason

			if trained {
				reviewer.forward.Trained++
			} else {
				reviewer.forward.Untrained++
				reviewer.forward.LastUntrainable = reason
			}
		}

		reviewer.forward.Reviewed++

		switch {
		case opportunity.Unreviewable:
			reviewer.forward.Unreviewable++
		case opportunity.Exposed:
			reviewer.forward.Exposed++
			reviewer.forward.Captured++
		default:
			reviewer.forward.Unexposed++
			reviewer.forward.Missed++
		}

		reviewer.forward.Recent = append(reviewer.forward.Recent, opportunity)
	}

	slices.SortFunc(reviewer.forward.Recent, func(left, right MissedOpportunity) int {
		if right.ToSequence != left.ToSequence {
			if right.ToSequence > left.ToSequence {
				return 1
			}
			return -1
		}
		return right.ToAt.Compare(left.ToAt)
	})

	if len(reviewer.forward.Recent) > recentReviewed {
		reviewer.forward.Recent = reviewer.forward.Recent[:recentReviewed]
	}
}

/*
Learning from what the tape actually offered.

The agent's own decisions are a poor teacher on their own. It issues them
continuously, only a few cover disjoint windows, and on a fee-dominated venue
the average move over any short window never covers the round trip — so trained
that way it can only ever conclude that acting is worse than waiting.

The tape answers the question directly instead. Hindsight discovers completed
excursions after the fact: the anchor where a move began, the extremum where it
exhausted, and the distance actually travelled between them. That is ground
truth, and it arrives without anyone labelling anything — the future tape is the
label. Every confirmed episode on every instrument is a training example, which
is a different order of evidence than the handful of the agent's own decisions
that qualify as independent.

What is trained is the association the agent needs at the moment of decision:
given these regions lit up, in the context of what this symbol had been doing,
what followed. The precursor context already carries that history, so the
retained context at the anchor is exactly "what led here".

Nothing here trains on an unconfirmed episode, and nothing trains on an episode
older than the retained trail. In both cases the honest reading is that the
evidence is not available, and an absent context is never replaced by a guess.
*/

/* episodeReference finds one role's capture coordinate on a confirmed episode. */
func episodeReference(
	episode hindsight.Episode, role hindsight.ReferenceRole,
) (hindsight.ReferencePoint, bool) {
	for _, reference := range episode.References {
		if reference.Role == role {
			return reference, true
		}
	}

	return hindsight.ReferencePoint{}, false
}

/*
train folds one confirmed excursion into the model as the outcome that actually
followed the context the agent held at the time.

Growth is net of the round trip the trade would have paid. A 40% move that costs
1.6% to enter and leave is a 38% move, and stating it net keeps the fee-dominated
economics inside the evidence rather than in a caveat beside it. Direction
matters too: this desk holds spot inventory, so an upward excursion is an
opportunity to have been long, while a downward one is not an opportunity to be
short — it is evidence that holding through it was the mistake.
*/
func (reviewer *PolicyReview) train(
	market *learningMarket, episode hindsight.Episode,
) (trained bool, reason string) {
	anchor, extremum, ok, reason := episodeBounds(episode)

	if !ok {
		return false, reason
	}

	begun, known := market.contextAt(anchor.Capture.Sequence)

	if !known {
		return false, "context at the anchor is no longer retained"
	}
	exhausted, known := market.contextAt(extremum.Capture.Sequence)

	if !known {
		return false, "context at the extremum is no longer retained"
	}

	return reviewer.local.Knowledge.trainEpisode(
		market.symbol, market.cost, begun, exhausted, episode, anchor, extremum,
	)
}

/*
episodeBounds names the two coordinates a confirmed excursion is trained
between: where the move began, and where it exhausted.
*/
func episodeBounds(
	episode hindsight.Episode,
) (anchor, extremum hindsight.ReferencePoint, ok bool, reason string) {
	anchor, found := episodeReference(episode, hindsight.ReferenceAnchor)

	if !found {
		return anchor, extremum, false, "no anchor reference"
	}

	/*
		A reversal's turning point is its extremum. It is the same shape as a
		peak — the move ran to here and then gave ground — and it is the
		episode kind that carries the whole trade: where to get in, where it
		exhausted, and what holding past it cost. Excluding it discards the
		clearest examples on the tape.
	*/
	for _, role := range []hindsight.ReferenceRole{
		hindsight.ReferencePeak, hindsight.ReferenceTrough, hindsight.ReferenceReversal,
	} {
		if extremum, found = episodeReference(episode, role); found {
			break
		}
	}

	if !found {
		return anchor, extremum, false, "no extremum reference"
	}

	if !anchor.HasValue || !extremum.HasValue || anchor.Value <= 0 || extremum.Value <= 0 {
		return anchor, extremum, false, "no priced anchor and extremum"
	}

	return anchor, extremum, true, ""
}

/*
trainEpisode folds one confirmed excursion into the model, given the contexts
that were in force where it began and where it exhausted. The live reviewer and
the boot-time warmup share this: an episode learned from the retained record
must train exactly what the same episode would have trained live.
*/
func (knowledge *Knowledge) trainEpisode(
	symbol string,
	cost float64,
	begun, exhausted observedContext,
	episode hindsight.Episode,
	anchor, extremum hindsight.ReferencePoint,
) (trained bool, reason string) {
	elapsed := extremum.VenueAt.Sub(anchor.VenueAt).Seconds()

	if elapsed <= 0 {
		return false, "no forward time between the anchor and the extremum"
	}

	/*
		The move is measured between the two priced coordinates themselves,
		never from the episode's net excursion.

		They are not the same number. A reversal ends near where it started, so
		its net excursion is close to zero while the leg into its turning point
		covered real ground — and that leg is the trade. Reading the net would
		train a seventeen percent run as though nothing had happened.

		Both legs of the round trip come out of it before it is evidence.
	*/
	ratio := extremum.Value / anchor.Value

	if ratio <= 0 {
		return false, "excursion is not a survivable price ratio"
	}
	growth := math.Log(ratio) - cost

	if growth == 0 {
		return false, "the move did not clear its own round trip"
	}
	authority := system.Cfg.Learning.EpisodeAuthority

	enter := LearningAction{Kind: types.ActionEnter}
	exit := LearningAction{Kind: types.ActionExit, Reduce: true}
	hold := LearningAction{Kind: types.ActionHold}

	if ratio < 1 {
		/*
			A fall is not an entry the desk could have taken. What it teaches is
			that holding through this context lost exactly this much, and that
			leaving was worth the give-back it avoided.
		*/
		if err := knowledge.observe(symbol, begun, hold, growth, elapsed, authority); err != nil {
			return false, err.Error()
		}

		if err := knowledge.observe(symbol, begun, exit, -growth, elapsed, authority); err != nil {
			return false, err.Error()
		}

		return true, ""
	}

	// Entering where the move began earned it; waiting there earned nothing
	// while it ran, which is what made waiting the worse of the two.
	if err := knowledge.observe(symbol, begun, enter, growth, elapsed, authority); err != nil {
		return false, err.Error()
	}

	if err := knowledge.observe(symbol, begun, hold, 0, elapsed, authority); err != nil {
		return false, err.Error()
	}

	// Leaving where it exhausted realised the move. Holding past it earned the
	// give-back instead, so the exit is trained against what staying cost.
	if err := knowledge.observe(symbol, exhausted, exit, growth, elapsed, authority); err != nil {
		return false, err.Error()
	}

	if giveBack, ok := episodeGiveBack(episode, extremum); ok {
		if err := knowledge.observe(symbol, exhausted, hold, giveBack, elapsed, authority); err != nil {
			return false, err.Error()
		}
	}

	return true, ""
}

/*
giveBack measures what holding past the extremum actually cost, from the
episode's own retracement. It reports nothing rather than zero when the episode
carries no reversal or exit reference: an unmeasured give-back is not a claim
that holding was free.
*/
func episodeGiveBack(
	episode hindsight.Episode, extremum hindsight.ReferencePoint,
) (float64, bool) {
	end, found := episodeReference(episode, hindsight.ReferenceReversal)

	if !found {
		end, found = episodeReference(episode, hindsight.ReferenceExitAnchor)
	}

	if !found || !end.HasValue || !extremum.HasValue || extremum.Value <= 0 || end.Value <= 0 {
		return 0, false
	}

	return math.Log(end.Value / extremum.Value), true
}

/*
observe trains one context/action pair in both the symbol's own scope and the
shared one, exactly as a resolved decision does. The account state is the one
the agent was actually in when it held this context, so evidence about entering
never lands on the scope for a desk that was already holding.
*/
func (knowledge *Knowledge) observe(
	symbol string,
	observed observedContext,
	action LearningAction,
	growth, elapsed, authority float64,
) error {
	state := observed.accountState

	if state == "" {
		state = "flat"
	}

	return knowledge.Model.Observe(
		[2]string{symbol, state}, observed.context, action,
		growth, elapsed, authority, [2]string{"", state},
	)
}

/*
EpisodeWarmup reports what the retained record was able to teach. Every count
is a fact about recoverable evidence, never a score: an episode whose context
was never journalled is unlearnable, not a miss.
*/
type EpisodeWarmup struct {
	Runs        int    `json:"runs"`
	Episodes    int    `json:"episodes"`
	Trained     int    `json:"trained"`
	Uncontexted int    `json:"uncontexted"`
	Unusable    int    `json:"unusable"`
	LastReason  string `json:"lastReason,omitempty"`
}

/*
WarmupEpisodes trains the model on excursions the captured record already
confirmed, joined to the contexts the policy lane journalled while they were
happening.

This is what makes the loop usable. The live reviewer only ever sees the run it
is part of, and an excursion is only confirmed once price has turned back from
its extremum — so a short run discovers nothing, and a restart discards every
opportunity the desk ever saw. The retained record does not expire that way: the
moves are already on it, already confirmed, and the contexts that preceded them
were written down as they were issued.

contexts must be the policy lane's issued events for the same run, so the join
is capture-ordered within one identity. cost answers the round trip a trade in
one symbol would pay; a historical spread is not retained, so the caller
supplies what it can account for and the evidence is net of that much rather
than of nothing.
*/
func (agent *Agent) WarmupEpisodes(
	episodes []hindsight.Episode,
	contexts []hindsight.LearningEvent,
	cost func(symbol string) float64,
) EpisodeWarmup {
	report := EpisodeWarmup{}
	trails := map[string][]observedContext{}

	for _, event := range contexts {
		if event.Symbol == "" || len(event.Context) == 0 {
			continue
		}
		state := "flat"

		if event.Inventory != "" && event.Inventory != "0" {
			state = "holding"
		}

		trails[event.Symbol] = append(trails[event.Symbol], observedContext{
			seq: event.Capture.Sequence, at: event.At, accountState: state,
			context: append([]uint64(nil), event.Context...),
		})
	}

	for symbol := range trails {
		slices.SortFunc(trails[symbol], func(left, right observedContext) int {
			return int(left.seq) - int(right.seq)
		})
	}

	for _, episode := range episodes {
		if !episode.Confirmed {
			continue
		}
		report.Episodes++
		anchor, extremum, ok, reason := episodeBounds(episode)

		if !ok {
			report.Unusable++
			report.LastReason = reason
			continue
		}
		begun, known := contextIn(trails[episode.Symbol], anchor.Capture.Sequence)

		if !known {
			report.Uncontexted++
			continue
		}
		exhausted, known := contextIn(trails[episode.Symbol], extremum.Capture.Sequence)

		if !known {
			report.Uncontexted++
			continue
		}
		trained, reason := agent.Knowledge.trainEpisode(
			episode.Symbol, cost(episode.Symbol), begun, exhausted, episode, anchor, extremum,
		)

		if !trained {
			report.Unusable++
			report.LastReason = reason
			continue
		}
		report.Trained++
	}

	return report
}

/*
contextIn answers what the policy lane was conditioning on at a capture
coordinate: the most recent journalled context at or before it. A coordinate
before the first retained context returns nothing, because a context that was
never written down cannot be recovered from one that was written later.
*/
func contextIn(trail []observedContext, seq hindsight.CaptureSequence) (observedContext, bool) {
	if len(trail) == 0 || trail[0].seq > seq {
		return observedContext{}, false
	}
	found := observedContext{}
	ok := false

	for _, entry := range trail {
		if entry.seq > seq {
			break
		}
		found, ok = entry, true
	}

	return found, ok
}
