package strategy

import (
	"context"
	"fmt"
	"math"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ScanVersion bumps when replay semantics change enough to invalidate stored
scan stamps and working drafts.
*/
const ScanVersion = 1

type detectorState int

const (
	stateInit detectorState = iota
	stateUndecided
	stateUp
	stateDown
)

type tradePoint struct {
	tick   int64
	seqIdx int64
	price  *decimal.Decimal
	priceF float64
	trade  *data.Value
}

type episode struct {
	class  string
	start  tradePoint // B: chronological start / ignition
	end    tradePoint // C: chronological end / exhaustion
	lowP   float64
	highP  float64
	trades int64
}

/*
Detector scans market tape from beginning to end and emits long, coherent
excursions ("up", "down", "chop", "flat") for each symbol/epoch tape.
*/
type Detector struct {
	*runtime.System
	price *broker.Price
	book  *broker.Snapshot

	// Structural & regime-coalescing parameters
	retraceRatio           float64
	maxContinuationRetrace float64
	boxOverlapRatio        float64
	minFlatTrades          int64
	maxFlatGlitchTrades    int64
	minEpisodeTrades       int64
	minStagnationTrades    int64
	stagnationMultiplier   int64

	// Streaming state per (epoch, label)
	epoch      int64
	label      string
	state      detectorState
	emaBounce  float64
	prevTrade  *tradePoint
	prevDelta  float64
	lastResult *data.Value

	// Undecided / chop / flat tracking
	segStart      tradePoint
	runLow        tradePoint
	runHigh       tradePoint
	flatStart     tradePoint
	flatLast      tradePoint
	flatMin       float64
	flatMax       float64
	flatCount     int64
	flatGlitchCnt int64

	// Active directional leg (stateUp / stateDown)
	legB         tradePoint
	legC         tradePoint
	legExtreme   tradePoint
	legTrades    int64
	tradesSinceC int64

	// Lookahead buffer for structural coalescing before Flush
	pending []episode
	emitted []*data.Value
	trace   *decisionTrace
}

/*
NewDetector creates a new detector and stores the tee used to publish
detected excursions. Every excursion class is defined against round-trip
friction, so the price system is a hard dependency.
*/
func NewDetector(
	ctx context.Context,
	price *broker.Price,
) *Detector {
	detector := &Detector{
		System:                 runtime.NewSystem(ctx, "detector"),
		price:                  price,
		retraceRatio:           0.32, // Pullback must retrace >= 32% of |C - B| (and clear friction) to end a leg
		maxContinuationRetrace: 0.58, // Higher-low dips < 58% merge into one macro leg when C is broken
		boxOverlapRatio:        0.68, // Alternating swings overlapping >= 68% coalesce into "chop"
		minFlatTrades:          24,   // Minimum trades within flatBand to emit an independent "flat" episode
		maxFlatGlitchTrades:    2,    // Ignore 1-2 trade stray wicks inside a long flat stretch
		minEpisodeTrades:       8,    // Absorb micro-fragments shorter than 8 trades into adjacent episodes
		minStagnationTrades:    80,   // Minimum trades after C before checking horizontal time-exhaustion
		stagnationMultiplier:   3,    // Exhaust leg into chop/flat if sideways trades after C >= 3x impulse trades
		state:                  stateInit,
		pending:                make([]episode, 0, 8),
	}

	if detector.price == nil || detector.price.ReferenceCash() == nil {
		detector.Transition(runtime.ERROR)
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[detector] price system with reference cash required",
			nil,
		))
		return detector
	}

	errnie.Info(fmt.Sprintf(
		"[detector] excursion detector initialized with friction cash budget %s",
		detector.price.ReferenceCash().String(),
	))

	return detector
}

/*
SetRetrace raises the pullback a leg will absorb, up to 90% of the move.
A committed detection that kept a deeper dip than the current ratio is how
the next scan learns that dip was still the same swing.
*/
func (detector *Detector) SetRetrace(ratio float64) {
	if ratio <= detector.retraceRatio || ratio >= 0.9 {
		return
	}

	detector.retraceRatio = ratio

	if ratio > detector.maxContinuationRetrace {
		detector.maxContinuationRetrace = ratio
	}
}

