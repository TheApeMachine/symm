package audit

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
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

type modelCall struct {
	Winner     string
	Confidence float64
	Contrast   float64
}

type trieNode struct {
	id         string
	token      string
	prefix     string
	count      uint64
	enterScore float64
	exitScore  float64
	children   map[string]*trieNode
}

type cognitiveModel struct {
	root     *trieNode
	records  int
	maxDepth int
}

func newCognitiveModel() *cognitiveModel {
	return &cognitiveModel{
		root: &trieNode{
			id:       "root",
			children: make(map[string]*trieNode),
		},
	}
}

func (model *cognitiveModel) Teach(context, action string, feedback float64) error {
	if action != "enter" && action != "exit" {
		return nil
	}

	parts := strings.Split(context, "/")
	current := model.root
	depth := 0

	for _, token := range parts {
		if token == "" {
			continue
		}

		depth++
		child, ok := current.children[token]

		if !ok {
			prefix := token

			if current.prefix != "" {
				prefix = current.prefix + "/" + token
			}

			child = &trieNode{
				id:       prefix,
				token:    token,
				prefix:   prefix,
				children: make(map[string]*trieNode),
			}
			current.children[token] = child
			model.records++
		}

		current = child
	}

	current.count++

	if depth > model.maxDepth {
		model.maxDepth = depth
	}

	if action == "enter" {
		current.enterScore += feedback
	}

	if action == "exit" {
		current.exitScore += feedback
	}

	return nil
}

func (model *cognitiveModel) Recall(context, stance string) (modelCall, error) {
	if context == "" {
		return modelCall{}, nil
	}

	parts := strings.Split(context, "/")
	current := model.root

	for _, token := range parts {
		if token == "" {
			continue
		}

		child, ok := current.children[token]

		if !ok {
			break
		}

		current = child
	}

	if current == model.root {
		return modelCall{}, nil
	}

	enter := current.enterScore
	exit := current.exitScore

	if stance == "enter" {
		exit = -1.0
	}

	if stance == "exit" {
		enter = -1.0
	}

	if enter <= 0 && exit <= 0 {
		return modelCall{}, nil
	}

	winner := "enter"
	winningScore := enter
	otherScore := exit

	if exit > enter {
		winner = "exit"
		winningScore = exit
		otherScore = enter
	}

	denom := math.Abs(enter) + math.Abs(exit)
	confidence := 1.0

	if denom > 0 {
		confidence = winningScore / denom
	}

	contrast := winningScore - otherScore

	return modelCall{
		Winner:     winner,
		Confidence: confidence,
		Contrast:   contrast,
	}, nil
}

func (model *cognitiveModel) Count(key string) (float64, error) {
	if key == "records" {
		return float64(model.records), nil
	}

	if key == "span" {
		return float64(model.maxDepth), nil
	}

	count := 0.0
	var countNodes func(node *trieNode)
	countNodes = func(node *trieNode) {
		if node != model.root {
			if key == "enter" && node.enterScore > 0 && node.enterScore > node.exitScore {
				count++
			}

			if key == "exit" && node.exitScore > 0 && node.exitScore > node.enterScore {
				count++
			}
		}

		for _, child := range node.children {
			countNodes(child)
		}
	}

	countNodes(model.root)
	return count, nil
}

func (model *cognitiveModel) CognitionTree() ui.CognitionTreeExport {
	var convert func(node *trieNode) *ui.TrieNodeJSON
	convert = func(node *trieNode) *ui.TrieNodeJSON {
		if node == nil {
			return nil
		}

		jsonNode := &ui.TrieNodeJSON{
			ID:          node.id,
			TokenPrefix: node.prefix,
			Count:       node.count,
		}

		if len(node.children) > 0 {
			jsonNode.Children = make([]*ui.TrieNodeJSON, 0, len(node.children))

			for _, child := range node.children {
				jsonNode.Children = append(jsonNode.Children, convert(child))
			}
		}

		return jsonNode
	}

	return ui.CognitionTreeExport{
		Root: convert(model.root),
	}
}

