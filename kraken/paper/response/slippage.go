package response

import (
	"fmt"
	"math"

	"github.com/theapemachine/datura"
)

type bookDepthLevel struct {
	price float64
	qty   float64
}

/*
slippageFill walks cached L2 depth for qty, otherwise half-spread on last.
*/
func (fillSimulator *FillSimulator) slippageFill(
	quote *datura.Artifact,
	side string,
	qty float64,
) (*datura.Artifact, error) {
	if qty <= 0 {
		return nil, fmt.Errorf("paper slippage: qty must be positive")
	}

	reference, err := fillSimulator.slippageReference(quote)

	if err != nil {
		return nil, err
	}

	levels := fillSimulator.depthLevels(quote, side)

	if len(levels) == 0 {
		return fillSimulator.halfSpreadFill(quote, side, reference), nil
	}

	return fillSimulator.depthSlippageFill(quote, side, qty, reference, levels)
}

func (fillSimulator *FillSimulator) slippageReference(quote *datura.Artifact) (float64, error) {
	reference := datura.Peek[float64](quote, "last")
	bid := datura.Peek[float64](quote, "bid")
	ask := datura.Peek[float64](quote, "ask")

	if reference <= 0 && bid > 0 && ask > 0 {
		reference = (bid + ask) / 2
	}

	if reference <= 0 {
		scope, _ := quote.Scope()

		return 0, fmt.Errorf("paper slippage: missing reference price for %s", scope)
	}

	return reference, nil
}

func (fillSimulator *FillSimulator) depthLevels(
	quote *datura.Artifact,
	side string,
) []bookDepthLevel {
	if quote == nil {
		return nil
	}

	levelKey := "asks"

	if side == "sell" {
		levelKey = "bids"
	}

	levels := make([]bookDepthLevel, 0, 16)

	for index := 0; index < 256; index++ {
		price := fillSimulator.payloadNumber(quote, levelKey, index, "price")
		qty := fillSimulator.payloadNumber(quote, levelKey, index, "qty")

		if price <= 0 {
			price = fillSimulator.payloadNumber(quote, levelKey, index, 0)
			qty = fillSimulator.payloadNumber(quote, levelKey, index, 1)
		}

		if price <= 0 {
			break
		}

		if qty <= 0 {
			qty = fillSimulator.payloadNumber(quote, levelKey, index, "quantity")
		}

		if qty <= 0 {
			continue
		}

		levels = append(levels, bookDepthLevel{price: price, qty: qty})
	}

	return levels
}

func (fillSimulator *FillSimulator) depthSlippageFill(
	quote *datura.Artifact,
	side string,
	qty float64,
	reference float64,
	levels []bookDepthLevel,
) (*datura.Artifact, error) {
	filledQty, cost := fillSimulator.walkDepthLevels(levels, qty)

	if filledQty <= 0 {
		return fillSimulator.halfSpreadFill(quote, side, reference), nil
	}

	avgPrice := cost / filledQty
	bestPrice := levels[0].price
	slippageBps := fillSimulator.slippageBpsFromBest(side, bestPrice, avgPrice)

	if filledQty < qty {
		return fillSimulator.partialDepthFill(
			quote, side, qty, reference, bestPrice, filledQty, avgPrice, slippageBps,
		), nil
	}

	return fillSimulator.fillArtifact(avgPrice, slippageBps, filledQty/qty), nil
}

func (fillSimulator *FillSimulator) walkDepthLevels(
	levels []bookDepthLevel,
	qty float64,
) (filledQty, cost float64) {
	for _, level := range levels {
		if level.price <= 0 || level.qty <= 0 {
			continue
		}

		remaining := qty - filledQty
		takeQty := level.qty

		if takeQty > remaining {
			takeQty = remaining
		}

		cost += takeQty * level.price
		filledQty += takeQty

		if filledQty >= qty {
			break
		}
	}

	return filledQty, cost
}

func (fillSimulator *FillSimulator) partialDepthFill(
	quote *datura.Artifact,
	side string,
	qty float64,
	reference float64,
	bestPrice float64,
	filledQty float64,
	avgPrice float64,
	slippageBps float64,
) *datura.Artifact {
	remainder := qty - filledQty
	fallback := fillSimulator.halfSpreadFill(quote, side, reference)
	fallbackPrice := datura.Peek[float64](fallback, "price")
	fallback.Release()

	blended := (avgPrice*filledQty + fallbackPrice*remainder) / qty
	slippageBps = fillSimulator.slippageBpsFromBest(side, bestPrice, blended)

	return fillSimulator.fillArtifact(blended, slippageBps, filledQty/qty)
}

func (fillSimulator *FillSimulator) applyExtraSlippageBps(
	price float64,
	side string,
	bps float64,
) float64 {
	if price <= 0 || bps <= 0 {
		return price
	}

	factor := bps / 10_000

	if side == "buy" {
		return price * (1 + factor)
	}

	return price * (1 - factor)
}

func (fillSimulator *FillSimulator) halfSpreadFill(
	quote *datura.Artifact,
	side string,
	reference float64,
) *datura.Artifact {
	bid := datura.Peek[float64](quote, "bid")
	ask := datura.Peek[float64](quote, "ask")
	mid := reference
	spreadBps := 0.0

	if bid > 0 && ask > 0 && ask >= bid {
		mid = (bid + ask) / 2
		spreadBps = (ask - bid) / mid * 10_000 / 2
	}

	price := mid

	if side == "buy" {
		price = mid * (1 + spreadBps/10_000)
	}

	if side == "sell" {
		price = mid * (1 - spreadBps/10_000)
	}

	return fillSimulator.fillArtifact(price, spreadBps, 0)
}

func (fillSimulator *FillSimulator) slippageBpsFromBest(
	side string,
	bestPrice, avgPrice float64,
) float64 {
	if bestPrice <= 0 || avgPrice <= 0 {
		return 0
	}

	if side == "buy" {
		return math.Max(0, (avgPrice-bestPrice)/bestPrice*10_000)
	}

	return math.Max(0, (bestPrice-avgPrice)/bestPrice*10_000)
}

func (fillSimulator *FillSimulator) midSpreadBps(quote *datura.Artifact) float64 {
	bid := datura.Peek[float64](quote, "bid")
	ask := datura.Peek[float64](quote, "ask")

	if bid <= 0 || ask <= 0 || ask < bid {
		return 0
	}

	mid := (bid + ask) / 2

	if mid <= 0 {
		return 0
	}

	return (ask - bid) / mid * 10_000 / 2
}

func (fillSimulator *FillSimulator) fillArtifact(
	price, slippageBps, depthCoverage float64,
) *datura.Artifact {
	fill := datura.Acquire("paper", datura.Artifact_Type_json)
	fill.WithRole("fill")
	fill.Poke(price, "price")
	fill.Poke(slippageBps, "slippage_bps")
	fill.Poke(depthCoverage, "depth_coverage")

	return fill
}
