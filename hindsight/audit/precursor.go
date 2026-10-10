package audit

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sort"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/strategy"
)

type excursionInterval struct {
	symbol    string
	class     string
	startTick int64
	bTick     int64
	cTick     int64
	bPrice    float64
	cPrice    float64
}

/*
AnalyzePrecursorSeparation measures two causal populations:
 1. A -> B: profitable long ignition precursor (up) versus losing/ordinary controls.
 2. B -> C: late holding state versus early holding state inside profitable long excursions.

Excursion windows are loaded directly from the archive rather than requiring them
to fall inside the audit's arbitrary first-N tick sample. Event and control tape
use the same ChannelsFrom -> Stream.Deform -> LitRegion path.

Every null permutes whole excursions, never single tokens: the tokens of one
excursion are consecutive ticks of one symbol and strongly dependent, so a
token-level shuffle would understate the null spread and pass noise. Skill is
learned on the earlier 60% of excursions (by B tick) and scored on the rest.
Friction clearance is NOT_A_TEST: the up class is defined by clearing the
round-trip fee, so its clearance rate restates the detector's construction.
*/
func AnalyzePrecursorSeparation(
	ctx context.Context,
	catalog *tables.Catalog,
	epoch int64,
	symbol string,
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
	permutations int,
	significance float64,
	detections []*data.Measurement,
	takerFee ...float64,
) Stage5PrecursorSeparation {
	if grid == nil {
		return insufficientPrecursor("Grid unavailable.")
	}

	if len(takerFee) == 0 || takerFee[0] <= 0 {
		return insufficientPrecursor("Taker fee must be strictly positive and declared with explicit provenance.")
	}

	fee := takerFee[0]

	if detections == nil {
		if catalog == nil {
			return insufficientPrecursor("Catalog unavailable.")
		}
		var minTick, maxTick int64
		if len(ticks) > 0 {
			minTick = ticks[0]
			maxTick = ticks[len(ticks)-1]
		}

		loaded, err := loadOrDetectExcursions(ctx, catalog, epoch, symbol, fee, minTick, maxTick)

		if err != nil {
			return insufficientPrecursor("Detection retrieval failed: " + err.Error())
		}

		detections = loaded
	}

	if len(detections) == 0 {
		return insufficientPrecursor("Zero excursions detected.")
	}

	excursions := make([]excursionInterval, 0, len(detections))
	classSet := make(map[string]struct{})
	for _, det := range detections {
		class := det.Meta("type")
		if class == "" {
			class = "unknown"
		}
		classSet[class] = struct{}{}

		startTick := int64(getMeasurementMetric(det, "start_tick"))
		bTick := int64(getMeasurementMetric(det, "b_tick"))
		cTick := int64(getMeasurementMetric(det, "c_tick"))

		if bTick <= 0 || cTick <= bTick {
			continue
		}

		lo, _ := strategy.PadWindow(bTick, cTick)
		if startTick > 0 && startTick < lo {
			lo = startTick
		}
		startTick = lo

		if bTick <= startTick {
			continue
		}

		bPrice := getMeasurementMetric(det, "b_price")
		cPrice := getMeasurementMetric(det, "c_price")

		excursions = append(excursions, excursionInterval{
			symbol:    det.Label,
			class:     class,
			startTick: startTick,
			bTick:     bTick,
			cTick:     cTick,
			bPrice:    bPrice,
			cPrice:    cPrice,
		})
	}

	classList := make([]string, 0, len(classSet))
	for class := range classSet {
		classList = append(classList, class)
	}
	sort.Strings(classList)

	if len(excursions) == 0 {
		result := insufficientPrecursor("Detections contained no valid A < B < C intervals.")
		result.DetectionsFound = len(detections)
		result.ExcursionsFound = classList
		return result
	}

	// Non-excursion background is taken only from the already loaded chronological
	// audit sample. It is supplemental control evidence; event populations do not
	// depend on the sample containing the excursion.
	backgroundTokens := sampledBackgroundTokens(
		grid, ticks, tickMeasurements, excursions,
	)

	// Per-excursion token counts, ordered by B tick for the held-out split.
	slices.SortFunc(excursions, func(left, right excursionInterval) int {
		return cmp.Compare(left.bTick, right.bTick)
	})

	var ignitionEvents, ignitionControls, earlyHolds, lateHolds []map[string]int

	for _, excursion := range excursions {
		eventTokens, err := archivedIntervalTokens(
			ctx, catalog, epoch, excursion.symbol, grid, excursion.startTick, excursion.cTick, tickMeasurements,
		)
		if err != nil {
			result := insufficientPrecursor("Event-window replay failed: " + err.Error())
			result.DetectionsFound = len(detections)
			result.ExcursionsFound = classList
			return result
		}

		ignition := make(map[string]int)
		early := make(map[string]int)
		late := make(map[string]int)
		mid := excursion.bTick + (excursion.cTick-excursion.bTick)/2

		for tick, token := range eventTokens {
			switch {
			case tick >= excursion.startTick && tick < excursion.bTick:
				ignition[token]++
			case tick >= excursion.bTick && tick <= mid:
				early[token]++
			case tick > mid && tick <= excursion.cTick:
				late[token]++
			}
		}

		// Desk semantics are long-only: profitable "up" excursions are the
		// positive A->B examples; down/up_friction/chop/flat are controls.
		if excursion.class == "up" {
			ignitionEvents = append(ignitionEvents, ignition)
			earlyHolds = append(earlyHolds, early)
			lateHolds = append(lateHolds, late)
			continue
		}

		ignitionControls = append(ignitionControls, ignition)
	}

	ignition := evaluateGroupHypothesis(
		"A -> B Ignition Precursor",
		"Long-only profitable up precursor versus losing/ordinary controls (excursion-label null)",
		ignitionEvents, ignitionControls, permutations, significance,
	)
	exhaustion := evaluatePairedHypothesis(
		"B -> C Holding Deterioration",
		"Late versus early holding state inside profitable up excursions (half-swap null)",
		lateHolds, earlyHolds, permutations, significance,
	)

	skill := evaluateHeldOutSkill(ignitionEvents, ignitionControls, excursions, permutations, significance)
	economic := evaluateEconomicRelevance(excursions, fee)
	verdict := ignition.Status

	return Stage5PrecursorSeparation{
		DetectionsFound:      len(detections),
		ExcursionsFound:      classList,
		IgnitionHypothesis:   ignition,
		ExhaustionHypothesis: exhaustion,
		PredictiveSkill:      skill,
		EconomicRelevance:    economic,
		BackgroundTokens:     backgroundTokens,
		SummaryText: fmt.Sprintf(
			"Precursor: %d detections (%s). A->B: %s, JSD %.3f, p=%.3f, N=%d/%d excursions. "+
				"B->C: %s, JSD %.3f, p=%.3f. Held-out skill: %s, MCC %.3f, p=%.3f. "+
				"Friction clearance %.1f%% (N=%d, fee=%.2fbps) is NOT_A_TEST. Background=%d.",
			len(detections), strings.Join(classList, "/"),
			ignition.Status, ignition.DivergenceBits, ignition.PValue,
			len(ignitionEvents), len(ignitionControls),
			exhaustion.Status, exhaustion.DivergenceBits, exhaustion.PValue,
			skill.Status, skill.MCC, skill.PValue,
			economic.FrictionClearanceRate*100.0, economic.EvaluatedExcursions, economic.RoundTripFeeRate*10000.0,
			totalTokenCount(backgroundTokens),
		),
		Status: verdict,
		Passed: passed(verdict),
	}
}

