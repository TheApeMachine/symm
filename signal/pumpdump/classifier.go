package pumpdump

import (
	"math"

	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/dist"
)

type ignitionMetrics struct {
	rvol             float64
	precursor        float64
	compression      float64
	rvolDecline      float64
	ignitionFloor    float64
	peerRvol         float64
	peerPrecursor    float64
	bookCompression  float64
	compressionFloor float64
	declineFloor     float64
	volumeDelta      float64
	logReturn        float64
	legAnchorLow     float64
	legAnchorHigh    float64
	exhaustionStamp  float64
	thinBook         float64
}

func classify(metrics ignitionMetrics) []dist.Share {
	lift := math.Max(0, metrics.rvol-1)
	liftRatio := relativeTo(lift, metrics.ignitionFloor)
	precursorRatio := relativeTo(metrics.precursor, metrics.peerPrecursor)
	compressionRatio := relativeTo(metrics.bookCompression, metrics.compressionFloor)
	declineRatio := relativeTo(metrics.rvolDecline, metrics.declineFloor)

	// Thin-book traps (TITCOIN) collapse toward no real ignition: a hollow,
	// wide-spread book on bottom-of-cross-section dollar volume is not a pump.
	structure := math.Max(0, 1-metrics.thinBook)

	ignitionMass := structure * liftRatio * precursorRatio / (1 + compressionRatio + declineRatio)
	compressionMass := structure * coiledMass(metrics, liftRatio, precursorRatio, compressionRatio)
	trendMass := trendMass(metrics, liftRatio, precursorRatio, declineRatio)
	steadyMass := steadyMass(metrics, liftRatio, precursorRatio, declineRatio)
	exhaustionMass := declineRatio / (1 + precursorRatio + compressionRatio)

	return []dist.Share{
		{Key: "ignition", Category: logic.CategoryVerticalIgnition, Mass: ignitionMass},
		{Key: "compression", Category: logic.CategoryCoiledCompression, Mass: compressionMass},
		{Key: "trend", Category: logic.CategoryOrganicTrend, Mass: trendMass},
		{Key: "steady", Category: logic.CategoryOrganicTrend, Mass: steadyMass},
		{Key: "exhaustion", Category: logic.CategoryFadedExhaustion, Mass: exhaustionMass},
	}
}

func coiledMass(
	metrics ignitionMetrics,
	liftRatio, precursorRatio, compressionRatio float64,
) float64 {
	if metrics.bookCompression <= 0 {
		return 0
	}

	if metrics.peerPrecursor > 0 && precursorRatio > 1 {
		return 0
	}

	moderateLift := metrics.rvol / (1 + liftRatio*liftRatio)
	quietPrecursor := 1 / (1 + precursorRatio)

	return compressionRatio * moderateLift * quietPrecursor
}

func trendMass(
	metrics ignitionMetrics,
	liftRatio, precursorRatio, declineRatio float64,
) float64 {
	if metrics.peerPrecursor <= 0 || metrics.peerRvol <= 0 {
		return 0
	}

	if metrics.peerRvol > 1 {
		return 0
	}

	return precursorRatio / (1 + precursorRatio + liftRatio + declineRatio)
}

func steadyMass(
	metrics ignitionMetrics,
	liftRatio, precursorRatio, declineRatio float64,
) float64 {
	if metrics.peerRvol <= 0 || metrics.peerRvol > 1 {
		return 0
	}

	return 1 / (1 + liftRatio + precursorRatio + declineRatio)
}

func relativeTo(sample, baseline float64) float64 {
	if sample <= 0 {
		return 0
	}

	if baseline <= 0 {
		return sample
	}

	return sample / baseline
}
