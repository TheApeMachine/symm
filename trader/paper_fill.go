package trader

import (
	"math/rand"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
)

var paperRejectRand = rand.New(rand.NewSource(1))

/*
paperSimulatedFill resolves one paper entry or exit from quote depth.
*/
func paperSimulatedFill(
	side string,
	notionalEUR float64,
	baseQty float64,
	last, bid, ask float64,
	bidLevels, askLevels []market.BookLevel,
) (fillPrice, filledBase, filledQuote float64, err error) {
	if paperShouldReject() {
		return 0, 0, 0, errOrderRejected
	}

	if side == "buy" && notionalEUR > 0 && len(askLevels) > 0 {
		return paperQuoteFill(askLevels, notionalEUR)
	}

	if side == "sell" && baseQty > 0 && len(bidLevels) > 0 {
		return paperBaseFill(bidLevels, baseQty)
	}

	fillSide := side
	quoteNotional := notionalEUR

	if baseQty > 0 && last > 0 && side == "sell" {
		quoteNotional = baseQty * last
	}

	fill := config.System.SlippageFill(
		last, bid, ask, fillSide, config.System.SlippageBPS,
		quoteNotional, bidLevels, askLevels,
	)

	if fill <= 0 {
		return 0, 0, 0, errInvalidFill
	}

	if side == "buy" {
		filledBase = roundBaseQty(notionalEUR/fill, paperLotDecimals)

		if filledBase <= 0 {
			return 0, 0, 0, errInvalidFill
		}

		filledQuote = spotProceedsEUR(filledBase, fill)

		return fill, filledBase, filledQuote, nil
	}

	filledBase = baseQty
	filledQuote = spotProceedsEUR(filledBase, fill)

	return fill, filledBase, filledQuote, nil
}

func paperQuoteFill(levels []market.BookLevel, quoteNotional float64) (float64, float64, float64, error) {
	result := market.DepthQuoteFill(levels, quoteNotional)

	if result.BaseQty <= 0 || result.VWAP <= 0 {
		return 0, 0, 0, errInsufficientDepth
	}

	coverage := result.QuoteProceeds / quoteNotional

	if !result.Complete && coverage < config.System.PaperMinFillCoverage {
		return 0, 0, 0, errInsufficientDepth
	}

	if result.QuoteProceeds < config.System.MinCostEUR {
		return 0, 0, 0, errInsufficientDepth
	}

	baseQty := roundBaseQty(result.BaseQty, paperLotDecimals)

	if baseQty <= 0 {
		return 0, 0, 0, errInsufficientDepth
	}

	fill := result.VWAP

	if config.System.SlippageBPS > 0 {
		fill += fill * config.System.SlippageBPS / 10000
	}

	proceeds := spotProceedsEUR(baseQty, fill)

	return fill, baseQty, proceeds, nil
}

func paperBaseFill(levels []market.BookLevel, baseQty float64) (float64, float64, float64, error) {
	result := market.DepthBaseFill(levels, baseQty)

	if result.BaseQty <= 0 || result.VWAP <= 0 {
		return 0, 0, 0, errInsufficientDepth
	}

	coverage := result.BaseQty / baseQty

	if !result.Complete && coverage < config.System.PaperMinFillCoverage {
		return 0, 0, 0, errInsufficientDepth
	}

	fill := result.VWAP

	if config.System.SlippageBPS > 0 {
		fill -= fill * config.System.SlippageBPS / 10000
	}

	if fill <= 0 {
		return 0, 0, 0, errInvalidFill
	}

	filledBase := roundBaseQty(result.BaseQty, paperLotDecimals)

	if filledBase <= 0 {
		return 0, 0, 0, errInsufficientDepth
	}

	proceeds := spotProceedsEUR(filledBase, fill)

	return fill, filledBase, proceeds, nil
}

func paperShouldReject() bool {
	rate := config.System.PaperOrderRejectRate

	if rate <= 0 {
		return false
	}

	return paperRejectRand.Float64() < rate
}

var errOrderRejected = errString("order rejected")
var errInsufficientDepth = errString("insufficient book depth")