func insufficientPrecursor(reason string) Stage5PrecursorSeparation {
	return Stage5PrecursorSeparation{
		IgnitionHypothesis: PrecursorHypothesis{
			Name:        "A -> B Ignition Precursor",
			Description: "Long-only profitable up precursor versus losing/ordinary controls",
			Status:      "INSUFFICIENT_DATA",
		},
		ExhaustionHypothesis: PrecursorHypothesis{
			Name:        "B -> C Holding Deterioration",
			Description: "Late versus early holding state inside profitable up excursions",
			Status:      "INSUFFICIENT_DATA",
		},
		PredictiveSkill: PrecursorPredictiveSkill{
			Status: "INSUFFICIENT_DATA",
		},
		EconomicRelevance: PrecursorEconomicRelevance{
			Status: "INSUFFICIENT_DATA",
		},
		SummaryText: "Precursor Separation: INSUFFICIENT_DATA (" + reason + ")",
		Status:      VerdictInsufficient,
		Passed:      false,
	}
}

func sampledBackgroundTokens(
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
	excursions []excursionInterval,
) map[string]int {
	result := make(map[string]int)

	for _, tick := range ticks {
		group := tickMeasurements[tick]

		if len(group) == 0 {
			continue
		}

		bySymbol := make(map[string][]*data.Measurement)

		for _, m := range group {
			if m != nil {
				bySymbol[m.Label] = append(bySymbol[m.Label], m)
			}
		}

		for sym, symMeas := range bySymbol {
			train := data.NewMeasurement(
				symMeas[0].Epoch,
				sym,
				"audit",
				symMeas[0].SeqIdx,
				tick,
			)
			train.At = symMeas[0].At
			train.From = symMeas[0].From
			train.Peers(symMeas...)
			train = train.Write()

			tokenBytes := grid.Observe(train)

			if len(tokenBytes) == 0 {
				continue
			}

			inExcursion := false

			for _, excursion := range excursions {
				if excursion.symbol == sym && tick >= excursion.startTick && tick <= excursion.cTick {
					inExcursion = true
					break
				}
			}

			if !inExcursion {
				result[string(tokenBytes)]++
			}
		}
	}

	return result
}

