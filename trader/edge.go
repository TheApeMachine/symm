package trader

import (
	"bytes"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/market"
)

type EdgeEstimate struct {
	EdgeKey           string
	ExpectedReturnBps float64
	FrictionBps       float64
	NetEdgeBps        float64
	SampleCount       int
	CalibrationReady  bool
	EdgeSource        string
}

type EdgeEstimator struct {
	economics  executionEconomics
	minSamples int
	tree       *dmt.Tree
}

func newEdgeEstimator(economics executionEconomics, tree *dmt.Tree) EdgeEstimator {
	minSamples := 30
	if viper.IsSet("market.story.forward_return_min_samples") {
		minSamples = viper.GetInt("market.story.forward_return_min_samples")
		if minSamples < 0 {
			minSamples = 0
		}
	}

	return EdgeEstimator{
		economics:  economics,
		minSamples: minSamples,
		tree:       tree,
	}
}

/*
Estimate consumes calibrated backend return evidence. Causal uplift is accepted
as an explanatory feature for score shaping, but it is not converted into money
and never makes an uncalibrated entry tradable.
*/
func (estimator EdgeEstimator) Estimate(
	action *datura.Artifact,
	causalUplift float64,
	hasCausalUplift bool,
) EdgeEstimate {
	_ = causalUplift
	_ = hasCausalUplift

	orderType := datura.Peek[string](action, "type")
	friction, _ := estimator.economics.roundTripHurdle(orderType)
	out := EdgeEstimate{
		FrictionBps: friction * 10_000,
		EdgeSource:  "unavailable",
	}

	expected, expectedOK := firstFiniteBps(action,
		[]any{"decision", "expected_return_bps"},
		[]any{"expected_return_bps"},
		[]any{"edge", "expected_return_bps"},
	)
	samples := firstPositiveInt(action,
		[]any{"decision", "sample_count"},
		[]any{"sample_count"},
		[]any{"edge", "sample_count"},
	)
	source := firstString(action,
		[]any{"decision", "edge_source"},
		[]any{"edge_source"},
		[]any{"edge", "source"},
	)
	out.EdgeKey = setupEdgeKey(action)

	if source != "" {
		out.EdgeSource = source
	}

	if treeEstimate, ok := estimator.treeEstimate(action); ok {
		if !expectedOK || treeEstimate.SampleCount > samples {
			expected = treeEstimate.ExpectedReturnBps
			expectedOK = true
			samples = treeEstimate.SampleCount
			out.EdgeSource = treeEstimate.EdgeSource
		}
	}

	if !expectedOK {
		return out
	}
	if out.EdgeKey == "" {
		out.EdgeSource = "setup_key_unavailable"
		return out
	}

	out.ExpectedReturnBps = expected
	out.SampleCount = samples
	out.NetEdgeBps = expected - out.FrictionBps

	if out.SampleCount >= estimator.minSamples {
		out.CalibrationReady = true
		if out.EdgeSource == "unavailable" {
			out.EdgeSource = "forward_return"
		}
	}

	return out
}

func (estimator EdgeEstimator) treeEstimate(action *datura.Artifact) (EdgeEstimate, bool) {
	if estimator.tree == nil || action == nil {
		return EdgeEstimate{}, false
	}

	symbol, _ := action.Scope()
	if symbol == "" {
		symbol = datura.Peek[string](action, "symbol")
	}
	if symbol == "" {
		return EdgeEstimate{}, false
	}

	edgeKey := setupEdgeKey(action)
	if edgeKey == "" {
		return EdgeEstimate{
			EdgeSource: "setup_key_unavailable",
		}, false
	}

	returns := estimator.forwardReturns(symbol, edgeKey)
	returns = append(returns, estimator.candidateOutcomeReturns(symbol, edgeKey)...)
	if len(returns) == 0 {
		return EdgeEstimate{}, false
	}

	sum := 0.0
	count := 0
	for _, value := range returns {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		sum += value
		count++
	}
	if count == 0 {
		return EdgeEstimate{}, false
	}

	return EdgeEstimate{
		EdgeKey:           edgeKey,
		ExpectedReturnBps: sum / float64(count),
		SampleCount:       count,
		EdgeSource:        "forward_return",
	}, true
}

type edgeFill struct {
	side    string
	price   float64
	stamp   int64
	edgeKey string
}

func (estimator EdgeEstimator) forwardReturns(symbol string, edgeKey string) []float64 {
	fills := estimator.executionFills(symbol, edgeKey)
	if len(fills) == 0 {
		return nil
	}

	marks := estimator.tickerMarks(symbol)
	horizon := edgeForwardReturnHorizon()
	returns := make([]float64, 0, len(fills))

	for _, fill := range fills {
		switch fill.side {
		case "buy", "enter":
			targetStamp := fill.stamp + horizon.Nanoseconds()
			if mark, ok := firstMarkAtOrAfter(marks, targetStamp); ok {
				returns = append(returns, returnBps(fill.price, mark.price))
			}
		}
	}

	return returns
}