/*
Step the replayed tape and hunt for excursions.
*/
func (detector *Detector) Step(
	value *data.Value,
) *data.Value {
	if value == nil || detector.price == nil {
		return nil
	}

	detector.lastResult = nil

	if value.Origin == "runtime:join" {
		for _, peer := range data.Layer[*data.Value](value) {
			if peer == nil || peer.Origin != "spot:trade" {
				continue
			}

			detector.processTrade(peer)
		}
	} else if value.Origin == "spot:trade" || value.Origin == "kraken" {
		detector.processTrade(value)
	}

	return detector.lastResult
}

func (detector *Detector) processTrade(peer *data.Value) {
	if peer == nil {
		return
	}

	var priceF float64

	if len(peer.Layers) > 0 {
		priceF = data.Attribute[float64](peer.Layers[0], "data", 0, "price")

		if priceF <= 0 {
			priceF = data.Attribute[float64](peer.Layers[0], "price")
		}
	}

	if priceF <= 0 || math.IsNaN(priceF) || math.IsInf(priceF, 0) {
		return
	}

	detector.stepTradePoint(
		int64(peer.Timestamp),
		peer.Scope,
		tradePoint{
			tick:   int64(peer.Tick),
			seqIdx: int64(peer.SeqIdx),
			price:  decimal.NewFromFloat64(priceF),
			priceF: priceF,
			trade:  peer,
		},
	)
}

/*
stepTradePoint runs one print through the excursion state machine. Live
spot:trade frames reach it through processTrade and sonic path reads; replay
steps stored tape prints directly without synthesizing JSON layers.
*/
func (detector *Detector) stepTradePoint(epoch int64, label string, pt tradePoint) {
	if pt.priceF <= 0 || math.IsNaN(pt.priceF) || math.IsInf(pt.priceF, 0) {
		return
	}

	if detector.epoch == 0 || detector.label == "" ||
		detector.epoch != epoch || detector.label != label {
		detector.flushConfirmedOnReset()
		detector.resetTape(epoch, label)
	}

	detector.updateMicrostructure(pt)
	detector.sample(pt)

	if detector.state == stateInit {
		detector.initFromPoint(pt)
		return
	}

	switch detector.state {
	case stateUndecided:
		detector.stepUndecided(pt)
	case stateUp:
		detector.stepUp(pt)
	case stateDown:
		detector.stepDown(pt)
	}
}

/*
updateMicrostructure tracks single-trade direction-flip bounces to measure the
effective bid-ask bounce floor of the book (e.g. ALTHEA 10% spread, AMI 36% spread).
*/
func (detector *Detector) updateMicrostructure(pt tradePoint) {
	if detector.prevTrade != nil && detector.prevTrade.priceF > 0 {
		delta := (pt.priceF - detector.prevTrade.priceF) / detector.prevTrade.priceF
		absDelta := math.Abs(delta)

		if absDelta > 0 {
			if detector.prevDelta != 0 && (delta*detector.prevDelta) < 0 {
				capped := math.Min(absDelta, 0.25)
				if detector.emaBounce == 0 {
					detector.emaBounce = capped
				} else {
					detector.emaBounce = 0.92*detector.emaBounce + 0.08*capped
				}
			} else {
				detector.emaBounce *= 0.992
			}
			detector.prevDelta = delta
		}
	}

	cp := pt
	detector.prevTrade = &cp
}

/*
roundTripFeeRate queries broker.Price for the symbol's authoritative taker fee
and returns the exact round-trip break-even fraction: (1 + f)/(1 - f) - 1.
*/
func (detector *Detector) roundTripFeeRate() float64 {
	if feeRate, ok := detector.price.FeeRate(detector.label); ok && feeRate > 0 && feeRate < 1 {
		return (1.0+feeRate)/(1.0-feeRate) - 1.0
	}

	return 0.008 // 80 bps fallback if symbol fee is not loaded
}

/*
effectiveDeadband combines the venue round-trip taker fee with the rolling
bid-ask bounce floor so spread bounce never triggers false directional legs.
*/
func (detector *Detector) effectiveDeadband() float64 {
	feeFloor := 2.0 * detector.roundTripFeeRate()
	spreadFloor := 2.2 * detector.emaBounce

	if detector.book != nil && len(detector.book.Bids) > 0 && len(detector.book.Asks) > 0 {
		bid := detector.book.Bids[0].Price
		ask := detector.book.Asks[0].Price

		if bid != nil && ask != nil && bid.Sign() > 0 && ask.Cmp(bid) > 0 {
			spread := (ask.Float64() - bid.Float64()) / bid.Float64()

			if spread > spreadFloor {
				spreadFloor = spread
			}
		}
	}

	return math.Max(feeFloor, spreadFloor)
}