func archivedIntervalTokens(
	ctx context.Context,
	catalog *tables.Catalog,
	epoch int64,
	symbol string,
	grid *store.Grid,
	startTick int64,
	endTick int64,
	tickMeasurements map[int64][]*data.Measurement,
) (map[int64]string, error) {
	if grid == nil {
		return nil, nil
	}

	tickMap := make(map[int64][]*data.Measurement)

	for tick := startTick; tick <= endTick; tick++ {
		list, exists := tickMeasurements[tick]

		if !exists {
			continue
		}

		for _, m := range list {
			if m != nil && (symbol == "" || m.Label == symbol) {
				tickMap[tick] = append(tickMap[tick], m)
			}
		}
	}

	if len(tickMap) == 0 {
		return nil, nil
	}

	matchingTicks := make([]int64, 0, len(tickMap))

	for tick := range tickMap {
		matchingTicks = append(matchingTicks, tick)
	}

	slices.Sort(matchingTicks)
	result := make(map[int64]string, len(matchingTicks))

	for _, tick := range matchingTicks {
		signals := tickMap[tick]

		if len(signals) == 0 {
			continue
		}

		train := data.NewMeasurement(
			signals[0].Epoch,
			symbol,
			"audit",
			signals[0].SeqIdx,
			tick,
		)
		train.At = signals[0].At
		train.From = signals[0].From
		train.Peers(signals...)
		train = train.Write()

		tokenBytes := grid.Observe(train)

		if len(tokenBytes) > 0 {
			result[tick] = string(tokenBytes)
		}
	}

	return result, nil
}

func totalTokenCount(tokens map[string]int) int {
	total := 0
	for _, count := range tokens {
		total += count
	}
	return total
}

func mergeTokens(groups ...[]map[string]int) map[string]int {
	merged := make(map[string]int)

	for _, group := range groups {
		for _, tokens := range group {
			for token, count := range tokens {
				merged[token] += count
			}
		}
	}

	return merged
}

