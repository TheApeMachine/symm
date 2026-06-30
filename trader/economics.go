package trader

import (
	"math"
	"strings"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

type executionEconomics struct {
	takerFeeBps float64
	makerFeeBps float64
	slippageBps float64
}

type economicPrice struct {
	edgeKey           string
	edge              float64
	hurdle            float64
	priced            bool
	liquidity         string
	expectedReturnBps float64
	frictionBps       float64
	netEdgeBps        float64
	sampleCount       int
	calibrationReady  bool
	edgeSource        string
}

func newExecutionEconomics() executionEconomics {
	return executionEconomics{
		takerFeeBps: viper.GetFloat64("trading.paper.taker_fee_bps"),
		makerFeeBps: viper.GetFloat64("trading.paper.maker_fee_bps"),
		slippageBps: viper.GetFloat64("trading.paper.slippage_bps"),
	}
}

func (economics executionEconomics) price(
	action *datura.Artifact,
	edge float64,
	hasEdge bool,
	tree *dmt.Tree,
) economicPrice {
	orderType := strings.ToLower(strings.TrimSpace(datura.Peek[string](action, "type")))
	if orderType == "" {
		orderType = "market"
	}

	hurdle, liquidity := economics.roundTripHurdle(orderType)
	out := economicPrice{
		hurdle:    hurdle,
		liquidity: liquidity,
	}
	estimate := newEdgeEstimator(economics, tree).Estimate(action, edge, hasEdge)
	out.edgeKey = estimate.EdgeKey
	out.expectedReturnBps = estimate.ExpectedReturnBps
	out.frictionBps = estimate.FrictionBps
	out.netEdgeBps = estimate.NetEdgeBps
	out.sampleCount = estimate.SampleCount
	out.calibrationReady = estimate.CalibrationReady
	out.edgeSource = estimate.EdgeSource
	out.edge = math.Max(0, estimate.ExpectedReturnBps/10_000)
	out.hurdle = math.Max(0, estimate.FrictionBps/10_000)
	out.priced = estimate.CalibrationReady

	return out
}

func (economics executionEconomics) roundTripHurdle(orderType string) (float64, string) {
	entryFee := economics.takerFeeBps
	liquidity := liquidityClassForOrderType(orderType)

	if liquidity == "maker" {
		entryFee = economics.makerFeeBps
	}

	exitFee := economics.takerFeeBps
	roundTripBps := entryFee + exitFee + 2*economics.slippageBps
	if roundTripBps < 0 || math.IsNaN(roundTripBps) || math.IsInf(roundTripBps, 0) {
		roundTripBps = 0
	}

	return roundTripBps / 10_000, liquidity
}

func liquidityClassForOrderType(orderType string) string {
	if strings.EqualFold(strings.TrimSpace(orderType), "limit") {
		return "maker"
	}

	return "taker"
}
