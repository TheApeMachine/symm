package strategy

import (
	"bytes"
	"context"
	"fmt"
	rand "math/rand/v2"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/system"
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
const defaultPrecursorHorizon = 8

func precursorHorizon() int {
	if system.Cfg != nil && system.Cfg.Learning.PrecursorHorizon > 0 {
		return system.Cfg.Learning.PrecursorHorizon
	}

	return defaultPrecursorHorizon
}

type Rehearsal struct {
	Chart    *Chart
	catalog  *tables.Catalog
	price    *broker.Price
	impulse  *impulse
	model    *Model
	reporter *Reporter
	epoch    int64
	grade    atomic.Pointer[skill]
}

func newRehearsal(
	catalog *tables.Catalog,
	price *broker.Price,
	impulse *impulse,
	model *Model,
	chart *Chart,
	reporter *Reporter,
	epoch int64,
) *Rehearsal {
	return &Rehearsal{
		Chart:    chart,
		catalog:  catalog,
		price:    price,
		impulse:  impulse,
		model:    model,
		reporter: reporter,
		epoch:    epoch,
	}
}

func (rehearsal *Rehearsal) log(format string, args ...any) {
	if rehearsal != nil && rehearsal.reporter != nil {
		rehearsal.reporter.Log(format, args...)
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
A trie that never learned enter can never open a paper position —
wait is abstention, not a graded leaf action.
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
	passIndex := 0

	for {
		passIndex++
		rehearsal.log("[PASS:%d START] epoch=%d", passIndex, rehearsal.epoch)
		graded, err := rehearsal.pass(ctx)

		if err != nil {
			rehearsal.log("[PASS:%d ERROR] %v", passIndex, err)
			return skill{}, err
		}

		gateStatus := "BLOCKED"

		if graded.open() {
			gateStatus = "OPEN"
		}

		hitRate := 0.0
		callsCount := graded.calls()

		if callsCount > 0 {
			hitRate = float64(graded.hits) / float64(callsCount) * 100
		}

		retainedRate := 0.0

		if callsCount > 0 {
			retainedRate = float64(graded.retained) / float64(callsCount) * 100
		}

		rehearsal.log(
			"[PASS:%d COMPLETE] trained=%d seen=%d hits=%d/%d (%.1f%%) retained=%d/%d (%.1f%%) baseline=%d gate=%s blocker=%q",
			passIndex, graded.trained, graded.seen,
			graded.hits, callsCount, hitRate,
			graded.retained, callsCount, retainedRate,
			graded.baseline(), gateStatus, graded.blocker(),
		)

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

		// Hard errors (missing tape, absurd B/C geometry, inconsistent
		// class, unpriceable friction) stop the pass — skipping those would
		// bias the trie and the skill gate. Soft-skip only when the tape
		// exists but forms no teachable phase (empty asked below). A left
		// pad past tick 0 is clamped, not an error.
		asked, hits, retained, err := rehearsal.learn(ctx, detection, -1)

		if err != nil {
			if strings.Contains(err.Error(), "lights no grid region") {
				rehearsal.log("[EXCURSION:SKIP] key=%s reason=%s", key, err.Error())
				continue
			}

			rehearsal.log("[EXCURSION:ERROR] key=%s err=%v", key, err)
			return skill{}, errnie.Err(errnie.Internal, "[rehearsal] unable to learn detection "+key, err)
		}

		// Soft-skip: tape exists, geometry forms no phase (see learn).
		if len(asked) == 0 {
			rehearsal.log("[EXCURSION:SKIP] key=%s reason=\"geometry forms no phase\"", key)
			continue
		}

		result.trained++
		result.hits += hits
		result.retained += retained

		for action, count := range asked {
			result.asked[action] += count
		}

		if result.trained%10 == 0 || result.trained == 1 {
			totalAsked := 0

			for _, count := range result.asked {
				totalAsked += count
			}

			hitRate := 0.0

			if totalAsked > 0 {
				hitRate = float64(result.hits) / float64(totalAsked) * 100
			}

			retainedRate := 0.0

			if totalAsked > 0 {
				retainedRate = float64(result.retained) / float64(totalAsked) * 100
			}

			rehearsal.log(
				"[PASS:PROGRESS] seen=%d trained=%d asked=%d hits=%d (%.1f%%) retained=%d (%.1f%%) latest=%s",
				len(seen), result.trained, totalAsked, result.hits, hitRate, result.retained, retainedRate, key,
			)
		}
	}

	result.seen = len(seen)
	return result, nil
}

/*
phase is one trainable stretch of a fragment: its deduplicated token
context, the action being graded or inhibited, signed feedback (positive
reinforces, negative weakens), and the frames the context was cut from
(nil for a correction of a wrong call, which augment never perturbs).
*/
type phase struct {
	context  string
	action   string
	feedback float64
	frames   [][]byte
}

/*
drill is the A-dependent stretch of a fragment: A may start on any frame
before limit, the stretch runs from A up to end, and it teaches enter with
feedback (negative dampens). It is what changes when A moves. Zero feedback
means the graded draw taught nothing from A, so neither does augment.
*/
type drill struct {
	start    int
	limit    int
	end      int
	feedback float64
}

/*
learn cuts one stored excursion into its trainable phases. Terminal trie
actions are enter and exit only (TRAINING.md: exit only if we have
entered). Wait is not a leaf — abstention is the precursor stance when
enter is not the call:

  - up: the precursor from a random A offset up to the frame before
    ignition B teaches enter, with the net round-trip return; the holding
    run from B+1 up to the frame before the peak C teaches exit, with the
    gross move captured. Enter and exit are taught together when both
    frames exist. A short B→C that leaves no holding frame after the
    fill-latency pullback still teaches enter (and exit on the ignition
    window when any frame remains), with feedback scaled down so short
    observations weaken rather than dominate or soft-skip.
  - up_friction / down / chop / flat: the losing sequence dampens enter
    on the precursor context (negative feedback). No wait basin is
    written. If enter was never strong there, dampening inserts a
    penalized enter that cognition may prune once useless. No exit
    without enter.

Wrong non-abstaining predictions on a graded phase are also weakened
(negative feedback on the mistaken action). Exit without enter is a hard
error. Feedback is signed and must be non-zero.

aOffset counts token frames from the first frame A may occupy; a negative
aOffset draws a fresh random A (TRAINING.md fragment rehearsal), so the trie
does not only memorize one precursor length. A is always strictly before B.
That draw is the graded one; augment then teaches the same fragment from
every other distinct A (see augment). Ground-truth ENTER/EXIT UI markers sit
on the sweet-spot frames of the graded phases that teach them; after Teach,
the same contexts are Recall'd under the stance that phase acts from (flat
for enter, holding for exit; no teach) and agreeing predictions are
reported alongside.

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

	switch class {
	case excursionUp, excursionUpShort, excursionDown, excursionChop, excursionFlat:
	default:
		return nil, 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"[rehearsal] detection has unknown excursion class: \""+class+"\"",
			nil,
		))
	}

	lo, hi, err := move.window()

	if err != nil {
		return nil, 0, 0, errnie.Error(err)
	}

	// tape errors on a missing or region-less tape, so from here on the
	// tape exists and every "nothing to learn" return is geometry alone.
	ticks, tokens, err := rehearsal.tape(ctx, detection, lo, hi)

	if err != nil {
		return nil, 0, 0, err
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
		stretch    drill
	)

	// Every phase slice is non-empty and tokens only holds non-empty frames,
	// so an empty context is a slicing bug, never a short window.
	add := func(action string, feedback float64, frames [][]byte) bool {
		context := contextOf(frames)

		if len(context) == 0 {
			return false
		}

		phases = append(phases, phase{context, action, feedback, frames})
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

		weakens = append(weakens, phase{context, action, -magnitude, frames})
	}

	switch class {
	case excursionUp, excursionUpShort, excursionDown:
		// Truly empty: no lit precursor before B (cannot place A < B), or B and
		// C share a frame. Short holding after fill pullback still teaches
		// (weakened) below — that is not empty.
		if ignition < 1 {
			rehearsal.log("[EXCURSION:SKIP] symbol=%s key=%d/%d reason=\"no lit precursor before ignition B (ignition_idx=%d B_tick=%d)\"", detection.Label, detection.Epoch, detection.Tick, ignition, move.b)
			return nil, 0, 0, nil
		}

		if ignition >= peak {
			rehearsal.log("[EXCURSION:SKIP] symbol=%s key=%d/%d reason=\"ignition B and peak C on same token frame (ignition_idx=%d peak_idx=%d)\"", detection.Label, detection.Epoch, detection.Tick, ignition, peak)
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

		horizon := precursorHorizon()
		floorA := 0

		if endB > horizon {
			floorA = endB - horizon
		}

		startA = drawA(floorA, endB)
		precursorFrames := tokens[startA:endB]
		stretch = drill{start: floorA, limit: endB, end: endB}

		/*
			Exit is only taught when enter is (TRAINING.md: "if we have entered").
			Losing precursors (up_friction, down) dampen enter on the same
			context — wait is abstention, never a terminal leaf.
		*/
		if precursor == actionEnter {
			startHolding := ignition + 1
			endC := peak
			scale := 1.0

			if peak > startHolding+1 {
				endC = peak - 1
			}

			if endC-startHolding > horizon {
				startHolding = endC - horizon
			}

			// Short B→C: still teach, but scale feedback down so brief
			// observations weaken rather than soft-skip or outshine longer ones.
			if startHolding >= endC {
				startHolding = ignition
				endC = min(peak+1, len(tokens))
				scale = shortObservationScale
			}

			stretch.feedback = precursorFeedback * scale

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
			// Dampen enter on the losing precursor; no wait leaf.
			if len(contextOf(precursorFrames)) == 0 {
				return nil, 0, 0, errnie.Error(errnie.Err(
					errnie.Internal,
					"[rehearsal] "+class+" phase has an empty context: "+detection.Label,
					nil,
				))
			}

			weaken(actionEnter, precursorFeedback, precursorFrames)

			if precursorFeedback > 0 {
				stretch.feedback = -precursorFeedback
			}
		}
	case excursionChop, excursionFlat:
		end := min(peak+1, len(tokens))

		// Need precursor frames before B so A < B (TRAINING.md).
		if ignition < 1 || ignition >= end {
			rehearsal.log("[EXCURSION:SKIP] symbol=%s key=%d/%d reason=\"no precursor or frames at/after B (ignition_idx=%d end_idx=%d)\"", detection.Label, detection.Epoch, detection.Tick, ignition, end)
			return nil, 0, 0, nil
		}

		horizon := precursorHorizon()
		floorA := 0

		if ignition > horizon {
			floorA = ignition - horizon
		}

		startA = drawA(floorA, ignition)

		if end-startA > horizon {
			end = startA + horizon
		}

		quietFrames := tokens[startA:end]
		stretch = drill{start: floorA, limit: ignition, end: end}

		// Chop/flat: dampen enter only — wait is abstention, not a leaf.
		if len(contextOf(quietFrames)) == 0 {
			return nil, 0, 0, errnie.Error(errnie.Err(
				errnie.Internal,
				"[rehearsal] "+class+" phase has an empty context: "+detection.Label,
				nil,
			))
		}

		weaken(actionEnter, -net, quietFrames)

		if net < 0 {
			stretch.feedback = net
		}
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

	rehearsal.log(
		"[EXCURSION:TRAIN] symbol=%s class=%s ticks=%d..%d B=%d C=%d bPrice=%s cPrice=%s gross=%+.4f%% net=%+.4f%% frames=%d phases=%d",
		detection.Label, class, lo, hi, move.b, move.c,
		move.bPrice.String(), move.cPrice.String(),
		move.gross*100, net*100, len(tokens), len(phases),
	)

	// The fragment's ground-truth lessons, before wrong-call corrections
	// join weakens: augment perturbs these, never a correction.
	lessons := append(slices.Clone(phases), weakens...)
	asked := make(map[string]int, len(phases))
	hits := 0

	for _, learned := range phases {
		call, err := rehearsal.model.Recall(learned.context, "")

		if err != nil {
			return nil, 0, 0, errnie.Error(err)
		}

		asked[learned.action]++
		hit := call.Winner == learned.action

		rehearsal.log(
			"[EXCURSION:PRE_RECALL] symbol=%s action=%s context=%q winner=%s conf=%.3f contrast=%.3f hit=%t",
			detection.Label, learned.action, learned.context, call.Winner, call.Confidence, call.Contrast, hit,
		)

		if hit {
			hits++
			continue
		}

		// Wrong non-abstaining call: actively inhibit the mistaken action.
		if call.Winner != "" {
			magnitude := learned.feedback
			if magnitude < 0 {
				magnitude = -magnitude
			}
			rehearsal.log(
				"[EXCURSION:INHIBIT] symbol=%s mistimed=%s context=%q feedback=%.4f",
				detection.Label, call.Winner, learned.context, -magnitude,
			)
			weakens = append(weakens, phase{learned.context, call.Winner, -magnitude, nil})
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

	if err := rehearsal.augment(
		tokens, stretch, contextOf(tokens[startA:stretch.end]), lessons,
		system.Cfg.Learning.RehearsalDropout,
	); err != nil {
		return nil, 0, 0, err
	}

	// Post-teach predict on the same contexts (no teach). Retained counts
	// unconditioned post-teach agreement for the skill gate, graded the same
	// way as the prequential hits. The predicted* markers ask from the
	// stance the phase acts in — flat for enter, holding for exit — because
	// enter and exit are never alternatives at one moment: unconditioned,
	// exit (taught on the same region spans with the larger gross feedback)
	// outweighs enter on every shared context, and enter never lights.
	predictEnter, predictExit := -1, -1
	retained := 0

	for _, learned := range phases {
		call, err := rehearsal.model.Recall(learned.context, "")

		if err != nil {
			return nil, 0, 0, errnie.Error(err)
		}

		retainedMatch := call.Winner == learned.action

		if retainedMatch {
			retained++
		}

		rehearsal.log(
			"[EXCURSION:POST_RECALL] symbol=%s action=%s context=%q winner=%s retained=%t",
			detection.Label, learned.action, learned.context, call.Winner, retainedMatch,
		)

		stanced, err := rehearsal.model.Recall(learned.context, learned.action)

		if err != nil {
			return nil, 0, 0, errnie.Error(err)
		}

		if stanced.Winner != learned.action {
			continue
		}

		switch learned.action {
		case actionEnter:
			predictEnter = enterFrame
		case actionExit:
			predictExit = exitFrame
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
augment teaches a fragment past its graded draw. Every pass draws one random
A, and that draw is the one the skill gate grades. A single draw ties the
trie to whichever precursor length it happened to pick, so the same fragment
is also taught from every other distinct A: A starting anywhere inside one
run of a repeated region yields the same deduplicated context, so the run
starts before the drill limit enumerate exactly the distinct precursors the
tape offers — a few per fragment, bounded by its region transitions, never a
count chosen here. These are taught, not asked, so the prequential grade
and its constant-policy baseline keep their meaning.

With dropout enabled (system.Learning.RehearsalDropout) each ground-truth
lesson and each augmented precursor is also taught once with one of its
region frames missing and once with one frame swapped for another region of
the same fragment tape — a single edit each, so no noise rate is invented.
*/
func (rehearsal *Rehearsal) augment(
	tokens [][]byte, precursor drill, graded string, lessons []phase, dropout bool,
) error {
	if precursor.end > len(tokens) || precursor.limit > precursor.end || precursor.start < 0 || precursor.start > precursor.limit {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"[rehearsal] drill is outside its tape: start=%d limit=%d end=%d frames=%d feedback=%g",
				precursor.start, precursor.limit, precursor.end, len(tokens), precursor.feedback,
			),
			nil,
		))
	}

	teach := func(context, action string, feedback float64) error {
		if err := rehearsal.model.Teach(context, action, feedback); err != nil {
			return errnie.Error(err)
		}

		return nil
	}

	for start := precursor.start; start < precursor.limit; start++ {
		if precursor.feedback == 0 {
			break
		}

		if start > precursor.start && bytes.Equal(tokens[start], tokens[start-1]) {
			continue
		}

		frames := tokens[start:precursor.end]
		context := contextOf(frames)

		if context == "" || context == graded {
			continue
		}

		if err := teach(context, actionEnter, precursor.feedback); err != nil {
			return err
		}

		lessons = append(lessons, phase{context, actionEnter, precursor.feedback, frames})
	}

	if !dropout {
		return nil
	}

	vocabulary := deduplicateTokens(tokens)

	for _, lesson := range lessons {
		base := deduplicateTokens(lesson.frames)

		if len(base) < 2 {
			continue
		}

		missing := rand.IntN(len(base))
		dropped := append(slices.Clone(base[:missing]), base[missing+1:]...)

		if context := contextOf(dropped); context != lesson.context {
			if err := teach(context, lesson.action, lesson.feedback); err != nil {
				return err
			}
		}

		swap := rand.IntN(len(base))
		var others [][]byte

		for _, token := range vocabulary {
			if !bytes.Equal(token, base[swap]) {
				others = append(others, token)
			}
		}

		if len(others) == 0 {
			continue
		}

		noisy := slices.Clone(base)
		noisy[swap] = others[rand.IntN(len(others))]

		if context := contextOf(noisy); context != lesson.context {
			if err := teach(context, lesson.action, lesson.feedback); err != nil {
				return err
			}
		}
	}

	return nil
}

/*
contextOf joins a stretch of token frames into its deduplicated context.
*/
func contextOf(frames [][]byte) string {
	return string(bytes.Join(deduplicateTokens(frames), []byte("/")))
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
expand the window when the detector already padded further. lo is never
negative (padWindow clamps). Absurd geometry (B before tick 0, or C not
after B) is a hard validation error — soft-skip only applies when the tape
exists but forms no teachable phase.
*/
func (move excursion) window() (lo, hi int64, err error) {
	if move.b < 0 || move.c <= move.b {
		return 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"[rehearsal] absurd excursion geometry: B=%d C=%d",
				move.b, move.c,
			),
			nil,
		))
	}

	// Always size to the move (TRAINING.md sweet-spot fragment), never the
	// full epoch start — that left B on a tiny stub or drowned A in days of
	// tape. end_tick may extend the right pad when the detector stored more.
	lo, hi = padWindow(move.b, move.c)

	if move.start >= 0 && move.start < lo {
		lo = move.start
	}

	if move.end > hi {
		hi = move.end
	}

	if hi < lo {
		return 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"[rehearsal] absurd tape window after pad: lo=%d hi=%d B=%d C=%d",
				lo, hi, move.b, move.c,
			),
			nil,
		))
	}

	return lo, hi, nil
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
class whose stored prices contradict it. An empty precursor action means
dampen enter only — wait is not a terminal action.
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

		// Empty precursor action: dampen enter only (no wait leaf).
		return "", -net, gross, nil
	}

	if gross >= 0 {
		return "", 0, 0, errnie.Error(errnie.Err(
			errnie.Validation, "[rehearsal] down excursion does not fall: "+symbol, nil,
		))
	}

	return "", -net, -gross, nil
}