/*
clearsEconomicFriction verifies that a move from lowPrice to highPrice clears
both the dynamic microstructure deadband and broker.Price's RoundTrip fee math.
*/
func (detector *Detector) clearsEconomicFriction(lowPrice, highPrice *decimal.Decimal) bool {
	if lowPrice == nil || highPrice == nil || lowPrice.Sign() <= 0 || highPrice.Cmp(lowPrice) <= 0 {
		return false
	}

	rel := (highPrice.Float64() - lowPrice.Float64()) / lowPrice.Float64()
	if rel < detector.effectiveDeadband() {
		return false
	}

	if pnl, _, err := detector.price.RoundTrip(detector.label, lowPrice, highPrice); err == nil && pnl != nil {
		return pnl.Sign() > 0
	}

	// Fallback if Normalizer/Instrument minimums are not seeded in unit tests:
	entryWithFee := detector.price.WithFee(detector.label, lowPrice, broker.BUY)
	exitWithFee := detector.price.WithFee(detector.label, highPrice, broker.SELL)

	if entryWithFee != nil && exitWithFee != nil {
		return exitWithFee.Cmp(entryWithFee) > 0
	}

	return true
}

func (detector *Detector) flatBand() float64 {
	return 0.25 * detector.roundTripFeeRate()
}

func (detector *Detector) resetTape(epoch int64, label string) {
	detector.epoch = epoch
	detector.label = label
	detector.state = stateInit
	detector.emaBounce = 0
	detector.prevTrade = nil
	detector.prevDelta = 0
	detector.pending = detector.pending[:0]
}

func (detector *Detector) initFromPoint(pt tradePoint) {
	detector.segStart = pt
	detector.runLow = pt
	detector.runHigh = pt
	detector.resetFlatTracker(pt)
	detector.state = stateUndecided
	detector.note("undecided", pt, "segment start")
}

func (detector *Detector) resetFlatTracker(pt tradePoint) {
	detector.flatStart = pt
	detector.flatLast = pt
	detector.flatMin = pt.priceF
	detector.flatMax = pt.priceF
	detector.flatCount = 1
	detector.flatGlitchCnt = 0
}

func (detector *Detector) stepUndecided(pt tradePoint) {
	fBand := detector.flatBand()

	// 1. Track sustained flat regimes (subordinated until minFlatTrades is reached).
	candMin := math.Min(detector.flatMin, pt.priceF)
	candMax := math.Max(detector.flatMax, pt.priceF)
	inFlat := candMin > 0 && ((candMax-candMin)/candMin) <= fBand

	if inFlat {
		detector.flatMin = candMin
		detector.flatMax = candMax
		detector.flatLast = pt
		detector.flatCount++
		detector.flatGlitchCnt = 0
	} else {
		detector.flatGlitchCnt++

		if detector.flatGlitchCnt > detector.maxFlatGlitchTrades {
			if detector.flatCount >= detector.minFlatTrades &&
				detector.flatLast.seqIdx > detector.flatStart.seqIdx {

				if detector.flatStart.seqIdx > detector.segStart.seqIdx {
					detector.enqueueEpisode(episode{
						class:  "chop",
						start:  detector.segStart,
						end:    detector.flatStart,
						lowP:   math.Min(detector.segStart.priceF, detector.flatStart.priceF),
						highP:  math.Max(detector.segStart.priceF, detector.flatStart.priceF),
						trades: detector.flatStart.seqIdx - detector.segStart.seqIdx,
					})
				}

				detector.enqueueEpisode(episode{
					class:  "flat",
					start:  detector.flatStart,
					end:    detector.flatLast,
					lowP:   detector.flatMin,
					highP:  detector.flatMax,
					trades: detector.flatCount,
				})

				detector.segStart = detector.flatLast
				detector.runLow = detector.flatLast
				detector.runHigh = detector.flatLast
			}

			detector.resetFlatTracker(pt)
		}
	}

	// 2. Track running extremes of the undecided window.
	if pt.price.Cmp(detector.runLow.price) <= 0 {
		detector.runLow = pt
	}

	if pt.price.Cmp(detector.runHigh.price) >= 0 {
		detector.runHigh = pt
	}

	// 3. Check if a breakout from runLow or runHigh clears economic friction.
	upClears := pt.seqIdx > detector.runLow.seqIdx &&
		detector.clearsEconomicFriction(detector.runLow.price, pt.price)

	downClears := pt.seqIdx > detector.runHigh.seqIdx &&
		detector.clearsEconomicFriction(pt.price, detector.runHigh.price)

	if upClears {
		if detector.runLow.seqIdx > detector.segStart.seqIdx {
			detector.enqueueEpisode(episode{
				class:  "chop",
				start:  detector.segStart,
				end:    detector.runLow,
				lowP:   detector.runLow.priceF,
				highP:  detector.runHigh.priceF,
				trades: detector.runLow.seqIdx - detector.segStart.seqIdx,
			})
		}

		detector.enterUp(detector.runLow, pt)
		return
	}

	if downClears {
		if detector.runHigh.seqIdx > detector.segStart.seqIdx {
			detector.enqueueEpisode(episode{
				class:  "chop",
				start:  detector.segStart,
				end:    detector.runHigh,
				lowP:   detector.runLow.priceF,
				highP:  detector.runHigh.priceF,
				trades: detector.runHigh.seqIdx - detector.segStart.seqIdx,
			})
		}

		detector.enterDown(detector.runHigh, pt)
	}
}

