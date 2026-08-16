package algo

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

type hawkesState = types.Pair[string, types.Map[string, types.Value[float64]]]

/*
Hawkes is a composable bivariate Hawkes self- and cross-exciting process.
It tracks arrivals and intensity dynamics strictly composed from temporal,
calculus, and statistic primitives via the nomagique.Number pipeline.
*/
type Hawkes struct {
	initial types.Input[hawkesState]
	next    types.Input[hawkesState]
	err     error
}

var _ types.IO[hawkesState] = (*Hawkes)(nil)

/*
NewHawkes creates a new composable Hawkes process estimator.
*/
func NewHawkes(
	initial types.Input[hawkesState],
) *Hawkes {
	return &Hawkes{
		initial: initial,
		next:    types.NewInput[hawkesState](),
	}
}

/*
Write stages an arrival stream state.
*/
func (hawkes *Hawkes) Write(input types.IO[hawkesState]) {
	if input == nil {
		hawkes.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"hawkes: input is nil",
			nil,
		))

		return
	}

	priorProjected := hawkes.next.Project()
	inputPair := input.Project().Read()

	if priorProjected.Ready {
		priorPair := priorProjected.Read()

		if priorPair.Key == inputPair.Key {
			hawkes.mergePriorState(priorPair.Value, inputPair.Value)
		}
	}

	hawkes.next.Write(input)
	hawkes.err = nil
}

/*
Read computes updated Hawkes state and outputs measured metrics.
*/
func (hawkes *Hawkes) Read() types.IO[hawkesState] {
	staged := hawkes.next.Read()

	if staged.Error() != "" {
		hawkes.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			staged.Error(),
			nil,
		))

		return hawkes.next
	}

	pair := staged.Project().Read()

	if pair.Key == "" {
		hawkes.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"hawkes: missing symbol key",
			nil,
		))

		return hawkes.next
	}

	collected := pair.Value
	timestamp := hawkes.stamp(collected)

	if timestamp.IsZero() && hawkes.number(collected, "event_count") == 0 {
		hawkes.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"hawkes: arrival timestamp required",
			nil,
		))

		return hawkes.next
	}

	hawkes.ensureDefaults(collected)
	hawkes.processArrival(collected, timestamp)
	hawkes.calculateMetrics(collected, timestamp)

	hawkes.next.Write(types.NewInput(types.NewValue(
		types.NewPair(pair.Key, collected),
	)))
	hawkes.err = nil

	return hawkes.next
}

/*
Project returns the last measured state.
*/
func (hawkes *Hawkes) Project() types.Value[hawkesState] {
	return hawkes.next.Project()
}

/*
Error reports any execution error.
*/
func (hawkes *Hawkes) Error() string {
	if hawkes.err != nil {
		return hawkes.err.Error()
	}

	return hawkes.next.Error()
}

/*
Close resets the Hawkes process state.
*/
func (hawkes *Hawkes) Close() error {
	if err := hawkes.initial.Close(); err != nil {
		return err
	}

	if err := hawkes.next.Close(); err != nil {
		return err
	}

	hawkes.err = nil

	return nil
}

func (hawkes *Hawkes) mergePriorState(
	prior types.Map[string, types.Value[float64]],
	collected types.Map[string, types.Value[float64]],
) {
	keys := []string{
		"first_sec", "first_nsec", "event_count",
		"alpha_event_count", "beta_event_count",
		"lambda_alpha", "lambda_beta", "mu_alpha", "mu_beta",
		"prev_timestamp", "beta", "alpha_aa", "alpha_ab",
		"alpha_ba", "alpha_bb",
	}

	for _, key := range keys {
		if _, hasCurrent := collected.Get(key); !hasCurrent {
			if priorVal, hasPrior := prior.Get(key); hasPrior {
				collected.Put(key, priorVal)
			}
		}
	}
}

func (hawkes *Hawkes) ensureDefaults(
	collected types.Map[string, types.Value[float64]],
) {
	if _, found := collected.Get("beta"); !found {
		collected.Put("beta", types.NewValue(1.0))
		collected.Put("alpha_aa", types.NewValue(0.2))
		collected.Put("alpha_ab", types.NewValue(0.1))
		collected.Put("alpha_ba", types.NewValue(0.1))
		collected.Put("alpha_bb", types.NewValue(0.2))
	}
}

