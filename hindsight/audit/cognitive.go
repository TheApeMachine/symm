package audit

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/ui"
)

type triePhase struct {
	context      string
	targetAction string
	feedback     float64
	class        string
	tick         int64
}

type taughtRecord struct {
	context  string
	action   string
	feedback float64
}

/*
AnalyzeCognitiveTrie evaluates the associative memory and Radix Trie learning dynamics:
prequential predictive skill against constant policy baselines and an empirical label-permuted null,
post-teach memory retention (catastrophic interference), trie graph topology and basin geometry,
and false-alarm trigger rates on unseen continuous background tape.
*/
func AnalyzeCognitiveTrie(
	ctx context.Context,
	catalog *tables.Catalog,
	epoch int64,
	symbol string,
	grid *store.Grid,
	orderedTicks []int64,
	tickMeasurements map[int64][]*data.Measurement,
	permutations int,
	detections []*data.Measurement,
) Stage6CognitiveTrie {
	if catalog == nil || grid == nil || grid.RegionsFormed() == 0 {
		return insufficientCognitiveTrie("Catalog or grid unavailable.")
	}

	if permutations <= 0 {
		permutations = 50
	}

	if detections == nil {
		loaded, loadErr := loadOrDetectExcursions(ctx, catalog, epoch, symbol)

		if loadErr != nil {
			return insufficientCognitiveTrie("Excursion detection failed: " + loadErr.Error())
		}

		detections = loaded
	}

	if len(detections) == 0 {
		return insufficientCognitiveTrie("Zero excursions detected.")
	}

	phases := extractTriePhases(ctx, catalog, epoch, symbol, grid, detections, tickMeasurements)

	if len(phases) == 0 {
		return insufficientCognitiveTrie("No valid sequential token contexts formed from excursions.")
	}

	model := strategy.NewModel()
	totalCalls := len(phases)
	hits := 0
	actionCounts := map[string]int{"enter": 0, "exit": 0, "wait": 0}
	abstentions := 0
	confidences := make([]float64, 0, totalCalls)
	contrasts := make([]float64, 0, totalCalls)
	taughtList := make([]taughtRecord, 0, totalCalls)

	for _, phaseItem := range phases {
		actionCounts[phaseItem.targetAction]++

		call, callErr := model.Recall(phaseItem.context, "")

		if callErr != nil {
			errnie.Error(callErr)
		}

		confidences = append(confidences, call.Confidence)
		contrasts = append(contrasts, call.Contrast)

		if call.Winner == "" {
			abstentions++
		}

		isHit := false

		if phaseItem.targetAction == "enter" && call.Winner == "enter" {
			isHit = true
		}

		if phaseItem.targetAction == "exit" && call.Winner == "exit" {
			isHit = true
		}

		if phaseItem.targetAction == "wait" && call.Winner == "" {
			isHit = true
		}

		if isHit {
			hits++
		}

		if phaseItem.targetAction == "enter" || phaseItem.targetAction == "exit" {
			teachErr := model.Teach(phaseItem.context, phaseItem.targetAction, phaseItem.feedback)

			if teachErr != nil {
				errnie.Error(teachErr)
			}

			taughtList = append(taughtList, taughtRecord{
				context: phaseItem.context, action: phaseItem.targetAction, feedback: phaseItem.feedback,
			})
		}

		if phaseItem.targetAction == "wait" {
			teachErr := model.Teach(phaseItem.context, "enter", phaseItem.feedback)

			if teachErr != nil {
				errnie.Error(teachErr)
			}

			taughtList = append(taughtList, taughtRecord{
				context: phaseItem.context, action: "wait", feedback: phaseItem.feedback,
			})
		}
	}

	bestBaselineHits := actionCounts["wait"]
	bestBaselinePolicy := "always_abstain"

	if actionCounts["enter"] > bestBaselineHits {
		bestBaselineHits = actionCounts["enter"]
		bestBaselinePolicy = "always_enter"
	}

	if actionCounts["exit"] > bestBaselineHits {
		bestBaselineHits = actionCounts["exit"]
		bestBaselinePolicy = "always_exit"
	}

	baselineHitRate := 0.0
	hitRate := 0.0

	if totalCalls > 0 {
		baselineHitRate = float64(bestBaselineHits) / float64(totalCalls)
		hitRate = float64(hits) / float64(totalCalls)
	}

	nullHits := make([]float64, permutations)
	rng := rand.New(rand.NewPCG(uint64(epoch), 12345))

	for permIndex := 0; permIndex < permutations; permIndex++ {
		permModel := strategy.NewModel()
		permActions := make([]string, len(phases))
		permFeedbacks := make([]float64, len(phases))

		for phaseIndex, ph := range phases {
			permActions[phaseIndex] = ph.targetAction
			permFeedbacks[phaseIndex] = ph.feedback
		}

		rng.Shuffle(len(permActions), func(idxA, idxB int) {
			permActions[idxA], permActions[idxB] = permActions[idxB], permActions[idxA]
			permFeedbacks[idxA], permFeedbacks[idxB] = permFeedbacks[idxB], permFeedbacks[idxA]
		})

		permHitCount := 0

		for phaseIndex, ph := range phases {
			act := permActions[phaseIndex]
			fb := permFeedbacks[phaseIndex]

			pCall, pErr := permModel.Recall(ph.context, "")

			if pErr != nil {
				errnie.Error(pErr)
			}

			hit := false

			if act == "enter" && pCall.Winner == "enter" {
				hit = true
			}

			if act == "exit" && pCall.Winner == "exit" {
				hit = true
			}

			if act == "wait" && pCall.Winner == "" {
				hit = true
			}

			if hit {
				permHitCount++
			}

			if act == "enter" || act == "exit" {
				_ = permModel.Teach(ph.context, act, fb)
			}

			if act == "wait" {
				_ = permModel.Teach(ph.context, "enter", fb)
			}
		}

		nullHits[permIndex] = float64(permHitCount)
	}

	sort.Float64s(nullHits)
	nullMean := computeMean(nullHits)
	nullStd := computeStdDev(nullHits, nullMean)
	nullP95 := nullHits[int(float64(permutations)*0.95)]

	betterCount := 0

	for _, nh := range nullHits {
		if nh >= float64(hits) {
			betterCount++
		}
	}

	empiricalPVal := float64(betterCount) / float64(permutations)
	separatesFromNull := hits > bestBaselineHits && float64(hits) >= nullP95

	retainedCount := 0

	for _, item := range taughtList {
		postCall, _ := model.Recall(item.context, "")

		if item.action == "enter" && postCall.Winner == "enter" {
			retainedCount++
		}

		if item.action == "exit" && postCall.Winner == "exit" {
			retainedCount++
		}

		if item.action == "wait" && postCall.Winner == "" {
			retainedCount++
		}
	}

	retentionRate := 0.0

	if len(taughtList) > 0 {
		retentionRate = float64(retainedCount) / float64(len(taughtList))
	}

	recordsCount, _ := model.Count("records")
	spanCount, _ := model.Count("span")
	enterBasins, _ := model.Count("enter")
	exitBasins, _ := model.Count("exit")

	export := model.CognitionTree()
	nodeStats := computeNodeMetrics(export.Root)

	spuriousCount := 0
	evaluatedTicks := 0

	if len(orderedTicks) > 0 {
		splitIdx := int(float64(len(orderedTicks)) * 0.60)
		unseenTicks := orderedTicks[splitIdx:]
		evaluatedTicks = len(unseenTicks)

		heldOutStreams := make(map[string]*store.Stream)
		var windowTokens []string

		for _, tickVal := range unseenTicks {
			measurements := tickMeasurements[tickVal]

			if len(measurements) == 0 {
				continue
			}

			bySymbol := make(map[string][]*data.Measurement)
			for _, m := range measurements {
				if m != nil {
					bySymbol[m.Label] = append(bySymbol[m.Label], m)
				}
			}

			for sym, symMeas := range bySymbol {
				st, ok := heldOutStreams[sym]
				if !ok {
					st = store.NewStream()
					heldOutStreams[sym] = st
				}

				observed := strategy.ChannelsFrom(symMeas...)
				deformations := st.Deform(observed.Raw)
				excited := observed.Excite(deformations)
				lit := grid.LitRegion(excited)

				if len(lit) == 0 {
					continue
				}

				tok := fmt.Sprintf("R%d", lit[0])

				if len(windowTokens) == 0 || windowTokens[len(windowTokens)-1] != tok {
					windowTokens = append(windowTokens, tok)
				}

				if limit := int(spanCount); limit >= 1 && len(windowTokens) > limit {
					windowTokens = windowTokens[len(windowTokens)-limit:]
				}

				qCtx := strings.Join(windowTokens, "/")
				bgCall, _ := model.Recall(qCtx, "")

				if bgCall.Winner != "" {
					spuriousCount++
				}
			}
		}
	}

	spuriousRate := 0.0

	if evaluatedTicks > 0 {
		spuriousRate = float64(spuriousCount) / float64(evaluatedTicks)
	}

	meanConf := 0.0
	meanCont := 0.0

	if len(confidences) > 0 {
		meanConf = computeMean(confidences)
		meanCont = computeMean(contrasts)
	}

	abstentionRate := 0.0

	if totalCalls > 0 {
		abstentionRate = float64(abstentions) / float64(totalCalls)
	}

	summaryText := fmt.Sprintf(
		"Cognitive Trie: %d phases (%d enter, %d exit, %d wait). Prequential hits=%d/%d (%.1f%%) vs best baseline (%s) %d/%d (%.1f%%). Null p95=%.1f hits (p=%.3f). Retention=%.1f%%. Spurious bg rate=%.2f%%.",
		len(phases), actionCounts["enter"], actionCounts["exit"], actionCounts["wait"],
		hits, totalCalls, hitRate*100,
		bestBaselinePolicy, bestBaselineHits, totalCalls, baselineHitRate*100,
		nullP95, empiricalPVal, retentionRate*100, spuriousRate*100,
	)

	return Stage6CognitiveTrie{
		DetectionsEvaluated: len(detections),
		PhasesFormed:        len(phases),
		ActionCounts:        actionCounts,
		Skill: TrieSkillMetrics{
			TotalCalls:            totalCalls,
			Hits:                  hits,
			HitRate:               hitRate,
			BaselineHits:          bestBaselineHits,
			BaselineHitRate:       baselineHitRate,
			BestBaselinePolicy:    bestBaselinePolicy,
			NullMeanHits:          nullMean,
			NullStdHits:           nullStd,
			Null95thPercentileHits: nullP95,
			SeparatesFromNull:     separatesFromNull,
			EmpiricalPValue:       empiricalPVal,
		},
		Retention: TrieRetentionMetrics{
			TotalTaught:   len(taughtList),
			RetainedCount: retainedCount,
			RetentionRate: retentionRate,
		},
		Topology: TrieTopologyMetrics{
			RecordsCount: recordsCount,
			SpanCount:    spanCount,
			EnterBasins:  enterBasins,
			ExitBasins:   exitBasins,
			TotalBasins:  int(enterBasins + exitBasins),
			NodeStats:    nodeStats,
		},
		AbstentionRate:      abstentionRate,
		MeanConfidence:      meanConf,
		MeanContrast:        meanCont,
		SpuriousTriggerRate: spuriousRate,
		SummaryText:         summaryText,
		Status:              "MEASURED",
		Passed:              true,
	}
}