func (detector *Detector) enterUp(b, c tradePoint) {
	detector.note("enter-up", c, fmt.Sprintf(
		"B tick %d seq %d price %.6g -> C tick %d seq %d price %.6g",
		b.tick, b.seqIdx, b.priceF, c.tick, c.seqIdx, c.priceF,
	))
	detector.state = stateUp
	detector.legB = b
	detector.legC = c
	detector.legExtreme = c
	detector.legTrades = c.seqIdx - b.seqIdx + 1
	detector.tradesSinceC = 0
	detector.tryAdoptPriorUp()
}

func (detector *Detector) enterDown(b, c tradePoint) {
	detector.note("enter-down", c, fmt.Sprintf(
		"B tick %d seq %d price %.6g -> C tick %d seq %d price %.6g",
		b.tick, b.seqIdx, b.priceF, c.tick, c.seqIdx, c.priceF,
	))
	detector.state = stateDown
	detector.legB = b
	detector.legC = c
	detector.legExtreme = c
	detector.legTrades = c.seqIdx - b.seqIdx + 1
	detector.tradesSinceC = 0
	detector.tryAdoptPriorDown()
}

func (detector *Detector) stepUp(pt tradePoint) {
	detector.legTrades++

	if pt.price.Cmp(detector.legC.price) >= 0 {
		detector.note("extend-c", pt, fmt.Sprintf("up C tick %d seq %d", pt.tick, pt.seqIdx))
		detector.legC = pt
		detector.legExtreme = pt
		detector.tradesSinceC = 0
		detector.tryAdoptPriorUp()
		return
	}

	detector.tradesSinceC++

	if pt.price.Cmp(detector.legExtreme.price) < 0 {
		detector.legExtreme = pt
	}

	legRange := detector.legC.priceF - detector.legB.priceF
	retraceAbs := detector.legC.priceF - pt.priceF

	var retraceOfLeg float64

	if legRange > 0 {
		retraceOfLeg = retraceAbs / legRange
	}

	brokeOrigin := pt.price.Cmp(detector.legB.price) < 0
	retraceClearsFee := detector.clearsEconomicFriction(pt.price, detector.legC.price)
	clearedProportional := retraceClearsFee && retraceOfLeg >= detector.retraceRatio

	if brokeOrigin || clearedProportional {
		detector.note("retrace", pt, fmt.Sprintf(
			"up brokeOrigin=%t proportional=%t retrace=%.3f feeClear=%t B tick %d seq %d C tick %d seq %d",
			brokeOrigin, clearedProportional, retraceOfLeg, retraceClearsFee,
			detector.legB.tick, detector.legB.seqIdx, detector.legC.tick, detector.legC.seqIdx,
		))
		detector.completeUpLeg()

		if retraceClearsFee && pt.seqIdx > detector.legC.seqIdx {
			detector.enterDown(detector.legC, pt)
		} else {
			detector.initFromPoint(detector.legC)
			detector.stepUndecided(pt)
		}

		return
	}

	// Horizontal time-exhaustion: if price stalls below C for >= 3x the impulse duration,
	// end the up leg at C and transition the post-C sideways box into chop/flat.
	impulseTrades := max(1, detector.legC.seqIdx-detector.legB.seqIdx)
	stagnationLimit := max(detector.minStagnationTrades, impulseTrades*detector.stagnationMultiplier)

	if detector.tradesSinceC >= stagnationLimit {
		detector.note("stagnation", pt, fmt.Sprintf(
			"up tradesSinceC=%d limit=%d B tick %d C tick %d",
			detector.tradesSinceC, stagnationLimit, detector.legB.tick, detector.legC.tick,
		))
		detector.completeUpLeg()
		detector.initFromPoint(detector.legC)
		detector.stepUndecided(pt)
	}
}