/*
clampTapeTicks forces a catalog read window onto non-negative ticks with
high >= low. Negative lows come from a left pad past the epoch start; those
are clamped rather than rejected so one early-B detection cannot halt the
whole rehearsal. A window that still cannot satisfy high >= low after clamp
is absurd geometry and a hard validation error.
*/
func clampTapeTicks(low, high int64) (int64, int64, error) {
	if low < 0 {
		low = 0
	}

	if high < low {
		return 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[rehearsal] absurd tape ticks after clamp: low=%d high=%d", low, high),
			nil,
		))
	}

	return low, high, nil
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
	startTick, highTick, err := clampTapeTicks(startTick, highTick)

	if err != nil {
		return nil, nil, err
	}

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

	ticks, tokens, stats, err := rehearsal.impulse.tokens(rows)

	if err != nil {
		return nil, nil, err
	}

	if len(tokens) == 0 {
		return nil, nil, errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"[rehearsal] signal/logic tape lights no grid region: %s (%d rows across %d ticks, peak region score %.6f <= 0)",
				window, len(rows), stats.uniqueTicks, stats.maxScore,
			),
			nil,
		)
	}

	rehearsal.log(
		"[EXCURSION:TAPE] %s rows=%d unique_ticks=%d lit_tokens=%d peak_score=%.4f",
		window, len(rows), stats.uniqueTicks, len(tokens), stats.maxScore,
	)

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