func extractTriePhases(
	ctx context.Context,
	catalog *tables.Catalog,
	epoch int64,
	symbol string,
	grid *store.Grid,
	detections []*data.Measurement,
	tickMeasurements map[int64][]*data.Measurement,
) []triePhase {
	var phases []triePhase

	for _, det := range detections {
		if det == nil {
			continue
		}

		class := det.Meta("type")

		if class == "" {
			class = "unknown"
		}

		startTick := int64(getMeasurementMetric(det, "start_tick"))
		bTick := int64(getMeasurementMetric(det, "b_tick"))
		cTick := int64(getMeasurementMetric(det, "c_tick"))

		if bTick <= startTick || cTick <= bTick {
			continue
		}

		eventTokens, err := archivedIntervalTokens(
			ctx, catalog, epoch, det.Label, grid, startTick, cTick, tickMeasurements,
		)

		if err != nil || len(eventTokens) == 0 {
			continue
		}

		intervalTicks := make([]int64, 0, len(eventTokens))

		for tickVal := range eventTokens {
			intervalTicks = append(intervalTicks, tickVal)
		}

		sort.Slice(intervalTicks, func(idxA, idxB int) bool {
			return intervalTicks[idxA] < intervalTicks[idxB]
		})

		var precursorTokens []string
		var holdingTokens []string

		for _, tickVal := range intervalTicks {
			if tickVal < bTick {
				precursorTokens = append(precursorTokens, eventTokens[tickVal])
			}

			if tickVal > bTick && tickVal <= cTick {
				holdingTokens = append(holdingTokens, eventTokens[tickVal])
			}
		}

		precursorCtx := contextOfTokens(precursorTokens)
		holdingCtx := contextOfTokens(holdingTokens)

		if class == "up" {
			if precursorCtx != "" {
				phases = append(phases, triePhase{
					context:      precursorCtx,
					targetAction: "enter",
					feedback:     1.0,
					class:        class,
					tick:         bTick,
				})
			}

			if holdingCtx != "" {
				phases = append(phases, triePhase{
					context:      holdingCtx,
					targetAction: "exit",
					feedback:     1.0,
					class:        class,
					tick:         cTick,
				})
			}
		}

		if class == "up_friction" || class == "down" || class == "chop" || class == "flat" {
			if precursorCtx != "" {
				phases = append(phases, triePhase{
					context:      precursorCtx,
					targetAction: "wait",
					feedback:     -1.0,
					class:        class,
					tick:         bTick,
				})
			}
		}
	}

	sort.Slice(phases, func(idxA, idxB int) bool {
		return phases[idxA].tick < phases[idxB].tick
	})

	return phases
}