func (detector *Detector) stepDown(pt tradePoint) {
	detector.legTrades++

	if pt.price.Cmp(detector.legC.price) <= 0 {
		detector.note("extend-c", pt, fmt.Sprintf("down C tick %d seq %d", pt.tick, pt.seqIdx))
		detector.legC = pt
		detector.legExtreme = pt
		detector.tradesSinceC = 0
		detector.tryAdoptPriorDown()
		return
	}

	detector.tradesSinceC++

	if pt.price.Cmp(detector.legExtreme.price) > 0 {
		detector.legExtreme = pt
	}

	legRange := detector.legB.priceF - detector.legC.priceF
	bounceAbs := pt.priceF - detector.legC.priceF

	var retraceOfLeg float64

	if legRange > 0 {
		retraceOfLeg = bounceAbs / legRange
	}

	brokeOrigin := pt.price.Cmp(detector.legB.price) > 0
	bounceClearsFee := detector.clearsEconomicFriction(detector.legC.price, pt.price)
	clearedProportional := bounceClearsFee && retraceOfLeg >= detector.retraceRatio

	if brokeOrigin || clearedProportional {
		detector.note("retrace", pt, fmt.Sprintf(
			"down brokeOrigin=%t proportional=%t retrace=%.3f feeClear=%t B tick %d seq %d C tick %d seq %d",
			brokeOrigin, clearedProportional, retraceOfLeg, bounceClearsFee,
			detector.legB.tick, detector.legB.seqIdx, detector.legC.tick, detector.legC.seqIdx,
		))
		detector.completeDownLeg()

		if bounceClearsFee && pt.seqIdx > detector.legC.seqIdx {
			detector.enterUp(detector.legC, pt)
		} else {
			detector.initFromPoint(detector.legC)
			detector.stepUndecided(pt)
		}

		return
	}

	impulseTrades := max(1, detector.legC.seqIdx-detector.legB.seqIdx)
	stagnationLimit := max(detector.minStagnationTrades, impulseTrades*detector.stagnationMultiplier)

	if detector.tradesSinceC >= stagnationLimit {
		detector.note("stagnation", pt, fmt.Sprintf(
			"down tradesSinceC=%d limit=%d B tick %d C tick %d",
			detector.tradesSinceC, stagnationLimit, detector.legB.tick, detector.legC.tick,
		))
		detector.completeDownLeg()
		detector.initFromPoint(detector.legC)
		detector.stepUndecided(pt)
	}
}

func (detector *Detector) completeUpLeg() {
	trades := detector.legC.seqIdx - detector.legB.seqIdx
	class := "up"

	if trades < 1 || !detector.clearsEconomicFriction(detector.legB.price, detector.legC.price) {
		class = "chop"
	}

	detector.note("complete", detector.legC, fmt.Sprintf(
		"up class=%s tickSpan=%d seqSpan=%d B tick %d seq %d C tick %d seq %d",
		class, detector.legC.tick-detector.legB.tick, trades,
		detector.legB.tick, detector.legB.seqIdx, detector.legC.tick, detector.legC.seqIdx,
	))
	detector.enqueueEpisode(episode{
		class:  class,
		start:  detector.legB,
		end:    detector.legC,
		lowP:   detector.legB.priceF,
		highP:  detector.legC.priceF,
		trades: trades,
	})
}

