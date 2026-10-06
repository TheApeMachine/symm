package strategy

import (
	"bytes"
	"context"
	"fmt"
	rand "math/rand/v2"
	"slices"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Rehearsal replays the stored excursions of past runs through the frozen
Impulse Map and teaches the Model their phases (TRAINING.md fragment
rehearsal). Every phase is asked before it is taught, so each pass also
grades the Model prequentially for the skill gate. It owns that grade; the
Chart owns the learned fragments the UI draws.
*/
// shortObservationScale damps Teach feedback on short B→C geometry so brief
// excursions still train (weaken short observations) instead of soft-skipping.
const shortObservationScale = 0.25

type Rehearsal struct {
	Chart   *Chart
	catalog *tables.Catalog
	price   *broker.Price
	impulse *impulse
	model   *Model
	epoch   int64
	grade   atomic.Pointer[skill]
}

func newRehearsal(
	catalog *tables.Catalog,
	price *broker.Price,
	impulse *impulse,
	model *Model,
	chart *Chart,
	epoch int64,
) *Rehearsal {
	return &Rehearsal{
		Chart:   chart,
		catalog: catalog,
		price:   price,
		impulse: impulse,
		model:   model,
		epoch:   epoch,
	}
}

/*
skill is one rehearsal pass's prequential grade. A constant policy (always
enter, always exit, always wait) scores exactly the calls whose truth is its
action, and an abstaining trie scores zero, so the best constant policy
(baseline) is the largest single-action share of the pass's calls.
*/
type skill struct {
	trained  int
	latest   int64
	seen     int
	hits     int
	retained int // post-teach agreement on graded phases
	asked    map[string]int
}

func (skill skill) baseline() int {
	best := 0

	for _, count := range skill.asked {
		best = max(best, count)
	}

	return best
}

func (skill skill) calls() int {
	calls := 0

	for _, count := range skill.asked {
		calls += count
	}

	return calls
}

/*
open is the skill gate (TRAINING.md: paper trading starts once the model "has
built up enough skill"): enter had ground truth to grade, and either the
prequential pass beat the best constant policy, or post-teach retention did.
Retention covers the case where random A offsets keep creating novel
contexts so prequential hits stay low even after the trie learned enter/exit.
A trie that only learned wait can never open a paper position.
*/
func (skill skill) open() bool {
	if skill.trained == 0 || skill.asked[actionEnter] == 0 {
		return false
	}

	baseline := skill.baseline()
	return skill.hits > baseline || skill.retained > baseline
}

/*
blocker names what keeps the gate closed. Past runs without a single
learnable excursion are not a lack of skill: there is nothing to grade yet.
*/
func (skill skill) blocker() string {
	if skill.trained == 0 {
		return fmt.Sprintf("nothing to rehearse: %d stored excursions of past runs form no phase", skill.seen)
	}

	if skill.asked[actionEnter] == 0 {
		return fmt.Sprintf("skill gate: no enter phase among %d rehearsed excursions", skill.trained)
	}

	return fmt.Sprintf(
		"skill gate: %d of %d calls correct (retained %d), constant-policy baseline %d",
		skill.hits, skill.calls(), skill.retained, skill.baseline(),
	)
}

/*
run rehearses pass after pass for as long as each pass grades better than
every pass before it, and answers the best grade. Every pass draws fresh
random A offsets, so one pass grading below the best is a worse draw of
precursors, not lost skill: the trie only ever gains associations. The best
pass is therefore the grade the trie earned, and an unlucky final draw never
closes the gate on it. A first pass that learns nothing has nothing to
rehearse. The grade is published after every pass for Step's blocker.
*/
func (rehearsal *Rehearsal) run(ctx context.Context) (skill, error) {
	var best *skill

	for {
		graded, err := rehearsal.pass(ctx)

		if err != nil {
			return skill{}, err
		}

		errnie.Info(fmt.Sprintf(
			"[rehearsal] pass: %d of %d excursions, %d of %d calls correct (retained %d), baseline %d",
			graded.trained, graded.seen, graded.hits, graded.calls(), graded.retained, graded.baseline(),
		))

		if best != nil && graded.hits <= best.hits {
			return *best, nil
		}

		best = &graded
		rehearsal.grade.Store(best)

		if graded.trained == 0 {
			return graded, nil
		}
	}
}

/*
pass rehearses every stored excursion of past runs once, each from a fresh
random A offset.
*/
func (rehearsal *Rehearsal) pass(ctx context.Context) (skill, error) {
	result := skill{asked: make(map[string]int)}

	if rehearsal.catalog == nil {
		return result, nil
	}

	seen := make(map[string]struct{})

	for detection, err := range rehearsal.catalog.Detections(ctx) {
		// A failed read is not the end of the stored excursions: a pass
		// over the part that loaded would grade the trie on a subset.
		if ctx.Err() != nil {
			return skill{}, ctx.Err()
		}

		if err != nil {
			return skill{}, errnie.Err(errnie.IO, "[rehearsal] unable to read stored detections", err)
		}

		if detection == nil || detection.Epoch >= rehearsal.epoch {
			continue
		}

		key := fmt.Sprintf(
			"%d/%s/%s/%d", detection.Epoch, detection.Label, detection.Meta("type"), detection.Tick,
		)

		if _, done := seen[key]; done {
			continue
		}

		seen[key] = struct{}{}
		result.latest = max(result.latest, detection.Epoch)

		// A detection that cannot be learned (missing tape, inconsistent
		// class, unpriceable friction) stops the pass. Skipping it would
		// bias the trie and the skill gate toward the excursions that
		// happened to load.
		asked, hits, retained, err := rehearsal.learn(ctx, detection, -1)

		if err != nil {
			return skill{}, errnie.Err(errnie.Internal, "[rehearsal] unable to learn detection "+key, err)
		}

		// Its tape exists, but its geometry forms no phase (see learn).
		if len(asked) == 0 {
			continue
		}

		result.trained++
		result.hits += hits
		result.retained += retained

		for action, count := range asked {
			result.asked[action] += count
		}
	}

	result.seen = len(seen)
	return result, nil
}

/*
phase is one trainable stretch of a fragment: its deduplicated token
context, the action being graded or inhibited, and signed feedback
(positive reinforces, negative weakens).
*/
type phase struct {
	context  string
	action   string
	feedback float64
}

/*
learn cuts one stored excursion into its trainable phases. Every class maps
onto the three trie actions (enter, exit, wait) by what the right call was
at that point of the tape (TRAINING.md: exit only if we have entered):

  - up: the precursor from a random A offset up to the frame before
    ignition B teaches enter, with the net round-trip return; the holding
    run from B+1 up to the frame before the peak C teaches exit, with the
    gross move captured. Enter and exit are taught together when both
    frames exist. A short B→C that leaves no holding frame after the
    fill-latency pullback still teaches enter (and exit on the ignition
    window when any frame remains), with feedback scaled down so short
    observations weaken rather than dominate or soft-skip.
  - up_friction / down / chop / flat: the precursor teaches wait with the
    loss or friction avoided, and actively weakens enter on the same
    context (negative feedback). Wait alone would train a wait expert that
    never beats the constant-wait skill baseline; inhibiting enter is what
    makes losing tape push the trie off enter. No exit without enter.

Wrong non-abstaining predictions on a graded phase are also weakened
(negative feedback on the mistaken action). Exit without enter is a hard
error. Feedback is signed and must be non-zero.

aOffset counts token frames from the first frame A may occupy; a negative
aOffset draws a fresh random A (TRAINING.md fragment rehearsal), so the trie
does not only memorize one precursor length. A is always strictly before B.
Ground-truth ENTER/EXIT UI markers sit on the sweet-spot frames of the phases
that teach them; after Teach, the same contexts are Recall'd (no teach) and
those predictions are reported alongside.

Before any phase is taught, the trie is asked which action it would take on
it. The returned map counts the questions per ground-truth action, and the
returned count is how many of them the trie answered correctly; abstention is
a miss. An empty map means the excursion was not learned.

Errors versus "nothing to learn": every stored detection must have its
signal/logic tape, and a detection whose tape is missing, or lights no grid
region, is data loss and returns an error that stops the pass. An empty map
with a nil error is returned only when the tape exists (or is never needed)
but the excursion's geometry cannot form a phase:

  - a directional excursion that ignites on the first tick of its tape has
    no precursor, so its tape is not read at all;
  - no token frame falls before ignition B (no precursor frame), or B and
    the peak C fall onto the same frame;
  - a chop/flat stretch has no token frame at or after B.
*/
func (rehearsal *Rehearsal) learn(
	ctx context.Context, detection *data.Measurement, aOffset int,
) (map[string]int, int, int, error) {
	move, err := excursionOf(detection)

	if err != nil {
		return nil, 0, 0, errnie.Error(err)
	}

	net, err := rehearsal.net(detection.Label, move.bPrice, move.cPrice)

	if err != nil {
		return nil, 0, 0, errnie.Error(err)
	}

	class := move.class
	lo, hi := move.window()

	// tape errors on a missing or region-less tape, so from here on the
	// tape exists and every "nothing to learn" return is geometry alone.
	ticks, tokens, err := rehearsal.tape(ctx, detection, lo, hi)

	if err != nil {
		return nil, 0, 0, errnie.Error(err)
	}

	ignition, _ := slices.BinarySearch(ticks, move.b)
	peak, _ := slices.BinarySearch(ticks, move.c)

	drawA := func(from, to int) int {
		// A occupies [from, to): to must be at least from+1 and strictly
		// before ignition so MarkA < MarkB.
		if to <= from {
			return from
		}

		if to-from == 1 {
			return from
		}

		if aOffset < 0 {
			return from + rand.IntN(to-from)
		}

		return from + min(aOffset, to-from-1)
	}

	var (
		phases     []phase
		weakens    []phase
		startA     int
		enterFrame = -1
		exitFrame  = -1
	)

	contextOf := func(frames [][]byte) string {
		return string(bytes.Join(deduplicateTokens(frames), []byte("/")))
	}

	// Every phase slice is non-empty and tokens only holds non-empty frames,
	// so an empty context is a slicing bug, never a short window.
	add := func(action string, feedback float64, frames [][]byte) bool {
		context := contextOf(frames)

		if len(context) == 0 {
			return false
		}

		phases = append(phases, phase{context, action, feedback})
		return true
	}

	weaken := func(action string, magnitude float64, frames [][]byte) {
		if magnitude <= 0 {
			return
		}

		context := contextOf(frames)

		if len(context) == 0 {
			return
		}

		weakens = append(weakens, phase{context, action, -magnitude})
	}

	switch class {
	case excursionUp, excursionUpShort, excursionDown:
		// Truly empty: no lit precursor before B (cannot place A < B), or B and
		// C share a frame. Short holding after fill pullback still teaches
		// (weakened) below — that is not empty.
		if ignition < 1 || ignition >= peak {
			return nil, 0, 0, nil
		}

		precursor, precursorFeedback, holdingFeedback, err := directionalFeedback(
			class, detection.Label, net, move.gross,
		)

		if err != nil {
			return nil, 0, 0, err
		}

		// Pull back context before B to account for order fill latency.
		endB := ignition

		if ignition > 1 {
			endB = ignition - 1
		}

		startA = drawA(0, endB)
		precursorFrames := tokens[startA:endB]

		/*
			Exit is only taught when enter is (TRAINING.md: "if we have entered").
			Losing precursors (up_friction, down) teach wait and actively weaken
			enter on the same context — wait alone would train a wait expert.
		*/
		if precursor == actionEnter {
			startHolding := ignition + 1
			endC := peak
			scale := 1.0

			if peak > startHolding+1 {
				endC = peak - 1
			}

			// Short B→C: still teach, but scale feedback down so brief
			// observations weaken rather than soft-skip or outshine longer ones.
			if startHolding >= endC {
				startHolding = ignition
				endC = min(peak+1, len(tokens))
				scale = shortObservationScale
			}

			if startHolding >= endC {
				// Only the precursor remains — teach enter weakened, no exit.
				if !add(actionEnter, precursorFeedback*scale, precursorFrames) {
					return nil, 0, 0, errnie.Error(errnie.Err(
						errnie.Internal,
						"[rehearsal] "+class+" phase has an empty context: "+detection.Label,
						nil,
					))
				}

				enterFrame = max(endB-1, startA)
			} else {
				if !add(actionEnter, precursorFeedback*scale, precursorFrames) ||
					!add(actionExit, holdingFeedback*scale, tokens[startHolding:endC]) {
					return nil, 0, 0, errnie.Error(errnie.Err(
						errnie.Internal,
						"[rehearsal] "+class+" phase has an empty context: "+detection.Label,
						nil,
					))
				}

				enterFrame = max(endB-1, startA)
				exitFrame = max(endC-1, startHolding)
			}
		} else {
			if !add(actionWait, precursorFeedback, precursorFrames) {
				return nil, 0, 0, errnie.Error(errnie.Err(
					errnie.Internal,
					"[rehearsal] "+class+" phase has an empty context: "+detection.Label,
					nil,
				))
			}

			weaken(actionEnter, precursorFeedback, precursorFrames)
		}
	case excursionChop, excursionFlat:
		end := min(peak+1, len(tokens))

		// Need precursor frames before B so A < B (TRAINING.md).
		if ignition < 1 || ignition >= end {
			return nil, 0, 0, nil
		}

		startA = drawA(0, ignition)
		quietFrames := tokens[startA:end]

		if !add(actionWait, -net, quietFrames) {
			return nil, 0, 0, errnie.Error(errnie.Err(
				errnie.Internal,
				"[rehearsal] "+class+" phase has an empty context: "+detection.Label,
				nil,
			))
		}

		weaken(actionEnter, -net, quietFrames)
	default:
		return nil, 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"[rehearsal] detection has unknown excursion class: \""+class+"\"",
			nil,
		))
	}

	for _, learned := range phases {
		if learned.feedback == 0 {
			return nil, 0, 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"[rehearsal] "+class+" "+learned.action+" phase has zero feedback: "+detection.Label,
				nil,
			))
		}
	}

	hasEnter, hasExit := false, false

	for _, learned := range phases {
		switch learned.action {
		case actionEnter:
			hasEnter = true
		case actionExit:
			hasExit = true
		}
	}

	if hasExit && !hasEnter {
		return nil, 0, 0, errnie.Error(errnie.Err(
			errnie.Conflict,
			"[rehearsal] exit phase without enter: "+class+" "+detection.Label,
			nil,
		))
	}

	asked := make(map[string]int, len(phases))
	hits := 0

	for _, learned := range phases {
		call, err := rehearsal.model.Recall(learned.context)

		if err != nil {
			return nil, 0, 0, errnie.Error(err)
		}

		asked[learned.action]++

		if call.Winner == learned.action {
			hits++
			continue
		}

		// Wrong non-abstaining call: actively inhibit the mistaken action.
		if call.Winner != "" {
			magnitude := learned.feedback
			if magnitude < 0 {
				magnitude = -magnitude
			}
			weakens = append(weakens, phase{learned.context, call.Winner, -magnitude})
		}
	}

	for _, learned := range phases {
		if err := rehearsal.model.Teach(learned.context, learned.action, learned.feedback); err != nil {
			return nil, 0, 0, errnie.Error(err)
		}
	}

	for _, inhibited := range weakens {
		if inhibited.feedback >= 0 {
			return nil, 0, 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"[rehearsal] weaken phase must carry negative feedback: "+detection.Label,
				nil,
			))
		}

		if err := rehearsal.model.Teach(inhibited.context, inhibited.action, inhibited.feedback); err != nil {
			return nil, 0, 0, errnie.Error(err)
		}
	}

	// Post-teach predict on the same contexts (no teach): which action the
	// model would now take. Ground-truth sweet spots stay on enter/exit;
	// predicted* frames light only when Recall agrees with the taught action.
	// Retained counts post-teach agreement for the skill gate.
	predictEnter, predictExit := -1, -1
	retained := 0

	for _, learned := range phases {
		call, err := rehearsal.model.Recall(learned.context)

		if err != nil {
			return nil, 0, 0, errnie.Error(err)
		}

		if call.Winner == learned.action {
			retained++
		}

		switch learned.action {
		case actionEnter:
			if call.Winner == actionEnter {
				predictEnter = enterFrame
			}
		case actionExit:
			if call.Winner == actionExit {
				predictExit = exitFrame
			}
		}
	}

	if err := rehearsal.Chart.draw(ctx, detection, move, frames{
		ticks:          ticks,
		tokens:         tokens,
		a:              startA,
		enter:          enterFrame,
		exit:           exitFrame,
		predictedEnter: predictEnter,
		predictedExit:  predictExit,
	}); err != nil {
		return nil, 0, 0, err
	}

	return asked, hits, retained, nil
}