/*
evaluateGroupHypothesis compares the pooled tokens of event excursions with
those of control excursions. The null reassigns the event/control label among
whole excursions, keeping the number of each.
*/
func evaluateGroupHypothesis(
	name string,
	description string,
	events []map[string]int,
	controls []map[string]int,
	permutations int,
	significance float64,
) PrecursorHypothesis {
	eventTokens := mergeTokens(events)
	controlTokens := mergeTokens(controls)
	result := PrecursorHypothesis{
		Name:              name,
		Description:       description,
		EventTokens:       eventTokens,
		ControlTokens:     controlTokens,
		EventTokenCount:   totalTokenCount(eventTokens),
		ControlTokenCount: totalTokenCount(controlTokens),
		Status:            VerdictInsufficient,
	}

	if len(events) == 0 || len(controls) == 0 || result.EventTokenCount == 0 || result.ControlTokenCount == 0 {
		return result
	}

	real := computeJSD(eventTokens, controlTokens)
	pool := append(append([]map[string]int(nil), events...), controls...)
	rng := rand.New(rand.NewSource(1791))
	null := make([]float64, 0, permutations)

	for range permutations {
		rng.Shuffle(len(pool), func(first, second int) { pool[first], pool[second] = pool[second], pool[first] })
		null = append(null, computeJSD(mergeTokens(pool[:len(events)]), mergeTokens(pool[len(events):])))
	}

	return finishHypothesis(result, real, null, permutations, significance)
}

/*
evaluatePairedHypothesis compares late with early halves of the same
excursions. The null swaps the two halves of each excursion independently
with probability one half, which keeps every excursion's own tokens.
*/
func evaluatePairedHypothesis(
	name string,
	description string,
	late []map[string]int,
	early []map[string]int,
	permutations int,
	significance float64,
) PrecursorHypothesis {
	lateTokens := mergeTokens(late)
	earlyTokens := mergeTokens(early)
	result := PrecursorHypothesis{
		Name:              name,
		Description:       description,
		EventTokens:       lateTokens,
		ControlTokens:     earlyTokens,
		EventTokenCount:   totalTokenCount(lateTokens),
		ControlTokenCount: totalTokenCount(earlyTokens),
		Status:            VerdictInsufficient,
	}

	if len(late) == 0 || len(late) != len(early) || result.EventTokenCount == 0 || result.ControlTokenCount == 0 {
		return result
	}

	real := computeJSD(lateTokens, earlyTokens)
	rng := rand.New(rand.NewSource(1791))
	null := make([]float64, 0, permutations)

	for range permutations {
		var nullLate, nullEarly []map[string]int

		for index := range late {
			if rng.Intn(2) == 0 {
				nullLate, nullEarly = append(nullLate, late[index]), append(nullEarly, early[index])
				continue
			}

			nullLate, nullEarly = append(nullLate, early[index]), append(nullEarly, late[index])
		}

		null = append(null, computeJSD(mergeTokens(nullLate), mergeTokens(nullEarly)))
	}

	return finishHypothesis(result, real, null, permutations, significance)
}

func finishHypothesis(
	result PrecursorHypothesis,
	real float64,
	null []float64,
	permutations int,
	significance float64,
) PrecursorHypothesis {
	null95 := quantileOf(null, 0.95)
	result.DivergenceBits = real
	result.NullDivergence95 = null95

	if null95 > 0 {
		result.SeparationRatio = real / null95
	}

	result.PValue = upperPValue(real, null)
	result.Status = hypothesisVerdict(result.PValue, permutations, significance)
	result.Passed = passed(result.Status)

	return result
}