func (detector *Detector) completeDownLeg() {
	trades := detector.legC.seqIdx - detector.legB.seqIdx
	class := "down"

	if trades < 1 || !detector.clearsEconomicFriction(detector.legC.price, detector.legB.price) {
		class = "chop"
	}

	detector.note("complete", detector.legC, fmt.Sprintf(
		"down class=%s tickSpan=%d seqSpan=%d B tick %d seq %d C tick %d seq %d",
		class, detector.legC.tick-detector.legB.tick, trades,
		detector.legB.tick, detector.legB.seqIdx, detector.legC.tick, detector.legC.seqIdx,
	))
	detector.enqueueEpisode(episode{
		class:  class,
		start:  detector.legB,
		end:    detector.legC,
		lowP:   detector.legC.priceF,
		highP:  detector.legB.priceF,
		trades: trades,
	})
}

/*
tryAdoptPriorUp merges a staircase uptrend (up1 -> shallow dip -> up2) into one
macro up leg as soon as the active leg breaks above up1's peak C1, provided the
intervening pullback held a higher low above B1 and did not stagnate into a
long sideways regime.
*/
func (detector *Detector) tryAdoptPriorUp() {
	n := len(detector.pending)

	if n == 0 {
		return
	}

	// Case 1: pending ends directly with an "up" episode.
	if detector.pending[n-1].class == "up" {
		prior := detector.pending[n-1]
		if detector.legC.price.Cmp(prior.end.price) >= 0 &&
			detector.legB.price.Cmp(prior.start.price) >= 0 {
			detector.legB = prior.start
			detector.pending = detector.pending[:n-1]
		}
		return
	}

	// Case 2: pending ends with [..., priorUp, dip] where dip is a shallow pullback.
	if n >= 2 && detector.pending[n-2].class == "up" {
		priorUp := detector.pending[n-2]
		dip := detector.pending[n-1]

		priorRange := priorUp.end.priceF - priorUp.start.priceF
		dipDepth := priorUp.end.priceF - detector.legB.priceF

		if priorRange > 0 &&
			detector.legC.price.Cmp(priorUp.end.price) >= 0 &&
			detector.legB.price.Cmp(priorUp.start.price) > 0 &&
			(dipDepth/priorRange) <= detector.maxContinuationRetrace &&
			dip.trades <= max(detector.minStagnationTrades, priorUp.trades*2) {
			detector.legB = priorUp.start
			detector.pending = detector.pending[:n-2]
		}
	}
}

/*
tryAdoptPriorDown mirrors tryAdoptPriorUp for staircase downtrends.
*/
func (detector *Detector) tryAdoptPriorDown() {
	n := len(detector.pending)

	if n == 0 {
		return
	}

	if detector.pending[n-1].class == "down" {
		prior := detector.pending[n-1]

		if detector.legC.price.Cmp(prior.end.price) <= 0 &&
			detector.legB.price.Cmp(prior.start.price) <= 0 {
			detector.legB = prior.start
			detector.pending = detector.pending[:n-1]
		}

		return
	}

	if n >= 2 && detector.pending[n-2].class == "down" {
		priorDown := detector.pending[n-2]
		bounce := detector.pending[n-1]

		priorRange := priorDown.start.priceF - priorDown.end.priceF
		bounceHeight := detector.legB.priceF - priorDown.end.priceF

		if priorRange > 0 &&
			detector.legC.price.Cmp(priorDown.end.price) <= 0 &&
			detector.legB.price.Cmp(priorDown.start.price) < 0 &&
			(bounceHeight/priorRange) <= detector.maxContinuationRetrace &&
			bounce.trades <= max(detector.minStagnationTrades, priorDown.trades*2) {
			detector.legB = priorDown.start
			detector.pending = detector.pending[:n-2]
		}
	}
}

