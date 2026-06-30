package trader

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/logic"
)

type CandidateOutcomeRecorder struct {
	horizon time.Duration
	pending []candidateOutcome
	seen    map[string]struct{}
}

type candidateOutcome struct {
	decision    datura.Map[any]
	tick        int64
	symbol      string
	side        string
	actionType  string
	source      string
	category    string
	edgeKey     string
	entryPrice  float64
	entryStamp  int64
	targetStamp int64
}

func NewCandidateOutcomeRecorder() *CandidateOutcomeRecorder {
	return &CandidateOutcomeRecorder{
		horizon: edgeForwardReturnHorizon(),
		seen:    make(map[string]struct{}),
	}
}

func (recorder *CandidateOutcomeRecorder) Observe(
	tickCount int64,
	decisions []datura.Map[any],
	tree *dmt.Tree,
	auditRecorder *audit.Recorder,
) {
	if recorder == nil || tree == nil {
		return
	}

	recorder.enqueue(tickCount, decisions, tree)
	recorder.flushMature(tree, auditRecorder)
}

func (recorder *CandidateOutcomeRecorder) enqueue(
	tickCount int64,
	decisions []datura.Map[any],
	tree *dmt.Tree,
) {
	if len(decisions) == 0 {
		return
	}
	if recorder.seen == nil {
		recorder.seen = make(map[string]struct{})
	}

	for _, decision := range decisions {
		pending, ok := recorder.pendingFromDecision(tickCount, decision, tree)
		if !ok {
			continue
		}
		key := pending.key()
		if _, exists := recorder.seen[key]; exists {
			continue
		}
		recorder.seen[key] = struct{}{}
		recorder.pending = append(recorder.pending, pending)
	}
}

func (recorder *CandidateOutcomeRecorder) pendingFromDecision(
	tickCount int64,
	decision datura.Map[any],
	tree *dmt.Tree,
) (candidateOutcome, bool) {
	symbol := mapString(decision, "symbol")
	actionType := mapString(decision, "type")
	side := mapString(decision, "side")
	source := mapString(decision, "source")
	category := mapString(decision, "category")

	if symbol == "" || actionType == "" || source == "" || category == "" {
		return candidateOutcome{}, false
	}
	if isOutcomeExit(actionType, side) {
		return candidateOutcome{}, false
	}

	mark, ok := latestTickerMark(tree, symbol)
	if !ok {
		return candidateOutcome{}, false
	}

	entryStamp := mark.stamp
	if entryStamp <= 0 {
		return candidateOutcome{}, false
	}

	return candidateOutcome{
		decision:    cloneDecision(decision),
		tick:        tickCount,
		symbol:      symbol,
		side:        side,
		actionType:  actionType,
		source:      source,
		category:    category,
		edgeKey:     mapString(decision, "edge_key"),
		entryPrice:  mark.price,
		entryStamp:  entryStamp,
		targetStamp: entryStamp + recorder.horizon.Nanoseconds(),
	}, true
}

func (recorder *CandidateOutcomeRecorder) flushMature(
	tree *dmt.Tree,
	auditRecorder *audit.Recorder,
) {
	if len(recorder.pending) == 0 {
		return
	}

	remaining := recorder.pending[:0]
	for _, pending := range recorder.pending {
		future, ok := firstTickerMarkAtOrAfter(tree, pending.symbol, pending.targetStamp)
		if !ok {
			remaining = append(remaining, pending)
			continue
		}
		if !pending.write(tree, auditRecorder, future) {
			remaining = append(remaining, pending)
		}
	}
	recorder.pending = remaining
}

func (pending candidateOutcome) write(
	tree *dmt.Tree,
	auditRecorder *audit.Recorder,
	future edgeMark,
) bool {
	if pending.entryPrice <= 0 || future.price <= 0 {
		return false
	}

	reward := signedReturn(pending.side, pending.entryPrice, future.price)
	if math.IsNaN(reward) || math.IsInf(reward, 0) {
		return false
	}

	row := cloneDecision(pending.decision)
	row["channel"] = "optimizer"
	row["event"] = "candidate_outcome"
	row["tick"] = pending.tick
	row["symbol"] = pending.symbol
	row["side"] = pending.side
	row["type"] = pending.actionType
	row["source"] = pending.source
	row["category"] = pending.category
	row["entry_price"] = pending.entryPrice
	row["entry_timestamp"] = pending.entryStamp
	row["future_price"] = future.price
	row["future_timestamp"] = future.stamp
	row["horizon"] = edgeForwardReturnHorizon().String()
	row["reward"] = reward
	row["reward_bps"] = reward * 10_000

	if pending.edgeKey != "" {
		row["edge_key"] = pending.edgeKey
		row["setup_key"] = pending.edgeKey
	}
	if strings.EqualFold(pending.actionType, string(logic.ActionMarket)) {
		row["filled"] = true
	} else if _, ok := row["fill_probability"]; !ok {
		return false
	}

	artifact := datura.Acquire("trader", datura.APPJSON).
		WithRole("candidate_outcome").
		WithScope(pending.symbol).
		WithPayload(datura.Map[any]{
			"channel": "candidate_outcome",
			"type":    "update",
			"data": []datura.Map[any]{
				{
					"symbol":         pending.symbol,
					"side":           pending.side,
					"order_type":     pending.actionType,
					"source":         pending.source,
					"category":       pending.category,
					"setup_key":      pending.edgeKey,
					"edge_key":       pending.edgeKey,
					"entry_price":    pending.entryPrice,
					"future_price":   future.price,
					"reward":         reward,
					"reward_bps":     reward * 10_000,
					"entry_stamp":    pending.entryStamp,
					"future_stamp":   future.stamp,
					"target_stamp":   pending.targetStamp,
					"decision_tick":  pending.tick,
					"entry_filled":   row["filled"],
					"candidate_type": "playbook",
				},
			},
		}.Marshal())
	artifact.SetTimestamp(future.stamp)
	tree.InsertArtifact(artifact.Prefix("role", "scope", "timestamp"), artifact)
	tree.InsertArtifact(artifact.Prefix("role", "timestamp", "scope"), artifact)

	if auditRecorder != nil {
		errnie.Error(auditRecorder.Write(row))
	}

	return true
}