/*
evaluateHeldOutSkill learns which tokens predict an up excursion from the
earlier 60% of excursions (by B tick) and scores that rule on the later 40%.
Its null reassigns the event/control label among the held-out excursions.
*/
func evaluateHeldOutSkill(
	events []map[string]int,
	controls []map[string]int,
	excursions []excursionInterval,
	permutations int,
	significance float64,
) PrecursorPredictiveSkill {
	// Rebuild the chronological order of the per-excursion maps.
	type labelled struct {
		tokens map[string]int
		event  bool
	}

	ordered := make([]labelled, 0, len(events)+len(controls))
	eventAt, controlAt := 0, 0

	for _, excursion := range excursions {
		if excursion.class == "up" && eventAt < len(events) {
			ordered = append(ordered, labelled{events[eventAt], true})
			eventAt++
			continue
		}

		if excursion.class != "up" && controlAt < len(controls) {
			ordered = append(ordered, labelled{controls[controlAt], false})
			controlAt++
		}
	}

	split := len(ordered) * 3 / 5
	var trainEvents, trainControls, testEvents, testControls []map[string]int

	for index, item := range ordered {
		switch {
		case index < split && item.event:
			trainEvents = append(trainEvents, item.tokens)
		case index < split:
			trainControls = append(trainControls, item.tokens)
		case item.event:
			testEvents = append(testEvents, item.tokens)
		default:
			testControls = append(testControls, item.tokens)
		}
	}

	if len(trainEvents) == 0 || len(trainControls) == 0 || len(testEvents) == 0 || len(testControls) == 0 {
		return PrecursorPredictiveSkill{
			EvaluatedSamples: totalTokenCount(mergeTokens(testEvents, testControls)),
			Status:           VerdictInsufficient,
		}
	}

	rule := precursorRule(mergeTokens(trainEvents), mergeTokens(trainControls))
	skill := scorePrecursorRule(rule, mergeTokens(testEvents), mergeTokens(testControls))

	pool := append(append([]map[string]int(nil), testEvents...), testControls...)
	rng := rand.New(rand.NewSource(1791))
	null := make([]float64, 0, permutations)

	for range permutations {
		rng.Shuffle(len(pool), func(first, second int) { pool[first], pool[second] = pool[second], pool[first] })
		shuffled := scorePrecursorRule(rule, mergeTokens(pool[:len(testEvents)]), mergeTokens(pool[len(testEvents):]))
		null = append(null, shuffled.MCC)
	}

	skill.PValue = upperPValue(skill.MCC, null)
	skill.Status = hypothesisVerdict(skill.PValue, permutations, significance)
	skill.Passed = passed(skill.Status)

	return skill
}

/*
precursorRule marks the tokens more frequent among event than control tokens.
*/
func precursorRule(eventTokens, controlTokens map[string]int) map[string]bool {
	eventCount := totalTokenCount(eventTokens)
	controlCount := totalTokenCount(controlTokens)
	rule := make(map[string]bool)

	for token, count := range eventTokens {
		if float64(count)/float64(eventCount) > float64(controlTokens[token])/float64(max(controlCount, 1)) {
			rule[token] = true
		}
	}

	return rule
}

/*
scorePrecursorRule classifies every token as event when the rule marks it and
reports the confusion statistics.
*/
func scorePrecursorRule(rule map[string]bool, eventTokens, controlTokens map[string]int) PrecursorPredictiveSkill {
	var tp, fp, fn, tn float64

	for token, count := range eventTokens {
		if rule[token] {
			tp += float64(count)
		} else {
			fn += float64(count)
		}
	}

	for token, count := range controlTokens {
		if rule[token] {
			fp += float64(count)
		} else {
			tn += float64(count)
		}
	}

	skill := PrecursorPredictiveSkill{EvaluatedSamples: int(tp + fp + fn + tn)}

	if tp+fp > 0 {
		skill.Precision = tp / (tp + fp)
	}

	if tp+fn > 0 {
		skill.Recall = tp / (tp + fn)
	}

	specificity := 0.0

	if tn+fp > 0 {
		specificity = tn / (tn + fp)
	}

	skill.BalancedAccuracy = (skill.Recall + specificity) / 2

	if denominator := math.Sqrt((tp + fp) * (tp + fn) * (tn + fp) * (tn + fn)); denominator > 0 {
		skill.MCC = (tp*tn - fp*fn) / denominator
	}

	if total := tp + fp + fn + tn; total > 0 {
		skill.PriorBaseRate = (tp + fn) / total
	}

	for token := range rule {
		skill.TopPrecursorTokens = append(skill.TopPrecursorTokens, token)
	}

	sort.Strings(skill.TopPrecursorTokens)

	return skill
}