/*
AnalyzeCognitiveTrie evaluates a SIMULATED trie: cognitiveModel, an in-memory
prefix model built inside the audit. It is not the production memory (the S3
symm bucket read by strategy.Step), and nothing here reads that bucket, so its
verdict says nothing about the keys paper trading would match. It measures the
simulated associative memory and Radix Trie learning dynamics:
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
	significance float64,
	detections []*data.Measurement,
	takerFee ...float64,
) Stage6CognitiveTrie {
	if catalog == nil || grid == nil {
		return insufficientCognitiveTrie("Catalog or grid unavailable.")
	}

	if permutations <= 0 {
		permutations = 50
	}

	var fee float64
	if len(takerFee) > 0 && takerFee[0] > 0 {
		fee = takerFee[0]
	}
	if fee <= 0 {
		return insufficientCognitiveTrie("Taker fee is required and must be strictly positive.")
	}

	if detections == nil {
		var minTick, maxTick int64
		if len(orderedTicks) > 0 {
			minTick = orderedTicks[0]
			maxTick = orderedTicks[len(orderedTicks)-1]
		}

		loaded, loadErr := loadOrDetectExcursions(ctx, catalog, epoch, symbol, fee, minTick, maxTick)

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

	model := newCognitiveModel()
	totalCalls := len(phases)
	hits := 0
	actionCounts := map[string]int{"enter": 0, "exit": 0, "wait": 0}
	actuals := make([]string, 0, totalCalls)
	preds := make([]string, 0, totalCalls)
	abstentions := 0
	confidences := make([]float64, 0, totalCalls)
	contrasts := make([]float64, 0, totalCalls)
	taughtList := make([]taughtRecord, 0, totalCalls)

	for _, phaseItem := range phases {
		actionCounts[phaseItem.targetAction]++
		actuals = append(actuals, phaseItem.targetAction)

		call, callErr := model.Recall(phaseItem.context, "")

		if callErr != nil {
			errnie.Error(callErr)
		}

		confidences = append(confidences, call.Confidence)
		contrasts = append(contrasts, call.Contrast)

		predAction := call.Winner

		if predAction == "" {
			predAction = "wait"
			abstentions++
		}

		preds = append(preds, predAction)

		if phaseItem.targetAction == predAction {
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

	balancedAcc, mcc, enterPrec, enterRec := computeClassificationMetrics(actuals, preds, actionCounts)

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

	abstainPreds := make([]string, totalCalls)

	for idx := range abstainPreds {
		abstainPreds[idx] = "wait"
	}

	baselineBalancedAcc, _, _, _ := computeClassificationMetrics(actuals, abstainPreds, actionCounts)

	nullHits := make([]float64, permutations)
	nullBalancedAccs := make([]float64, permutations)
	rng := rand.New(rand.NewPCG(uint64(epoch), 12345))

	for permIndex := 0; permIndex < permutations; permIndex++ {
		permModel := newCognitiveModel()
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
		permPreds := make([]string, totalCalls)
		permCounts := map[string]int{"enter": 0, "exit": 0, "wait": 0}

		for phaseIndex, ph := range phases {
			act := permActions[phaseIndex]
			fb := permFeedbacks[phaseIndex]
			permCounts[act]++

			pCall, pErr := permModel.Recall(ph.context, "")

			if pErr != nil {
				errnie.Error(pErr)
			}

			predAct := pCall.Winner

			if predAct == "" {
				predAct = "wait"
			}

			permPreds[phaseIndex] = predAct

			if act == predAct {
				permHitCount++
			}

			if act == "enter" || act == "exit" {
				teachErr := permModel.Teach(ph.context, act, fb)

				if teachErr != nil {
					errnie.Error(teachErr)
				}
			}

			if act == "wait" {
				teachErr := permModel.Teach(ph.context, "enter", fb)

				if teachErr != nil {
					errnie.Error(teachErr)
				}
			}
		}

		nullHits[permIndex] = float64(permHitCount)
		permBalAcc, _, _, _ := computeClassificationMetrics(permActions, permPreds, permCounts)
		nullBalancedAccs[permIndex] = permBalAcc
	}

	sort.Float64s(nullHits)
	nullMean := computeMean(nullHits)
	nullStd := computeStdDev(nullHits, nullMean)
	nullP95 := quantileOf(nullHits, 0.95)

	sort.Float64s(nullBalancedAccs)
	nullMeanBalAcc := computeMean(nullBalancedAccs)
	nullP95BalAcc := quantileOf(nullBalancedAccs, 0.95)

	// Add-one p of the balanced accuracy against the label-permuted null; it
	// must also beat the best constant policy.
	empiricalPVal := upperPValue(balancedAcc, nullBalancedAccs)
	verdict := hypothesisVerdict(empiricalPVal, len(nullBalancedAccs), significance)

	if verdict == VerdictSupported && balancedAcc <= baselineBalancedAcc {
		verdict = VerdictNotSupported
	}

	separatesFromNull := verdict == VerdictSupported

	retainedCount := 0

	for _, item := range taughtList {
		postCall, postErr := model.Recall(item.context, "")

		if postErr != nil {
			errnie.Error(postErr)
		}

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

	recordsCount, countErr := model.Count("records")

	if countErr != nil {
		errnie.Error(countErr)
	}

	spanCount, countErr := model.Count("span")

	if countErr != nil {
		errnie.Error(countErr)
	}

	enterBasins, countErr := model.Count("enter")

	if countErr != nil {
		errnie.Error(countErr)
	}

	exitBasins, countErr := model.Count("exit")

	if countErr != nil {
		errnie.Error(countErr)
	}

	export := model.CognitionTree()
	nodeStats := computeNodeMetrics(export.Root)

	spuriousCount := 0
	evaluatedTicks := 0

	if len(orderedTicks) > 0 {
		splitIdx := int(float64(len(orderedTicks)) * 0.60)
		unseenTicks := orderedTicks[splitIdx:]
		evaluatedTicks = len(unseenTicks)

		windowTokens := make(map[string][]string)

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
				train := data.NewMeasurement(
					symMeas[0].Epoch,
					sym,
					"audit",
					symMeas[0].SeqIdx,
					tickVal,
				)
				train.At = symMeas[0].At
				train.From = symMeas[0].From
				train.Peers(symMeas...)
				train.Write()

				tokenBytes := grid.Observe(train)

				if len(tokenBytes) == 0 {
					continue
				}

				tok := string(tokenBytes)
				symWindow := windowTokens[sym]

				if len(symWindow) == 0 || symWindow[len(symWindow)-1] != tok {
					symWindow = append(symWindow, tok)
				}

				if limit := int(spanCount); limit >= 1 && len(symWindow) > limit {
					symWindow = symWindow[len(symWindow)-limit:]
				}

				windowTokens[sym] = symWindow

				qCtx := strings.Join(symWindow, "/")
				bgCall, bgErr := model.Recall(qCtx, "")

				if bgErr != nil {
					errnie.Error(bgErr)
				}

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

	s3Audit := auditS3PrefixMemory(phases)

	summaryText := fmt.Sprintf(
		"SIMULATED trie (in-memory model, not the production S3 memory): %d phases (%d enter, %d exit, %d wait). Prequential hits=%d/%d (%.1f%%) [Balanced Acc: %.1f%%, MCC: %.3f, Enter Prec/Rec: %.1f%%/%.1f%%] vs best baseline (%s) %d/%d (%.1f%%, Balanced Acc: %.1f%%). Null p95=%.1f hits, Balanced Acc p95=%.1f%% (p=%.3f). S3 Memory: %d keys, %d collisions, time-to-disambiguation=%d tokens. Retention=%.1f%%. Spurious bg rate=%.2f%%.",
		len(phases), actionCounts["enter"], actionCounts["exit"], actionCounts["wait"],
		hits, totalCalls, hitRate*100,
		balancedAcc*100, mcc, enterPrec*100, enterRec*100,
		bestBaselinePolicy, bestBaselineHits, totalCalls, baselineHitRate*100,
		baselineBalancedAcc*100,
		nullP95, nullP95BalAcc*100, empiricalPVal,
		s3Audit.TotalPrefixKeys, s3Audit.PrefixCollisions, s3Audit.TimeToDisambiguation,
		retentionRate*100, spuriousRate*100,
	)

	return Stage6CognitiveTrie{
		DetectionsEvaluated: len(detections),
		PhasesFormed:        len(phases),
		ActionCounts:        actionCounts,
		Skill: TrieSkillMetrics{
			TotalCalls:               totalCalls,
			Hits:                     hits,
			HitRate:                  hitRate,
			BalancedAccuracy:         balancedAcc,
			MCC:                      mcc,
			EnterPrecision:           enterPrec,
			EnterRecall:              enterRec,
			BaselineHits:             bestBaselineHits,
			BaselineHitRate:          baselineHitRate,
			BestBaselinePolicy:       bestBaselinePolicy,
			BaselineBalancedAccuracy: baselineBalancedAcc,
			NullMeanHits:             nullMean,
			NullStdHits:              nullStd,
			Null95thPercentileHits:   nullP95,
			NullMeanBalancedAccuracy: nullMeanBalAcc,
			Null95thBalancedAccuracy: nullP95BalAcc,
			SeparatesFromNull:        separatesFromNull,
			EmpiricalPValue:          empiricalPVal,
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
		S3Memory:            s3Audit,
		SummaryText:         summaryText,
		Status:              verdict,
		Passed:              passed(verdict),
	}
}

func auditS3PrefixMemory(phases []triePhase) S3MemoryAudit {
	if len(phases) == 0 {
		return S3MemoryAudit{
			SummaryText: "No phases available to audit S3 prefix memory.",
			Passed:      false,
		}
	}

	uniqueKeys := make(map[string]struct{})
	contextActionMap := make(map[string]map[string]int)
	prefixByDepth := make(map[int]map[string]map[string]int)
	maxDepth := 0
	totalDepth := 0

	prequentialHits := 0
	prequentialCalls := 0
	storedPrefixes := make(map[string]string)

	for _, ph := range phases {
		if ph.context == "" {
			continue
		}

		key := ph.context + "/" + ph.targetAction + ".json"
		uniqueKeys[key] = struct{}{}

		tokens := strings.Split(ph.context, "/")
		depth := len(tokens)
		if depth > maxDepth {
			maxDepth = depth
		}
		totalDepth += depth

		if contextActionMap[ph.context] == nil {
			contextActionMap[ph.context] = make(map[string]int)
		}
		contextActionMap[ph.context][ph.targetAction]++

		for k := 1; k <= depth; k++ {
			subPrefix := strings.Join(tokens[:k], "/")
			if prefixByDepth[k] == nil {
				prefixByDepth[k] = make(map[string]map[string]int)
			}
			if prefixByDepth[k][subPrefix] == nil {
				prefixByDepth[k][subPrefix] = make(map[string]int)
			}
			prefixByDepth[k][subPrefix][ph.targetAction]++
		}

		if storedAct, ok := storedPrefixes[ph.context]; ok {
			prequentialCalls++
			if storedAct == ph.targetAction {
				prequentialHits++
			}
		}

		storedPrefixes[ph.context] = ph.targetAction
	}

	prefixCollisions := 0
	conflictingContinuations := 0

	for _, actions := range contextActionMap {
		if actions["enter"] > 0 && actions["wait"] > 0 {
			prefixCollisions++
		}
		if actions["enter"] > 0 && actions["exit"] > 0 {
			conflictingContinuations++
		}
	}

	meanDepth := 0.0
	if len(phases) > 0 {
		meanDepth = float64(totalDepth) / float64(len(phases))
	}

	timeToDisambiguation := maxDepth
	for k := 1; k <= maxDepth; k++ {
		depthCollisions := 0
		totalPrefixesAtK := len(prefixByDepth[k])
		if totalPrefixesAtK == 0 {
			continue
		}
		for _, actions := range prefixByDepth[k] {
			if actions["enter"] > 0 && (actions["wait"] > 0 || actions["exit"] > 0) {
				depthCollisions++
			}
		}
		collisionRate := float64(depthCollisions) / float64(totalPrefixesAtK)
		if collisionRate <= 0.05 {
			timeToDisambiguation = k
			break
		}
	}

	prequentialAcc := 0.0
	if prequentialCalls > 0 {
		prequentialAcc = float64(prequentialHits) / float64(prequentialCalls)
	}

	var storageBytes int64
	for k := range uniqueKeys {
		storageBytes += int64(128 + len(k))
	}

	summary := fmt.Sprintf(
		"Simulated prefix memory (keys the audit would write, not the S3 bucket): %d keys across %d unique prefixes (mean depth=%.1f, max=%d). "+
			"Prefix collisions (enter vs wait)=%d, conflicting continuations (enter vs exit)=%d. "+
			"Time-to-disambiguation=%d tokens. Prequential retrieval accuracy=%.1f%% (%d/%d calls). Storage=%d bytes.",
		len(uniqueKeys), len(contextActionMap), meanDepth, maxDepth,
		prefixCollisions, conflictingContinuations,
		timeToDisambiguation, prequentialAcc*100, prequentialHits, prequentialCalls, storageBytes,
	)

	return S3MemoryAudit{
		TotalPrefixKeys:          len(uniqueKeys),
		PrefixCollisions:         prefixCollisions,
		ConflictingContinuations: conflictingContinuations,
		MeanPrefixDepth:          meanDepth,
		MaxPrefixDepth:           maxDepth,
		TimeToDisambiguation:     timeToDisambiguation,
		PrequentialRetrievalAcc:  prequentialAcc,
		StorageBytes:             storageBytes,
		SummaryText:              summary,
		Passed:                   prefixCollisions == 0,
	}
}

func computeClassificationMetrics(
	actuals []string,
	preds []string,
	actionCounts map[string]int,
) (float64, float64, float64, float64) {
	if len(actuals) == 0 || len(actuals) != len(preds) {
		return 0.0, 0.0, 0.0, 0.0
	}

	recalls := make([]float64, 0, 3)

	for class, count := range actionCounts {
		if count <= 0 {
			continue
		}

		tp := 0

		for idx := range actuals {
			if actuals[idx] == class && preds[idx] == class {
				tp++
			}
		}

		recalls = append(recalls, float64(tp)/float64(count))
	}

	balancedAcc := 0.0

	for _, rec := range recalls {
		balancedAcc += rec
	}

	if len(recalls) > 0 {
		balancedAcc /= float64(len(recalls))
	}

	tpEnter := 0
	fpEnter := 0
	fnEnter := 0
	tnEnter := 0

	for idx := range actuals {
		isActualEnter := actuals[idx] == "enter"
		isPredEnter := preds[idx] == "enter"

		if isActualEnter && isPredEnter {
			tpEnter++
		}

		if !isActualEnter && isPredEnter {
			fpEnter++
		}

		if isActualEnter && !isPredEnter {
			fnEnter++
		}

		if !isActualEnter && !isPredEnter {
			tnEnter++
		}
	}

	enterPrec := 0.0

	if tpEnter+fpEnter > 0 {
		enterPrec = float64(tpEnter) / float64(tpEnter+fpEnter)
	}

	enterRec := 0.0

	if tpEnter+fnEnter > 0 {
		enterRec = float64(tpEnter) / float64(tpEnter+fnEnter)
	}

	mccNumerator := float64(tpEnter*tnEnter - fpEnter*fnEnter)
	mccDenom := math.Sqrt(float64(tpEnter+fpEnter) * float64(tpEnter+fnEnter) * float64(tnEnter+fpEnter) * float64(tnEnter+fnEnter))
	mcc := 0.0

	if mccDenom > 0 {
		mcc = mccNumerator / mccDenom
	}

	return balancedAcc, mcc, enterPrec, enterRec
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

		slices.Sort(intervalTicks)

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