func (pending candidateOutcome) key() string {
	return strings.Join([]string{
		pending.symbol,
		pending.side,
		pending.actionType,
		pending.source,
		pending.category,
		datura.FormatTimestamp(pending.entryStamp),
	}, "|")
}

func cloneDecision(decision datura.Map[any]) datura.Map[any] {
	out := make(datura.Map[any], len(decision))
	for key, value := range decision {
		out[key] = value
	}
	return out
}

func isOutcomeExit(actionType string, side string) bool {
	if strings.EqualFold(side, string(logic.SideSell)) {
		return true
	}
	switch logic.ActionType(actionType) {
	case logic.ActionStopLoss, logic.ActionStopLossLimit,
		logic.ActionTakeProfit, logic.ActionTakeProfitLimit,
		logic.ActionTrailingStop, logic.ActionTrailingStopLimit,
		logic.ActionSettlePosition:
		return true
	default:
		return false
	}
}

func signedReturn(side string, entry float64, future float64) float64 {
	if strings.EqualFold(side, string(logic.SideSell)) {
		return (entry - future) / entry
	}
	return (future - entry) / entry
}

func latestTickerMark(tree *dmt.Tree, symbol string) (edgeMark, bool) {
	target := strings.ToUpper(strings.TrimSpace(symbol))
	if target == "" || tree == nil {
		return edgeMark{}, false
	}

	if artifact, ok := artifactAtKey(tree, []byte("latest/ticker/"+target)); ok {
		if mark, ok := tickerMarkFromArtifact(artifact, target); ok {
			return mark, true
		}
	}

	marks := tickerMarksFromTree(tree, target)
	if len(marks) == 0 {
		return edgeMark{}, false
	}

	return marks[len(marks)-1], true
}

func firstTickerMarkAtOrAfter(tree *dmt.Tree, symbol string, stamp int64) (edgeMark, bool) {
	marks := tickerMarksFromTree(tree, strings.ToUpper(strings.TrimSpace(symbol)))
	for _, mark := range marks {
		if mark.stamp >= stamp && mark.price > 0 {
			return mark, true
		}
	}
	return edgeMark{}, false
}

func tickerMarksFromTree(tree *dmt.Tree, target string) []edgeMark {
	if tree == nil || target == "" {
		return nil
	}

	marks := make([]edgeMark, 0)
	for artifact := range tree.Seek([]byte("ticker/" + target + "/")) {
		if mark, ok := tickerMarkFromArtifact(artifact, target); ok {
			marks = append(marks, mark)
		}
	}
	for artifact := range tree.Seek([]byte("ticker/")) {
		if mark, ok := tickerMarkFromArtifact(artifact, target); ok {
			marks = append(marks, mark)
		}
	}

	sort.Slice(marks, func(first, second int) bool {
		return marks[first].stamp < marks[second].stamp
	})
	return marks
}

func tickerMarkFromArtifact(artifact *datura.Artifact, target string) (edgeMark, bool) {
	if artifact == nil {
		return edgeMark{}, false
	}
	for rowIndex := 0; ; rowIndex++ {
		symbol := datura.Peek[string](artifact, "data", rowIndex, "symbol")
		if symbol == "" {
			break
		}
		if strings.ToUpper(strings.TrimSpace(symbol)) != target {
			continue
		}
		price := datura.Peek[float64](artifact, "data", rowIndex, "last")
		if price <= 0 {
			price = datura.Peek[float64](artifact, "data", rowIndex, "price")
		}
		if price <= 0 {
			continue
		}
		return edgeMark{price: price, stamp: artifact.Timestamp()}, true
	}
	return edgeMark{}, false
}

func artifactAtKey(tree *dmt.Tree, key []byte) (*datura.Artifact, bool) {
	value, ok := tree.Get(key)
	if !ok || len(value) == 0 {
		return nil, false
	}
	artifact := &datura.Artifact{}
	if _, err := artifact.Unpack(value); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: unpack latest ticker mark",
			err,
		))
		return nil, false
	}
	return artifact, true
}

func mapString(row datura.Map[any], key string) string {
	value, ok := row[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}