func evaluateEconomicRelevance(
	excursions []excursionInterval,
	takerFee float64,
) PrecursorEconomicRelevance {
	roundTripFee := 2.0 * takerFee
	evaluated := 0
	grossSum := 0.0
	netSum := 0.0
	profitable := 0
	unprofitable := 0

	for _, excursion := range excursions {
		if excursion.bPrice <= 0 || excursion.cPrice <= 0 {
			continue
		}

		if excursion.class != "up" && excursion.class != "up_friction" {
			continue
		}

		evaluated++
		grossReturn := (excursion.cPrice - excursion.bPrice) / excursion.bPrice
		netReturn := grossReturn - roundTripFee
		grossSum += grossReturn
		netSum += netReturn

		if netReturn > 0 {
			profitable++
			continue
		}

		unprofitable++
	}

	if evaluated == 0 {
		return PrecursorEconomicRelevance{
			TakerFeeRate:     takerFee,
			RoundTripFeeRate: roundTripFee,
			Status:           "INSUFFICIENT_DATA",
			Passed:           false,
		}
	}

	meanGross := grossSum / float64(evaluated)
	meanNet := netSum / float64(evaluated)
	clearanceRate := float64(profitable) / float64(evaluated)

	return PrecursorEconomicRelevance{
		TakerFeeRate:           takerFee,
		RoundTripFeeRate:       roundTripFee,
		EvaluatedExcursions:    evaluated,
		GrossMeanReturn:        meanGross,
		NetMeanReturn:          meanNet,
		FrictionClearanceRate:  clearanceRate,
		ProfitableExcursions:   profitable,
		UnprofitableExcursions: unprofitable,
		// The up class is defined by clearing the round-trip fee, so this
		// rate restates the detector's construction and cannot fail.
		Status: VerdictNotATest,
		Passed: false,
	}
}

func getMeasurementMetric(m *data.Measurement, key string) float64 {
	if m == nil {
		return 0
	}
	for entry := range m.Read() {
		if entry != nil && entry.Key == key && entry.Metric != nil {
			return entry.Metric.Raw
		}
	}
	return 0
}

/*
computeJSD calculates the Jensen-Shannon Divergence in bits between two token distributions.
*/
func computeJSD(distributionP, distributionQ map[string]int) float64 {
	totalObservationsP := 0
	for _, count := range distributionP {
		totalObservationsP += count
	}

	totalObservationsQ := 0
	for _, count := range distributionQ {
		totalObservationsQ += count
	}

	if totalObservationsP == 0 || totalObservationsQ == 0 {
		return 0.0
	}

	allKeys := make(map[string]struct{})
	for key := range distributionP {
		allKeys[key] = struct{}{}
	}
	for key := range distributionQ {
		allKeys[key] = struct{}{}
	}

	divergencePM := 0.0
	divergenceQM := 0.0

	for key := range allKeys {
		probP := float64(distributionP[key]) / float64(totalObservationsP)
		probQ := float64(distributionQ[key]) / float64(totalObservationsQ)
		probMid := 0.5 * (probP + probQ)

		if probP > 0 && probMid > 0 {
			divergencePM += probP * math.Log2(probP/probMid)
		}

		if probQ > 0 && probMid > 0 {
			divergenceQM += probQ * math.Log2(probQ/probMid)
		}
	}

	jsd := 0.5*divergencePM + 0.5*divergenceQM

	if jsd < 0 {
		return 0.0
	}

	return jsd
}

func loadOrDetectExcursions(
	ctx context.Context,
	catalog *tables.Catalog,
	epoch int64,
	symbol string,
	takerFee float64,
	bounds ...int64,
) ([]*data.Measurement, error) {
	if takerFee <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[audit] taker fee must be strictly positive",
			nil,
		))
	}

	var minTick, maxTick int64
	if len(bounds) >= 2 {
		minTick = bounds[0]
		maxTick = bounds[1]
	}

	var detections []*data.Measurement

	for det, err := range catalog.Detections(ctx, epoch) {
		if err != nil {
			return nil, errnie.Error(err)
		}

		if det != nil && (symbol == "" || det.Label == symbol) {
			bTick := int64(getMeasurementMetric(det, "b_tick"))
			if minTick > 0 && bTick < minTick {
				continue
			}
			if maxTick > 0 && bTick > maxTick {
				continue
			}

			detections = append(detections, det)
		}
	}

	// Stored detections only. They were produced by `symm detect` at the fee
	// it was run with; detections do not record that fee, so the caller must
	// declare it and the report states it as unverified provenance. There is
	// no in-memory re-detection at a different fee.
	return detections, nil
}