func (estimator EdgeEstimator) candidateOutcomeReturns(symbol string, edgeKey string) []float64 {
	if estimator.tree == nil {
		return nil
	}

	target := strings.ToUpper(strings.TrimSpace(symbol))
	edgeKey = normalizeSetupKey(edgeKey)
	out := make([]float64, 0)

	collect := func(prefix []byte) {
		for artifact := range estimator.tree.Seek(prefix) {
			for rowIndex := 0; ; rowIndex++ {
				rowSymbol := datura.Peek[string](artifact, "data", rowIndex, "symbol")
				if rowSymbol == "" {
					break
				}
				if strings.ToUpper(strings.TrimSpace(rowSymbol)) != target {
					continue
				}
				rowEdgeKey := normalizeSetupKey(firstString(artifact,
					[]any{"data", rowIndex, "edge_key"},
					[]any{"data", rowIndex, "setup_key"},
				))
				if edgeKey != "" && rowEdgeKey != edgeKey {
					continue
				}
				rewardBps := datura.Peek[float64](artifact, "data", rowIndex, "reward_bps")
				if rewardBps == 0 {
					reward := datura.Peek[float64](artifact, "data", rowIndex, "reward")
					rewardBps = reward * 10_000
				}
				if rewardBps == 0 || math.IsNaN(rewardBps) || math.IsInf(rewardBps, 0) {
					continue
				}
				out = append(out, rewardBps)
			}
		}
	}

	collect([]byte("candidate_outcome/" + target + "/"))

	return out
}

func (estimator EdgeEstimator) executionFills(symbol string, edgeKey string) []edgeFill {
	target := strings.ToUpper(strings.TrimSpace(symbol))
	edgeKey = normalizeSetupKey(edgeKey)
	fills := make([]edgeFill, 0)

	for artifact := range estimator.tree.Seek([]byte("executions/")) {
		for rowIndex := 0; ; rowIndex++ {
			rowSymbol := datura.Peek[string](artifact, "data", rowIndex, "symbol")
			if rowSymbol == "" {
				break
			}
			if strings.ToUpper(rowSymbol) != target {
				continue
			}
			status := strings.ToLower(datura.Peek[string](artifact, "data", rowIndex, "order_status"))
			if status != "" && status != "filled" {
				continue
			}
			rowEdgeKey := executionSetupKey(artifact, rowIndex)
			if edgeKey != "" && rowEdgeKey != edgeKey {
				continue
			}
			price := datura.Peek[float64](artifact, "data", rowIndex, "avg_price")
			if price <= 0 {
				price = datura.Peek[float64](artifact, "data", rowIndex, "last_price")
			}
			if price <= 0 {
				price = datura.Peek[float64](artifact, "data", rowIndex, "price")
			}
			if price <= 0 {
				continue
			}
			fills = append(fills, edgeFill{
				side:    strings.ToLower(datura.Peek[string](artifact, "data", rowIndex, "side")),
				price:   price,
				stamp:   artifact.Timestamp(),
				edgeKey: rowEdgeKey,
			})
		}
	}

	sort.Slice(fills, func(first, second int) bool {
		return fills[first].stamp < fills[second].stamp
	})

	return fills
}

func edgeForwardReturnHorizon() time.Duration {
	for _, key := range []string{
		"trading.edge.forward_return_horizon",
		"market.story.forward_return_horizon",
	} {
		raw := strings.TrimSpace(viper.GetString(key))
		if raw == "" {
			continue
		}
		duration, err := time.ParseDuration(raw)
		if err == nil && duration > 0 {
			return duration
		}
	}

	return 5 * time.Minute
}