/*
enqueueEpisode coalesces adjacent identical regimes, absorbs micro-slivers, and
demotes self-canceling box whip-saws into a single contiguous "chop" stretch.
*/
func (detector *Detector) enqueueEpisode(ep episode) {
	if ep.end.seqIdx <= ep.start.seqIdx {
		detector.note("drop", ep.end, fmt.Sprintf(
			"class=%s end seq %d is not after start seq %d",
			ep.class, ep.end.seqIdx, ep.start.seqIdx,
		))
		return
	}

	// 1. Absorb micro-slivers into the preceding episode if one exists.
	if ep.trades < detector.minEpisodeTrades && len(detector.pending) > 0 {
		detector.note("absorb", ep.end, fmt.Sprintf(
			"class=%s trades=%d startTick=%d endTick=%d startSeq=%d endSeq=%d",
			ep.class, ep.trades, ep.start.tick, ep.end.tick, ep.start.seqIdx, ep.end.seqIdx,
		))
		last := &detector.pending[len(detector.pending)-1]
		last.end = ep.end
		last.lowP = math.Min(last.lowP, ep.lowP)
		last.highP = math.Max(last.highP, ep.highP)
		last.trades = last.end.seqIdx - last.start.seqIdx
		return
	}

	// 2. Merge adjacent episodes of the same class.
	if len(detector.pending) > 0 && detector.pending[len(detector.pending)-1].class == ep.class {
		detector.note("merge", ep.end, fmt.Sprintf(
			"class=%s startTick=%d endTick=%d startSeq=%d endSeq=%d",
			ep.class, ep.start.tick, ep.end.tick, ep.start.seqIdx, ep.end.seqIdx,
		))
		last := &detector.pending[len(detector.pending)-1]
		last.end = ep.end
		last.lowP = math.Min(last.lowP, ep.lowP)
		last.highP = math.Max(last.highP, ep.highP)
		last.trades = last.end.seqIdx - last.start.seqIdx
		return
	}

	detector.note("enqueue", ep.end, fmt.Sprintf(
		"class=%s startTick=%d endTick=%d startSeq=%d endSeq=%d trades=%d",
		ep.class, ep.start.tick, ep.end.tick, ep.start.seqIdx, ep.end.seqIdx, ep.trades,
	))
	detector.pending = append(detector.pending, ep)

	// 3. Detect 3-swing overlapping whip-saws inside a range box and collapse into "chop".
	detector.coalesceBoxWhipsaws()

	// 4. Flush the oldest episode once it is safely beyond the lookahead window.
	for len(detector.pending) > 3 {
		oldest := detector.pending[0]
		detector.pending = detector.pending[1:]
		detector.lastResult = detector.Flush(
			oldest.class,
			oldest.start.tick, oldest.end.tick,
			oldest.start.seqIdx, oldest.end.seqIdx,
			oldest.start.price, oldest.end.price,
			oldest.start.trade, oldest.end.trade,
		)
	}
}

/*
coalesceBoxWhipsaws detects alternating up/down swings that retrace >= 68% of
each other without escaping the bounding box and collapses them into "chop".
*/
func (detector *Detector) coalesceBoxWhipsaws() {
	n := len(detector.pending)

	if n < 3 {
		return
	}

	s1 := detector.pending[n-3]
	s2 := detector.pending[n-2]
	s3 := detector.pending[n-1]

	isAlternating := (s1.class == "up" && s2.class == "down" && s3.class == "up") ||
		(s1.class == "down" && s2.class == "up" && s3.class == "down")

	if !isAlternating {
		return
	}

	amp1 := math.Abs(s1.end.priceF - s1.start.priceF)
	amp2 := math.Abs(s2.end.priceF - s2.start.priceF)
	amp3 := math.Abs(s3.end.priceF - s3.start.priceF)

	if amp1 <= 0 || amp2 <= 0 {
		return
	}

	r12 := amp2 / amp1
	r23 := amp3 / amp2

	// If both counter-swings wipe out most of the prior swing without breaking out (> 1.35x),
	// they are oscillating inside a horizontal chop box.
	if r12 >= detector.boxOverlapRatio && r12 <= 1.35 &&
		r23 >= detector.boxOverlapRatio && r23 <= 1.35 {
		merged := episode{
			class:  "chop",
			start:  s1.start,
			end:    s3.end,
			lowP:   math.Min(s1.lowP, math.Min(s2.lowP, s3.lowP)),
			highP:  math.Max(s1.highP, math.Max(s2.highP, s3.highP)),
			trades: s3.end.seqIdx - s1.start.seqIdx,
		}
		detector.pending = detector.pending[:n-3]
		detector.enqueueEpisode(merged)
	}
}

func (detector *Detector) flushConfirmedOnReset() {
	for _, value := range detector.drain() {
		detector.lastResult = value
	}
}