func contextOfTokens(tokens []string) string {
	var deduped []string

	for _, tok := range tokens {
		if len(tok) == 0 {
			continue
		}

		if len(deduped) == 0 || deduped[len(deduped)-1] != tok {
			deduped = append(deduped, tok)
		}
	}

	return strings.Join(deduped, "/")
}

func computeNodeMetrics(root *ui.TrieNodeJSON) TrieNodeMetrics {
	if root == nil {
		return TrieNodeMetrics{}
	}

	var depths []int
	var internalCount int
	var totalNodes int
	maxDepth := 0

	var walk func(node *ui.TrieNodeJSON, currentDepth int)
	walk = func(node *ui.TrieNodeJSON, currentDepth int) {
		if node == nil {
			return
		}

		totalNodes++
		depths = append(depths, currentDepth)

		if currentDepth > maxDepth {
			maxDepth = currentDepth
		}

		if len(node.Children) > 0 {
			internalCount++
		}

		for _, child := range node.Children {
			walk(child, currentDepth+1)
		}
	}

	walk(root, 0)

	meanDepthVal := 0.0

	if len(depths) > 0 {
		sum := 0
		for _, d := range depths {
			sum += d
		}
		meanDepthVal = float64(sum) / float64(len(depths))
	}

	branching := 0.0

	if internalCount > 0 {
		branching = float64(totalNodes-1) / float64(internalCount)
	}

	return TrieNodeMetrics{
		TotalNodes:      totalNodes,
		MaxDepth:        maxDepth,
		MeanDepth:       meanDepthVal,
		BranchingFactor: branching,
	}
}

func insufficientCognitiveTrie(reason string) Stage6CognitiveTrie {
	return Stage6CognitiveTrie{
		Status:      "INSUFFICIENT_DATA",
		SummaryText: fmt.Sprintf("Cognitive Trie: %s", reason),
		Passed:      false,
	}
}

func computeMean(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}

	sum := 0.0

	for _, val := range values {
		sum += val
	}

	return sum / float64(len(values))
}

func computeStdDev(values []float64, meanVal float64) float64 {
	if len(values) < 2 {
		return 0.0
	}

	varianceSum := 0.0

	for _, val := range values {
		diff := val - meanVal
		varianceSum += diff * diff
	}

	return math.Sqrt(varianceSum / float64(len(values)-1))
}
