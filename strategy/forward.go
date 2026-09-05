package strategy

import (
	"context"
	"slices"
	"time"

	"github.com/theapemachine/symm/hindsight"
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
	Reviewed     uint64              `json:"reviewed"`
	Exposed      uint64              `json:"exposed"`
	Unexposed    uint64              `json:"unexposed"`
	Captured     uint64              `json:"captured"`
	Missed       uint64              `json:"missed"`
	Unreviewable uint64              `json:"unreviewable"`
	At           time.Time           `json:"at"`
	Recent       []MissedOpportunity `json:"recent"`
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