/*
excursion is one stored detection read back: its class, its start, B, C, and
optional end ticks, its B and C prices, and the gross move from B to C.
*/
type excursion struct {
	class  string
	start  int64
	b      int64
	c      int64
	end    int64
	bPrice *decimal.Decimal
	cPrice *decimal.Decimal
	gross  float64
}

/*
window is the tape fragment rehearsal and chart read: precursor left of B and
some tape right of C (TRAINING.md), sized to the move itself. Stored start/end
expand the window when the detector already padded further.
*/
func (move excursion) window() (lo, hi int64) {
	// Always size to the move (TRAINING.md sweet-spot fragment), never the
	// full epoch start — that left B on a tiny stub or drowned A in days of
	// tape. end_tick may extend the right pad when the detector stored more.
	lo, hi = padWindow(move.b, move.c)

	if move.end > hi {
		hi = move.end
	}

	return lo, hi
}

func excursionOf(detection *data.Measurement) (excursion, error) {
	start, b, c, err := tables.DetectionTicks(detection)

	if err != nil {
		return excursion{}, errnie.Error(err)
	}

	bPrice, cPrice, err := tables.DetectionPrices(detection)

	if err != nil {
		return excursion{}, errnie.Error(err)
	}

	end := c

	if entry := data.Pull(detection.Read("end_tick")); entry != nil && entry.Err == nil && entry.Metric != nil {
		if raw := int64(entry.Metric.Raw); raw >= c {
			end = raw
		}
	}

	return excursion{
		class:  detection.Meta("type"),
		start:  start,
		b:      b,
		c:      c,
		end:    end,
		bPrice: bPrice,
		cPrice: cPrice,
		gross:  cPrice.Sub(bPrice).SetScale(decimal.DefaultScale).Div(bPrice).Float64(),
	}, nil
}