func (hawkes *Hawkes) processArrival(
	collected types.Map[string, types.Value[float64]],
	timestamp time.Time,
) {
	if !timestamp.IsZero() {
		if _, hasFirst := collected.Get("first_sec"); !hasFirst {
			collected.Put("first_sec", types.NewValue(float64(timestamp.Unix())))
			collected.Put("first_nsec", types.NewValue(float64(timestamp.Nanosecond())))
		}

		collected.Put("last_sec", types.NewValue(float64(timestamp.Unix())))
		collected.Put("last_nsec", types.NewValue(float64(timestamp.Nanosecond())))
	}

	mark := hawkes.number(collected, "mark")

	if mark == 0 {
		return
	}

	eventCount := hawkes.number(collected, "event_count") + 1
	collected.Put("event_count", types.NewValue(eventCount))

	sec := float64(timestamp.Unix()) + float64(timestamp.Nanosecond())*1e-9
	intervalMap := types.NewMap[string, types.Value[float64]]()
	intervalMap.Put("timestamp", types.NewValue(sec))

	if prevTs, hasPrev := collected.Get("prev_timestamp"); hasPrev {
		intervalMap.Put("previous", prevTs)
		intervalMap.Put("has_seen", types.NewValue(1.0))
	}

	intervalPrimitive := temporal.NewInterval(types.NewInput(types.NewValue(intervalMap)))
	intervalOut := nomagique.Number(types.NewInput(types.NewValue(intervalMap)), intervalPrimitive).Project().Read()
	deltaVal, _ := intervalOut.Get("delta")
	delta := deltaVal.Read()
	collected.Put("prev_timestamp", types.NewValue(sec))

	hawkes.applyDecayAndJump(collected, mark, delta)
}

func (hawkes *Hawkes) applyDecayAndJump(
	collected types.Map[string, types.Value[float64]],
	mark float64,
	delta float64,
) {
	beta := hawkes.number(collected, "beta")
	span := 1.0

	if beta > 0 {
		span = 1.0 / beta
	}

	clockMap := types.NewMap[string, types.Value[float64]]()
	clockMap.Put("age", types.NewValue(delta))
	clockMap.Put("span", types.NewValue(span))
	clockPrimitive := temporal.NewClock(types.NewInput(types.NewValue(clockMap)))
	clockOut := nomagique.Number(types.NewInput(types.NewValue(clockMap)), clockPrimitive).Project().Read()
	progressVal, _ := clockOut.Get("progress")
	progress := progressVal.Read()

	expPrimitive := calculus.NewExponential(types.NewInput(types.NewValue(progress)))
	expOut := nomagique.Number(types.NewInput(types.NewValue(progress)), expPrimitive).Project().Read()

	muAlpha := hawkes.number(collected, "mu_alpha")
	muBeta := hawkes.number(collected, "mu_beta")
	excessAlpha := math.Max(hawkes.number(collected, "lambda_alpha")-muAlpha, 0)
	excessBeta := math.Max(hawkes.number(collected, "lambda_beta")-muBeta, 0)

	decayAlphaMap := types.NewMap[string, types.Value[float64]]()
	decayAlphaMap.Put("level", types.NewValue(excessAlpha))
	decayAlphaMap.Put("clock", types.NewValue(progress))
	decayAlphaMap.Put("shape", types.NewValue(expOut))
	decayAlphaPrimitive := calculus.NewDecay(types.NewInput(types.NewValue(decayAlphaMap)))
	decayedAlphaOut := nomagique.Number(types.NewInput(types.NewValue(decayAlphaMap)), decayAlphaPrimitive).Project().Read()
	decAlphaVal, _ := decayedAlphaOut.Get("result")

	decayBetaMap := types.NewMap[string, types.Value[float64]]()
	decayBetaMap.Put("level", types.NewValue(excessBeta))
	decayBetaMap.Put("clock", types.NewValue(progress))
	decayBetaMap.Put("shape", types.NewValue(expOut))
	decayBetaPrimitive := calculus.NewDecay(types.NewInput(types.NewValue(decayBetaMap)))
	decayedBetaOut := nomagique.Number(types.NewInput(types.NewValue(decayBetaMap)), decayBetaPrimitive).Project().Read()
	decBetaVal, _ := decayedBetaOut.Get("result")

	lambdaAlpha := muAlpha + decAlphaVal.Read()
	lambdaBeta := muBeta + decBetaVal.Read()

	if mark > 0 {
		alphaCount := hawkes.number(collected, "alpha_event_count") + 1
		collected.Put("alpha_event_count", types.NewValue(alphaCount))

		attackAlphaMap := types.NewMap[string, types.Value[float64]]()
		attackAlphaMap.Put("base", types.NewValue(lambdaAlpha))
		attackAlphaMap.Put("jump", types.NewValue(hawkes.number(collected, "alpha_aa")))
		attackAlphaPrimitive := calculus.NewAttack(types.NewInput(types.NewValue(attackAlphaMap)))
		attackAlphaOut := nomagique.Number(types.NewInput(types.NewValue(attackAlphaMap)), attackAlphaPrimitive).Project().Read()
		aaVal, _ := attackAlphaOut.Get("result")
		lambdaAlpha = aaVal.Read()

		attackBetaMap := types.NewMap[string, types.Value[float64]]()
		attackBetaMap.Put("base", types.NewValue(lambdaBeta))
		attackBetaMap.Put("jump", types.NewValue(hawkes.number(collected, "alpha_ba")))
		attackBetaPrimitive := calculus.NewAttack(types.NewInput(types.NewValue(attackBetaMap)))
		attackBetaOut := nomagique.Number(types.NewInput(types.NewValue(attackBetaMap)), attackBetaPrimitive).Project().Read()
		baVal, _ := attackBetaOut.Get("result")
		lambdaBeta = baVal.Read()
	}

	if mark < 0 {
		betaCount := hawkes.number(collected, "beta_event_count") + 1
		collected.Put("beta_event_count", types.NewValue(betaCount))

		attackBetaMap := types.NewMap[string, types.Value[float64]]()
		attackBetaMap.Put("base", types.NewValue(lambdaBeta))
		attackBetaMap.Put("jump", types.NewValue(hawkes.number(collected, "alpha_bb")))
		attackBetaPrimitive := calculus.NewAttack(types.NewInput(types.NewValue(attackBetaMap)))
		attackBetaOut := nomagique.Number(types.NewInput(types.NewValue(attackBetaMap)), attackBetaPrimitive).Project().Read()
		bbVal, _ := attackBetaOut.Get("result")
		lambdaBeta = bbVal.Read()

		attackAlphaMap := types.NewMap[string, types.Value[float64]]()
		attackAlphaMap.Put("base", types.NewValue(lambdaAlpha))
		attackAlphaMap.Put("jump", types.NewValue(hawkes.number(collected, "alpha_ab")))
		attackAlphaPrimitive := calculus.NewAttack(types.NewInput(types.NewValue(attackAlphaMap)))
		attackAlphaOut := nomagique.Number(types.NewInput(types.NewValue(attackAlphaMap)), attackAlphaPrimitive).Project().Read()
		abVal, _ := attackAlphaOut.Get("result")
		lambdaAlpha = abVal.Read()
	}

	collected.Put("lambda_alpha", types.NewValue(lambdaAlpha))
	collected.Put("lambda_beta", types.NewValue(lambdaBeta))
}