func (detector *Detector) replayTrade(frame *data.Value, symbol string, tradeID int64, price float64) {
	detector.stepTradePoint(int64(frame.Epoch), symbol, tradePoint{
		tick:   int64(frame.Tick),
		seqIdx: tradeID,
		price:  decimal.NewFromFloat64(price),
		priceF: price,
	})
}

/*
Replay steps one stored tick. The book and fee are the ones stamped on that
tick; a missing or invalid book is skipped. Returned values are the excursions
that became safe to flush while stepping.
*/
func (detector *Detector) Replay(
	frame *data.Value,
	tick *broker.SymbolTick,
) []*data.Value {
	if frame == nil || tick == nil || tick.Book == nil || tick.BookErr != nil {
		return nil
	}

	detector.book = tick.Book

	if detector.price != nil {
		detector.price.InstallPair(tick.Instrument)

		if tick.Price.FeeRate != nil {
			detector.price.SetFee(tick.Symbol, kraken.TradeVolumeFee{
				Fee: decimal.NewFromFloat64(*tick.Price.FeeRate * 100),
			})
		}
	}

	before := len(detector.emitted)

	for i := range tick.Trades {
		trade := tick.Trades[i]
		price := trade.Price.Float64()

		if price <= 0 {
			continue
		}

		detector.replayTrade(frame, tick.Symbol, trade.TradeID, price)
	}

	out := append([]*data.Value(nil), detector.emitted[before:]...)
	detector.emitted = detector.emitted[:before]

	return out
}

/*
Finish flushes excursions still held for lookahead after the tape ends.
*/
func (detector *Detector) Finish() []*data.Value {
	detector.drain()
	out := detector.emitted
	detector.emitted = nil
	detector.book = nil

	return out
}

func (detector *Detector) drain() []*data.Value {
	var out []*data.Value

	for len(detector.pending) > 0 {
		oldest := detector.pending[0]
		detector.pending = detector.pending[1:]
		value := detector.Flush(
			oldest.class,
			oldest.start.tick, oldest.end.tick,
			oldest.start.seqIdx, oldest.end.seqIdx,
			oldest.start.price, oldest.end.price,
			oldest.start.trade, oldest.end.trade,
		)
		detector.lastResult = value

		if value != nil {
			out = append(out, value)
		}
	}

	return out
}

/*
Flush writes one detected excursion of the given class to storeTee.
bTick/bSeqIdx/bPrice/bTrade is chronological start B (ignition) and
cTick/cSeqIdx/cPrice/cTrade is chronological end C (exhaustion).
*/
func (detector *Detector) Flush(
	class string,
	bTick, cTick int64,
	bSeqIdx, cSeqIdx int64,
	bPrice, cPrice *decimal.Decimal,
	bTrade, cTrade *data.Value,
) *data.Value {
	halfMoveSeqIdx := cSeqIdx - bSeqIdx
	halfMoveTick := cTick - bTick

	tenPercentMoveSeqIdx := halfMoveSeqIdx / 10
	tenPercentMoveTick := halfMoveTick / 10

	aIdx := bSeqIdx - halfMoveSeqIdx
	aTick := bTick - halfMoveTick
	bIdx := bSeqIdx - tenPercentMoveSeqIdx
	bTickVal := bTick - tenPercentMoveTick
	cIdx := cSeqIdx - tenPercentMoveSeqIdx
	cTickVal := cTick - tenPercentMoveTick
	dIdx := cSeqIdx + halfMoveSeqIdx
	dTick := cTick + halfMoveTick

	detector.noteMarks(class, aTick, bTickVal, cTickVal, dTick, aIdx, bIdx, cIdx, dIdx, cPrice)

	value := data.NewValue(
		detector.Name(), class, detector.label,
	)
	value.Epoch = uint64(detector.epoch)
	value.Tick = uint64(cTick)

	detector.emitted = append(detector.emitted, value)

	value.Append([]float64{
		float64(aIdx),
		float64(aTick),
		float64(bIdx),
		float64(bTickVal),
		float64(cIdx),
		float64(cTickVal),
		float64(dIdx),
		float64(dTick),
		bPrice.Float64(),
		cPrice.Float64(),
	}, "a_idx", "a_tick", "b_idx", "b_tick", "c_idx", "c_tick", "d_idx", "d_tick", "b_price", "c_price")

	return value
}