/*
directionalFeedback answers the precursor action and the feedback of the
precursor and holding phases of a directional excursion, and rejects a
class whose stored prices contradict it.
*/
func directionalFeedback(
	class, symbol string, net, gross float64,
) (string, float64, float64, error) {
	switch class {
	case excursionUp:
		if net <= 0 {
			return "", 0, 0, errnie.Error(errnie.Err(
				errnie.Validation, "[rehearsal] up excursion does not clear friction: "+symbol, nil,
			))
		}

		return actionEnter, net, gross, nil
	case excursionUpShort:
		if net > 0 {
			return "", 0, 0, errnie.Error(errnie.Err(
				errnie.Validation, "[rehearsal] up_friction excursion clears friction: "+symbol, nil,
			))
		}

		return actionWait, -net, gross, nil
	}

	if gross >= 0 {
		return "", 0, 0, errnie.Error(errnie.Err(
			errnie.Validation, "[rehearsal] down excursion does not fall: "+symbol, nil,
		))
	}

	return actionWait, -net, -gross, nil
}

/*
tape reads the excursion's sensory signal/logic tape from its start up to
the peak and encodes one region token per tick on the frozen grid. A stored
detection always has its tape, so a window with no sensory rows, or whose
rows light no grid region at all, is an error, never an empty window.
*/
func (rehearsal *Rehearsal) tape(
	ctx context.Context, detection *data.Measurement, startTick, highTick int64,
) ([]int64, [][]byte, error) {
	window := fmt.Sprintf(
		"%s epoch %d ticks %d..%d", detection.Label, detection.Epoch, startTick, highTick,
	)

	var rows []*data.Measurement

	for measurement, err := range rehearsal.catalog.SignalLogic(
		ctx, detection.Epoch, detection.Label, startTick, highTick,
	) {
		// A failed read must not look like a missing or shorter tape.
		if err != nil {
			return nil, nil, errnie.Err(errnie.IO, "[rehearsal] unable to read signal/logic tape: "+window, err)
		}

		if isSolver(measurement) {
			continue
		}

		rows = append(rows, measurement)
	}

	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	if len(rows) == 0 {
		return nil, nil, errnie.Err(
			errnie.NotFound, "[rehearsal] detection has no signal/logic tape: "+window, nil,
		)
	}

	ticks, tokens, err := rehearsal.impulse.tokens(rows)

	if err != nil {
		return nil, nil, err
	}

	if len(tokens) == 0 {
		return nil, nil, errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[rehearsal] signal/logic tape lights no grid region: %s (%d rows)", window, len(rows)),
			nil,
		)
	}

	return ticks, tokens, nil
}

/*
net is the round-trip return of buying at entry and selling at exit after
taker friction, relative to the entry cost.
*/
func (rehearsal *Rehearsal) net(symbol string, entry, exit *decimal.Decimal) (float64, error) {
	pnl, cost, err := rehearsal.price.RoundTrip(symbol, entry, exit)

	if err != nil {
		return 0, errnie.Error(err)
	}

	if pnl == nil || cost == nil || cost.Sign() <= 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "[rehearsal] round trip has no cost basis: "+symbol, nil,
		))
	}

	return pnl.SetScale(decimal.DefaultScale).Div(cost).Float64(), nil
}

func deduplicateTokens(tokens [][]byte) [][]byte {
	var deduped [][]byte

	for _, tok := range tokens {
		if len(tok) == 0 {
			continue
		}

		if len(deduped) == 0 || !bytes.Equal(deduped[len(deduped)-1], tok) {
			deduped = append(deduped, tok)
		}
	}

	return deduped
}