func (hawkes *Hawkes) calculateMetrics(
	collected types.Map[string, types.Value[float64]],
	timestamp time.Time,
) {
	firstSec := hawkes.number(collected, "first_sec")
	firstNsec := hawkes.number(collected, "first_nsec")
	lastSec := hawkes.number(collected, "last_sec")
	lastNsec := hawkes.number(collected, "last_nsec")

	firstTime := time.Unix(int64(firstSec), int64(firstNsec)).UTC()
	lastTime := time.Unix(int64(lastSec), int64(lastNsec)).UTC()
	duration := lastTime.Sub(firstTime).Seconds()

	if duration <= 0 {
		duration = 1.0
	}

	rateAlphaMap := types.NewMap[string, types.Value[float64]]()
	rateAlphaMap.Put("count", types.NewValue(hawkes.number(collected, "alpha_event_count")))
	rateAlphaMap.Put("duration", types.NewValue(duration))
	rateAlphaPrimitive := calculus.NewRate(types.NewInput(types.NewValue(rateAlphaMap)))
	rateAlphaOut := nomagique.Number(types.NewInput(types.NewValue(rateAlphaMap)), rateAlphaPrimitive).Project().Read()
	raVal, _ := rateAlphaOut.Get("rate")
	rateAlpha := raVal.Read()

	rateBetaMap := types.NewMap[string, types.Value[float64]]()
	rateBetaMap.Put("count", types.NewValue(hawkes.number(collected, "beta_event_count")))
	rateBetaMap.Put("duration", types.NewValue(duration))
	rateBetaPrimitive := calculus.NewRate(types.NewInput(types.NewValue(rateBetaMap)))
	rateBetaOut := nomagique.Number(types.NewInput(types.NewValue(rateBetaMap)), rateBetaPrimitive).Project().Read()
	rbVal, _ := rateBetaOut.Get("rate")
	rateBeta := rbVal.Read()

	collected.Put("mu_alpha", types.NewValue(rateAlpha))
	collected.Put("mu_beta", types.NewValue(rateBeta))

	lambdaAlpha := math.Max(hawkes.number(collected, "lambda_alpha"), rateAlpha)
	lambdaBeta := math.Max(hawkes.number(collected, "lambda_beta"), rateBeta)
	collected.Put("lambda_alpha", types.NewValue(lambdaAlpha))
	collected.Put("lambda_beta", types.NewValue(lambdaBeta))

	branchingMap := types.NewMap[string, types.Value[float64]]()
	branchingMap.Put("alpha_aa", types.NewValue(hawkes.number(collected, "alpha_aa")))
	branchingMap.Put("alpha_ab", types.NewValue(hawkes.number(collected, "alpha_ab")))
	branchingMap.Put("alpha_ba", types.NewValue(hawkes.number(collected, "alpha_ba")))
	branchingMap.Put("alpha_bb", types.NewValue(hawkes.number(collected, "alpha_bb")))
	branchingMap.Put("beta", types.NewValue(hawkes.number(collected, "beta")))

	branchingPrimitive := statistic.NewBranching(types.NewInput(types.NewValue(branchingMap)))
	branchingOut := nomagique.Number(types.NewInput(types.NewValue(branchingMap)), branchingPrimitive).Project().Read()

	llParams := types.NewMap[string, types.Value[float64]]()
	llHawkes := math.Log(math.Max(lambdaAlpha, 1e-6)) + math.Log(math.Max(lambdaBeta, 1e-6))
	llPoisson := math.Log(math.Max(rateAlpha, 1e-6)) + math.Log(math.Max(rateBeta, 1e-6))
	llParams.Put("ll_hawkes", types.NewValue(llHawkes))
	llParams.Put("ll_poisson", types.NewValue(llPoisson))
	llParams.Put("ll_self", types.NewValue(llPoisson*1.1))

	likelihoodPrimitive := statistic.NewLikelihood(types.NewInput(types.NewValue(llParams)))
	likelihoodOut := nomagique.Number(types.NewInput(types.NewValue(llParams)), likelihoodPrimitive).Project().Read()

	collected.Put("ready", types.NewValue(1.0))
	collected.Put("observation", types.NewValue(1.0))
	collected.Put("alpha_arrival_rate", types.NewValue(rateAlpha))
	collected.Put("beta_arrival_rate", types.NewValue(rateBeta))
	collected.Put("fold", types.NewValue(1.0/hawkes.number(collected, "beta")))
	collected.Put("observed_from_sec", types.NewValue(firstSec))
	collected.Put("observed_from_nsec", types.NewValue(firstNsec))
	collected.Put("observed_at_sec", types.NewValue(float64(timestamp.Unix())))
	collected.Put("observed_at_nsec", types.NewValue(float64(timestamp.Nanosecond())))

	hawkes.copyMapValues(branchingOut, collected)
	hawkes.copyMapValues(likelihoodOut, collected)
}

func (hawkes *Hawkes) copyMapValues(
	src types.Map[string, types.Value[float64]],
	dst types.Map[string, types.Value[float64]],
) {
	keys := []string{
		"spectral_radius", "offspring_aa", "offspring_ab",
		"offspring_ba", "offspring_bb", "descendants_alpha",
		"descendants_beta", "ll_delta_poisson", "ll_delta_self",
	}

	for _, key := range keys {
		if val, found := src.Get(key); found {
			dst.Put(key, val)
		}
	}
}

func (hawkes *Hawkes) stamp(collected types.Map[string, types.Value[float64]]) time.Time {
	_, hasSeconds := collected.Get("unix_sec")
	_, hasNanos := collected.Get("unix_nsec")

	if !hasSeconds && !hasNanos {
		return time.Time{}
	}

	return time.Unix(
		int64(hawkes.number(collected, "unix_sec")),
		int64(hawkes.number(collected, "unix_nsec")),
	).UTC()
}

func (hawkes *Hawkes) number(
	collected types.Map[string, types.Value[float64]],
	name string,
) float64 {
	val, found := collected.Get(name)

	if !found {
		return 0
	}

	return val.Read()
}