func setupEdgeKey(action *datura.Artifact) string {
	if action == nil {
		return ""
	}

	explicit := firstString(action,
		[]any{"decision", "setup_key"},
		[]any{"decision", "edge_key"},
		[]any{"edge", "key"},
		[]any{"setup_key"},
		[]any{"params", "setup_key"},
	)
	if explicit != "" {
		return normalizeSetupKey(explicit)
	}

	symbol := firstString(action,
		[]any{"symbol"},
		[]any{"scope"},
	)
	if symbol == "" {
		symbol, _ = action.Scope()
	}
	source := firstString(action,
		[]any{"reason_source"},
		[]any{"journey", "story", "source"},
		[]any{"source"},
	)
	category := firstString(action,
		[]any{"reason_category"},
		[]any{"journey", "story", "category"},
		[]any{"category"},
	)
	side := firstString(action, []any{"side"})
	actionType := firstString(action, []any{"type"})
	orderType := firstString(action,
		[]any{"order_type"},
		[]any{"params", "order_type"},
	)
	if orderType == "" {
		orderType = actionType
	}
	regime := firstString(action,
		[]any{"regime"},
		[]any{"decision", "regime"},
		[]any{"output", "regime"},
		[]any{"journey", "story", "regime"},
	)
	liquidity := firstString(action,
		[]any{"execution", "liquidity"},
		[]any{"liquidity"},
		[]any{"decision", "liquidity"},
	)
	if symbol == "" || source == "" || category == "" || side == "" ||
		actionType == "" || orderType == "" {
		return ""
	}
	if regime == "" {
		regime = "any"
	}
	if liquidity == "" {
		liquidity = liquidityClassForOrderType(orderType)
	}

	horizon := edgeForwardReturnHorizon().String()

	return normalizeSetupKey(strings.Join([]string{
		symbol,
		side,
		actionType,
		orderType,
		source,
		category,
		regime,
		liquidity,
		horizon,
	}, "|"))
}

func executionSetupKey(artifact *datura.Artifact, rowIndex int) string {
	if artifact == nil {
		return ""
	}

	for _, path := range [][]any{
		{"data", rowIndex, "setup_key"},
		{"data", rowIndex, "edge_key"},
		{"data", rowIndex, "decision", "setup_key"},
		{"data", rowIndex, "decision", "edge_key"},
		{"setup_key"},
		{"edge_key"},
	} {
		if value := datura.Peek[string](artifact, path...); value != "" {
			return normalizeSetupKey(value)
		}
	}

	return ""
}

func normalizeSetupKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.Join(strings.Fields(key), "_")
	return key
}

type edgeMark struct {
	price float64
	stamp int64
}

func (estimator EdgeEstimator) tickerMarks(symbol string) []edgeMark {
	target := strings.ToUpper(strings.TrimSpace(symbol))
	marks := make([]edgeMark, 0)

	collect := func(prefix []byte) {
		for artifact := range estimator.tree.Seek(prefix) {
			for rowIndex := 0; ; rowIndex++ {
				row, err := market.SymbolFromTicker(artifact, rowIndex)
				if err != nil || row == nil {
					break
				}
				if strings.ToUpper(row.Name) != target || row.Price <= 0 {
					continue
				}
				marks = append(marks, edgeMark{price: row.Price, stamp: artifact.Timestamp()})
			}
		}
	}

	collect([]byte("ticker/" + target + "/"))
	collect([]byte("ticker/"))

	sort.Slice(marks, func(first, second int) bool {
		return marks[first].stamp < marks[second].stamp
	})

	return marks
}

func firstMarkAtOrAfter(marks []edgeMark, stamp int64) (edgeMark, bool) {
	for _, mark := range marks {
		if mark.stamp >= stamp && mark.price > 0 {
			return mark, true
		}
	}

	return edgeMark{}, false
}

func returnBps(entry, exit float64) float64 {
	if entry <= 0 || exit <= 0 {
		return 0
	}

	return (exit - entry) / entry * 10_000
}

func firstFiniteBps(action *datura.Artifact, paths ...[]any) (float64, bool) {
	for _, path := range paths {
		if !artifactPathExists(action, path...) {
			continue
		}

		value := datura.Peek[float64](action, path...)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}

		return value, true
	}

	return 0, false
}

func artifactPathExists(action *datura.Artifact, path ...any) bool {
	if action == nil {
		return false
	}

	for _, region := range []func() ([]byte, error){
		action.Attributes,
		func() ([]byte, error) {
			return action.DecryptPayload(), nil
		},
	} {
		content, err := region()
		if err != nil {
			continue
		}
		content = bytes.TrimSpace(content)
		if len(content) == 0 || (content[0] != '{' && content[0] != '[') {
			continue
		}
		if !json.Valid(content) {
			continue
		}

		node, err := sonic.Get(content, path...)
		if err == nil && node.Exists() {
			return true
		}
	}

	return false
}

func firstPositiveInt(action *datura.Artifact, paths ...[]any) int {
	for _, path := range paths {
		value := datura.Peek[int](action, path...)
		if value > 0 {
			return value
		}
		if asFloat := datura.Peek[float64](action, path...); asFloat > 0 {
			return int(asFloat)
		}
	}

	return 0
}

func firstString(action *datura.Artifact, paths ...[]any) string {
	for _, path := range paths {
		value := datura.Peek[string](action, path...)
		if value != "" {
			return value
		}
	}

	return ""
}
