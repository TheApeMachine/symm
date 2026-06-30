package trader

import (
	"math"
	"sort"
	"strings"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/market"
)

type EdgeEstimate struct {
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
	minSamples := viper.GetInt("market.story.forward_return_min_samples")
	if minSamples <= 0 {
		minSamples = 30
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

	returns := estimator.realizedAndForwardReturns(symbol)
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
		ExpectedReturnBps: sum / float64(count),
		SampleCount:       count,
		EdgeSource:        "fill_forward_return",
	}, true
}

type edgeFill struct {
	side  string
	price float64
	stamp int64
}

func (estimator EdgeEstimator) realizedAndForwardReturns(symbol string) []float64 {
	fills := estimator.executionFills(symbol)
	if len(fills) == 0 {
		return nil
	}

	marks := estimator.tickerMarks(symbol)
	returns := make([]float64, 0, len(fills))
	openBuys := make([]edgeFill, 0)

	for _, fill := range fills {
		switch fill.side {
		case "buy", "enter":
			openBuys = append(openBuys, fill)
			if mark, ok := firstMarkAfter(marks, fill.stamp); ok {
				returns = append(returns, returnBps(fill.price, mark.price))
			}
		case "sell", "exit":
			if len(openBuys) == 0 {
				continue
			}
			entry := openBuys[0]
			openBuys = openBuys[1:]
			returns = append(returns, returnBps(entry.price, fill.price))
		}
	}

	return returns
}

func (estimator EdgeEstimator) executionFills(symbol string) []edgeFill {
	target := strings.ToUpper(strings.TrimSpace(symbol))
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
				side:  strings.ToLower(datura.Peek[string](artifact, "data", rowIndex, "side")),
				price: price,
				stamp: artifact.Timestamp(),
			})
		}
	}

	sort.Slice(fills, func(first, second int) bool {
		return fills[first].stamp < fills[second].stamp
	})

	return fills
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

func firstMarkAfter(marks []edgeMark, stamp int64) (edgeMark, bool) {
	for _, mark := range marks {
		if mark.stamp > stamp && mark.price > 0 {
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
		value := datura.Peek[float64](action, path...)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		if value != 0 {
			return value, true
		}
	}

	return 0, false
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
